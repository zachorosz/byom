package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/page"
	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/storage"
)

// ScanStore is the scanner's store: the scan lifecycle (generation
// bumps, scan rows, sweep) and per-dir file synchronization.
type ScanStore struct {
	db *sql.DB
}

func NewScanStore(db *sql.DB) *ScanStore {
	return &ScanStore{db: db}
}

func (s *ScanStore) BeginScan(ctx context.Context, locationID uuid.UUID) (int64, uuid.UUID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, uuid.Nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	// Any prior 'running' row is a crashed scan; mark it aborted so the
	// scans table stays truthful.
	if _, err := tx.ExecContext(ctx,
		`UPDATE scans SET state='aborted', finish_time=?
         WHERE location_id=? AND state='running'`, now, locationID); err != nil {
		return 0, uuid.Nil, err
	}
	// Crash recovery deliberately reaches into the parse queue's columns
	// (see ParseQueueStore): dirs still locked were being parsed by a
	// worker that died with the process, so re-enqueue them (dirty=1)
	// rather than leaving them stuck forever.
	if _, err := tx.ExecContext(ctx,
		`UPDATE dirs SET locked_generation = NULL, dirty = 1
		 WHERE location_id = ? AND locked_generation IS NOT NULL`, locationID); err != nil {
		return 0, uuid.Nil, err
	}

	var gen int64
	if err := tx.QueryRowContext(ctx,
		`UPDATE locations SET scan_generation = scan_generation + 1
         WHERE id=? RETURNING scan_generation`, locationID).Scan(&gen); err != nil {
		return 0, uuid.Nil, err
	}

	scanID, _ := uuid.NewV7()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO scans (id, location_id, generation, state, start_time)
         VALUES (?, ?, ?, 'running', ?)`, scanID, locationID, gen, now); err != nil {
		return 0, uuid.Nil, err
	}

	if err := tx.Commit(); err != nil {
		return 0, uuid.Nil, err
	}

	return gen, scanID, nil
}

func (s *ScanStore) FinishScan(ctx context.Context, scanID uuid.UUID, progress scan.Progress, scanErr error) error {
	state := "done"
	var errMsg sql.NullString
	switch {
	case errors.Is(scanErr, context.Canceled):
		state = "aborted"
	case scanErr != nil:
		state = "failed"
		errMsg.String = scanErr.Error()
		errMsg.Valid = true
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE scans
		 SET state=?, finish_time=?, error=?, dirs_seen=?, dirs_missing=?, files_seen=?, files_missing=?
		 WHERE id=? AND state='running'`,
		state, time.Now().UTC(), errMsg,
		progress.DirsSeen, progress.DirsMissing, progress.FilesSeen, progress.FilesMissing,
		scanID); err != nil {
		return err
	}
	return nil
}

func (s *ScanStore) DirID(ctx context.Context, locationID uuid.UUID, relpath string) (uuid.UUID, error) {
	var id uuid.UUID
	if err := s.db.QueryRowContext(ctx,
		"SELECT id FROM dirs WHERE location_id=? AND relpath=?", locationID, relpath).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = storage.ErrNotExists
		}
		return uuid.Nil, fmt.Errorf("get dir id: %w", err)
	}
	return id, nil
}

func scanFile(scan func(dest ...any) error) (storage.File, error) {
	var f storage.File
	return f, scan(&f.ID, &f.Name, &f.Kind, &f.Size, &f.ModTime)
}

func (s *ScanStore) KnownFiles(ctx context.Context, dirID uuid.UUID) (map[string]storage.File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, kind, size_bytes, mod_time
		 FROM files WHERE dir_id = ?`, dirID)
	if err != nil {
		return nil, fmt.Errorf("failed to list: %w", err)
	}
	defer rows.Close()

	files := map[string]storage.File{}
	for rows.Next() {
		f, err := scanFile(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to parse: %w", err)
		}
		files[f.Name] = f
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate: %w", err)
	}

	return files, nil
}

func (s *ScanStore) SyncDir(ctx context.Context, payload scan.SyncPayload) (uuid.UUID, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()

	// IMPORTANT: dirty is monotonic here. we only ever set it to 1,
	// never clear it. Clearing happens exclusively in the claim step
	// (ParseQueueStore.DirtyDirs), so a re-sync that happens mid-parse
	// correctly re-enqueues the dir instead of racing the parser's
	// completion write.

	newDirID, _ := uuid.NewV7() // candidate only used if not exists
	var dirID uuid.UUID
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO dirs (id, location_id, relpath, seen_generation, dirty)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (location_id, relpath) DO UPDATE SET
			seen_generation = excluded.seen_generation,
			missing = 0,
			dirty = CASE WHEN excluded.dirty = 1 THEN 1 ELSE dirs.dirty END
		 RETURNING id`,
		newDirID, payload.LocationID, payload.RelPath, payload.Generation, payload.Dirty,
	).Scan(&dirID); err != nil {
		return uuid.Nil, err
	}

	_, err = markMissingFiles(ctx, tx, payload.Missing)
	if err != nil {
		return uuid.Nil, err
	}

	if err := upsertFiles(ctx, tx, dirID, payload.Changed); err != nil {
		return uuid.Nil, err
	}

	if err := tx.Commit(); err != nil {
		return uuid.Nil, err
	}
	return dirID, nil
}

