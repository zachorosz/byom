package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/storage"
)

type release struct {
	dirID   uuid.UUID
	redirty bool
}

type fakeParseStore struct {
	mu       sync.Mutex
	dirty    map[uuid.UUID]bool
	locked   map[uuid.UUID]bool
	claims   map[uuid.UUID]int
	releases []release
	claimed  chan uuid.UUID
	released chan uuid.UUID
	files    []storage.File
}

func (f *fakeParseStore) markDirty(dirID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dirty == nil {
		f.dirty = map[uuid.UUID]bool{}
	}
	f.dirty[dirID] = true
}

func (f *fakeParseStore) DirtyDirs(_ context.Context, limit int) ([]ClaimedDir, error) {
	f.mu.Lock()
	var claimed []ClaimedDir
	for dirID, isDirty := range f.dirty {
		if len(claimed) >= limit {
			break
		}
		if !isDirty || f.locked[dirID] {
			continue
		}
		f.dirty[dirID] = false
		if f.locked == nil {
			f.locked = map[uuid.UUID]bool{}
		}
		f.locked[dirID] = true
		if f.claims == nil {
			f.claims = map[uuid.UUID]int{}
		}
		f.claims[dirID]++
		claimed = append(claimed, ClaimedDir{ID: dirID, RelPath: "Album"})
	}
	notify := f.claimed
	f.mu.Unlock()

	if notify != nil {
		for _, c := range claimed {
			notify <- c.ID
		}
	}
	return claimed, nil
}

func (f *fakeParseStore) DirFiles(context.Context, uuid.UUID) ([]storage.File, error) {
	return f.files, nil
}

func (f *fakeParseStore) ReleaseDir(_ context.Context, dirID uuid.UUID, _ int64) error {
	f.mu.Lock()
	delete(f.locked, dirID)
	f.releases = append(f.releases, release{dirID: dirID})
	notify := f.released
	f.mu.Unlock()
	if notify != nil {
		notify <- dirID
	}
	return nil
}

func (f *fakeParseStore) ReleaseAndRedirty(_ context.Context, dirID uuid.UUID, _ int64) error {
	f.mu.Lock()
	delete(f.locked, dirID)
	if f.dirty == nil {
		f.dirty = map[uuid.UUID]bool{}
	}
	f.dirty[dirID] = true
	f.releases = append(f.releases, release{dirID: dirID, redirty: true})
	notify := f.released
	f.mu.Unlock()
	if notify != nil {
		notify <- dirID
	}
	return nil
}

func (f *fakeParseStore) redirties() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.releases {
		if r.redirty {
			n++
		}
	}
	return n
}

func (f *fakeParseStore) claimCount(dirID uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.claims[dirID]
}

type fakeLocations struct{ root string }

func (f fakeLocations) Location(_ context.Context, id uuid.UUID) (storage.Location, error) {
	root := f.root
	if root == "" {
		root = "/music"
	}
	return storage.Location{ID: id, URI: "file://" + root, Available: true}, nil
}

type fakeImages struct{}

func (fakeImages) Add(context.Context, io.Reader) (library.Image, error) {
	return library.Image{}, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPipeline_GivesUpAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	store := &fakeParseStore{}
	p := &Pipeline{
		Store:     store,
		Locations: fakeLocations{},
		Images:    fakeImages{},
		Logger:    discardLogger(),
		OnResult: func(context.Context, ParseResult) error {
			return errors.New("import always fails")
		},
	}

	dir := ClaimedDir{
		ID:         uuid.Must(uuid.NewV7()),
		RelPath:    "Album",
		LocationID: uuid.Must(uuid.NewV7()),
	}
	for range maxParseAttempts + 3 {
		p.parseClaimed(ctx, p.logger(), dir)
	}

	if got, want := store.redirties(), maxParseAttempts-1; got != want {
		t.Errorf("re-dirties after %d failing parses = %d, want %d",
			maxParseAttempts+3, got, want)
	}
	last := store.releases[len(store.releases)-1]
	if last.redirty {
		t.Errorf("final release re-dirtied the dir, want it released clean so the pump stops reclaiming it")
	}
}

func TestPipeline_ResetsAttemptsAfterSuccess(t *testing.T) {
	ctx := context.Background()
	store := &fakeParseStore{}
	fail := true
	p := &Pipeline{
		Store:     store,
		Locations: fakeLocations{},
		Images:    fakeImages{},
		Logger:    discardLogger(),
		OnResult: func(context.Context, ParseResult) error {
			if fail {
				return errors.New("transient failure")
			}
			return nil
		},
	}

	dir := ClaimedDir{ID: uuid.Must(uuid.NewV7()), RelPath: "Album", LocationID: uuid.Must(uuid.NewV7())}
	for range maxParseAttempts - 1 {
		p.parseClaimed(ctx, p.logger(), dir)
	}
	fail = false
	p.parseClaimed(ctx, p.logger(), dir)

	fail = true
	store.releases = nil
	for range maxParseAttempts - 1 {
		p.parseClaimed(ctx, p.logger(), dir)
	}

	if got, want := store.redirties(), maxParseAttempts-1; got != want {
		t.Errorf("re-dirties after a success reset = %d, want %d", got, want)
	}
}

