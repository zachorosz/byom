package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/page"
	"github.com/zachorosz/byom/storage"
)

type LocationStore struct {
	db *sql.DB
}

func NewLocationStore(db *sql.DB) *LocationStore {
	return &LocationStore{db: db}
}

func (s *LocationStore) Insert(ctx context.Context, loc storage.Location) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO locations (id, uri, available, scan_generation)
		VALUES (?, ?, 1, 0)
	`, loc.ID, loc.URI,
	); err != nil {
		if isUniqueConstraintError(err) {
			err = storage.ErrExists
		}
		return fmt.Errorf("insert location: %w", err)
	}
	return nil
}

func scanLocation(scan func(dest ...any) error) (storage.Location, error) {
	var loc storage.Location
	return loc, scan(&loc.ID, &loc.URI, &loc.Available)
}

func (s *LocationStore) Location(ctx context.Context, id uuid.UUID) (storage.Location, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, uri, available FROM locations WHERE id = ?`, id)
	loc, err := scanLocation(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = storage.ErrNotExists
		}
		return loc, fmt.Errorf("get location: %w", err)
	}
	return loc, nil
}

// Locations returns a page of locations ordered by ID, resuming after
// token. The returned token fetches the next page and is empty once
// the listing is exhausted.
func (s *LocationStore) Locations(ctx context.Context, token string, limit int) ([]storage.Location, string, error) {
	limit = page.Size(limit)

	q := `SELECT id, uri, available FROM locations`
	var args []any
	if token != "" {
		cur, err := page.DecodeToken(token, 1)
		if err != nil {
			return nil, "", err
		}
		q += ` WHERE id > ?`
		args = append(args, cur[0])
	}
	q += ` ORDER BY id LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list locations: %w", err)
	}
	defer rows.Close()

	var locations []storage.Location
	for rows.Next() {
		loc, err := scanLocation(rows.Scan)
		if err != nil {
			return nil, "", fmt.Errorf("scan location: %w", err)
		}
		locations = append(locations, loc)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate locations: %w", err)
	}

	if len(locations) <= limit {
		return locations, "", nil
	}
	locations = locations[:limit]
	return locations, page.EncodeToken(locations[limit-1].ID.String()), nil
}

// Update replaces a location's URI. It returns storage.ErrNotExists if
// no such location exists, or storage.ErrExists if another location
// already uses the URI.
func (s *LocationStore) Update(ctx context.Context, loc storage.Location) error {
	var id uuid.UUID
	err := s.db.QueryRowContext(ctx,
		`UPDATE locations SET uri = ? WHERE id = ? RETURNING id`,
		loc.URI, loc.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("location %s: %w", loc.ID, storage.ErrNotExists)
	}
	if err != nil {
		if isUniqueConstraintError(err) {
			err = storage.ErrExists
		}
		return fmt.Errorf("update location: %w", err)
	}
	return nil
}

// Delete removes a location and, by cascade, every dir, file, and
// library row derived from it. It returns storage.ErrNotExists if no
// such location exists.
func (s *LocationStore) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM locations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete location: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete location: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("location %s: %w", id, storage.ErrNotExists)
	}
	return nil
}
