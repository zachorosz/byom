package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/metadata"
)

// seedLibrary imports one album per title under a fresh dir, each
// credited to artist and holding one track per title index.
func seedLibrary(t *testing.T, db *sql.DB, artist library.Artist, titles ...string) {
	t.Helper()
	dirID, fileIDs := seedDir(t, db, len(titles))

	albums := make([]metadata.ImportAlbum, 0, len(titles))
	for i, title := range titles {
		albums = append(albums, testAlbum(dirID, artist, title, "group-"+title, fileIDs[i:i+1]))
	}
	if err := NewLibraryStore(db).ReplaceDirAlbums(context.Background(), dirID, albums); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dirID, %d albums) failed: %v", len(albums), err)
	}
}

func TestLibraryReadStore_Artist(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	want := insertTestArtist(t, NewLibraryStore(db), "Artist A")

	s := NewLibraryReadStore(db)
	got, err := s.Artist(ctx, want.ID)
	if err != nil {
		t.Fatalf("Artist(ctx, %v) failed: %v", want.ID, err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Artist(ctx, %v) mismatch (-want +got):\n%s", want.ID, diff)
	}
}

func TestLibraryReadStore_Artist_NotFound(t *testing.T) {
	s := NewLibraryReadStore(newTestDB(t))

	id := uuid.Must(uuid.NewV7())
	_, err := s.Artist(context.Background(), id)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Artist(ctx, %v) error = %v, want library.ErrNotFound", id, err)
	}
}

func TestLibraryReadStore_Artists_Paginates(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	w := NewLibraryStore(db)
	// Inserted out of order: the listing must come back by sort name.
	c := insertTestArtist(t, w, "Artist C")
	a := insertTestArtist(t, w, "Artist A")
	b := insertTestArtist(t, w, "Artist B")

	s := NewLibraryReadStore(db)
	got, next, err := s.Artists(ctx, "", 2)
	if err != nil {
		t.Fatalf(`Artists(ctx, "", 2) failed: %v`, err)
	}
	if diff := cmp.Diff([]library.Artist{a, b}, got); diff != "" {
		t.Errorf(`Artists(ctx, "", 2) mismatch (-want +got):\n%s`, diff)
	}
	if next == "" {
		t.Fatal(`Artists(ctx, "", 2) next page token = "", want non-empty`)
	}

	got, next, err = s.Artists(ctx, next, 2)
	if err != nil {
		t.Fatalf("Artists(ctx, %q, 2) failed: %v", next, err)
	}
	if diff := cmp.Diff([]library.Artist{c}, got); diff != "" {
		t.Errorf("Artists(ctx, token, 2) mismatch (-want +got):\n%s", diff)
	}
	if next != "" {
		t.Errorf("Artists(ctx, token, 2) next page token = %q, want empty", next)
	}
}

func TestLibraryReadStore_Artists_InvalidToken(t *testing.T) {
	s := NewLibraryReadStore(newTestDB(t))

	_, _, err := s.Artists(context.Background(), "not-a-token!", 10)
	if !errors.Is(err, library.ErrInvalidPageToken) {
		t.Errorf(`Artists(ctx, "not-a-token!", 10) error = %v, want library.ErrInvalidPageToken`, err)
	}
}

func TestLibraryReadStore_Albums_FiltersByArtist(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	w := NewLibraryStore(db)
	a := insertTestArtist(t, w, "Artist A")
	b := insertTestArtist(t, w, "Artist B")
	seedLibrary(t, db, a, "Album One", "Album Two")
	seedLibrary(t, db, b, "Album Three")

	s := NewLibraryReadStore(db)
	got, next, err := s.Albums(ctx, a.ID, "", 10)
	if err != nil {
		t.Fatalf(`Albums(ctx, %v, "", 10) failed: %v`, a.ID, err)
	}
	if next != "" {
		t.Errorf(`Albums(ctx, artistID, "", 10) next page token = %q, want empty`, next)
	}

	wantTitles := []string{"Album One", "Album Two"}
	gotTitles := make([]string, len(got))
	for i, al := range got {
		gotTitles[i] = al.Title
	}
	if diff := cmp.Diff(wantTitles, gotTitles); diff != "" {
		t.Errorf(`Albums(ctx, artistID, "", 10) titles mismatch (-want +got):\n%s`, diff)
	}

	wantArtists := []library.AlbumArtist{{ArtistID: a.ID, CreditedName: a.Name}}
	if diff := cmp.Diff(wantArtists, got[0].Artists); diff != "" {
		t.Errorf(`Albums(ctx, artistID, "", 10) artists mismatch (-want +got):\n%s`, diff)
	}
}

