package sqlite

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
)

func TestImageIndex_Upsert(t *testing.T) {
	ctx := context.Background()
	s := NewImageIndex(newTestDB(t))

	img := library.Image{
		ID:          uuid.Must(uuid.NewV7()),
		ContentHash: "abc123",
		MimeType:    "image/png",
		Width:       3,
		Height:      2,
	}
	got, err := s.Upsert(ctx, img)
	if err != nil {
		t.Fatalf("Upsert(ctx, %+v) failed: %v", img, err)
	}
	if got.ID != img.ID {
		t.Errorf("Upsert() ID = %v, want %v", got.ID, img.ID)
	}

	// Same content hash under a fresh candidate ID (a restart) must keep
	// the original row's ID instead of failing the unique constraint.
	dup := img
	dup.ID = uuid.Must(uuid.NewV7())
	dup.Width = 30
	got, err = s.Upsert(ctx, dup)
	if err != nil {
		t.Fatalf("Upsert(ctx, %+v) with duplicate hash failed: %v", dup, err)
	}
	if got.ID != img.ID {
		t.Errorf("Upsert(ctx, dup) ID = %v, want original ID %v", got.ID, img.ID)
	}

	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM images WHERE content_hash = ?`, img.ContentHash).Scan(&n); err != nil {
		t.Fatalf("count images failed: %v", err)
	}
	if n != 1 {
		t.Errorf("images rows for hash %q = %d, want 1", img.ContentHash, n)
	}
}
