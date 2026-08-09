package metadata

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
)

type replaceCall struct {
	dirID  uuid.UUID
	albums []ImportAlbum
}

type fakeLibraryStore struct {
	artists  map[string][]library.Artist // norm name → matches
	inserted []library.Artist
	lookups  int
	replaced []replaceCall
}

func (f *fakeLibraryStore) ArtistsByAlias(_ context.Context, normName string) ([]library.Artist, error) {
	f.lookups++
	return f.artists[normName], nil
}

func (f *fakeLibraryStore) InsertArtist(_ context.Context, artist library.Artist, aliases ...string) error {
	f.inserted = append(f.inserted, artist)
	if f.artists == nil {
		f.artists = map[string][]library.Artist{}
	}
	for _, alias := range aliases {
		f.artists[alias] = append(f.artists[alias], artist)
	}
	return nil
}

func (f *fakeLibraryStore) ReplaceDirAlbums(_ context.Context, dirID uuid.UUID, albums []ImportAlbum) error {
	f.replaced = append(f.replaced, replaceCall{dirID: dirID, albums: albums})
	return nil
}

func TestImporter_Import(t *testing.T) {
	ctx := context.Background()
	store := &fakeLibraryStore{}
	im := &Importer{Library: store}

	dirID := uuid.Must(uuid.NewV7())
	fileID := uuid.Must(uuid.NewV7())
	res := ParseResult{
		DirID: dirID,
		Albums: []ParsedAlbum{{
			Metadata: AlbumMetadata{Title: "Album"},
			// "artist a" normalizes to the same artist as "Artist A".
			Artists: []Credit{{CreditedName: "Artist A"}, {CreditedName: "artist a"}},
			Tracks: []ParsedTrack{{
				FileID:   fileID,
				Metadata: TrackMetadata{DiscNumber: 1, TrackNumber: 1, Title: "Track"},
				Credits: []Credit{
					{CreditedName: "Artist A", Role: "Performer"},
					{CreditedName: "artist a", Role: "Performer"},
					{CreditedName: "Artist A", Role: "Producer"},
					{CreditedName: "  ", Role: "Engineer"},
				},
			}},
		}},
	}

	if err := im.Import(ctx, res); err != nil {
		t.Fatalf("Import(ctx, res) failed: %v", err)
	}

	if got, want := len(store.inserted), 1; got != want {
		t.Fatalf("Import(ctx, res) inserted %d artists, want %d", got, want)
	}
	artistID := store.inserted[0].ID

	if got, want := len(store.replaced), 1; got != want {
		t.Fatalf("Import(ctx, res) made %d ReplaceDirAlbums calls, want %d", got, want)
	}
	call := store.replaced[0]
	if call.dirID != dirID {
		t.Errorf("ReplaceDirAlbums dirID = %v, want %v", call.dirID, dirID)
	}
	if got, want := len(call.albums), 1; got != want {
		t.Fatalf("ReplaceDirAlbums got %d albums, want %d", got, want)
	}
	album := call.albums[0]

	wantArtists := []library.AlbumArtist{{ArtistID: artistID, CreditedName: "Artist A"}}
	if diff := cmp.Diff(wantArtists, album.Album.Artists); diff != "" {
		t.Errorf("album artists mismatch (-want +got):\n%s", diff)
	}

	if got, want := len(album.Tracks), 1; got != want {
		t.Fatalf("album has %d tracks, want %d", got, want)
	}
	track := album.Tracks[0]
	if track.AlbumID != album.Album.ID {
		t.Errorf("track AlbumID = %v, want album ID %v", track.AlbumID, album.Album.ID)
	}

	wantCredits := []library.TrackCredit{
		{ArtistID: artistID, CreditedName: "Artist A", Role: "Performer"},
		{ArtistID: artistID, CreditedName: "Artist A", Role: "Producer"},
	}
	if diff := cmp.Diff(wantCredits, track.Credits); diff != "" {
		t.Errorf("track credits mismatch (-want +got):\n%s", diff)
	}
}

func TestImporter_Import_ArtistCache(t *testing.T) {
	ctx := context.Background()
	existing := library.Artist{ID: uuid.Must(uuid.NewV7()), Name: "Artist A", SortName: "Artist A"}
	store := &fakeLibraryStore{
		artists: map[string][]library.Artist{
			library.NormalizeArtistName("Artist A"): {existing},
		},
	}
	im := &Importer{Library: store}

	res := ParseResult{
		DirID: uuid.Must(uuid.NewV7()),
		Albums: []ParsedAlbum{{
			Metadata: AlbumMetadata{Title: "Album"},
			Artists:  []Credit{{CreditedName: "Artist A"}},
		}},
	}

	for range 2 {
		if err := im.Import(ctx, res); err != nil {
			t.Fatalf("Import() failed: %v", err)
		}
	}

	if got, want := len(store.inserted), 0; got != want {
		t.Errorf("imports of a known artist inserted %d artists, want %d", got, want)
	}
	if got, want := store.lookups, 1; got != want {
		t.Errorf("imports of a known artist did %d lookups, want %d (cache)", got, want)
	}

	album := store.replaced[len(store.replaced)-1].albums[0]
	wantArtists := []library.AlbumArtist{{ArtistID: existing.ID, CreditedName: "Artist A"}}
	if diff := cmp.Diff(wantArtists, album.Album.Artists); diff != "" {
		t.Errorf("album artists mismatch (-want +got):\n%s", diff)
	}
}
