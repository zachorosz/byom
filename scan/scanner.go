package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/zachorosz/byom/storage"
)

// State is a scan's lifecycle state. All but StateCancelling are
// persisted; cancelling lives only in memory, between a cancel request
// and the scan unwinding.
type State string

const (
	StateRunning    State = "running"
	StateCancelling State = "cancelling"
	StateDone       State = "done"
	StateFailed     State = "failed"
	StateAborted    State = "aborted"
)

// Scan is the record of one scan of a location.
type Scan struct {
	ID         uuid.UUID
	LocationID uuid.UUID
	State      State
	StartTime  time.Time
	FinishTime time.Time
	Error      string
	Progress   Progress
}

// Progress counts what a scan has synchronized so far. The missing
// counts stay zero until the sweep, which only runs on success.
type Progress struct {
	DirsSeen     int64
	DirsMissing  int64
	FilesSeen    int64
	FilesMissing int64
}

// run is one scan in flight: its filesystem, identity, cancellation,
// and live counters. Scanner owns the shared configuration and store.
type run struct {
	scanID     uuid.UUID
	locationID uuid.UUID
	gen        int64
	cancel     context.CancelFunc

	fsys fs.FS

	dirsSeen     atomic.Int64
	dirsMissing  atomic.Int64
	filesSeen    atomic.Int64
	filesMissing atomic.Int64
	cancelling   atomic.Bool
}

// progress returns the counts so far, during the scan or after.
func (r *run) progress() Progress {
	return Progress{
		DirsSeen:     r.dirsSeen.Load(),
		DirsMissing:  r.dirsMissing.Load(),
		FilesSeen:    r.filesSeen.Load(),
		FilesMissing: r.filesMissing.Load(),
	}
}

var (
	// ErrScanRunning reports that the location already has a scan in flight.
	ErrScanRunning = errors.New("scan already running")
	// ErrNotRunning reports that a scan has already finished.
	ErrNotRunning = errors.New("scan not running")
)

type Store interface {
	BeginScan(ctx context.Context, locationID uuid.UUID) (gen int64, scanID uuid.UUID, err error)
	FinishScan(ctx context.Context, scanID uuid.UUID, progress Progress, scanErr error) error
	AbortRunningScans(ctx context.Context) (int64, error)
	Scan(ctx context.Context, id uuid.UUID) (Scan, error)
	Scans(ctx context.Context, locationID uuid.UUID, state State, token string, limit int) ([]Scan, string, error)
	DirID(ctx context.Context, locationID uuid.UUID, relpath string) (uuid.UUID, error)
	KnownFiles(ctx context.Context, dirID uuid.UUID) (map[string]storage.File, error)
	SyncDir(ctx context.Context, payload SyncPayload) (uuid.UUID, error)
	Sweep(ctx context.Context, locationID uuid.UUID, gen int64) (dirs, files int64, err error)
}

// LocationResolver resolves the location a scan runs against.
type LocationResolver interface {
	Location(ctx context.Context, id uuid.UUID) (storage.Location, error)
}

const (
	// closeTimeout bounds the bookkeeping that must outlive a cancelled scan.
	closeTimeout = 30 * time.Second
)

// Scanner synchronizes locations with the database: it opens a scan
// record, walks the tree with a worker pool, and closes the record. A
// scan outlives the request that started it and stops only on Cancel
// or Shutdown.
type Scanner struct {
	Store     Store
	Locations LocationResolver
	// Workers is the number of concurrent sync workers. Defaults to
	// runtime.NumCPU() when <= 0.
	Workers int
	// OnDirty, if set, is called after each synced dir whose change set
	// needs (re)parsing.
	OnDirty func()
	Logger  *slog.Logger

	mu     sync.Mutex
	byLoc  map[uuid.UUID]*run
	byScan map[uuid.UUID]*run
	wg     sync.WaitGroup
}

func (s *Scanner) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Scanner) workerCount() int {
	if s.Workers > 0 {
		return s.Workers
	}
	return runtime.NumCPU()
}

// Recover marks scans that a previous process left running as aborted.
// Call it at startup, before serving.
func (s *Scanner) Recover(ctx context.Context) error {
	n, err := s.Store.AbortRunningScans(ctx)
	if err != nil {
		return fmt.Errorf("recover scans: %w", err)
	}
	if n > 0 {
		s.logger().Warn("aborted scans left running by a previous process", slog.Int64("count", n))
	}
	return nil
}

// Start opens a scan record and runs the scan in the background,
// returning once the record exists. It reports ErrScanRunning if the
// location is already being scanned. The scan is detached from ctx.
func (s *Scanner) Start(ctx context.Context, locationID uuid.UUID) (Scan, error) {
	loc, err := s.Locations.Location(ctx, locationID)
	if err != nil {
		return Scan{}, err
	}
	root, err := loc.Root()
	if err != nil {
		return Scan{}, err
	}

	// The cancel func is set before the location is claimed so Shutdown
	// can always reach a claimed run, even one still opening its record.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r := &run{
		locationID: locationID,
		cancel:     cancel,
		fsys:       os.DirFS(root),
	}
	if err := s.claim(r); err != nil {
		cancel()
		return Scan{}, err
	}

	gen, scanID, err := s.Store.BeginScan(ctx, locationID)
	if err != nil {
		s.release(r)
		return Scan{}, fmt.Errorf("begin scan: %w", err)
	}

	s.mu.Lock()
	r.scanID = scanID
	r.gen = gen
	s.byScan[scanID] = r
	s.mu.Unlock()

	s.wg.Go(func() {
		defer cancel()
		s.runScan(runCtx, r)
	})

	return s.Scan(ctx, scanID)
}

