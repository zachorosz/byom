// Package scan synchronizes the on-disk state of a location with the
// database: it walks the directory tree, computes per-directory change
// sets, and marks changed dirs dirty for the metadata parse pipeline.
package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"runtime"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/zachorosz/byom/storage"
)

// Store is the persistence surface the scanner needs.
type Store interface {
	BeginScan(ctx context.Context, locationID uuid.UUID) (gen int64, scanID uuid.UUID, err error)
	FinishScan(ctx context.Context, scanID uuid.UUID, scanErr error) error
	DirID(ctx context.Context, locationID uuid.UUID, relpath string) (uuid.UUID, error)
	KnownFiles(ctx context.Context, dirID uuid.UUID) (map[string]storage.File, error)
	SyncDir(ctx context.Context, payload SyncPayload) (uuid.UUID, error)
	Sweep(ctx context.Context, locationID uuid.UUID, gen int64) (int64, error)
}

type Scanner struct {
	Store Store
	// Workers is the number of concurrent sync workers.
	// Defaults to runtime.NumCPU() when <= 0.
	Workers int
	// OnDirty, if set, is called after each synced dir whose change set
	// needs (re)parsing.
	OnDirty func()
}

// Scan walks fsys post-order, merging disc folders into their parent
// album dir, and syncs each dir's files against the store under a new
// scan generation. Dirs and files not seen this generation are swept
// (marked missing) afterwards.
func (s *Scanner) Scan(ctx context.Context, fsys fs.FS, loc storage.Location) error {
	gen, scanID, err := s.Store.BeginScan(ctx, loc.ID)
	if err != nil {
		return fmt.Errorf("begin scan: %w", err)
	}

	workers := s.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	dirs := make(chan walkResult, 32)
	syncPool := s.startSyncPool(ctx, workers, loc.ID, gen, dirs)

	g, gctx := errgroup.WithContext(ctx)
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
		return walkPostOrder(fsys, ".", merger.process)
	})

	walkErr := g.Wait()
	syncErr := syncPool.Wait()
	scanErr := errors.Join(walkErr, syncErr)

	// sweep dirs not found during this generation
	if _, err := s.Store.Sweep(ctx, loc.ID, gen); err != nil {
		slog.Warn("sweep failed", slog.Any("error", err))
	}

	if err := s.Store.FinishScan(ctx, scanID, scanErr); err != nil {
		slog.Warn("finish scan failed", slog.Any("error", err))
	}

	return scanErr
}
