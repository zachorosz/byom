package images

import (
	"bytes"
	"context"
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zachorosz/byom/library"
)

// newTestHandler returns a handler backed by a store holding one PNG,
// along with that image's content hash.
func newTestHandler(t *testing.T, width, height int) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	s, err := NewStore(ctx, t.TempDir(), &fakeIndex{byHash: map[string]library.Image{}})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	img, err := s.Add(ctx, bytes.NewReader(pngBytes(t, width, height)))
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	return NewHandler(s), img.ContentHash
}

func TestHandler_ServesOriginal(t *testing.T) {
	h, sha := newTestHandler(t, 40, 20)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/images/"+sha, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /images/{hash} status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "image/png"; got != want {
		t.Errorf("GET /images/{hash} Content-Type = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Cache-Control"), "public, max-age=31536000, immutable"; got != want {
		t.Errorf("GET /images/{hash} Cache-Control = %q, want %q", got, want)
	}
}

func TestHandler_ServesResizedDerivative(t *testing.T) {
	h, sha := newTestHandler(t, 800, 800)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/images/"+sha+"?size=320", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET ?size=320 status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "image/jpeg"; got != want {
		t.Errorf("GET ?size=320 Content-Type = %q, want %q", got, want)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("DecodeConfig of response body failed: %v", err)
	}
	if cfg.Width != 320 || cfg.Height != 320 {
		t.Errorf("GET ?size=320 dimensions = %dx%d, want 320x320", cfg.Width, cfg.Height)
	}
}

func TestHandler_NotModified(t *testing.T) {
	h, sha := newTestHandler(t, 40, 20)

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/images/"+sha, nil))
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("GET /images/{hash} ETag = \"\", want a non-empty validator")
	}

	req := httptest.NewRequest(http.MethodGet, "/images/"+sha, nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	h.ServeHTTP(second, req)

	if second.Code != http.StatusNotModified {
		t.Errorf("GET with If-None-Match status = %d, want %d", second.Code, http.StatusNotModified)
	}
}

func TestHandler_Rejects(t *testing.T) {
	h, sha := newTestHandler(t, 40, 20)
	unknown := strings.Repeat("a", 64)

	tests := []struct {
		name   string
		target string
		want   int
	}{
		{name: "unsupported size", target: "/images/" + sha + "?size=100", want: http.StatusBadRequest},
		{name: "non numeric size", target: "/images/" + sha + "?size=huge", want: http.StatusBadRequest},
		{name: "malformed hash", target: "/images/" + strings.Repeat("z", 64), want: http.StatusBadRequest},
		{name: "short hash", target: "/images/abc", want: http.StatusBadRequest},
		{name: "unknown blob", target: "/images/" + unknown, want: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
			if rec.Code != tc.want {
				t.Errorf("GET %s status = %d, want %d", tc.target, rec.Code, tc.want)
			}
		})
	}
}
