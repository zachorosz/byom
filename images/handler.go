package images

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
)

// immutableCache is safe because a URL's bytes are fixed by its content
// hash: different bytes always mean a different URL.
const immutableCache = "public, max-age=31536000, immutable"

// NewHandler returns an http.Handler serving image blobs from s at
// GET /images/{hash}. An optional size query parameter selects a
// derivative width from SupportedSizes; without it the stored original is
// served. Responses may be cached indefinitely.
func NewHandler(s *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /images/{hash}", func(w http.ResponseWriter, r *http.Request) {
		size, err := parseSize(r.URL.Query().Get("size"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		hash := r.PathValue("hash")
		f, mime, err := s.Open(hash, size)
		switch {
		case errors.Is(err, ErrInvalidHash), errors.Is(err, ErrUnsupportedSize):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		case errors.Is(err, fs.ErrNotExist):
			http.NotFound(w, r)
			return
		case err != nil:
			http.Error(w, "read image", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			http.Error(w, "read image", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", immutableCache)
		w.Header().Set("ETag", strconv.Quote(fmt.Sprintf("%s@%d", hash, size)))
		// An empty name keeps ServeContent from re-sniffing the type it
		// would otherwise infer from a file extension.
		http.ServeContent(w, r, "", info.ModTime(), f)
	})
	return mux
}

// parseSize resolves the size query parameter, mapping an absent value to
// 0 for the stored original.
func parseSize(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || !supportedSize(n) {
		return 0, fmt.Errorf("size must be one of %v", SupportedSizes)
	}
	return n, nil
}
