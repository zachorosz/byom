package metadata

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/storage"
	"golang.org/x/sync/errgroup"
)

// ClaimedDir is a dir claimed from the parse queue. LockedGeneration is
// the generation captured at claim time and must be handed back when
// releasing the claim.
type ClaimedDir struct {
	ID               uuid.UUID
	RelPath          string
	LocationID       uuid.UUID
	LockedGeneration int64
}

type ParseResult struct {
	DirID uuid.UUID
	// AlbumMetadata
	// AlbumArtists
	// Tracks
	// ParseErrors
}

type ParseQueue interface {
	DirtyDirs(context.Context, int) ([]ClaimedDir, error)
}

// ParseStore is the claimed-dir surface a parser worker needs: reading
// a claimed dir's files and releasing the claim when done or failed.
type ParseStore interface {
	DirFiles(ctx context.Context, dirID uuid.UUID) ([]storage.File, error)
	ReleaseDir(ctx context.Context, dirID uuid.UUID, lockedGen int64) error
	ReleaseAndRedirty(ctx context.Context, dirID uuid.UUID, lockedGen int64) error
}

// LocationResolver resolves a location ID to its location.
type LocationResolver interface {
	Location(ctx context.Context, id uuid.UUID) (storage.Location, error)
}

type ParseDispatcher struct {
	store       ParseQueue
	dirtyNotify chan struct{}
	toParse     chan<- ClaimedDir
	logger      *slog.Logger
}

func NewParseDispatcher(store ParseQueue, toParse chan<- ClaimedDir) *ParseDispatcher {
	return &ParseDispatcher{
		store:       store,
		dirtyNotify: make(chan struct{}, 1),
		toParse:     toParse,
		logger:      slog.Default(),
	}
}

// Wake is a coalescing trigger that notifies the dispatcher to wake up and
// start processing dirty directories.
func (d *ParseDispatcher) Wake() {
	select {
	case d.dirtyNotify <- struct{}{}:
	default:
		// dispatcher is already awake or a token is already queued.
		// we can safely drop this notification.
	}
}

func (d *ParseDispatcher) Run(ctx context.Context) error {
	// Startup sweep to process any dirty dirs leftover from previous runs.
	d.pump(ctx)

	// Wait for live triggers via Wake in an event loop.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.dirtyNotify:
			d.pump(ctx) // wake up and drain the DB until empty
		}
	}
}

// pump pulls from the DB and blocks if the parser queue is full.
// It returns when there are exactly 0 dirty directories left.
func (d *ParseDispatcher) pump(ctx context.Context) error {
	for {
		claimed, err := d.store.DirtyDirs(ctx, 50)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// TODO: maybe do something better here?
			d.logger.Error("fetch dirty directories failed", slog.Any("error", err))
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
			case d.toParse <- c:
				// if the parser queue is full, this gracefully blocks,
				// applying backpressure all the way up to this loop.
			}
		}
	}
}

func StartParserPool(
	ctx context.Context,
	workers int,
	store ParseStore,
	locations LocationResolver,
	toParse <-chan ClaimedDir,
	out chan<- ParseResult,
) *errgroup.Group {
	g, gctx := errgroup.WithContext(ctx)
	for i := range workers {
		g.Go(func() error {
			return runParserWorker(gctx, i, store, locations, toParse, out)
		})
	}
	return g
}

func runParserWorker(
	ctx context.Context,
	workerID int,
	store ParseStore,
	locations LocationResolver,
	in <-chan ClaimedDir,
	out chan<- ParseResult,
) error {
	logger := slog.With(slog.Int("worker", workerID))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case payload, ok := <-in:
			if !ok {
				return nil
			}

			loc, err := locations.Location(ctx, payload.LocationID)
			if err != nil {
				logger.Error("unknown location", slog.String("location_id", payload.LocationID.String()))
				if relErr := store.ReleaseAndRedirty(ctx, payload.ID, payload.LockedGeneration); relErr != nil {
					logger.Warn("release+dirty failed", slog.Any("error", relErr),
						slog.String("dir_id", payload.ID.String()))
				}
				continue
			}

			root, err := loc.Root()
			if err != nil {
				logger.Error("resolve location root failed", slog.Any("error", err),
					slog.String("location_id", loc.ID.String()))
				if relErr := store.ReleaseAndRedirty(ctx, payload.ID, payload.LockedGeneration); relErr != nil {
					logger.Warn("release+dirty failed", slog.Any("error", relErr),
						slog.String("dir_id", payload.ID.String()))
				}
				continue
			}

			files, err := store.DirFiles(ctx, payload.ID)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// A transient DB error here shouldn't kill the whole pool, but
				// it also can't be silently dropped, since we have a parse lock
				// on this dir (locked_generation is set).
				logger.Error("read dir files failed", slog.Any("error", err),
					slog.String("dir_id", payload.ID.String()))

				// Release the lock but re-request a retry by leaving dirty
				// set.
				//
				// TODO: max retries and escalate to log.Error. if failing
				// because of something persistent (e.g corrupt row, dir deleted
				// mid-parse but not swept), we will spin on it every pump().
				if relErr := store.ReleaseAndRedirty(ctx, payload.ID, payload.LockedGeneration); relErr != nil {
					logger.Warn("release+dirty failed", slog.Any("error", relErr),
						slog.String("dir_id", payload.ID.String()))
				}
				continue
			}

			dir := filepath.Join(root, payload.RelPath)
			res := parseDir(ctx, dir, payload.ID, files)

			if err := store.ReleaseDir(ctx, payload.ID, payload.LockedGeneration); err != nil {
				// Lock already gone (crash-recovery reap or duplicate
				// dispatch). Log but don't fail the parse itself, the
				// work already happened successfully.
				logger.Warn("release dir lock failed", slog.Any("error", err),
					slog.String("dir_id", payload.ID.String()))
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- res:
			}
		}
	}
}

func parseDir(_ context.Context, _ string, dirID uuid.UUID, _ []storage.File) ParseResult {
	res := ParseResult{
		DirID: dirID,
	}
	return res
}
