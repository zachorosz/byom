package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/storage"
)

func TestScanStore_KnownFiles_PreservesModTimeSubSecondPrecision(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewScanStore(db)

	locID := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(ctx,
		`INSERT INTO locations (id, uri) VALUES (?, ?)`, locID, "file:///music/"+locID.String()); err != nil {
		t.Fatalf("seed location failed: %v", err)
	}

	want := time.Date(2026, 8, 12, 21, 11, 10, 835293476, time.UTC)
	dirID, err := s.SyncDir(ctx, scan.SyncPayload{
		LocationID: locID,
		RelPath:    "Album",
		Generation: 1,
		Changed: []storage.File{
			{Name: "a.flac", Kind: storage.FileAudio, Size: 1, ModTime: want},
		},
	})
	if err != nil {
		t.Fatalf("SyncDir() failed: %v", err)
	}

	known, err := s.KnownFiles(ctx, dirID)
	if err != nil {
		t.Fatalf("KnownFiles(ctx, %v) failed: %v", dirID, err)
	}

	got := known["a.flac"].ModTime
	if !got.Equal(want) {
		t.Errorf("ModTime = %v, want %v", got, want)
	}
}
