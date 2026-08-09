package metadata

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Pipeline runs the metadata parse pipeline: a dispatcher claims dirty
// dirs from the store and a worker pool parses their files. Staging
// between the two is internal; Run starts everything and blocks until
// ctx is cancelled.
type Pipeline struct {
	Store     ParseStore
	Locations LocationResolver
	Images    ImageStore
	// Workers is the number of concurrent parser workers.
	// Defaults to runtime.NumCPU() when <= 0.
	Workers int
	// OnResult, if set, is called with each parsed dir's result while
	// the dir's parse claim is still held. A nil return releases the
	// claim clean; an error releases it dirty so the dir is retried.
	OnResult func(ctx context.Context, res ParseResult) error
	Logger   *slog.Logger

	notifyOnce  sync.Once
	dirtyNotify chan struct{}
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
	workers := p.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	toParse := make(chan ClaimedDir, 100)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return p.dispatch(gctx, toParse) })
	for i := range workers {
		g.Go(func() error { return p.runWorker(gctx, i, toParse) })
	}
	return g.Wait()
}

func (p *Pipeline) dispatch(ctx context.Context, toParse chan<- ClaimedDir) error {
	// Startup sweep to process any dirty dirs leftover from previous runs.
	if err := p.pump(ctx, toParse); err != nil {
		return err
	}

	// Wait for live triggers via Wake in an event loop.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.notify():
			// wake up and drain the DB until empty
			if err := p.pump(ctx, toParse); err != nil {
				return err
			}
		}
	}
}

// pump pulls from the DB and blocks if the parser queue is full.
// It returns when there are exactly 0 dirty directories left.
func (p *Pipeline) pump(ctx context.Context, toParse chan<- ClaimedDir) error {
	for {
		claimed, err := p.Store.DirtyDirs(ctx, 50)
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
		p.releaseAndRedirty(logger, dir)
		return
	}

	root, err := loc.Root()
	if err != nil {
		logger.Error("resolve location root failed", slog.Any("error", err))
		p.releaseAndRedirty(logger, dir)
		return
	}

	files, err := p.Store.DirFiles(ctx, dir.ID)
	if err != nil {
		// A transient DB error here shouldn't kill the whole pool, but
		// it also can't be silently dropped, since we have a parse lock
		// on this dir (locked_generation is set).
		logger.Error("read dir files failed", slog.Any("error", err))
		p.releaseAndRedirty(logger, dir)
		return
	}

	path := filepath.Join(root, dir.RelPath)
	res := parseDir(ctx, p.Images, path, dir.ID, files)

	if p.OnResult != nil {
		if err := p.OnResult(ctx, res); err != nil {
			// The result wasn't persisted; keep the dir dirty so the
			// parse is retried.
			logger.Error("handle parse result failed", slog.Any("error", err))
			p.releaseAndRedirty(logger, dir)
			return
		}
	}

	if err := p.Store.ReleaseDir(ctx, dir.ID, dir.LockedGeneration); err != nil {
		// Lock already gone (crash-recovery reap or duplicate
		// dispatch). Log but don't fail the parse itself, the
		// work already happened successfully.
		logger.Warn("release dir lock failed", slog.Any("error", err))
	}
}

// releaseAndRedirty releases the lock but re-requests a retry by leaving the
// dirty flag set. Detatched from the original context so that cancellations
// don't lock unfinished dirs.
func (p *Pipeline) releaseAndRedirty(logger *slog.Logger, dir ClaimedDir) {
	// TODO: max retries and escalate to log.Error. if failing
	// because of something persistent (e.g corrupt row, dir deleted
	// mid-parse but not swept), we will spin on it every pump().
	ctx := context.Background()
	if err := p.Store.ReleaseAndRedirty(ctx, dir.ID, dir.LockedGeneration); err != nil {
		logger.Warn("release+dirty failed", slog.Any("error", err))
	}
}
