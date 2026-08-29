package sqlite

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/scan"
)

func TestParseQueueStore_DirtyDirsReclaimsResyncedDir(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	scans := NewScanStore(db)
	queue := NewParseQueueStore(db)

	locID := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(ctx,
		`INSERT INTO locations (id, uri) VALUES (?, ?)`, locID, "file:///music"); err != nil {
		t.Fatalf("seed location failed: %v", err)
	}

	dirID, err := scans.SyncDir(ctx, scan.SyncPayload{
		LocationID: locID, RelPath: "Album", Generation: 1, Dirty: true,
	})
	if err != nil {
		t.Fatalf("SyncDir() returned unexpected error: %v", err)
	}
	claimed, err := queue.DirtyDirs(ctx, 10)
	if err != nil {
		t.Fatalf("DirtyDirs() returned unexpected error: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("DirtyDirs() claimed %d dirs, want 1", len(claimed))
	}
	if err := queue.ReleaseDir(ctx, dirID, claimed[0].LockedGeneration); err != nil {
		t.Fatalf("ReleaseDir() returned unexpected error: %v", err)
	}

	if _, err := scans.SyncDir(ctx, scan.SyncPayload{
		LocationID: locID, RelPath: "Album", Generation: 2, Dirty: true,
	}); err != nil {
		t.Fatalf("SyncDir() returned unexpected error: %v", err)
	}
	claimed, err = queue.DirtyDirs(ctx, 10)
	if err != nil {
		t.Fatalf("DirtyDirs() returned unexpected error: %v", err)
	}
	if len(claimed) != 1 {
		t.Errorf("DirtyDirs() claimed %d dirs after a dirty re-sync, want 1", len(claimed))
	}
}

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

	for _, name := range []string{"1", "z", "2", "01", "A", "10", "Z", "a"} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO files (id, dir_id, name, kind, size_bytes, mod_time)
			 VALUES (?, ?, ?, 'audio', 1, 1)`,
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
	want := []string{"01", "1", "10", "2", "a", "A", "z", "Z"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("DirFiles() names mismatch (-want +got):\n%s", diff)
	}
}
