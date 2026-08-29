package scan

import (
	"context"
	"errors"

	"github.com/google/uuid"

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

func (s *Scanner) syncWorker(ctx context.Context, r *run, in <-chan walkResult) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res, ok := <-in:
			if !ok {
				return nil
			}
			if err := s.syncDir(ctx, r, res); err != nil {
				return err
			}
		}
	}
}

// syncDir writes one walked dir's change set to the store and counts it.
func (s *Scanner) syncDir(ctx context.Context, r *run, res walkResult) error {
	known := map[string]storage.File{}

	dirID, err := s.Store.DirID(ctx, r.locationID, res.dir)
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
		LocationID: r.locationID,
		RelPath:    res.dir,
		Generation: r.gen,
	}
	payload.Changed, payload.Missing, payload.Dirty = computeChangeset(known, res)
	payload.Dirty = payload.Dirty || r.force

	if _, err := s.Store.SyncDir(ctx, payload); err != nil {
		return err
	}

	r.dirsSeen.Add(1)
	r.filesSeen.Add(int64(len(res.files)))
	r.filesMissing.Add(int64(len(payload.Missing)))

	if payload.Dirty && s.OnDirty != nil {
		s.OnDirty()
	}
	return nil
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
	return changed, missing, len(missing) > 0 || len(changed) > 0
}
