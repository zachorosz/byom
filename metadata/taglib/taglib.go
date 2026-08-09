package taglib

import (
	"go.senan.xyz/taglib"

	"github.com/zachorosz/byom/library"
)

// ReadAudio reads audio properties and raw tags for the audio file at
// path. path must be a physical path on disk; taglib cannot read through
// an fs.FS.
func ReadAudio(path string) (library.AudioProperties, map[string][]string, error) {
	props, err := taglib.ReadProperties(path)
	if err != nil {
		return library.AudioProperties{}, nil, err
	}

	tags, err := taglib.ReadTags(path)
	if err != nil {
		return library.AudioProperties{}, nil, err
	}

	audio := library.AudioProperties{
		Codec:      props.Format,
		SampleRate: int(props.SampleRate),
		BitDepth:   int(props.BitDepth),
		Channels:   int(props.Channels),
		Bitrate:    int(props.BitRate),
		Duration:   props.Length,
	}
	return audio, tags, nil
}
