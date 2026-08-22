package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/page"
	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/storage"
)

func insertTestLocation(t *testing.T, s *LocationStore, uri string) storage.Location {
	t.Helper()
	loc := storage.Location{ID: uuid.Must(uuid.NewV7()), URI: uri, Available: true}
	if err := s.Insert(context.Background(), loc); err != nil {
		t.Fatalf("Insert(%+v) failed: %v", loc, err)
	}
	return loc
}

func TestLocationStore_Insert_DuplicateURI(t *testing.T) {
	s := NewLocationStore(newTestDB(t))
	insertTestLocation(t, s, "file:///music")

	dup := storage.Location{ID: uuid.Must(uuid.NewV7()), URI: "file:///music"}
	err := s.Insert(context.Background(), dup)
	if !errors.Is(err, storage.ErrExists) {
		t.Errorf("Insert() error = %v, want storage.ErrExists", err)
	}
}

func TestLocationStore_Locations_Paginates(t *testing.T) {
	ctx := context.Background()
	s := NewLocationStore(newTestDB(t))
	// UUIDv7 sorts by creation time, which is the listing order.
	first := insertTestLocation(t, s, "file:///a")
	second := insertTestLocation(t, s, "file:///b")
	third := insertTestLocation(t, s, "file:///c")

	got, next, err := s.Locations(ctx, "", 2)
	if err != nil {
		t.Fatalf("Locations() failed: %v", err)
	}
	if diff := cmp.Diff([]storage.Location{first, second}, got); diff != "" {
		t.Errorf("Locations() first page mismatch (-want +got):\n%s", diff)
	}
	if next == "" {
		t.Fatal(`Locations() next page token = "", want non-empty`)
	}

	got, next, err = s.Locations(ctx, next, 2)
	if err != nil {
		t.Fatalf("Locations() failed: %v", err)
	}
	if diff := cmp.Diff([]storage.Location{third}, got); diff != "" {
		t.Errorf("Locations() second page mismatch (-want +got):\n%s", diff)
	}
	if next != "" {
		t.Errorf("Locations() next page token = %q, want empty", next)
	}
}

func TestLocationStore_Locations_InvalidToken(t *testing.T) {
	s := NewLocationStore(newTestDB(t))

	_, _, err := s.Locations(context.Background(), "not-a-token!", 10)
	if !errors.Is(err, page.ErrInvalidToken) {
		t.Errorf("Locations() error = %v, want page.ErrInvalidToken", err)
	}
}

func TestLocationStore_Update(t *testing.T) {
	ctx := context.Background()
	s := NewLocationStore(newTestDB(t))
	loc := insertTestLocation(t, s, "file:///music")

	loc.URI = "file:///moved"
	if err := s.Update(ctx, loc); err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	got, err := s.Location(ctx, loc.ID)
	if err != nil {
		t.Fatalf("Location() failed: %v", err)
	}
	if diff := cmp.Diff(loc, got); diff != "" {
		t.Errorf("Location() mismatch (-want +got):\n%s", diff)
	}
}

func TestLocationStore_Update_UnknownLocation(t *testing.T) {
	s := NewLocationStore(newTestDB(t))

	loc := storage.Location{ID: uuid.Must(uuid.NewV7()), URI: "file:///music"}
	err := s.Update(context.Background(), loc)
	if !errors.Is(err, storage.ErrNotExists) {
		t.Errorf("Update() error = %v, want storage.ErrNotExists", err)
	}
}

func TestLocationStore_Update_DuplicateURI(t *testing.T) {
	s := NewLocationStore(newTestDB(t))
	insertTestLocation(t, s, "file:///a")
	second := insertTestLocation(t, s, "file:///b")

	second.URI = "file:///a"
	err := s.Update(context.Background(), second)
	if !errors.Is(err, storage.ErrExists) {
		t.Errorf("Update() error = %v, want storage.ErrExists", err)
	}
}

func TestLocationStore_Update_ScanRunning(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLocationStore(db)
	loc := insertTestLocation(t, s, "file:///music")
	if _, _, err := NewScanStore(db).BeginScan(ctx, loc.ID); err != nil {
		t.Fatalf("BeginScan() failed: %v", err)
	}

	loc.URI = "file:///moved"
	err := s.Update(ctx, loc)
	if !errors.Is(err, scan.ErrScanRunning) {
		t.Errorf("Update() error = %v, want scan.ErrScanRunning", err)
	}
}

func TestLocationStore_Delete_ScanRunning(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLocationStore(db)
	loc := insertTestLocation(t, s, "file:///music")
	if _, _, err := NewScanStore(db).BeginScan(ctx, loc.ID); err != nil {
		t.Fatalf("BeginScan() failed: %v", err)
	}

	err := s.Delete(ctx, loc.ID)
	if !errors.Is(err, scan.ErrScanRunning) {
		t.Errorf("Delete() error = %v, want scan.ErrScanRunning", err)
	}
}

func TestLocationStore_Delete_UnknownLocation(t *testing.T) {
	s := NewLocationStore(newTestDB(t))

	err := s.Delete(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, storage.ErrNotExists) {
		t.Errorf("Delete() error = %v, want storage.ErrNotExists", err)
	}
}

func TestLocationStore_Delete_CascadesToScannedRows(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLocationStore(db)

	// seedDir inserts its own location, so read the ID back from the dir
	// it created rather than minting a second one.
	dirID, _ := seedDir(t, db, 2)
	var locID uuid.UUID
	if err := db.QueryRowContext(ctx,
		`SELECT location_id FROM dirs WHERE id = ?`, dirID).Scan(&locID); err != nil {
		t.Fatalf("read dir location failed: %v", err)
	}

	if err := s.Delete(ctx, locID); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	for _, table := range []string{"dirs", "files"} {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count %s failed: %v", table, err)
		}
		if n != 0 {
			t.Errorf("Delete() left %d %s rows, want %d", n, table, 0)
		}
	}
}
