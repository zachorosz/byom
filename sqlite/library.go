package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/library"
)

type LibraryStore struct {
	db *sql.DB
}

func NewLibraryStore(db *sql.DB) *LibraryStore {
	return &LibraryStore{db: db}
}

func (s *LibraryStore) InsertStorage(ctx context.Context, st library.Storage) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO storages (id, uri, available, scan_generation)
		VALUES (?, ?, 1, 0)
	`, st.ID, st.URI,
	); err != nil {
		if isUniqueConstraintError(err) {
			err = library.ErrExists
		}
		return fmt.Errorf("insert storage: %w", err)
	}
	return nil
}

func scanStorage(scan func(dest ...any) error) (library.Storage, error) {
	var st library.Storage
	return st, scan(&st.ID, &st.URI, &st.Available)
}

func (s *LibraryStore) Storage(ctx context.Context, id uuid.UUID) (library.Storage, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, uri, available FROM storages WHERE id = ?`, id)
	st, err := scanStorage(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = library.ErrNotExists
		}
		return st, fmt.Errorf("get storage: %w", err)
	}
	return st, nil
}

func (s *LibraryStore) Storages(ctx context.Context) ([]library.Storage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, uri, available FROM storages`)
	if err != nil {
		return nil, fmt.Errorf("failed to list: %w", err)
	}
	defer rows.Close()

	var storages []library.Storage
	for rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate: %w", err)
		}
		st, err := scanStorage(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to parse: %w", err)
		}
		storages = append(storages, st)
	}

	return storages, nil
}

func (s *LibraryStore) BeginScan(ctx context.Context, storageID uuid.UUID) (int64, uuid.UUID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, uuid.Nil, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	// Any prior 'running' row is a crashed scan; mark it aborted so the
	// scans table stays truthful.
	if _, err := tx.ExecContext(ctx,
		`UPDATE scans SET state='aborted', finish_time=?
         WHERE storage_id=? AND state='running'`, now, storageID); err != nil {
		return 0, uuid.Nil, err
	}
	// release any dirs still locked. those were being parsed by a worker that
	// died with the process, so re-enqueue them (dirty=1) rather than leaving
	// them stuck forever.
	if _, err := tx.ExecContext(ctx,
		`UPDATE dirs SET locked_generation = NULL, dirty = 1
		 WHERE storage_id = ? AND locked_generation IS NOT NULL`, storageID); err != nil {
		return 0, uuid.Nil, err
	}

	var gen int64
	if err := tx.QueryRowContext(ctx,
		`UPDATE storages SET scan_generation = scan_generation + 1
         WHERE id=? RETURNING scan_generation`, storageID).Scan(&gen); err != nil {
		return 0, uuid.Nil, err
	}

	scanID, _ := uuid.NewV7()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO scans (id, storage_id, generation, state, start_time)
         VALUES (?, ?, ?, 'running', ?)`, scanID, storageID, gen, now); err != nil {
		return 0, uuid.Nil, err
	}

	if err := tx.Commit(); err != nil {
		return 0, uuid.Nil, err
	}

	return gen, scanID, nil
}

func (s *LibraryStore) FinishScan(ctx context.Context, scanID uuid.UUID, scanErr error) error {
	state := "done"
	var errMsg sql.NullString
	if scanErr != nil {
		state = "failed"
		errMsg.String = scanErr.Error()
		errMsg.Valid = true
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE scans SET state=?, finish_time=?, error=?
         WHERE id=? AND state='running'`, state, time.Now().Unix(), errMsg, scanID); err != nil {
		return err
	}
	return nil
}

func (s *LibraryStore) DirID(ctx context.Context, storageID uuid.UUID, relpath string) (uuid.UUID, error) {
	var id uuid.UUID
	if err := s.db.QueryRowContext(ctx,
		"SELECT id FROM dirs WHERE storage_id=? AND relpath=?", storageID, relpath).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = library.ErrNotExists
		}
		return uuid.Nil, fmt.Errorf("get dir id: %w", err)
	}
	return id, nil
}

func scanFile(scan func(dest ...any) error) (library.File, error) {
	var f library.File
	return f, scan(&f.ID, &f.Name, &f.Kind, &f.Size, &f.ModTime)
}

func (s *LibraryStore) KnownFiles(ctx context.Context, dirID uuid.UUID) (map[string]library.File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, kind, size_bytes, mod_time
		 FROM files WHERE dir_id = ?`, dirID)
	if err != nil {
		return nil, fmt.Errorf("failed to list: %w", err)
	}
	defer rows.Close()

	files := map[string]library.File{}
	for rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate: %w", err)
		}
		f, err := scanFile(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to parse: %w", err)
		}
		files[f.Name] = f
	}

	return files, nil
}

