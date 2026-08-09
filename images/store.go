package images

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	// Register decoders for the formats isSupportedImage accepts;
	// image.DecodeConfig only sees registered formats.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/library"
)

// Index records stored images in the database.
type Index interface {
	Upsert(context.Context, library.Image) (library.Image, error)
}

// Store is a content-addressed blob store for images of any kind.
type Store struct {
	index Index
	root  string

	mu    sync.RWMutex
	known map[string]library.Image // sha256 -> image
}

func NewStore(ctx context.Context, root string, index Index) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(root, "images"), 0o755); err != nil {
		return nil, fmt.Errorf("images: create root: %w", err)
	}
	s := &Store{
		index: index,
		root:  root,
		known: map[string]library.Image{},
	}
	return s, nil
}

func (s *Store) Add(ctx context.Context, r io.Reader) (library.Image, error) {
	var buf bytes.Buffer
	h := sha256.New()
	if _, err := io.Copy(&buf, io.TeeReader(r, h)); err != nil {
		return library.Image{}, err
	}
	sha := hex.EncodeToString(h.Sum(nil))

	// Fast path: SHA already known this run.
	s.mu.RLock()
	if img, ok := s.known[sha]; ok {
		s.mu.RUnlock()
		return img, nil
	}
	s.mu.RUnlock()

	// Slow path: probe format + dimensions before touching disk so we
	// reject non-images cheaply.
	data := buf.Bytes()
	mime := http.DetectContentType(data)
	if !isSupportedImage(mime) {
		return library.Image{}, fmt.Errorf("images: unsupported mime %q", mime)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return library.Image{}, fmt.Errorf("images: decode config: %w", err)
	}

	if err := s.writeBytes(sha, data); err != nil {
		return library.Image{}, err
	}

	// Under write lock: double-check the map then mint + insert + register.
	s.mu.Lock()
	defer s.mu.Unlock()
	if img, ok := s.known[sha]; ok {
		return img, nil
	}
	img, err := s.index.Upsert(ctx, library.Image{
		ID:          uuid.Must(uuid.NewV7()),
		ContentHash: sha,
		MimeType:    mime,
		Width:       cfg.Width,
		Height:      cfg.Height,
	})
	if err != nil {
		return library.Image{}, err
	}
	s.known[sha] = img
	return img, nil
}

// writeBytes stages the file under a .tmp name and atomically renames it
// into place.
func (s *Store) writeBytes(sha string, data []byte) error {
	dir := filepath.Join(s.root, "images", sha[:2])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	final := filepath.Join(dir, sha)
	if _, err := os.Stat(final); err == nil {
		return nil
	}
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func isSupportedImage(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}
