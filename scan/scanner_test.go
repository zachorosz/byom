package scan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/storage"
)

func newSyncTestRun(store Store, fsys fstest.MapFS) (*Scanner, *run) {
	s := &Scanner{
		Store:   store,
		Workers: 1,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return s, &run{
		locationID: uuid.Must(uuid.NewV7()),
		gen:        1,
		fsys:       fsys,
	}
}

func TestSyncFailedScanDoesNotSweep(t *testing.T) {
	fsys := fstest.MapFS{
		"a/one.flac": {Data: []byte("x")},
		"b/two.flac": {Data: []byte("x")},
	}
	syncErr := errors.New("sync dir failed")
	store := newFakeScanStore()
	store.syncDir = func(context.Context, SyncPayload) (uuid.UUID, error) {
		return uuid.Nil, syncErr
	}

	s, r := newSyncTestRun(store, fsys)
	err := s.sync(context.Background(), r)

	if !errors.Is(err, syncErr) {
		t.Fatalf("sync() returned an unexpected error: %v", err)
	}
	if got := store.sweptCalled(); got {
		t.Errorf("sync() swept = %v, want false", got)
	}
}

func TestSyncSuccessfulScanSweeps(t *testing.T) {
	fsys := fstest.MapFS{
		"a/one.flac": {Data: []byte("x")},
		"b/two.flac": {Data: []byte("x")},
	}
	store := newFakeScanStore()
	s, r := newSyncTestRun(store, fsys)
	if err := s.sync(context.Background(), r); err != nil {
		t.Fatalf("sync() returned an unexpected error: %v", err)
	}
	if got := store.sweptCalled(); !got {
		t.Errorf("sync() swept = %v, want true", got)
	}
}

func TestSyncCountsProgress(t *testing.T) {
	fsys := fstest.MapFS{
		"a/one.flac":   {Data: []byte("x")},
		"a/two.flac":   {Data: []byte("x")},
		"b/three.flac": {Data: []byte("x")},
	}
	store := newFakeScanStore()
	store.sweep = func(context.Context, uuid.UUID, int64) (int64, int64, error) {
		return 3, 4, nil
	}
	s, r := newSyncTestRun(store, fsys)
	if err := s.sync(context.Background(), r); err != nil {
		t.Fatalf("sync() returned an unexpected error: %v", err)
	}

	want := Progress{DirsSeen: 3, DirsMissing: 3, FilesSeen: 3, FilesMissing: 4}
	if diff := cmp.Diff(want, r.progress()); diff != "" {
		t.Errorf("progress() mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncWorkerErrorCancelsWalk(t *testing.T) {
	fsys := make(fstest.MapFS, 40)
	for i := range 40 {
		fsys[fmt.Sprintf("album-%02d/track.flac", i)] = &fstest.MapFile{Data: []byte("x")}
	}
	syncErr := errors.New("sync dir failed")
	store := newFakeScanStore()
	store.syncDir = func(context.Context, SyncPayload) (uuid.UUID, error) {
		return uuid.Nil, syncErr
	}
	s, r := newSyncTestRun(store, fsys)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.sync(ctx, r) }()

	select {
	case err := <-done:
		if !errors.Is(err, syncErr) {
			t.Errorf("sync() error = %v, want sync error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("sync() did not stop after a worker error")
	}
}

// fakeScanStore is an in-memory Store with optional behavior hooks for
// synchronization operations.
type fakeScanStore struct {
	mu         sync.Mutex
	scans      map[uuid.UUID]Scan
	gen        int64
	abortCalls int64
	// finishGate, when non-nil, holds FinishScan until it is closed.
	finishGate chan struct{}
	// finished receives each scan ID once its record is final.
	finished chan uuid.UUID
	syncDir  func(context.Context, SyncPayload) (uuid.UUID, error)
	sweep    func(context.Context, uuid.UUID, int64) (int64, int64, error)
	swept    bool
}

func newFakeScanStore() *fakeScanStore {
	return &fakeScanStore{
		scans:    make(map[uuid.UUID]Scan),
		finished: make(chan uuid.UUID, 4),
	}
}

func (f *fakeScanStore) BeginScan(ctx context.Context, locationID uuid.UUID) (int64, uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gen++
	id := uuid.Must(uuid.NewV7())
	f.scans[id] = Scan{
		ID:         id,
		LocationID: locationID,
		State:      StateRunning,
		StartTime:  time.Now(),
	}
	return f.gen, id, nil
}

func (f *fakeScanStore) FinishScan(ctx context.Context, scanID uuid.UUID, progress Progress, scanErr error) error {
	if f.finishGate != nil {
		<-f.finishGate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	sc := f.scans[scanID]
	switch {
	case errors.Is(scanErr, context.Canceled):
		sc.State = StateAborted
	case scanErr != nil:
		sc.State = StateFailed
		sc.Error = scanErr.Error()
	default:
		sc.State = StateDone
	}
	sc.FinishTime = time.Now()
	sc.Progress = progress
	f.scans[scanID] = sc
	f.finished <- scanID
	return nil
}

func (f *fakeScanStore) DirID(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	return uuid.Nil, storage.ErrNotExists
}

func (f *fakeScanStore) KnownFiles(context.Context, uuid.UUID) (map[string]storage.File, error) {
	return map[string]storage.File{}, nil
}

func (f *fakeScanStore) SyncDir(ctx context.Context, payload SyncPayload) (uuid.UUID, error) {
	if f.syncDir != nil {
		return f.syncDir(ctx, payload)
	}
	return uuid.Must(uuid.NewV7()), nil
}

func (f *fakeScanStore) Sweep(ctx context.Context, locationID uuid.UUID, gen int64) (int64, int64, error) {
	f.mu.Lock()
	f.swept = true
	f.mu.Unlock()
	if f.sweep != nil {
		return f.sweep(ctx, locationID, gen)
	}
	return 0, 0, nil
}

func (f *fakeScanStore) sweptCalled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.swept
}

func (f *fakeScanStore) AbortRunningScans(ctx context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.abortCalls++
	return f.abortCalls, nil
}

func (f *fakeScanStore) Scan(ctx context.Context, id uuid.UUID) (Scan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sc, ok := f.scans[id]
	if !ok {
		return Scan{}, storage.ErrNotExists
	}
	return sc, nil
}

func (f *fakeScanStore) Scans(ctx context.Context, locationID uuid.UUID, state State, token string, limit int) ([]Scan, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var scans []Scan
	for _, sc := range f.scans {
		if locationID != uuid.Nil && sc.LocationID != locationID {
			continue
		}
		if state != "" && sc.State != state {
			continue
		}
		scans = append(scans, sc)
	}
	return scans, "", nil
}

type fixedLocationResolver struct {
	loc storage.Location
}

func (f *fixedLocationResolver) Location(context.Context, uuid.UUID) (storage.Location, error) {
	return f.loc, nil
}

// newScannerWithTestLibrary wires store to a two-album tree on disk.
func newScannerWithTestLibrary(t *testing.T, store *fakeScanStore) (*Scanner, uuid.UUID) {
	t.Helper()

	root := t.TempDir()
	for _, album := range []string{"a", "b"} {
		dir := filepath.Join(root, album)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("create album dir failed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, album+".flac"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write track failed: %v", err)
		}
	}

	locationID := uuid.Must(uuid.NewV7())
	s := &Scanner{
		Store:     store,
		Locations: &fixedLocationResolver{loc: storage.Location{ID: locationID, URI: root}},
		Workers:   1,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return s, locationID
}

func syncUntilReleased(release <-chan struct{}) func(context.Context, SyncPayload) (uuid.UUID, error) {
	return func(ctx context.Context, _ SyncPayload) (uuid.UUID, error) {
		select {
		case <-release:
			return uuid.Must(uuid.NewV7()), nil
		case <-ctx.Done():
			return uuid.Nil, ctx.Err()
		}
	}
}

func TestScannerRecoverAbortsStaleScans(t *testing.T) {
	store := newFakeScanStore()
	s, _ := newScannerWithTestLibrary(t, store)

	if err := s.Recover(context.Background()); err != nil {
		t.Fatalf("Recover() returned an unexpected error: %v", err)
	}
	if got := store.abortCalls; got != 1 {
		t.Errorf("Recover() aborted = %d, want %d", got, 1)
	}
}

func TestScannerStartRejectsSecondScanOfSameLocation(t *testing.T) {
	ctx := context.Background()
	release := make(chan struct{})
	store := newFakeScanStore()
	store.syncDir = syncUntilReleased(release)
	s, locationID := newScannerWithTestLibrary(t, store)
	t.Cleanup(func() {
		close(release)
		s.Shutdown(ctx)
	})

	if _, err := s.Start(ctx, locationID); err != nil {
		t.Fatalf("Start() returned an unexpected error: %v", err)
	}

	_, err := s.Start(ctx, locationID)
	if !errors.Is(err, ErrScanRunning) {
		t.Errorf("Start() second call error = %v, want ErrScanRunning", err)
	}
}

func TestScannerCancel(t *testing.T) {
	ctx := context.Background()
	release := make(chan struct{})
	store := newFakeScanStore()
	store.syncDir = syncUntilReleased(release)
	store.finishGate = make(chan struct{})
	s, locationID := newScannerWithTestLibrary(t, store)

	started, err := s.Start(ctx, locationID)
	if err != nil {
		t.Fatalf("Start() returned an unexpected error: %v", err)
	}
	if started.State != StateRunning {
		t.Fatalf("Start() state = %q, want %q", started.State, StateRunning)
	}

	if err := s.Cancel(ctx, started.ID); err != nil {
		t.Fatalf("Cancel() returned an unexpected error: %v", err)
	}

	// The record is still running while the scan unwinds, so the
	// scanner must report the transient cancelling state.
	got, err := s.Scan(ctx, started.ID)
	if err != nil {
		t.Fatalf("Scan() returned an unexpected error: %v", err)
	}
	if got.State != StateCancelling {
		t.Errorf("Scan() state = %q, want %q", got.State, StateCancelling)
	}

	close(store.finishGate)
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() returned an unexpected error: %v", err)
	}

	got, err = s.Scan(ctx, started.ID)
	if err != nil {
		t.Fatalf("Scan() returned an unexpected error: %v", err)
	}
	if got.State != StateAborted {
		t.Errorf("Scan() state after cancel = %q, want %q", got.State, StateAborted)
	}
}

func TestScannerScansFiltersCancellingState(t *testing.T) {
	ctx := context.Background()
	release := make(chan struct{})
	store := newFakeScanStore()
	store.syncDir = syncUntilReleased(release)
	store.finishGate = make(chan struct{})
	s, locationID := newScannerWithTestLibrary(t, store)
	t.Cleanup(func() {
		close(store.finishGate)
		s.Shutdown(ctx)
	})

	started, err := s.Start(ctx, locationID)
	if err != nil {
		t.Fatalf("Start() returned an unexpected error: %v", err)
	}
	if err := s.Cancel(ctx, started.ID); err != nil {
		t.Fatalf("Cancel() returned an unexpected error: %v", err)
	}

	got, _, err := s.Scans(ctx, uuid.Nil, StateCancelling, "", 0)
	if err != nil {
		t.Fatalf("Scans() returned an unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != started.ID {
		t.Errorf("Scans(StateCancelling) = %+v, want [%s]", got, started.ID)
	}
	if got[0].State != StateCancelling {
		t.Errorf("Scans(StateCancelling)[0].State = %q, want %q", got[0].State, StateCancelling)
	}
}

func TestScannerCancelFinishedScan(t *testing.T) {
	ctx := context.Background()
	s, locationID := newScannerWithTestLibrary(t, newFakeScanStore())

	started, err := s.Start(ctx, locationID)
	if err != nil {
		t.Fatalf("Start() returned an unexpected error: %v", err)
	}
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() returned an unexpected error: %v", err)
	}

	err = s.Cancel(ctx, started.ID)
	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("Cancel() error = %v, want ErrNotRunning", err)
	}
}

func TestScannerCancelUnknownScan(t *testing.T) {
	s, _ := newScannerWithTestLibrary(t, newFakeScanStore())

	err := s.Cancel(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, storage.ErrNotExists) {
		t.Errorf("Cancel() error = %v, want storage.ErrNotExists", err)
	}
}

func TestScannerReportsProgress(t *testing.T) {
	ctx := context.Background()
	store := newFakeScanStore()
	store.sweep = func(context.Context, uuid.UUID, int64) (int64, int64, error) {
		return 1, 2, nil
	}
	s, locationID := newScannerWithTestLibrary(t, store)

	started, err := s.Start(ctx, locationID)
	if err != nil {
		t.Fatalf("Start() returned an unexpected error: %v", err)
	}
	// Let the scan finish on its own; Shutdown would cancel it.
	<-store.finished
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() returned an unexpected error: %v", err)
	}

	got, err := s.Scan(ctx, started.ID)
	if err != nil {
		t.Fatalf("Scan() returned an unexpected error: %v", err)
	}
	if got.State != StateDone {
		t.Fatalf("Scan() state = %q, want %q", got.State, StateDone)
	}

	// Two album dirs plus the root, two files, and the sweep's counts.
	want := Progress{DirsSeen: 3, DirsMissing: 1, FilesSeen: 2, FilesMissing: 2}
	if diff := cmp.Diff(want, got.Progress); diff != "" {
		t.Errorf("Scan() progress mismatch (-want +got):\n%s", diff)
	}
}
