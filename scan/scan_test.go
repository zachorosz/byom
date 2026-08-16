package scan

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/storage"
)

type fakeStore struct {
	syncErr error

	mu     sync.Mutex
	swept  bool
	finish error
}

func (f *fakeStore) BeginScan(ctx context.Context, locationID uuid.UUID) (int64, uuid.UUID, error) {
	return 1, uuid.Must(uuid.NewV7()), nil
}

func (f *fakeStore) FinishScan(ctx context.Context, scanID uuid.UUID, scanErr error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finish = scanErr
	return nil
}

func (f *fakeStore) DirID(ctx context.Context, locationID uuid.UUID, relpath string) (uuid.UUID, error) {
	return uuid.Nil, storage.ErrNotExists
}

func (f *fakeStore) KnownFiles(ctx context.Context, dirID uuid.UUID) (map[string]storage.File, error) {
	return map[string]storage.File{}, nil
}

func (f *fakeStore) SyncDir(ctx context.Context, payload SyncPayload) (uuid.UUID, error) {
	return uuid.Must(uuid.NewV7()), f.syncErr
}

func (f *fakeStore) Sweep(ctx context.Context, locationID uuid.UUID, gen int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.swept = true
	return 0, nil
}

func (f *fakeStore) sweptCalled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.swept
}

func TestScanFailedScanDoesNotSweep(t *testing.T) {
	fsys := fstest.MapFS{
		"a/one.flac": {Data: []byte("x")},
		"b/two.flac": {Data: []byte("x")},
	}
	syncErr := errors.New("sync dir failed")
	store := &fakeStore{syncErr: syncErr}
	scanner := &Scanner{Store: store, Workers: 1}
	loc := storage.Location{ID: uuid.Must(uuid.NewV7())}

	err := scanner.Scan(context.Background(), fsys, loc)

	if !errors.Is(err, syncErr) {
		t.Fatalf("Scan() returned an unexpected error: %v", err)
	}

	if got := store.sweptCalled(); got != false {
		t.Errorf("Scan() swept = %v, want %v", got, false)
	}
}

func TestScanSuccessfulScanSweeps(t *testing.T) {
	fsys := fstest.MapFS{
		"a/one.flac": {Data: []byte("x")},
		"b/two.flac": {Data: []byte("x")},
	}
	store := &fakeStore{}
	scanner := &Scanner{Store: store, Workers: 1}
	loc := storage.Location{ID: uuid.Must(uuid.NewV7())}

	if err := scanner.Scan(context.Background(), fsys, loc); err != nil {
		t.Fatalf("Scan() returned an unexpected error: %v", err)
	}

	if got := store.sweptCalled(); got != true {
		t.Errorf("Scan() swept = %v, want %v", got, true)
	}
}
