package images

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachorosz/byom/library"
)

// fakeIndex mimics the store's content-hash dedupe: an image whose
// hash is already known keeps the existing row.
type fakeIndex struct {
	byHash map[string]library.Image
}

func (idx *fakeIndex) Upsert(_ context.Context, img library.Image) (library.Image, error) {
	if existing, ok := idx.byHash[img.ContentHash]; ok {
		return existing, nil
	}
	idx.byHash[img.ContentHash] = img
	return img, nil
}

func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("png.Encode failed: %v", err)
	}
	return buf.Bytes()
}

func TestStore_Add(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	s, err := NewStore(ctx, root, &fakeIndex{byHash: map[string]library.Image{}})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	data := pngBytes(t, 3, 2)
	img, err := s.Add(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if img.MimeType != "image/png" {
		t.Errorf("Add() mime = %q, want %q", img.MimeType, "image/png")
	}
	if img.Width != 3 || img.Height != 2 {
		t.Errorf("Add() dimensions = %dx%d, want 3x2", img.Width, img.Height)
	}
	blob := filepath.Join(root, "images", img.ContentHash[:2], img.ContentHash)
	if _, err := os.Stat(blob); err != nil {
		t.Errorf("Add() blob missing at %s: %v", blob, err)
	}

	again, err := s.Add(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Add() second call failed: %v", err)
	}
	if again.ID != img.ID {
		t.Errorf("Add() second call ID = %v, want first call's ID %v", again.ID, img.ID)
	}
}

func TestStore_Add_AfterRestart(t *testing.T) {
	// A restarted process has an empty in-memory index while the blob
	// and DB row survive on disk; Add must return the existing image
	// instead of failing on content-hash uniqueness.
	ctx := context.Background()
	root := t.TempDir()
	index := &fakeIndex{byHash: map[string]library.Image{}}

	s1, err := NewStore(ctx, root, index)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	data := pngBytes(t, 4, 4)
	img, err := s1.Add(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	s2, err := NewStore(ctx, root, index)
	if err != nil {
		t.Fatalf("NewStore after restart failed: %v", err)
	}
	again, err := s2.Add(ctx, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Add after restart failed: %v", err)
	}
	if again.ID != img.ID {
		t.Errorf("Add after restart ID = %v, want original ID %v", again.ID, img.ID)
	}
}

func TestStore_Add_NonImage(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, t.TempDir(), &fakeIndex{byHash: map[string]library.Image{}})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	if img, err := s.Add(ctx, bytes.NewReader([]byte("not an image"))); err == nil {
		t.Errorf("Add() = %+v, want error", img)
	}
}
