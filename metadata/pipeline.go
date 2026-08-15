package metadata

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// maxParseAttempts is how many times a dir may fail to parse before the
// pipeline stops re-queueing it.
const maxParseAttempts = 3

// defaultRetryBackoff is the base delay before a failed dir is
// re-queued, used when Pipeline.RetryBackoff is unset.
const defaultRetryBackoff = time.Second

// defaultParseWorkers is how many dirs are parsed concurrently when
// Pipeline.Workers is unset. Parsing is I/O bound so this is sized
// to hide latency, not to match cores.
const defaultParseWorkers = 16

// workerCount is the number of dirs parsed concurrently.
func (p *Pipeline) workerCount() int {
	if p.Workers > 0 {
		return p.Workers
	}
	return defaultParseWorkers
}

// retryDelay returns how long to wait before re-queueing a dir that has
// failed to parse the given number of consecutive times.
func (p *Pipeline) retryDelay(failures int) time.Duration {
	base := p.RetryBackoff
	if base <= 0 {
		base = defaultRetryBackoff
	}
	if failures < 1 {
		failures = 1
	}
	return base << (failures - 1)
}

// Pipeline runs the metadata parse pipeline: a dispatcher claims dirty
// dirs from the store and a worker pool parses their files. Staging
// between the two is internal; Run starts everything and blocks until
// ctx is cancelled.
type Pipeline struct {
	Store     ParseStore
	Locations LocationResolver
	Images    ImageStore
	// Workers is the number of dirs parsed concurrently. Defaults to
	// defaultParseWorkers when <= 0.
	Workers int
	// OnResult, if set, is called with each parsed dir's result while
	// the dir's parse claim is still held. A nil return releases the
	// claim clean; an error releases it dirty so the dir is retried.
	OnResult func(ctx context.Context, res ParseResult) error
	// RetryBackoff is the base delay before a dir that failed to parse
	// is re-queued; it doubles with each consecutive failure. Defaults
	// to defaultRetryBackoff when <= 0.
	RetryBackoff time.Duration
	Logger       *slog.Logger

	notifyOnce  sync.Once
	dirtyNotify chan struct{}

	attemptsMu sync.Mutex
	attempts   map[uuid.UUID]parseAttempts
}

// parseAttempts counts consecutive failed parses of one dir within a
// single scan generation. A dir re-synced by a later scan arrives with a
// higher generation and starts over with a clean count.
type parseAttempts struct {
	generation int64
	failures   int
}

// Wake is a coalescing trigger that notifies the pipeline to wake up
// and start processing dirty directories. Safe to call before Run.
func (p *Pipeline) Wake() {
	select {
	case p.notify() <- struct{}{}:
	default:
		// dispatcher is already awake or a token is already queued.
		// we can safely drop this notification.
	}
}

func (p *Pipeline) notify() chan struct{} {
	p.notifyOnce.Do(func() { p.dirtyNotify = make(chan struct{}, 1) })
	return p.dirtyNotify
}

func (p *Pipeline) logger() *slog.Logger {
	if p.Logger != nil {
		return p.Logger
	}
	return slog.Default()
}

func (p *Pipeline) Run(ctx context.Context) error {
	workers := p.workerCount()

	// Queue depth and claim batch scale with the worker count: queued
	// dirs are already locked, so they are lost work on a crash, and
	// latency-bound workers gain nothing from a deeper queue.
	toParse := make(chan ClaimedDir, workers)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return p.dispatch(gctx, workers, toParse) })
	for i := range workers {
		g.Go(func() error { return p.runWorker(gctx, i, toParse) })
	}
	return g.Wait()
}

func (p *Pipeline) dispatch(ctx context.Context, claimBatch int, toParse chan<- ClaimedDir) error {
	// Startup sweep to process any dirty dirs leftover from previous runs.
	if err := p.pump(ctx, claimBatch, toParse); err != nil {
		return err
	}

	// Wait for live triggers via Wake in an event loop.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.notify():
			// wake up and drain the DB until empty
			if err := p.pump(ctx, claimBatch, toParse); err != nil {
				return err
			}
		}
	}
}

// pump pulls from the DB and blocks if the parser queue is full.
// It returns when there are exactly 0 dirty directories left.
func (p *Pipeline) pump(ctx context.Context, claimBatch int, toParse chan<- ClaimedDir) error {
	for {
		claimed, err := p.Store.DirtyDirs(ctx, claimBatch)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// TODO: maybe do something better here?
			p.logger().Error("fetch dirty directories failed", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
			continue
		}
		if len(claimed) == 0 {
			return nil // DB is clean, exit pump and go back to sleep.
		}
		for _, c := range claimed {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case toParse <- c:
				// if the parser queue is full, this gracefully blocks,
				// applying backpressure all the way up to this loop.
			}
		}
	}
}

