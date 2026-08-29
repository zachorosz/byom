package images

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/library"
	"golang.org/x/sync/singleflight"

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

	// derive collapses concurrent misses for the same derivative
	derive singleflight.Group

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

	if err := writeFile(s.blobPath(sha), data); err != nil {
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

// writeFile stages data under a unique temp name in the destination
// directory and atomically renames it into place. An existing file is
// left untouched.
func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
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

// SupportedSizes are the derivative widths Open accepts. Bounding the set
// bounds the on-disk cache to one derivative per size per image.
var SupportedSizes = []int{160, 320, 640}

var (
	// ErrInvalidHash reports a content hash that is not 64 lowercase hex
	// characters.
	ErrInvalidHash = errors.New("images: invalid content hash")
	// ErrUnsupportedSize reports a size outside SupportedSizes.
	ErrUnsupportedSize = errors.New("images: unsupported size")
)

// Open returns the stored image's bytes at the requested size together
// with its MIME type. A size of 0 yields the stored original; any other
// size must appear in SupportedSizes, and its derivative is generated on
// the first request and read from the on-disk cache thereafter.
//
// Open returns ErrInvalidHash for a malformed hash, ErrUnsupportedSize for
// an unknown size, and an error wrapping fs.ErrNotExist when the blob is
// not stored. The caller closes the returned file.
func (s *Store) Open(sha string, size int) (*os.File, string, error) {
	if !validHash(sha) {
		return nil, "", ErrInvalidHash
	}
	if size != 0 && !supportedSize(size) {
		return nil, "", ErrUnsupportedSize
	}
	if size == 0 {
		return s.openOriginal(sha)
	}
	f, err := s.openDerivative(sha, size)
	if err != nil {
		return nil, "", err
	}
	return f, "image/jpeg", nil
}

// openOriginal opens the stored blob and reports its MIME type.
func (s *Store) openOriginal(sha string) (*os.File, string, error) {
	f, err := os.Open(s.blobPath(sha))
	if err != nil {
		return nil, "", err
	}
	_, format, err := image.DecodeConfig(f)
	if err != nil {
		f.Close()
		return nil, "", fmt.Errorf("images: decode config: %w", err)
	}
	mime, ok := mimeForFormat(format)
	if !ok {
		f.Close()
		return nil, "", fmt.Errorf("images: unsupported format %q", format)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, "", err
	}
	return f, mime, nil
}

// openDerivative opens the cached derivative, generating it from the
// original on a miss.
func (s *Store) openDerivative(sha string, size int) (*os.File, error) {
	path := s.derivativePath(sha, size)
	f, err := os.Open(path)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	key := fmt.Sprintf("%s@%d", sha, size)
	if _, err, _ := s.derive.Do(key, func() (any, error) {
		return nil, s.makeDerivative(sha, size, path)
	}); err != nil {
		return nil, err
	}
	return os.Open(path)
}

// makeDerivative reads the original and writes its resized JPEG to path.
func (s *Store) makeDerivative(sha string, size int, path string) error {
	data, err := os.ReadFile(s.blobPath(sha))
	if err != nil {
		return err
	}
	out, err := resizeJPEG(data, size)
	if err != nil {
		return err
	}
	return writeFile(path, out)
}

// blobPath locates the stored original for a content hash.
func (s *Store) blobPath(sha string) string {
	return filepath.Join(s.root, "images", sha[:2], sha)
}

// derivativePath locates the cached resize beside its original.
func (s *Store) derivativePath(sha string, size int) string {
	return filepath.Join(s.root, "images", sha[:2], fmt.Sprintf("%s@%d.jpg", sha, size))
}

// supportedSize reports whether n is a width Open will serve.
func supportedSize(n int) bool {
	return slices.Contains(SupportedSizes, n)
}

// validHash reports whether s is a well-formed content hash. Callers
// must check this before building a path from sha.
func validHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	// Blobs are written under lowercase hex; accepting uppercase would alias
	// to the same file on a case-insensitive filesystem.
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}
