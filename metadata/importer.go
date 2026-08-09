package metadata

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
)

type LibraryStore interface {
	ArtistsByAlias(ctx context.Context, normName string) ([]library.Artist, error)
	InsertArtist(ctx context.Context, artist library.Artist, aliases ...string) error
	ReplaceDirAlbums(ctx context.Context, dirID uuid.UUID, albums []ImportAlbum) error
}

// ImportAlbum is a fully resolved album ready for persistence.
type ImportAlbum struct {
	Album  library.Album
	Tracks []library.Track
}

// Importer persists parse results into the library.
//
// Import is safe for concurrent use, but all imports must flow through
// a single Importer: its artist cache is what prevents concurrent
// workers from creating the same artist twice.
type Importer struct {
	Library LibraryStore

	mu      sync.Mutex
	artists map[string]uuid.UUID // normalized name → artist ID
}

// Import persists res, replacing whatever was previously imported for
// the dir. Returning an error leaves the dir dirty for a retry.
func (im *Importer) Import(ctx context.Context, res ParseResult) error {
	// A dir's images belong to every album found in it; the common
	// case is one album per dir.
	images := albumImages(res.Images)

	albums := make([]ImportAlbum, 0, len(res.Albums))
	for _, pa := range res.Albums {
		al, err := im.buildAlbum(ctx, res.DirID, pa)
		if err != nil {
			return err
		}
		al.Album.Images = images
		albums = append(albums, al)
	}
	if err := im.Library.ReplaceDirAlbums(ctx, res.DirID, albums); err != nil {
		return fmt.Errorf("replace dir albums: %w", err)
	}
	slog.Debug("album imported", slog.String("dir_id", res.DirID.String()), slog.Any("albums", albums))
	return nil
}

func (im *Importer) buildAlbum(ctx context.Context, dirID uuid.UUID, pa ParsedAlbum) (ImportAlbum, error) {
	albumID, err := uuid.NewV7()
	if err != nil {
		return ImportAlbum{}, err
	}

	album := library.Album{
		ID:                  albumID,
		DirID:               dirID,
		Type:                pa.Metadata.Type,
		Title:               pa.Metadata.Title,
		ReleaseDate:         pa.Metadata.ReleaseDate,
		OriginalReleaseDate: pa.Metadata.OriginalReleaseDate,
		Bootleg:             pa.Metadata.Bootleg,
		Compilation:         pa.Metadata.Compilation,
		Live:                pa.Metadata.Live,
	}

	seenArtists := map[uuid.UUID]bool{}
	for _, c := range pa.Artists {
		artistID, err := im.internArtist(ctx, c.CreditedName)
		if err != nil {
			return ImportAlbum{}, err
		}
		if artistID == uuid.Nil || seenArtists[artistID] {
			continue
		}
		seenArtists[artistID] = true
		album.Artists = append(album.Artists, library.AlbumArtist{
			ArtistID:     artistID,
			CreditedName: c.CreditedName,
		})
	}

	album.GroupKey, err = library.AlbumGroupKey(album)
	if err != nil {
		return ImportAlbum{}, err
	}

	tracks := make([]library.Track, 0, len(pa.Tracks))
	for _, pt := range pa.Tracks {
		trackID, err := uuid.NewV7()
		if err != nil {
			return ImportAlbum{}, err
		}
		track := library.Track{
			ID:                  trackID,
			AlbumID:             albumID,
			DiscNumber:          pt.Metadata.DiscNumber,
			DiscSubtitle:        pt.Metadata.DiscSubtitle,
			TrackNumber:         pt.Metadata.TrackNumber,
			Title:               pt.Metadata.Title,
			ReleaseDate:         pt.Metadata.ReleaseDate,
			OriginalReleaseDate: pt.Metadata.OriginalReleaseDate,
			FileID:              pt.FileID,
			Audio:               pt.Audio,
			Duration:            pt.Duration,
			StartOffset:         pt.StartOffset,
		}

		type creditKey struct {
			artistID uuid.UUID
			role     string
		}
		seen := map[creditKey]bool{}
		for _, c := range pt.Credits {
			artistID, err := im.internArtist(ctx, c.CreditedName)
			if err != nil {
				return ImportAlbum{}, err
			}
			key := creditKey{artistID: artistID, role: c.Role}
			if artistID == uuid.Nil || seen[key] {
				continue
			}
			seen[key] = true
			track.Credits = append(track.Credits, library.TrackCredit{
				ArtistID:     artistID,
				CreditedName: c.CreditedName,
				Role:         c.Role,
			})
		}
		tracks = append(tracks, track)
	}

	return ImportAlbum{Album: album, Tracks: tracks}, nil
}

func albumImages(imgs []ParsedImage) []library.AlbumImage {
	seen := map[uuid.UUID]bool{}

	links := make([]library.AlbumImage, 0, len(imgs))
	haveCover := false
	for _, pi := range imgs {
		if seen[pi.ImageID] {
			continue
		}
		seen[pi.ImageID] = true

		link := library.AlbumImage{ImageID: pi.ImageID, Kind: pi.Kind}
		if !haveCover && pi.Kind == library.ImageCover {
			link.SetCover = true
			haveCover = true
		}
		links = append(links, link)
	}
	return links
}

// internArtist resolves a credited name to an artist ID. Names that normalize
// to empty return uuid.Nil; ambiguous aliases resolve to the oldest artist.
func (im *Importer) internArtist(ctx context.Context, name string) (uuid.UUID, error) {
	norm := library.NormalizeArtistName(name)
	if norm == "" {
		return uuid.Nil, nil
	}

	im.mu.Lock()
	defer im.mu.Unlock()

	if id, ok := im.artists[norm]; ok {
		return id, nil
	}

	matches, err := im.Library.ArtistsByAlias(ctx, norm)
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup artist %q: %w", norm, err)
	}

	var id uuid.UUID
	if len(matches) > 0 {
		id = matches[0].ID // oldest artist for now
	} else {
		id, err = uuid.NewV7()
		if err != nil {
			return uuid.Nil, err
		}
		name = strings.TrimSpace(name)
		artist := library.Artist{ID: id, Name: name, SortName: name}
		if err := im.Library.InsertArtist(ctx, artist, norm); err != nil {
			return uuid.Nil, fmt.Errorf("insert artist %q: %w", norm, err)
		}
	}

	if im.artists == nil {
		im.artists = map[string]uuid.UUID{}
	}
	im.artists[norm] = id
	return id, nil
}