func (p *Pipeline) runWorker(ctx context.Context, workerID int, in <-chan ClaimedDir) error {
	logger := p.logger().With(slog.Int("worker", workerID))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case dir := <-in:
			p.parseClaimed(ctx, logger, dir)
		}
	}
}

// parseClaimed parses one claimed dir end to end: resolve its location,
// parse its files, hand the result to OnResult, and release the claim.
func (p *Pipeline) parseClaimed(ctx context.Context, logger *slog.Logger, dir ClaimedDir) {
	logger = logger.With(
		slog.String("location_id", dir.LocationID.String()),
		slog.String("dir_id", dir.ID.String()),
		slog.Int64("locked_generation", dir.LockedGeneration),
	)

	loc, err := p.Locations.Location(ctx, dir.LocationID)
	if err != nil {
		logger.Error("unknown location", slog.Any("error", err))
		p.releaseFailed(ctx, logger, dir)
		return
	}

	root, err := loc.Root()
	if err != nil {
		logger.Error("resolve location root failed", slog.Any("error", err))
		p.releaseFailed(ctx, logger, dir)
		return
	}

	files, err := p.Store.DirFiles(ctx, dir.ID)
	if err != nil {
		logger.Error("read dir files failed", slog.Any("error", err))
		p.releaseFailed(ctx, logger, dir)
		return
	}

	path := filepath.Join(root, dir.RelPath)
	res := parseDir(ctx, p.Images, path, dir.ID, files)

	// A dir with a broken file still imports, so this is the only
	// record that anything was dropped.
	// TODO: persist w/ Importer
	for _, pe := range res.Errors {
		logger.Warn("parse file failed",
			slog.String("file_id", pe.FileID.String()),
			slog.String("reason", pe.Message))
	}

	if p.OnResult != nil {
		if err := p.OnResult(ctx, res); err != nil {
			// The result wasn't persisted; keep the dir dirty so the
			// parse is retried.
			logger.Error("handle parse result failed", slog.Any("error", err))
			p.releaseFailed(ctx, logger, dir)
			return
		}
	}

	p.clearAttempts(dir.ID)

	if err := p.Store.ReleaseDir(ctx, dir.ID, dir.LockedGeneration); err != nil {
		// Lock already gone (crash-recovery reap or duplicate
		// dispatch). Log but don't fail the parse itself, the
		// work already happened successfully.
		logger.Warn("release dir lock failed", slog.Any("error", err))
	}
}

// releaseFailed releases the lock after a failed parse, leaving the dir
// dirty for a retry until it has burned maxParseAttempts on this
// generation, then releasing it clean so the dispatcher stops
// reclaiming it. Detached from the original context so that
// cancellations don't lock unfinished dirs.
func (p *Pipeline) releaseFailed(parentCtx context.Context, logger *slog.Logger, dir ClaimedDir) {
	detachedCtx := context.WithoutCancel(parentCtx)

	releaseCtx, cancel := context.WithTimeout(detachedCtx, 5*time.Second)
	defer cancel()

	failures := p.recordFailure(dir)
	if failures >= maxParseAttempts {
		logger.Error("giving up on dir after repeated parse failures",
			slog.Int("failures", failures),
			slog.Int("max_attempts", maxParseAttempts))
		if err := p.Store.ReleaseDir(releaseCtx, dir.ID, dir.LockedGeneration); err != nil {
			logger.Warn("release dir lock failed", slog.Any("error", err))
		}
		return
	}

	if err := p.Store.ReleaseAndRedirty(releaseCtx, dir.ID, dir.LockedGeneration); err != nil {
		logger.Warn("release+dirty failed", slog.Any("error", err))
		return
	}

	if parentCtx.Err() != nil || releaseCtx.Err() != nil {
		// skip re-enqueue if canceled or timeout
		return
	}

	delay := p.retryDelay(failures)
	logger.Warn("re-queueing dir after failed parse",
		slog.Int("failures", failures),
		slog.Duration("retry_in", delay))
	time.AfterFunc(delay, p.Wake)
}

// recordFailure counts a failed parse of dir and returns the running
// total for dir's current generation.
func (p *Pipeline) recordFailure(dir ClaimedDir) int {
	p.attemptsMu.Lock()
	defer p.attemptsMu.Unlock()

	if p.attempts == nil {
		p.attempts = map[uuid.UUID]parseAttempts{}
	}
	at := p.attempts[dir.ID]
	if at.generation != dir.LockedGeneration {
		at = parseAttempts{generation: dir.LockedGeneration}
	}
	at.failures++
	p.attempts[dir.ID] = at
	return at.failures
}

func (p *Pipeline) clearAttempts(dirID uuid.UUID) {
	p.attemptsMu.Lock()
	defer p.attemptsMu.Unlock()
	delete(p.attempts, dirID)
}
