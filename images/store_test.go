package images

import (
	"bytes"
	"context"
	"image"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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

func jpegBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatalf("jpeg.Encode failed: %v", err)
	}
	return buf.Bytes()
}

func gifBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewPaletted(image.Rect(0, 0, width, height), palette.Plan9), nil); err != nil {
		t.Fatalf("gif.Encode failed: %v", err)
	}
	return buf.Bytes()
}

func webpBytes(t *testing.T) []byte {
	t.Helper()
	// 3x2 red WebP image since Go standard library lacks a native WebP encoder
	return []byte{
		0x52, 0x49, 0x46, 0x46, 0x3c, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50,
		0x56, 0x50, 0x38, 0x20, 0x30, 0x00, 0x00, 0x00, 0xd0, 0x01, 0x00, 0x9d,
		0x01, 0x2a, 0x03, 0x00, 0x02, 0x00, 0x02, 0x00, 0x34, 0x25, 0xa0, 0x02,
		0x74, 0xba, 0x01, 0xf8, 0x00, 0x03, 0xb0, 0x00, 0xfe, 0xf0, 0xc4, 0x0b,
		0xff, 0x20, 0xb9, 0x61, 0x75, 0xc8, 0xd7, 0xff, 0x20, 0x3f, 0xe4, 0x07,
		0xfc, 0x80, 0xff, 0xf8, 0xf2, 0x00, 0x00, 0x00,
	}
}

func TestStore_Add_PersistsBlobAndDedupes(t *testing.T) {
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

func TestStore_Add_SupportsEveryWalkedFormat(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		encode   func(*testing.T) []byte
		wantMime string
	}{
		{name: "PNG", encode: func(t *testing.T) []byte { return pngBytes(t, 3, 2) }, wantMime: "image/png"},
		{name: "JPEG", encode: func(t *testing.T) []byte { return jpegBytes(t, 3, 2) }, wantMime: "image/jpeg"},
		{name: "GIF", encode: func(t *testing.T) []byte { return gifBytes(t, 3, 2) }, wantMime: "image/gif"},
		{name: "WEBP", encode: func(t *testing.T) []byte { return webpBytes(t) }, wantMime: "image/webp"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()

			s, err := NewStore(ctx, root, &fakeIndex{byHash: map[string]library.Image{}})
			if err != nil {
				t.Fatalf("NewStore(%q) returned unexpected error: %v", root, err)
			}

			img, err := s.Add(ctx, bytes.NewReader(tc.encode(t)))
			if err != nil {
				t.Fatalf("Add() returned unexpected error: %v", err)
			}
			if img.MimeType != tc.wantMime {
				t.Errorf("Add() mime = %q, want %q", img.MimeType, tc.wantMime)
			}
			if img.Width != 3 || img.Height != 2 {
				t.Errorf("Add() dimensions = %dx%d, want 3x2", img.Width, img.Height)
			}
		})
	}
}

type blockingIndex struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (idx *blockingIndex) Upsert(_ context.Context, img library.Image) (library.Image, error) {
	// Only the first caller blocks. sync.Once would be wrong here: it
	// makes every caller wait for the first, which would look like a
	// lock problem even when there is none.
	if idx.calls.Add(1) == 1 {
		close(idx.entered)
		<-idx.release
	}
	return img, nil
}

func TestStore_Add_DoesNotHoldLockDuringUpsert(t *testing.T) {
	ctx := context.Background()
	idx := &blockingIndex{entered: make(chan struct{}), release: make(chan struct{})}
	s, err := NewStore(ctx, t.TempDir(), idx)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	stuck := make(chan struct{})
	go func() {
		defer close(stuck)
		if _, err := s.Add(ctx, bytes.NewReader(pngBytes(t, 8, 8))); err != nil {
			t.Errorf("blocked Add failed: %v", err)
		}
	}()

	select {
	case <-idx.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first Add never reached the index")
	}

	// A different image must be able to make progress meanwhile.
	done := make(chan error, 1)
	go func() {
		_, err := s.Add(ctx, bytes.NewReader(pngBytes(t, 4, 4)))
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("concurrent Add failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("concurrent Add blocked behind an in-flight index write, want the lock released across it")
	}

	close(idx.release)
	<-stuck
}
