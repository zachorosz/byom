package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zachorosz/byom/library"
)

type ImageIndex struct {
	db *sql.DB
}

func NewImageIndex(db *sql.DB) *ImageIndex {
	return &ImageIndex{db: db}
}

// Upsert inserts img, or refreshes the existing row when the content
// hash is already known, returning the stored image with its ID.
func (s *ImageIndex) Upsert(ctx context.Context, img library.Image) (library.Image, error) {
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO images (id, content_hash, mime, width, height)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (content_hash) DO UPDATE SET
			mime = excluded.mime,
			width = excluded.width,
			height = excluded.height
		 RETURNING id`,
		img.ID, img.ContentHash, img.MimeType, img.Width, img.Height,
	).Scan(&img.ID); err != nil {
		return library.Image{}, fmt.Errorf("upsert image: %w", err)
	}
	return img, nil
}