func upsertFiles(ctx context.Context, tx *sql.Tx, dirID uuid.UUID, files []storage.File) error {
	if len(files) == 0 {
		return nil
	}

	var args []any
	rows := make([]string, len(files))
	for i, f := range files {
		id := f.ID
		if id == uuid.Nil {
			id, _ = uuid.NewV7()
		}
		rows[i] = "(?, ?, ?, ?, ?, ?)"
		args = append(args, id, dirID, f.Name, f.Kind, f.Size, f.ModTime)
	}

	stmt := `INSERT INTO files (id, dir_id, name, kind, size_bytes, mod_time)
		VALUES ` + strings.Join(rows, ",") + `
		ON CONFLICT (dir_id, name) DO UPDATE SET
			kind = excluded.kind,
			size_bytes = excluded.size_bytes,
			mod_time = excluded.mod_time,
			missing = 0
	`
	_, err := tx.ExecContext(ctx, stmt, args...)
	return err
}

func markMissingFiles(ctx context.Context, tx *sql.Tx, files []uuid.UUID) (int64, error) {
	if len(files) == 0 {
		return 0, nil
	}

	ph := make([]string, len(files))
	args := make([]any, len(files))
	for i, id := range files {
		ph[i] = "?"
		args[i] = id
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE files SET missing=1
		 WHERE id IN (`+strings.Join(ph, ",")+`) AND missing=0`, args...)
	if err != nil {
		return 0, err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (s *ScanStore) AbortRunningScans(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE scans SET state='aborted' WHERE state IN ('running', 'cancelling')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanScan(scanFn func(dest ...any) error) (scan.Scan, error) {
	var sc scan.Scan
	var start, finish sql.NullTime
	var errStr sql.NullString
	if err := scanFn(&sc.ID, &sc.LocationID, &sc.State, &start, &finish, &errStr,
		&sc.Progress.DirsSeen, &sc.Progress.DirsMissing,
		&sc.Progress.FilesSeen, &sc.Progress.FilesMissing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = storage.ErrNotExists
		}
		return sc, err
	}
	if start.Valid {
		sc.StartTime = start.Time
	}
	if finish.Valid {
		sc.FinishTime = finish.Time
	}
	sc.Error = errStr.String
	return sc, nil
}

func (s *ScanStore) Scan(ctx context.Context, id uuid.UUID) (scan.Scan, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, location_id, state, start_time, finish_time, error, dirs_seen, dirs_missing, files_seen, files_missing
		 FROM scans WHERE id = ?`, id)
	return scanScan(row.Scan)
}

func (s *ScanStore) Scans(ctx context.Context, locationID uuid.UUID, state scan.State, token string, limit int) ([]scan.Scan, string, error) {
	limit = page.Size(limit)
	q := `SELECT id, location_id, state, start_time, finish_time, error, dirs_seen, dirs_missing, files_seen, files_missing FROM scans`
	var args []any
	var conditions []string

	if locationID != uuid.Nil {
		conditions = append(conditions, "location_id = ?")
		args = append(args, locationID)
	}
	if state != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, string(state))
	}
	if token != "" {
		cur, err := page.DecodeToken(token, 2)
		if err != nil {
			return nil, "", err
		}
		startTime, err := strconv.ParseInt(cur[0], 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("%w: invalid scan start time", page.ErrInvalidToken)
		}
		conditions = append(conditions, "(start_time < ? OR (start_time = ? AND id > ?))")
		cursor := time.Unix(0, startTime).UTC()
		args = append(args, cursor, cursor, cur[1])
	}

	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	q += " ORDER BY start_time DESC, id ASC LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list scans: %w", err)
	}
	defer rows.Close()

	var scans []scan.Scan
	for rows.Next() {
		sc, err := scanScan(rows.Scan)
		if err != nil {
			return nil, "", fmt.Errorf("scan scan row: %w", err)
		}
		scans = append(scans, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate scans: %w", err)
	}

	if len(scans) <= limit {
		return scans, "", nil
	}
	scans = scans[:limit]
	last := scans[limit-1]
	return scans, page.EncodeToken(
		strconv.FormatInt(last.StartTime.UnixNano(), 10), last.ID.String()), nil
}

func (s *ScanStore) Sweep(ctx context.Context, locationID uuid.UUID, gen int64) (int64, int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE files SET missing = 1
		 WHERE dir_id IN (
			SELECT id FROM dirs
			WHERE location_id = ? AND seen_generation < ? AND missing = 0
		 )`, locationID, gen,
	)
	if err != nil {
		return 0, 0, err
	}

	files, err := res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}

	res, err = tx.ExecContext(ctx,
		`UPDATE dirs SET missing = 1
		 WHERE location_id = ? AND seen_generation < ? AND missing = 0`,
		locationID, gen)
	if err != nil {
		return 0, 0, err
	}

	dirs, err := res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return dirs, files, nil
}
