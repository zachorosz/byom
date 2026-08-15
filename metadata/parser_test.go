package metadata

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/storage"
)

// recordingImageStore records every image handed to it, so tests can
// assert on which files the parser chose to read.
type recordingImageStore struct {
	mu    sync.Mutex
	bytes [][]byte
}

func (s *recordingImageStore) Add(_ context.Context, r io.Reader) (library.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return library.Image{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bytes = append(s.bytes, data)
	return library.Image{ID: uuid.Must(uuid.NewV7())}, nil
}

func (s *recordingImageStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bytes)
}

// writeImageDir writes a PNG per name into a new temp dir and returns
// the dir plus matching storage.File entries.
func writeImageDir(t *testing.T, names ...string) (string, []storage.File) {
	t.Helper()
	dir := t.TempDir()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("png.Encode failed: %v", err)
	}

	files := make([]storage.File, 0, len(names))
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
			t.Fatalf("write %s failed: %v", name, err)
		}
		files = append(files, storage.File{
			ID: uuid.Must(uuid.NewV7()), Name: name, Kind: storage.FileImage,
		})
	}
	return dir, files
}

// TestParseDir_ReadsOnlyEagerImageKinds pins the lazy-cache policy. An
// image's kind is knowable from its filename alone, so the parser can
// decide what to fetch without spending a single byte. Only the art a
// library view needs up front is read; everything else stays a files
// row to be materialized on demand.
func TestParseDir_ReadsOnlyEagerImageKinds(t *testing.T) {
	ctx := context.Background()
	dir, files := writeImageDir(t,
		"cover.jpg", "artist.png", // eager
		"back.png", "media.png", "booklet-01.png", "poster.png", // deferred
	)

	images := &recordingImageStore{}
	res := parseDir(ctx, images, dir, uuid.Must(uuid.NewV7()), files)

	if got, want := images.count(), 2; got != want {
		t.Errorf("images read from disk = %d, want %d (cover and artist only)", got, want)
	}

	var gotKinds []library.ImageKind
	for _, img := range res.Images {
		gotKinds = append(gotKinds, img.Kind)
	}
	slices.Sort(gotKinds)
	want := []library.ImageKind{library.ImageArtist, library.ImageCover}
	if !slices.Equal(gotKinds, want) {
		t.Errorf("parseDir() image kinds = %v, want %v", gotKinds, want)
	}
	if len(res.Errors) != 0 {
		t.Errorf("parseDir() errors = %v, want none (deferring is not a failure)", res.Errors)
	}
}
