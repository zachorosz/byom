package scan

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/zachorosz/byom/storage"
)

// SyncPayload is the change set the scanner hands to the store for one
// synced dir.
type SyncPayload struct {
	LocationID uuid.UUID
	RelPath    string
	Generation int64
	Changed    []storage.File
	Missing    []uuid.UUID
	// When true, the change set will be queued for parsing.
	Dirty bool
}

func (s *Scanner) startSyncPool(
	ctx context.Context,
	workers int,
	locationID uuid.UUID,
	gen int64,
	in <-chan walkResult,
) *errgroup.Group {
	g, gctx := errgroup.WithContext(ctx)
	for range workers {
		g.Go(func() error {
			return s.runSyncWorker(gctx, locationID, gen, in)
		})
	}
	return g
}

func (s *Scanner) runSyncWorker(
	ctx context.Context,
	locationID uuid.UUID,
	gen int64,
	in <-chan walkResult,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-in:
			if !ok {
				return nil
			}

			known := map[string]storage.File{}

			dirID, err := s.Store.DirID(ctx, locationID, res.dir)
			if err != nil && !errors.Is(err, storage.ErrNotExists) {
				return err
			}
			if err == nil {
				known, err = s.Store.KnownFiles(ctx, dirID)
				if err != nil {
					return err
				}
			}

			payload := SyncPayload{
				LocationID: locationID,
				RelPath:    res.dir,
				Generation: gen,
			}
			payload.Changed, payload.Missing, payload.Dirty = computeChangeset(known, res)

			if _, err := s.Store.SyncDir(ctx, payload); err != nil {
				return err
			}
			if payload.Dirty && s.OnDirty != nil {
				s.OnDirty()
			}
		}
	}
}

func computeChangeset(
	known map[string]storage.File,
	res walkResult,
) (changed []storage.File, missing []uuid.UUID, dirty bool) {
	for _, f := range res.files {
		name := f.Name()
		old, ok := known[name]
		if ok {
			delete(known, name)
		}
		info, err := f.Info()
		switch {
		case err != nil && ok: // any known file that fails to stat is missing for now.
			missing = append(missing, old.ID)
		case err == nil:
			cur := storage.File{
				Name:    name,
				Kind:    f.kind,
				Size:    info.Size(),
				ModTime: info.ModTime(),
			}
			if ok {
				cur.ID = old.ID
			}
			if !old.Missing && old.Size == cur.Size && old.ModTime.Equal(cur.ModTime) {
				continue
			}
			changed = append(changed, cur)
		}
	}
	for _, leftover := range known {
		missing = append(missing, leftover.ID)
	}
	dirty = len(missing) > 0 || len(changed) > 0
	return
}
