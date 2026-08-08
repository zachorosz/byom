package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
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

func (s *LocationStore) Locations(ctx context.Context) ([]storage.Location, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, uri, available FROM locations`)
	if err != nil {
		return nil, fmt.Errorf("failed to list: %w", err)
	}
	defer rows.Close()

	var locations []storage.Location
	for rows.Next() {
		loc, err := scanLocation(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to parse: %w", err)
		}
		locations = append(locations, loc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate: %w", err)
	}

	return locations, nil
}
