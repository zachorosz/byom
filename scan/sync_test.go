package scan

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/storage"
)

// dirWalkResult reads one dir of fsys and builds the walkResult the
// walker would produce for it.
func dirWalkResult(t *testing.T, fsys fs.FS, dir string) walkResult {
	t.Helper()
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		t.Fatalf("fs.ReadDir(%q) failed: %v", dir, err)
	}
	res := walkResult{dir: dir}
	for _, e := range entries {
		kind, ok := classifyKind(e)
		if !ok {
			continue
		}
		res.files = append(res.files, walkEntry{DirEntry: e, kind: kind})
	}
	return res
}

func TestComputeChangeset(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"a/keep.flac": {Data: []byte("xx"), ModTime: now},
		"a/grew.flac": {Data: []byte("xxxx"), ModTime: now},
		"a/new.flac":  {Data: []byte("x"), ModTime: now},
		"a/back.flac": {Data: []byte("xx"), ModTime: now},
	}
	res := dirWalkResult(t, fsys, "a")

	keepID := uuid.Must(uuid.NewV7())
	grewID := uuid.Must(uuid.NewV7())
	goneID := uuid.Must(uuid.NewV7())
	backID := uuid.Must(uuid.NewV7())
	known := map[string]storage.File{
		"keep.flac": {ID: keepID, Size: 2, ModTime: now},
		"grew.flac": {ID: grewID, Size: 2, ModTime: now},
		"gone.flac": {ID: goneID, Size: 2, ModTime: now},
		"back.flac": {ID: backID, Size: 2, ModTime: now, Missing: true},
	}

	changed, missing, dirty := computeChangeset(known, res)

	// keep.flac is unchanged and must not appear; back.flac was marked
	// missing and must resurrect even though its stats are unchanged.
	wantChanged := []storage.File{
		{ID: backID, Name: "back.flac", Kind: storage.FileAudio, Size: 2, ModTime: now},
		{ID: grewID, Name: "grew.flac", Kind: storage.FileAudio, Size: 4, ModTime: now},
		{Name: "new.flac", Kind: storage.FileAudio, Size: 1, ModTime: now},
	}
	if diff := cmp.Diff(wantChanged, changed); diff != "" {
		t.Errorf("computeChangeset() changed mismatch (-want +got):\n%s", diff)
	}
	if wantMissing := []uuid.UUID{goneID}; !cmp.Equal(wantMissing, missing) {
		t.Errorf("computeChangeset() missing = %v, want %v", missing, wantMissing)
	}
	if !dirty {
		t.Errorf("computeChangeset() dirty = false, want true")
	}
}

func TestSyncDirForce(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"a/keep.flac": {Data: []byte("xx"), ModTime: now},
	}
	dirID := uuid.Must(uuid.NewV7())

	tests := []struct {
		name  string
		force bool
		want  bool
	}{
		{name: "UnchangedDirStaysClean", force: false, want: false},
		{name: "ForcedUnchangedDirIsQueued", force: true, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got SyncPayload
			store := newFakeScanStore()
			store.dirID = func(context.Context, uuid.UUID, string) (uuid.UUID, error) {
				return dirID, nil
			}
			store.knownFiles = func(context.Context, uuid.UUID) (map[string]storage.File, error) {
				return map[string]storage.File{
					"keep.flac": {ID: uuid.Must(uuid.NewV7()), Size: 2, ModTime: now},
				}, nil
			}
			store.syncDir = func(_ context.Context, p SyncPayload) (uuid.UUID, error) {
				got = p
				return dirID, nil
			}

			dirtied := 0
			s := &Scanner{Store: store, OnDirty: func() { dirtied++ }}
			r := &run{locationID: uuid.Must(uuid.NewV7()), gen: 1, force: tc.force}

			if err := s.syncDir(t.Context(), r, dirWalkResult(t, fsys, "a")); err != nil {
				t.Fatalf("syncDir() returned unexpected error: %v", err)
			}

			if got.Dirty != tc.want {
				t.Errorf("syncDir(force=%v) Dirty = %v, want %v", tc.force, got.Dirty, tc.want)
			}
			if len(got.Changed) != 0 {
				t.Errorf("syncDir(force=%v) Changed = %v, want empty", tc.force, got.Changed)
			}
			if len(got.Missing) != 0 {
				t.Errorf("syncDir(force=%v) Missing = %v, want empty", tc.force, got.Missing)
			}

			wantDirtied := 0
			if tc.want {
				wantDirtied = 1
			}
			if dirtied != wantDirtied {
				t.Errorf("syncDir(force=%v) called OnDirty %d times, want %d", tc.force, dirtied, wantDirtied)
			}
		})
	}
}

func TestComputeChangesetNoChanges(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		"a/keep.flac": {Data: []byte("xx"), ModTime: now},
	}
	res := dirWalkResult(t, fsys, "a")

	known := map[string]storage.File{
		"keep.flac": {ID: uuid.Must(uuid.NewV7()), Size: 2, ModTime: now},
	}

	changed, missing, dirty := computeChangeset(known, res)

	if len(changed) != 0 {
		t.Errorf("computeChangeset() changed = %v, want empty", changed)
	}
	if len(missing) != 0 {
		t.Errorf("computeChangeset() missing = %v, want empty", missing)
	}
	if dirty {
		t.Errorf("computeChangeset() dirty = true, want false")
	}
}
