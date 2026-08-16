package audiometa

import (
	"context"
	"fmt"

	"github.com/simonhull/audiometa"
	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/metadata/taglib"
)

// ReadAudio extracts metadata and tags using the pure Go audiometa library.
func ReadAudio(ctx context.Context, path string) (library.AudioProperties, map[string][]string, error) {
	file, err := audiometa.OpenContext(ctx, path, audiometa.WithIgnoreWarnings())
	if err != nil {
		// Fallback to the WASM-based taglib for unsupported formats or corrupted files
		audio, tags, taglibErr := taglib.ReadAudio(path)
		if taglibErr != nil {
			return library.AudioProperties{}, nil, fmt.Errorf("audiometa failed (%v), and taglib fallback failed: %w", err, taglibErr)
		}
		return audio, tags, nil
	}
	defer file.Close()

	audio := library.AudioProperties{
		Codec:      file.Audio.Codec,
		SampleRate: file.Audio.SampleRate,
		BitDepth:   file.Audio.BitDepth,
		Channels:   file.Audio.Channels,
		Bitrate:    file.Audio.Bitrate,
		Duration:   file.Audio.Duration,
	}

	tags := make(map[string][]string)
	for k, v := range file.Tags.All() {
		tags[k] = v
	}

	return audio, tags, nil
}
