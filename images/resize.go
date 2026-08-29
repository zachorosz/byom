package images

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"

	"golang.org/x/image/draw"
)

// jpegQuality trades derivative size against visible artifacts on album art.
const jpegQuality = 82

// fit returns w and h scaled to fit inside a max-by-max box with the
// aspect ratio preserved. Dimensions already inside the box are returned
// unchanged; fit never upscales.
func fit(w, h, max int) (int, int) {
	if w <= max && h <= max {
		return w, h
	}
	scale := float64(max) / float64(w)
	if h > w {
		scale = float64(max) / float64(h)
	}
	return atLeastOne(w, scale), atLeastOne(h, scale)
}

// atLeastOne scales n, clamping to a minimum of one pixel so extreme
// aspect ratios still produce a decodable image.
func atLeastOne(n int, scale float64) int {
	scaled := int(math.Round(float64(n) * scale))
	if scaled < 1 {
		return 1
	}
	return scaled
}

// resizeJPEG returns the encoded image in data scaled to fit inside a
// max-by-max box and re-encoded as JPEG. Sources smaller than the box
// keep their dimensions rather than being upscaled.
func resizeJPEG(data []byte, max int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("images: decode: %w", err)
	}

	bounds := src.Bounds()
	w, h := fit(bounds.Dx(), bounds.Dy(), max)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("images: encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}
