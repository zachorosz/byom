package metadata

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// metadataKeys are the tag keys the fixtures carry, minus the encoder key
// whose name and value vary by reader and encoder build.
var metadataKeys = []string{"ALBUM", "ALBUMARTIST", "ARTIST", "DATE", "TITLE", "TRACKNUMBER"}

func TestReadAudio(t *testing.T) {
	want := map[string][]string{
		"ALBUM":       {"Flying Microtonal Banana"},
		"ALBUMARTIST": {"King Gizzard & the Lizard Wizard"},
		"ARTIST":      {"King Gizzard & the Lizard Wizard"},
		"DATE":        {"2017"},
		"TITLE":       {"Rattlesnake"},
		"TRACKNUMBER": {"1/9"},
	}

	for _, name := range []string{"track.mp3", "track.m4a", "track.flac", "track.opus"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)

			_, tags, err := readAudio(context.Background(), path)
			if err != nil {
				t.Fatalf("readAudio(%s) returned unexpected error: %v", path, err)
			}

			norm := normTags(tags)
			got := map[string][]string{}
			for _, k := range metadataKeys {
				if v, ok := norm[k]; ok {
					got[k] = v
				}
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("readAudio(%s) tags mismatch (-want +got):\n%s", path, diff)
			}
		})
	}
}

func TestReadAudioMapsAlbum(t *testing.T) {
	want := AlbumMetadata{
		Title:       "Flying Microtonal Banana",
		ReleaseDate: "2017",
	}
	wantArtists := []Credit{{CreditedName: "King Gizzard & the Lizard Wizard"}}

	for _, name := range []string{"track.mp3", "track.m4a", "track.flac", "track.opus"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)

			_, tags, err := readAudio(context.Background(), path)
			if err != nil {
				t.Fatalf("readAudio(%s) returned unexpected error: %v", path, err)
			}
			norm := normTags(tags)

			if diff := cmp.Diff(want, mapAlbum(norm)); diff != "" {
				t.Errorf("mapAlbum(readAudio(%s)) mismatch (-want +got):\n%s", path, diff)
			}
			if diff := cmp.Diff(wantArtists, mapAlbumArtists(norm)); diff != "" {
				t.Errorf("mapAlbumArtists(readAudio(%s)) mismatch (-want +got):\n%s", path, diff)
			}
		})
	}
}