// claim reserves the location for r so two concurrent starts cannot
// both open a record for it.
func (s *Scanner) claim(r *run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.byLoc == nil {
		s.byLoc = make(map[uuid.UUID]*run)
		s.byScan = make(map[uuid.UUID]*run)
	}
	if _, ok := s.byLoc[r.locationID]; ok {
		return fmt.Errorf("location %s: %w", r.locationID, ErrScanRunning)
	}
	s.byLoc[r.locationID] = r
	return nil
}

// runScan syncs the tree and closes the record, whatever the outcome.
func (s *Scanner) runScan(ctx context.Context, r *run) {
	scanErr := s.sync(ctx, r)

	if scanErr != nil && !errors.Is(scanErr, context.Canceled) {
		s.logger().Error("scan failed",
			slog.Any("error", scanErr),
			slog.String("location_id", r.locationID.String()),
			slog.String("scan_id", r.scanID.String()))
	}

	// ctx may already be cancelled; closing the record runs detached
	// from it, but time-bounded so shutdown cannot hang.
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeTimeout)
	defer cancel()
	if err := s.Store.FinishScan(closeCtx, r.scanID, r.progress(), scanErr); err != nil {
		s.logger().Error("finish scan failed",
			slog.Any("error", err), slog.String("scan_id", r.scanID.String()))
	}

	s.release(r)
}

// sync walks the run's filesystem post-order, merging disc folders
// into their parent album dirs, and marks unseen items missing only if
// the walk completed. The counts land on the run either way.
func (s *Scanner) sync(ctx context.Context, r *run) error {
	dirs := make(chan walkResult, 32)
	g, gctx := errgroup.WithContext(ctx)

	for range s.workerCount() {
		g.Go(func() error {
			return s.syncWorker(gctx, r, dirs)
		})
	}

	g.Go(func() error {
		defer close(dirs)
		merger := newDiscMerger(func(res walkResult) error {
			select {
			case <-gctx.Done():
				return gctx.Err()
			case dirs <- res:
			}
			return nil
		})
		return walkPostOrder(r.fsys, ".", merger.process)
	})

	// Only sweep if the scan completed successfully; otherwise, unvisited
	// directories would be incorrectly marked as missing.
	if err := g.Wait(); err != nil {
		return err
	}
	if err := s.sweep(ctx, r); err != nil {
		s.logger().Warn("sweep failed", slog.Any("error", err))
	}
	return nil
}

func (s *Scanner) sweep(ctx context.Context, r *run) error {
	dirs, files, err := s.Store.Sweep(ctx, r.locationID, r.gen)
	if err != nil {
		return err
	}
	r.dirsMissing.Add(dirs)
	r.filesMissing.Add(files)
	return nil
}

// Cancel stops a running scan. The scan reports StateCancelling until
// it unwinds and records itself as aborted.
func (s *Scanner) Cancel(ctx context.Context, scanID uuid.UUID) error {
	s.mu.Lock()
	r, ok := s.byScan[scanID]
	running := ok && s.byLoc[r.locationID] == r
	s.mu.Unlock()

	if !running {
		// Read it back so an unknown ID and an already-finished scan
		// report differently.
		if _, err := s.Store.Scan(ctx, scanID); err != nil {
			return err
		}
		return fmt.Errorf("scan %s: %w", scanID, ErrNotRunning)
	}

	r.cancelling.Store(true)
	r.cancel()
	return nil
}

// Scan returns the scan with id, carrying live progress while the scan
// is still tracked in memory.
func (s *Scanner) Scan(ctx context.Context, id uuid.UUID) (Scan, error) {
	sc, err := s.Store.Scan(ctx, id)
	if err != nil {
		return Scan{}, err
	}
	return s.overlay(sc), nil
}

// Scans returns a page of scans, newest first, optionally restricted
// to a location and a state.
func (s *Scanner) Scans(ctx context.Context, locationID uuid.UUID, state State, token string, limit int) ([]Scan, string, error) {
	scans, next, err := s.Store.Scans(ctx, locationID, state, token, limit)
	if err != nil {
		return nil, "", err
	}
	for i := range scans {
		scans[i] = s.overlay(scans[i])
	}
	return scans, next, nil
}

// Shutdown cancels every running scan and waits for them to record how
// they ended, or until ctx is done.
func (s *Scanner) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	for _, r := range s.byLoc {
		r.cancel()
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// overlay fills in live progress and the transient cancelling state.
func (s *Scanner) overlay(sc Scan) Scan {
	s.mu.Lock()
	r, ok := s.byScan[sc.ID]
	s.mu.Unlock()
	if !ok {
		return sc
	}

	sc.Progress = r.progress()
	if sc.State == StateRunning && r.cancelling.Load() {
		sc.State = StateCancelling
	}
	return sc
}

// release frees all in-memory state. Completed scans read their durable
// state, including progress, from the store.
func (s *Scanner) release(r *run) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r.fsys = nil // the walk is over; don't pin it for the cache's lifetime
	delete(s.byLoc, r.locationID)
	delete(s.byScan, r.scanID)
}
