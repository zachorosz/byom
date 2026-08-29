package metadata

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/storage"
)

// ClaimedDir is a dir claimed from the parse queue. LockedGeneration is
// the generation captured at claim time and must be handed back when
// releasing the claim.
type ClaimedDir struct {
	ID               uuid.UUID
	RelPath          string
	LocationID       uuid.UUID
	LockedGeneration int64
}

type ParseResult struct {
	DirID  uuid.UUID
	Albums []ParsedAlbum
	Images []ParsedImage
	Errors []ParseError
}

// ParsedImage is an image added to the image store during parse,
// classified by its filename.
type ParsedImage struct {
	ImageID uuid.UUID
	Kind    library.ImageKind
}

// ParseError records a per-file parse failure.
type ParseError struct {
	FileID  uuid.UUID
	Message string
}

type ParsedAlbum struct {
	Metadata AlbumMetadata
	Artists  []Credit
	Tracks   []ParsedTrack
}

type ParsedTrack struct {
	FileID      uuid.UUID
	Metadata    TrackMetadata
	Credits     []Credit
	Audio       library.AudioProperties
	Duration    time.Duration
	StartOffset time.Duration
}

// ParseStore is the parse pipeline's persistence surface: claiming
// dirty dirs, reading a claimed dir's files, and releasing the claim
// when done or failed.
type ParseStore interface {
	DirtyDirs(ctx context.Context, limit int) ([]ClaimedDir, error)
	DirFiles(ctx context.Context, dirID uuid.UUID) ([]storage.File, error)
	ReleaseDir(ctx context.Context, dirID uuid.UUID, lockedGen int64) error
	ReleaseAndRedirty(ctx context.Context, dirID uuid.UUID, lockedGen int64) error
}

// LocationResolver resolves a location ID to its location.
type LocationResolver interface {
	Location(ctx context.Context, id uuid.UUID) (storage.Location, error)
}

type ImageStore interface {
	Add(context.Context, io.Reader) (library.Image, error)
}

// parseDir parses the files of a synced dir. dir must be the absolute
// path of the dir on disk; f.Name is joined onto it per file.
func parseDir(
	ctx context.Context,
	images ImageStore,
	dir string,
	dirID uuid.UUID,
	files []storage.File,
) ParseResult {
	res := ParseResult{DirID: dirID}

	albums := map[string]*ParsedAlbum{}

	for _, f := range files {
		fp := filepath.Join(dir, f.Name)
		switch f.Kind {
		case storage.FileAudio:
			audio, tags, err := readAudio(ctx, fp)
			if err != nil {
				res.Errors = append(res.Errors, ParseError{FileID: f.ID, Message: err.Error()})
				continue
			}
			tags = normTags(tags)

			album := mapAlbum(tags)
			albumArtists := mapAlbumArtists(tags)

			track := ParsedTrack{
				FileID:   f.ID,
				Metadata: mapTrack(tags),
				Credits:  mapCredits(tags),
				Audio:    audio,
				// TODO: cuesheets
				Duration:    audio.Duration,
				StartOffset: 0,
			}

			key := albumMetaKey(album, albumArtists)
			if al, ok := albums[key]; ok {
				al.Tracks = append(albums[key].Tracks, track)
			} else {
				albums[key] = &ParsedAlbum{
					Metadata: album,
					Artists:  albumArtists,
					Tracks:   []ParsedTrack{track},
				}
			}

		case storage.FileImage:
			kind := classifyImage(f.Name)
			if !eagerImageKind(kind) {
				continue
			}

			file, err := os.Open(fp)
			if err != nil {
				res.Errors = append(res.Errors, ParseError{FileID: f.ID, Message: err.Error()})
				continue
			}
			img, err := images.Add(ctx, file)
			if err != nil {
				file.Close()
				res.Errors = append(res.Errors, ParseError{FileID: f.ID, Message: err.Error()})
				continue
			}
			file.Close()
			res.Images = append(res.Images, ParsedImage{
				ImageID: img.ID,
				Kind:    kind,
			})
		default:
			res.Errors = append(res.Errors, ParseError{FileID: f.ID, Message: fmt.Sprintf("unknown file kind %q", f.Kind)})
		}
	}

	for _, al := range albums {
		res.Albums = append(res.Albums, *al)
	}

	return res
}

// eagerImageKind reports whether an image of this kind is fetched
// during the parse.
func eagerImageKind(kind library.ImageKind) bool {
	return kind == library.ImageCover || kind == library.ImageArtist
}

// classifyImage classifies an image by its base filename. Files from
// merged disc dirs carry a disc prefix (e.g. "CD1/cover.jpg"), so only
// the base name is considered.
func classifyImage(name string) library.ImageKind {
	base := strings.ToLower(path.Base(name))
	base = strings.TrimSuffix(base, path.Ext(base))
	switch base {
	case "cover", "front", "folder", "album", "albumart":
		return library.ImageCover
	case "back":
		return library.ImageBack
	case "disc", "cd", "media", "label":
		return library.ImageDisc
	case "artist":
		return library.ImageArtist
	}
	return library.ImageOther
}

func albumMetaKey(meta AlbumMetadata, artists []Credit) string {
	artistNames := make([]string, len(artists))
	for i, a := range artists {
		artistNames[i] = a.CreditedName
	}
	fields := []string{strings.Join(artistNames, "/"), meta.Title}
	return strings.Join(fields, string('\x00'))
}
