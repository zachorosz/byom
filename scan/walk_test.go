package scan

import (
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// walkAndMerge runs walkPostOrder through a discMerger over fsys and
// returns the emitted file names keyed by emitted dir.
func walkAndMerge(t *testing.T, fsys fstest.MapFS) map[string][]string {
	t.Helper()
	emitted := map[string][]string{}
	merger := newDiscMerger(func(res walkResult) error {
		var names []string
		for _, f := range res.files {
			names = append(names, f.Name())
		}
		emitted[res.dir] = names
		return nil
	})
	if err := walkPostOrder(fsys, ".", merger.process); err != nil {
		t.Fatalf(`walkPostOrder() failed: %v`, err)
	}
	return emitted
}

// sortStrings makes emitted file-name order irrelevant to comparisons;
// merged disc files are appended in map iteration order.
var sortStrings = cmpopts.SortSlices(func(a, b string) bool { return a < b })

func TestWalkFlatAlbum(t *testing.T) {
	fsys := fstest.MapFS{
		"Artist/Album/01.flac":     {},
		"Artist/Album/02.Flac":     {},
		"Artist/Album/cover.jpg":   {},
		"Artist/Album/notes.txt":   {},
		"Artist/Album/.hidden.mp3": {},
		"Artist/.git/junk.flac":    {},
	}

	got := walkAndMerge(t, fsys)

	want := map[string][]string{
		".":            nil,
		"Artist":       nil,
		"Artist/Album": {"01.flac", "02.Flac", "cover.jpg"},
	}
	if diff := cmp.Diff(want, got, sortStrings); diff != "" {
		t.Errorf("walkPostOrder emitted unexpected dirs/files (-want +got):\n%s", diff)
	}
}

func TestWalkMergesDiscDirs(t *testing.T) {
	fsys := fstest.MapFS{
		"Album/CD1/01.flac":    {},
		"Album/Disc 2/01.flac": {},
		"Album/cover.jpg":      {},
	}

	got := walkAndMerge(t, fsys)

	want := map[string][]string{
		".":     nil,
		"Album": {"CD1/01.flac", "Disc 2/01.flac", "cover.jpg"},
	}
	if diff := cmp.Diff(want, got, sortStrings); diff != "" {
		t.Errorf("walkPostOrder emitted unexpected dirs/files (-want +got):\n%s", diff)
	}
}

func TestIsDisc(t *testing.T) {
	tests := []struct {
		dir  string
		want bool
	}{
		{dir: "CD1", want: true},
		{dir: "cd 2", want: true},
		{dir: "Disc_3 - Remixes", want: true},
		{dir: "Album/CD1", want: true},
		{dir: "Discography", want: false},
		{dir: "CD", want: false},
		{dir: "Vinyl 1", want: false},
		{dir: "Album", want: false},
	}
	for _, tt := range tests {
		if got := isDisc(tt.dir); got != tt.want {
			t.Errorf("isDisc(%q) = %v, want %v", tt.dir, got, tt.want)
		}
	}
}
