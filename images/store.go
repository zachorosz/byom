package images

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/library"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
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

	// Slow path: probe format + dimensions before touching disk.
	data := buf.Bytes()
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return library.Image{}, fmt.Errorf("images: decode config: %w", err)
	}
	mime, ok := mimeForFormat(format)
	if !ok {
		return library.Image{}, fmt.Errorf("images: unsupported format %q", format)
	}

	if err := s.writeBytes(sha, data); err != nil {
		return library.Image{}, err
	}

	// Upsert outside the lock to prevent stalling concurrent parse workers.
	// Racing callers converge on content_hash.
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

	// Publish under the lock; first writer wins so callers agree.
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.known[sha]; ok {
		return existing, nil
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

// mimeForFormat maps a registered decoder's format name to its MIME type.
func mimeForFormat(format string) (string, bool) {
	switch format {
	case "jpeg":
		return "image/jpeg", true
	case "png":
		return "image/png", true
	case "gif":
		return "image/gif", true
	case "webp":
		return "image/webp", true
	}
	return "", false
}
