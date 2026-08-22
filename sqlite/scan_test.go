package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/scan"
	"github.com/zachorosz/byom/storage"
)

func TestScanStore_FinishScanPersistsProgressAndAbortedState(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewScanStore(db)

	locID := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(ctx,
		`INSERT INTO locations (id, uri) VALUES (?, ?)`, locID, "file:///music/"+locID.String()); err != nil {
		t.Fatalf("seed location failed: %v", err)
	}
	_, scanID, err := s.BeginScan(ctx, locID)
	if err != nil {
		t.Fatalf("BeginScan() failed: %v", err)
	}

	wantProgress := scan.Progress{DirsSeen: 3, DirsMissing: 2, FilesSeen: 9, FilesMissing: 4}
	if err := s.FinishScan(ctx, scanID, wantProgress, context.Canceled); err != nil {
		t.Fatalf("FinishScan() failed: %v", err)
	}

	got, err := s.Scan(ctx, scanID)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if got.State != scan.StateAborted {
		t.Errorf("State = %q, want %q", got.State, scan.StateAborted)
	}
	if got.Progress != wantProgress {
		t.Errorf("Progress = %+v, want %+v", got.Progress, wantProgress)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}

	if err := s.FinishScan(ctx, scanID, scan.Progress{}, errors.New("ignored")); err != nil {
		t.Fatalf("second FinishScan() failed: %v", err)
	}
}

func TestScanStore_AbortRunningScans(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewScanStore(db)

	seedLocation := func() uuid.UUID {
		id := uuid.Must(uuid.NewV7())
		if _, err := db.ExecContext(ctx,
			`INSERT INTO locations (id, uri) VALUES (?, ?)`, id, "file:///music/"+id.String()); err != nil {
			t.Fatalf("seed location failed: %v", err)
		}
		return id
	}

	var running []uuid.UUID
	for range 2 {
		_, scanID, err := s.BeginScan(ctx, seedLocation())
		if err != nil {
			t.Fatalf("BeginScan() failed: %v", err)
		}
		running = append(running, scanID)
	}

	// A scan that already finished must be left untouched.
	_, doneID, err := s.BeginScan(ctx, seedLocation())
	if err != nil {
		t.Fatalf("BeginScan() failed: %v", err)
	}
	if err := s.FinishScan(ctx, doneID, scan.Progress{}, nil); err != nil {
		t.Fatalf("FinishScan() failed: %v", err)
	}
	wantDone, err := s.Scan(ctx, doneID)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	n, err := s.AbortRunningScans(ctx)
	if err != nil {
		t.Fatalf("AbortRunningScans() failed: %v", err)
	}
	if n != int64(len(running)) {
		t.Errorf("AbortRunningScans() = %d, want %d", n, len(running))
	}

	for _, id := range running {
		got, err := s.Scan(ctx, id)
		if err != nil {
			t.Fatalf("Scan() failed: %v", err)
		}
		if got.State != scan.StateAborted {
			t.Errorf("State = %q, want %q", got.State, scan.StateAborted)
		}
		if got.FinishTime.IsZero() {
			t.Error("FinishTime is zero, want non-zero")
		}
	}

	gotDone, err := s.Scan(ctx, doneID)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if diff := cmp.Diff(wantDone, gotDone); diff != "" {
		t.Errorf("AbortRunningScans() changed a finished scan (-want +got):\n%s", diff)
	}
}

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
		t.Fatalf("KnownFiles() failed: %v", err)
	}

	got := known["a.flac"].ModTime
	if !got.Equal(want) {
		t.Errorf("ModTime = %v, want %v", got, want)
	}
}