func TestLibraryReadStore_Albums_Unfiltered(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	a := insertTestArtist(t, NewLibraryStore(db), "Artist A")
	seedLibrary(t, db, a, "Album One", "Album Two")

	s := NewLibraryReadStore(db)
	got, _, err := s.Albums(ctx, uuid.Nil, "", 10)
	if err != nil {
		t.Fatalf(`Albums(ctx, uuid.Nil, "", 10) failed: %v`, err)
	}
	if len(got) != 2 {
		t.Errorf(`Albums(ctx, uuid.Nil, "", 10) = %d albums, want 2`, len(got))
	}
}

func TestLibraryReadStore_Tracks_PaginatesInDiscOrder(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	w := NewLibraryStore(db)
	artist := insertTestArtist(t, w, "Artist A")

	// testAlbum numbers tracks 1..n on disc 1, so the listing order is
	// the seeded file order.
	dirID, fileIDs := seedDir(t, db, 3)
	al := testAlbum(dirID, artist, "Album One", "group-1", fileIDs)
	if err := w.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dirID, album) failed: %v", err)
	}
	albumID := al.Album.ID

	s := NewLibraryReadStore(db)
	got, next, err := s.Tracks(ctx, albumID, "", 2)
	if err != nil {
		t.Fatalf(`Tracks(ctx, %v, "", 2) failed: %v`, albumID, err)
	}
	if diff := cmp.Diff([]int{1, 2}, trackNumbers(got)); diff != "" {
		t.Errorf(`Tracks(ctx, albumID, "", 2) track numbers mismatch (-want +got):\n%s`, diff)
	}
	if next == "" {
		t.Fatal(`Tracks(ctx, albumID, "", 2) next page token = "", want non-empty`)
	}

	wantCredits := []library.TrackCredit{{ArtistID: artist.ID, CreditedName: artist.Name, Role: "Performer"}}
	if diff := cmp.Diff(wantCredits, got[0].Credits); diff != "" {
		t.Errorf(`Tracks(ctx, albumID, "", 2) credits mismatch (-want +got):\n%s`, diff)
	}

	got, next, err = s.Tracks(ctx, albumID, next, 2)
	if err != nil {
		t.Fatalf("Tracks(ctx, albumID, %q, 2) failed: %v", next, err)
	}
	if diff := cmp.Diff([]int{3}, trackNumbers(got)); diff != "" {
		t.Errorf("Tracks(ctx, albumID, token, 2) track numbers mismatch (-want +got):\n%s", diff)
	}
	if next != "" {
		t.Errorf("Tracks(ctx, albumID, token, 2) next page token = %q, want empty", next)
	}
}

func trackNumbers(tracks []library.Track) []int {
	nums := make([]int, len(tracks))
	for i, t := range tracks {
		nums[i] = t.TrackNumber
	}
	return nums
}

func TestLibraryReadStore_Track_NotFound(t *testing.T) {
	s := NewLibraryReadStore(newTestDB(t))

	id := uuid.Must(uuid.NewV7())
	_, err := s.Track(context.Background(), id)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Track(ctx, %v) error = %v, want library.ErrNotFound", id, err)
	}
}

func TestLibraryReadStore_Album_NotFound(t *testing.T) {
	s := NewLibraryReadStore(newTestDB(t))

	id := uuid.Must(uuid.NewV7())
	_, err := s.Album(context.Background(), id)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Album(ctx, %v) error = %v, want library.ErrNotFound", id, err)
	}
}