func (s *LibraryStore) DirFiles(ctx context.Context, dirID uuid.UUID) ([]library.File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, kind, size_bytes, mod_time
		 FROM files WHERE dir_id = ? AND missing = 0`, dirID)
	if err != nil {
		return nil, fmt.Errorf("failed to list: %w", err)
	}
	defer rows.Close()

	var files []library.File
	for rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate: %w", err)
		}
		f, err := scanFile(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to parse: %w", err)
		}
		files = append(files, f)
	}

	return files, nil
}

func (s *LibraryStore) SyncDir(ctx context.Context, payload library.SyncPayload) (uuid.UUID, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()

	// IMPORTANT: dirty is monotonic here. we only ever set it to 1,
	// never clear it. Clearing happens exclusively in the claim step
	// (DirtyDirs), so a re-sync that happens mid-parse correctly
	// re-enqueues the dir instead of racing the parser's completion write.

	newDirID, _ := uuid.NewV7() // candidate only used if not exists
	var dirID uuid.UUID
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO dirs (id, storage_id, relpath, seen_generation, dirty)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (storage_id, relpath) DO UPDATE SET
			seen_generation = excluded.seen_generation,
			missing = 0,
			dirty = CASE WHEN excluded.dirty = 1 THEN 1 ELSE dirs.dirty END
		 RETURNING id`,
		newDirID, payload.StorageID, payload.RelPath, payload.Generation, payload.Dirty,
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

func upsertFiles(ctx context.Context, tx *sql.Tx, dirID uuid.UUID, files []library.File) error {
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

func (s *LibraryStore) DirtyDirs(ctx context.Context, limit int) ([]library.ClaimedDir, error) {
	rows, err := s.db.QueryContext(ctx, `
		UPDATE dirs SET dirty = 0, locked_generation = seen_generation 
		WHERE id IN (
			SELECT id FROM dirs 
			WHERE dirty = 1 AND locked_generation IS NULL 
			LIMIT ?
		)
		RETURNING id, locked_generation;
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []library.ClaimedDir
	for rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate: %w", err)
		}
		var c library.ClaimedDir
		if err := rows.Scan(&c.ID, &c.LockedGeneration); err != nil {
			return nil, fmt.Errorf("failed to parse: %w", err)
		}
		claimed = append(claimed, c)
	}

	return claimed, nil
}

// ReleaseDir releases the parse lock. It does not touch `dirty`. If a
// concurrent SyncDir re-set dirty=1 while this dir was being parsed, that
// flag survives and the dir will be picked up again on the next pump.
func (s *LibraryStore) ReleaseDir(ctx context.Context, dirID uuid.UUID, lockedGen int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE dirs SET locked_generation = NULL
		 WHERE id = ? AND locked_generation = ?`,
		dirID, lockedGen)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Lock was already released/reclaimed out from under us.
		// e.g. a crash-recovery sweep reset it. Not fatal, but worth knowing about.
		return errors.New("lock lost")
	}
	return nil
}

// ReleaseAndRedirty releases the parse lock and unconditionally re-sets
// dirty=1, for use when a claimed dir failed to parse and needs to be
// retried on the next pump rather than left un-parsed with no lock and
// no queue membership.
func (s *LibraryStore) ReleaseAndRedirty(ctx context.Context, dirID uuid.UUID, lockedGen int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE dirs SET locked_generation = NULL, dirty = 1
		 WHERE id = ? AND locked_generation = ?`,
		dirID, lockedGen)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("lock lost")
	}
	return nil
}

func (s *LibraryStore) Sweep(ctx context.Context, storageID uuid.UUID, gen int64) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE files SET missing = 1
		 WHERE dir_id IN (
			SELECT id FROM dirs
			WHERE storage_id = ? AND seen_generation < ? AND missing = 0
		 )`, storageID, gen,
	)
	if err != nil {
		return 0, err
	}

	files, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE dirs SET missing = 1
		 WHERE storage_id = ? AND seen_generation < ? AND missing = 0`,
		storageID, gen); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return files, nil
}
