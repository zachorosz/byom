package sqlite

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

func TestParseQueueStore_DirFilesOrdersByName(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewParseQueueStore(db)

	locID := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(ctx,
		`INSERT INTO locations (id, uri) VALUES (?, ?)`, locID, "file:///music"); err != nil {
		t.Fatalf("seed location failed: %v", err)
	}

	dirID := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(ctx,
		`INSERT INTO dirs (id, location_id, relpath, seen_generation) VALUES (?, ?, 'Album', 1)`,
		dirID, locID); err != nil {
		t.Fatalf("seed dir failed: %v", err)
	}

	for _, name := range []string{"front.jpg", "02 - b.flac", "cover.jpg", "01 - a.flac", "back.jpg"} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO files (id, dir_id, name, kind, size_bytes, mod_time)
			 VALUES (?, ?, ?, 'image', 1, 1)`,
			uuid.Must(uuid.NewV7()), dirID, name); err != nil {
			t.Fatalf("seed file %s failed: %v", name, err)
		}
	}

	files, err := s.DirFiles(ctx, dirID)
	if err != nil {
		t.Fatalf("DirFiles() returned unexpected error: %v", err)
	}

	var got []string
	for _, f := range files {
		got = append(got, f.Name)
	}
	want := []string{"01 - a.flac", "02 - b.flac", "back.jpg", "cover.jpg", "front.jpg"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("DirFiles() names mismatch (-want +got):\n%s", diff)
	}
}
