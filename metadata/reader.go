package metadata

import (
	"context"
	"fmt"

	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/metadata/audiometa"
	"github.com/zachorosz/byom/metadata/taglib"
)

func readAudio(ctx context.Context, path string) (library.AudioProperties, map[string][]string, error) {
	audio, tags, err := audiometa.ReadAudio(ctx, path)
	if err == nil && len(tags) > 0 {
		return audio, tags, nil
	}

	audio, tags, taglibErr := taglib.ReadAudio(path)
	if taglibErr != nil {
		if err != nil {
			return library.AudioProperties{}, nil, fmt.Errorf("audiometa failed (%v), taglib fallback failed: %w", err, taglibErr)
		}
		return library.AudioProperties{}, nil, fmt.Errorf("audiometa returned no tags, taglib fallback failed: %w", taglibErr)
	}
	return audio, tags, nil
}
