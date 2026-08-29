package images

import (
	"bytes"
	"image"
	"testing"
)

func TestFit(t *testing.T) {
	tests := []struct {
		name      string
		w, h, max int
		wantW     int
		wantH     int
	}{
		{name: "square larger than box", w: 4000, h: 4000, max: 320, wantW: 320, wantH: 320},
		{name: "landscape larger than box", w: 1000, h: 500, max: 320, wantW: 320, wantH: 160},
		{name: "portrait larger than box", w: 500, h: 1000, max: 320, wantW: 160, wantH: 320},
		{name: "already inside box is untouched", w: 200, h: 100, max: 320, wantW: 200, wantH: 100},
		{name: "exactly the box is untouched", w: 320, h: 320, max: 320, wantW: 320, wantH: 320},
		{name: "extreme ratio keeps at least one pixel", w: 10000, h: 3, max: 320, wantW: 320, wantH: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := fit(tc.w, tc.h, tc.max)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Errorf("fit(%d, %d, %d) = %d, %d, want %d, %d",
					tc.w, tc.h, tc.max, gotW, gotH, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestResizeJPEG_ScalesDownAndReencodes(t *testing.T) {
	out, err := resizeJPEG(pngBytes(t, 800, 400), 320)
	if err != nil {
		t.Fatalf("resizeJPEG() returned unexpected error: %v", err)
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("DecodeConfig of resized output failed: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("resizeJPEG() format = %q, want %q", format, "jpeg")
	}
	if cfg.Width != 320 || cfg.Height != 160 {
		t.Errorf("resizeJPEG() dimensions = %dx%d, want 320x160", cfg.Width, cfg.Height)
	}
}

func TestResizeJPEG_DoesNotUpscale(t *testing.T) {
	out, err := resizeJPEG(pngBytes(t, 100, 100), 640)
	if err != nil {
		t.Fatalf("resizeJPEG() returned unexpected error: %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("DecodeConfig of resized output failed: %v", err)
	}
	if cfg.Width != 100 || cfg.Height != 100 {
		t.Errorf("resizeJPEG() dimensions = %dx%d, want 100x100", cfg.Width, cfg.Height)
	}
}

func TestResizeJPEG_RejectsNonImage(t *testing.T) {
	if out, err := resizeJPEG([]byte("not an image"), 320); err == nil {
		t.Errorf("resizeJPEG() = %d bytes, want error", len(out))
	}
}