func TestPipeline_RetriesFailedDirAfterQueueDrains(t *testing.T) {
	store := &fakeParseStore{claimed: make(chan uuid.UUID, maxParseAttempts*2)}
	dirID := uuid.Must(uuid.NewV7())
	store.markDirty(dirID)

	p := &Pipeline{
		Store:        store,
		Locations:    fakeLocations{},
		Images:       fakeImages{},
		Workers:      1,
		RetryBackoff: time.Millisecond,
		Logger:       discardLogger(),
		OnResult: func(context.Context, ParseResult) error {
			return errors.New("import always fails")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	timeout := time.After(5 * time.Second)
	for attempt := range maxParseAttempts {
		select {
		case <-store.claimed:
		case <-timeout:
			t.Fatalf("dir claimed %d times, want %d: dispatcher slept after attempt %d instead of retrying",
				store.claimCount(dirID), maxParseAttempts, attempt)
		}
	}

	cancel()
	<-done

	if got, want := store.claimCount(dirID), maxParseAttempts; got != want {
		t.Errorf("claims of a permanently failing dir = %d, want %d", got, want)
	}
}

func TestPipeline_RetryDelayGrowsWithFailures(t *testing.T) {
	p := &Pipeline{RetryBackoff: 100 * time.Millisecond}

	tests := []struct {
		failures int
		want     time.Duration
	}{
		{failures: 1, want: 100 * time.Millisecond},
		{failures: 2, want: 200 * time.Millisecond},
		{failures: 3, want: 400 * time.Millisecond},
	}
	for _, tc := range tests {
		if got := p.retryDelay(tc.failures); got != tc.want {
			t.Errorf("retryDelay(%d) = %v, want %v", tc.failures, got, tc.want)
		}
	}
}

func TestPipeline_RetryDelayDefaultsWhenUnset(t *testing.T) {
	p := &Pipeline{}
	if got, want := p.retryDelay(1), defaultRetryBackoff; got != want {
		t.Errorf("retryDelay(1) with no RetryBackoff = %v, want %v", got, want)
	}
}

func TestPipeline_BacksOffBeforeRetryingFailedDir(t *testing.T) {
	store := &fakeParseStore{
		claimed:  make(chan uuid.UUID, maxParseAttempts*2),
		released: make(chan uuid.UUID, maxParseAttempts*2),
	}
	dirID := uuid.Must(uuid.NewV7())
	store.markDirty(dirID)

	p := &Pipeline{
		Store:        store,
		Locations:    fakeLocations{},
		Images:       fakeImages{},
		Workers:      1,
		RetryBackoff: time.Hour,
		Logger:       discardLogger(),
		OnResult: func(context.Context, ParseResult) error {
			return errors.New("import always fails")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	select {
	case <-store.claimed: // the startup pump's claim
	case <-time.After(5 * time.Second):
		t.Fatal("dir was never claimed")
	}

	select {
	case <-store.released:
	case <-time.After(5 * time.Second):
		t.Fatal("dir was never released")
	}

	select {
	case <-store.claimed:
		t.Errorf("dir reclaimed within the backoff window, want the retry deferred by %v", p.RetryBackoff)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	<-done
}

func TestPipeline_BoundsClaimedWorkByWorkerCount(t *testing.T) {
	const workers, queued = 2, 200

	store := &fakeParseStore{claimed: make(chan uuid.UUID, queued)}
	for range queued {
		store.markDirty(uuid.Must(uuid.NewV7()))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &Pipeline{
		Store:     store,
		Locations: fakeLocations{},
		Images:    fakeImages{},
		Workers:   workers,
		Logger:    discardLogger(),
		OnResult: func(ctx context.Context, _ ParseResult) error {
			<-ctx.Done() // hold the claim so the queue backs up
			return ctx.Err()
		},
	}

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	claims := 0
	for i := range 3 * workers {
		select {
		case <-store.claimed:
			claims++
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for claim %d", i+1)
		}
	}

	select {
	case <-store.claimed:
		claims++
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	<-done

	if want := 3 * workers; claims > want {
		t.Errorf("dirs claimed while all %d workers are blocked = %d, want <= %d", workers, claims, want)
	}
}

func TestPipeline_WorkerCountDefaults(t *testing.T) {
	tests := []struct {
		name    string
		workers int
		want    int
	}{
		{name: "Explicit", workers: 4, want: 4},
		{name: "Unset", workers: 0, want: defaultParseWorkers},
		{name: "Negative", workers: -1, want: defaultParseWorkers},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Pipeline{Workers: tc.workers}
			if got := p.workerCount(); got != tc.want {
				t.Errorf("workerCount() with Workers=%d = %d, want %d", tc.workers, got, tc.want)
			}
		})
	}
}

type captureHandler struct {
	mu      sync.Mutex
	records []string
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, b.String())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) matching(substr string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, r := range h.records {
		if strings.Contains(r, substr) {
			out = append(out, r)
		}
	}
	return out
}

func TestPipeline_ReportsPerFileParseErrors(t *testing.T) {
	ctx := context.Background()

	root := t.TempDir()
	albumDir := filepath.Join(root, "Album")
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", albumDir, err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "cover.jpg"), []byte("not an image"), 0o644); err != nil {
		t.Fatalf("write cover.jpg failed: %v", err)
	}

	fileID := uuid.Must(uuid.NewV7())
	store := &fakeParseStore{
		files: []storage.File{{ID: fileID, Name: "cover.jpg", Kind: storage.FileImage}},
	}
	handler := &captureHandler{}

	p := &Pipeline{
		Store:     store,
		Locations: fakeLocations{root: root},
		Images:    failingImageStore{},
		Logger:    slog.New(handler),
	}

	dir := ClaimedDir{
		ID:         uuid.Must(uuid.NewV7()),
		RelPath:    "Album",
		LocationID: uuid.Must(uuid.NewV7()),
	}
	p.parseClaimed(ctx, p.logger(), dir)

	got := handler.matching(fileID.String())
	if len(got) == 0 {
		t.Errorf("no log record mentions the failed file %s; records: %v", fileID, handler.records)
	}
}

type failingImageStore struct{}

func (failingImageStore) Add(context.Context, io.Reader) (library.Image, error) {
	return library.Image{}, errors.New("unsupported image")
}
