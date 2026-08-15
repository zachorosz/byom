package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/metadata"
	"github.com/zachorosz/byom/storage"
)

type ParseQueueStore struct {
	db *sql.DB
}

func NewParseQueueStore(db *sql.DB) *ParseQueueStore {
	return &ParseQueueStore{db: db}
}

func (s *ParseQueueStore) DirtyDirs(ctx context.Context, limit int) ([]metadata.ClaimedDir, error) {
	rows, err := s.db.QueryContext(ctx, `
		UPDATE dirs SET dirty = 0, locked_generation = seen_generation
		WHERE id IN (
			SELECT id FROM dirs
			WHERE dirty = 1 AND locked_generation IS NULL
			LIMIT ?
		)
		RETURNING id, relpath, location_id, locked_generation;
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []metadata.ClaimedDir
	for rows.Next() {
		var c metadata.ClaimedDir
		if err := rows.Scan(&c.ID, &c.RelPath, &c.LocationID, &c.LockedGeneration); err != nil {
			return nil, fmt.Errorf("failed to parse: %w", err)
		}
		claimed = append(claimed, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate: %w", err)
	}

	return claimed, nil
}

// DirFiles lists the non-missing files of a claimed dir for parsing, ordered by
// file name.
func (s *ParseQueueStore) DirFiles(ctx context.Context, dirID uuid.UUID) ([]storage.File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, kind, size_bytes, mod_time
		 FROM files WHERE dir_id = ? AND missing = 0
		 ORDER BY name`, dirID)
	if err != nil {
		return nil, fmt.Errorf("failed to list: %w", err)
	}
	defer rows.Close()

	var files []storage.File
	for rows.Next() {
		f, err := scanFile(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to parse: %w", err)
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate: %w", err)
	}

	return files, nil
}

// ReleaseDir releases the parse lock. It does not touch `dirty`. If a
// concurrent SyncDir re-set dirty=1 while this dir was being parsed, that
// flag survives and the dir will be picked up again on the next pump.
func (s *ParseQueueStore) ReleaseDir(ctx context.Context, dirID uuid.UUID, lockedGen int64) error {
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
func (s *ParseQueueStore) ReleaseAndRedirty(ctx context.Context, dirID uuid.UUID, lockedGen int64) error {
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
