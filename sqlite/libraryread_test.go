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
	"github.com/zachorosz/byom/page"
)

// seedVersionGroup imports each title as its own dir under a shared
// group key, making them alternate versions of one release. The first
// title becomes the group's primary version.
func seedVersionGroup(t *testing.T, db *sql.DB, artist library.Artist, groupKey string, titles ...string) []library.Album {
	t.Helper()
	s := NewLibraryStore(db)

	albums := make([]library.Album, 0, len(titles))
	for _, title := range titles {
		dirID, fileIDs := seedDir(t, db, 1)
		al := testAlbum(dirID, artist, title, groupKey, fileIDs)
		if err := s.ReplaceDirAlbums(context.Background(), dirID, []metadata.ImportAlbum{al}); err != nil {
			t.Fatalf("ReplaceDirAlbums(ctx, dirID, %q) failed: %v", title, err)
		}
		albums = append(albums, al.Album)
	}
	return albums
}

// albumSpec describes one album to seed. A zero artist credits the
// album to the artist passed to seedAlbums.
type albumSpec struct {
	title        string
	typ          library.AlbumType
	bootleg      bool
	artist       library.Artist
	releaseDate  string
	originalDate string
}

// seedAlbums imports each spec as its own album under one dir, each in
// its own version group.
func seedAlbums(t *testing.T, db *sql.DB, artist library.Artist, specs ...albumSpec) {
	t.Helper()
	dirID, fileIDs := seedDir(t, db, len(specs))

	albums := make([]metadata.ImportAlbum, 0, len(specs))
	for i, spec := range specs {
		credited := spec.artist
		if credited.ID == uuid.Nil {
			credited = artist
		}
		al := testAlbum(dirID, credited, spec.title, "group-"+spec.title, fileIDs[i:i+1])
		al.Album.Type = spec.typ
		al.Album.Bootleg = spec.bootleg
		al.Album.ReleaseDate = spec.releaseDate
		al.Album.OriginalReleaseDate = spec.originalDate
		albums = append(albums, al)
	}
	if err := NewLibraryStore(db).ReplaceDirAlbums(context.Background(), dirID, albums); err != nil {
		t.Fatalf("ReplaceDirAlbums(%d albums) failed: %v", len(albums), err)
	}
}

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

func TestLibraryStore_Artist(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	want := insertTestArtist(t, NewLibraryStore(db), "Artist A")

	s := NewLibraryStore(db)
	got, err := s.Artist(ctx, want.ID)
	if err != nil {
		t.Fatalf("Artist(ctx, %v) failed: %v", want.ID, err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Artist(ctx, %v) mismatch (-want +got):\n%s", want.ID, diff)
	}
}

func TestLibraryStore_Artist_NotFound(t *testing.T) {
	s := NewLibraryStore(newTestDB(t))

	id := uuid.Must(uuid.NewV7())
	_, err := s.Artist(context.Background(), id)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Artist(ctx, %v) error = %v, want library.ErrNotFound", id, err)
	}
}

func TestLibraryStore_Artists_Paginates(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	w := NewLibraryStore(db)
	// Inserted out of order: the listing must come back by sort name.
	c := insertTestArtist(t, w, "Artist C")
	a := insertTestArtist(t, w, "Artist A")
	b := insertTestArtist(t, w, "Artist B")

	s := NewLibraryStore(db)
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

func TestLibraryStore_Artists_InvalidToken(t *testing.T) {
	s := NewLibraryStore(newTestDB(t))

	_, _, err := s.Artists(context.Background(), "not-a-token!", 10)
	if !errors.Is(err, page.ErrInvalidToken) {
		t.Errorf(`Artists(ctx, "not-a-token!", 10) error = %v, want page.ErrInvalidToken`, err)
	}
}

func TestLibraryStore_Albums_FiltersByArtist(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	w := NewLibraryStore(db)
	a := insertTestArtist(t, w, "Artist A")
	b := insertTestArtist(t, w, "Artist B")
	seedLibrary(t, db, a, "Album One", "Album Two")
	seedLibrary(t, db, b, "Album Three")

	s := NewLibraryStore(db)
	got, next, err := s.Albums(ctx, library.AlbumQuery{ArtistID: a.ID, IncludeAllVersions: true}, "", 10)
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

func TestLibraryStore_Albums_Unfiltered(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	a := insertTestArtist(t, NewLibraryStore(db), "Artist A")
	seedLibrary(t, db, a, "Album One", "Album Two")

	s := NewLibraryStore(db)
	filter := library.AlbumQuery{IncludeAllVersions: true}
	got, _, err := s.Albums(ctx, filter, "", 10)
	if err != nil {
		t.Fatalf("Albums(%+v) failed: %v", filter, err)
	}
	if len(got) != 2 {
		t.Errorf("Albums(%+v) = %d albums, want 2", filter, len(got))
	}
}

func TestLibraryStore_Albums_PrimaryVersionsByDefault(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	artist := insertTestArtist(t, NewLibraryStore(db), "Artist A")
	versions := seedVersionGroup(t, db, artist, "group-1", "Album", "Album (Remaster)")
	seedLibrary(t, db, artist, "Other Album")

	s := NewLibraryStore(db)
	filter := library.AlbumQuery{}
	got, _, err := s.Albums(ctx, filter, "", 10)
	if err != nil {
		t.Fatalf("Albums(%+v) failed: %v", filter, err)
	}

	// One entry for the version group plus the ungrouped album, which
	// is its own group's primary.
	want := []string{"Album", "Other Album"}
	if diff := cmp.Diff(want, albumTitles(got)); diff != "" {
		t.Errorf("Albums(%+v) titles mismatch (-want +got):\n%s", filter, diff)
	}

	allVersions := library.AlbumQuery{IncludeAllVersions: true}
	all, _, err := s.Albums(ctx, allVersions, "", 10)
	if err != nil {
		t.Fatalf("Albums(%+v) failed: %v", allVersions, err)
	}
	if want := len(versions) + 1; len(all) != want {
		t.Errorf("Albums(%+v) = %d albums, want %d", allVersions, len(all), want)
	}
}

func TestLibraryStore_Albums_FiltersByType(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	artist := insertTestArtist(t, NewLibraryStore(db), "King Gizzard & the Lizard Wizard")
	seedAlbums(t, db, artist,
		albumSpec{title: "Nonagon Infinity", typ: library.AlbumMain},
		albumSpec{title: "Polygondwanaland", typ: library.AlbumMain},
		albumSpec{title: "Rattlesnake", typ: library.AlbumSingle},
		albumSpec{title: "Willoughby's Beach", typ: library.AlbumEP},
		albumSpec{title: "Untagged Rehearsal", typ: library.AlbumTypeUnknown},
	)

	tests := []struct {
		name  string
		types []library.AlbumType
		want  []string
	}{
		{
			name:  "noTypes",
			types: nil,
			want: []string{
				"Nonagon Infinity", "Polygondwanaland", "Rattlesnake",
				"Untagged Rehearsal", "Willoughby's Beach",
			},
		},
		{
			name:  "singleType",
			types: []library.AlbumType{library.AlbumMain},
			want:  []string{"Nonagon Infinity", "Polygondwanaland"},
		},
		{
			name:  "severalTypes",
			types: []library.AlbumType{library.AlbumEP, library.AlbumSingle},
			want:  []string{"Rattlesnake", "Willoughby's Beach"},
		},
		{
			name:  "untaggedOnly",
			types: []library.AlbumType{library.AlbumTypeUnknown},
			want:  []string{"Untagged Rehearsal"},
		},
	}

	s := NewLibraryStore(db)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := library.AlbumQuery{Types: tt.types}
			got, _, err := s.Albums(ctx, filter, "", 10)
			if err != nil {
				t.Fatalf("Albums(%+v) failed: %v", filter, err)
			}
			if diff := cmp.Diff(tt.want, albumTitles(got)); diff != "" {
				t.Errorf("Albums(%+v) titles mismatch (-want +got):\n%s", filter, diff)
			}
		})
	}
}

func TestLibraryStore_Albums_FiltersBootlegs(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	artist := insertTestArtist(t, NewLibraryStore(db), "King Gizzard & the Lizard Wizard")
	seedAlbums(t, db, artist,
		albumSpec{title: "Nonagon Infinity", typ: library.AlbumMain},
		albumSpec{title: "Live at Levitation '14", typ: library.AlbumOther, bootleg: true},
	)

	tests := []struct {
		name     string
		bootlegs library.BootlegFilter
		want     []string
	}{
		{
			name:     "unset",
			bootlegs: library.BootlegsInclude,
			want:     []string{"Live at Levitation '14", "Nonagon Infinity"},
		},
		{
			name:     "exclude",
			bootlegs: library.BootlegsExclude,
			want:     []string{"Nonagon Infinity"},
		},
		{
			name:     "only",
			bootlegs: library.BootlegsOnly,
			want:     []string{"Live at Levitation '14"},
		},
	}

	s := NewLibraryStore(db)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := library.AlbumQuery{Bootlegs: tt.bootlegs}
			got, _, err := s.Albums(ctx, filter, "", 10)
			if err != nil {
				t.Fatalf("Albums(%+v) failed: %v", filter, err)
			}
			if diff := cmp.Diff(tt.want, albumTitles(got)); diff != "" {
				t.Errorf("Albums(%+v) titles mismatch (-want +got):\n%s", filter, diff)
			}
		})
	}
}

// seedOrderingLibrary imports a fixed set of albums across two artists
// with contrasting dates, and returns the albums in no useful order.
func seedOrderingLibrary(t *testing.T, db *sql.DB) {
	t.Helper()
	s := NewLibraryStore(db)
	gizzard := insertSortedArtist(t, s, "King Gizzard & the Lizard Wizard", "King Gizzard")
	murlocs := insertSortedArtist(t, s, "The Murlocs", "Murlocs, The")

	seedAlbums(t, db, gizzard,
		albumSpec{
			title: "Nonagon Infinity", artist: gizzard,
			releaseDate: "2016-04-29", originalDate: "2016-04-29",
		},
		albumSpec{
			title: "Polygondwanaland", artist: gizzard,
			releaseDate: "2017-11-17", originalDate: "2017-11-17",
		},
		albumSpec{
			title: "Bittersweet Demons", artist: murlocs,
			releaseDate: "2021-06-25", originalDate: "2021-06-25",
		},
		// No dates at all: an unknown date is not an ancient one.
		albumSpec{title: "Anonymous Rehearsal", artist: murlocs},
	)
}

func TestLibraryStore_Albums_Ordering(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedOrderingLibrary(t, db)

	tests := []struct {
		name       string
		order      library.AlbumOrder
		descending bool
		want       []string
	}{
		{
			name:  "titleByDefault",
			order: library.AlbumOrderTitle,
			want: []string{
				"Anonymous Rehearsal", "Bittersweet Demons",
				"Nonagon Infinity", "Polygondwanaland",
			},
		},
		{
			// King Gizzard before Murlocs, each artist's albums in
			// original release order.
			name:  "artistThenOriginalDate",
			order: library.AlbumOrderArtist,
			want: []string{
				"Nonagon Infinity", "Polygondwanaland",
				"Bittersweet Demons", "Anonymous Rehearsal",
			},
		},
		{
			name:  "releaseDateUnknownLast",
			order: library.AlbumOrderReleaseDate,
			want: []string{
				"Nonagon Infinity", "Polygondwanaland",
				"Bittersweet Demons", "Anonymous Rehearsal",
			},
		},
		{
			name:       "descendingFlipsTheWholeOrder",
			order:      library.AlbumOrderTitle,
			descending: true,
			want: []string{
				"Polygondwanaland", "Nonagon Infinity",
				"Bittersweet Demons", "Anonymous Rehearsal",
			},
		},
	}

	s := NewLibraryStore(db)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := library.AlbumQuery{Order: tt.order, Descending: tt.descending}
			got, _, err := s.Albums(ctx, query, "", 10)
			if err != nil {
				t.Fatalf("Albums(%+v) failed: %v", query, err)
			}
			if diff := cmp.Diff(tt.want, albumTitles(got)); diff != "" {
				t.Errorf("Albums(%+v) titles mismatch (-want +got):\n%s", query, diff)
			}
		})
	}
}

func TestLibraryStore_Albums_OrdersRecentlyAddedByID(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	artist := insertTestArtist(t, NewLibraryStore(db), "Artist A")
	// IDs are UUIDv7, so import order is creation order.
	seedLibrary(t, db, artist, "First Import", "Second Import", "Third Import")

	s := NewLibraryStore(db)
	query := library.AlbumQuery{Order: library.AlbumOrderRecentlyAdded, Descending: true}
	got, _, err := s.Albums(ctx, query, "", 10)
	if err != nil {
		t.Fatalf("Albums(%+v) failed: %v", query, err)
	}

	want := []string{"Third Import", "Second Import", "First Import"}
	if diff := cmp.Diff(want, albumTitles(got)); diff != "" {
		t.Errorf("Albums(%+v) titles mismatch (-want +got):\n%s", query, diff)
	}
}

func TestLibraryStore_Albums_PaginatesWithinAnOrder(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedOrderingLibrary(t, db)

	// Paging must neither skip nor repeat a row across the boundary,
	// which is what the trailing id key in every ordering guarantees.
	s := NewLibraryStore(db)
	query := library.AlbumQuery{Order: library.AlbumOrderArtist}

	var titles []string
	token := ""
	for range 4 {
		page, next, err := s.Albums(ctx, query, token, 2)
		if err != nil {
			t.Fatalf("Albums(%+v, %q) failed: %v", query, token, err)
		}
		titles = append(titles, albumTitles(page)...)
		if next == "" {
			break
		}
		token = next
	}

	want := []string{
		"Nonagon Infinity", "Polygondwanaland",
		"Bittersweet Demons", "Anonymous Rehearsal",
	}
	if diff := cmp.Diff(want, titles); diff != "" {
		t.Errorf("Albums(%+v) paged titles mismatch (-want +got):\n%s", query, diff)
	}
}

func TestLibraryStore_Albums_RejectsOrderChangeMidListing(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedOrderingLibrary(t, db)

	s := NewLibraryStore(db)
	_, next, err := s.Albums(ctx, library.AlbumQuery{}, "", 1)
	if err != nil {
		t.Fatalf("Albums(library.AlbumQuery{}) failed: %v", err)
	}
	if next == "" {
		t.Fatal(`Albums(library.AlbumQuery{}) next page token = "", want non-empty`)
	}

	tests := []struct {
		name    string
		changed library.AlbumQuery
	}{
		{name: "order", changed: library.AlbumQuery{Order: library.AlbumOrderArtist}},
		{name: "direction", changed: library.AlbumQuery{Descending: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := s.Albums(ctx, tt.changed, next, 1)
			if !errors.Is(err, page.ErrInvalidToken) {
				t.Errorf("Albums(%+v, %q) error = %v, want page.ErrInvalidToken", tt.changed, next, err)
			}
		})
	}
}

func TestLibraryStore_Albums_RejectsFilterChangeMidListing(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	artist := insertTestArtist(t, NewLibraryStore(db), "Artist A")
	seedVersionGroup(t, db, artist, "group-1", "Album", "Album (Remaster)")
	seedLibrary(t, db, artist, "Other Album")

	s := NewLibraryStore(db)
	_, next, err := s.Albums(ctx, library.AlbumQuery{}, "", 1)
	if err != nil {
		t.Fatalf("Albums(library.AlbumQuery{}) failed: %v", err)
	}
	if next == "" {
		t.Fatal(`Albums(library.AlbumQuery{}) next page token = "", want non-empty`)
	}

	// Changing the filter mid-listing would splice two different result
	// sets together, so the stale token must be refused.
	tests := []struct {
		name    string
		changed library.AlbumQuery
	}{
		{name: "versions", changed: library.AlbumQuery{IncludeAllVersions: true}},
		{name: "types", changed: library.AlbumQuery{Types: []library.AlbumType{library.AlbumMain}}},
		{name: "bootlegs", changed: library.AlbumQuery{Bootlegs: library.BootlegsExclude}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := s.Albums(ctx, tt.changed, next, 1)
			if !errors.Is(err, page.ErrInvalidToken) {
				t.Errorf("Albums(%+v, %q) error = %v, want page.ErrInvalidToken", tt.changed, next, err)
			}
		})
	}
}

func TestLibraryStore_AlbumVersions(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	artist := insertTestArtist(t, NewLibraryStore(db), "Artist A")
	versions := seedVersionGroup(t, db, artist, "group-1", "Album", "Album (Remaster)")
	seedLibrary(t, db, artist, "Other Album")

	// Asking from the non-primary version must still return the whole
	// group, primary first.
	s := NewLibraryStore(db)
	remasterID := versions[1].ID
	got, err := s.AlbumVersions(ctx, remasterID)
	if err != nil {
		t.Fatalf("AlbumVersions(%v) failed: %v", remasterID, err)
	}

	want := []string{"Album", "Album (Remaster)"}
	if diff := cmp.Diff(want, albumTitles(got)); diff != "" {
		t.Errorf("AlbumVersions(%v) titles mismatch (-want +got):\n%s", remasterID, diff)
	}

	wantArtists := []library.AlbumArtist{{ArtistID: artist.ID, CreditedName: artist.Name}}
	if diff := cmp.Diff(wantArtists, got[0].Artists); diff != "" {
		t.Errorf("AlbumVersions(%v) artists mismatch (-want +got):\n%s", remasterID, diff)
	}
}

func TestLibraryStore_AlbumVersions_NotFound(t *testing.T) {
	s := NewLibraryStore(newTestDB(t))

	id := uuid.Must(uuid.NewV7())
	_, err := s.AlbumVersions(context.Background(), id)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("AlbumVersions(%v) error = %v, want library.ErrNotFound", id, err)
	}
}

func albumTitles(albums []library.Album) []string {
	titles := make([]string, len(albums))
	for i, al := range albums {
		titles[i] = al.Title
	}
	return titles
}

func TestLibraryStore_Tracks_PaginatesInDiscOrder(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	w := NewLibraryStore(db)
	artist := insertTestArtist(t, w, "Artist A")

	// testAlbum numbers tracks 1..n on disc 1, so the listing order is
	// the seeded file order.
	dirID, fileIDs := seedDir(t, db, 3)
	al := testAlbum(dirID, artist, "Album One", "group-1", fileIDs)
	if err := w.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums() failed: %v", err)
	}
	albumID := al.Album.ID

	s := NewLibraryStore(db)
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

func TestLibraryStore_Track_NotFound(t *testing.T) {
	s := NewLibraryStore(newTestDB(t))

	id := uuid.Must(uuid.NewV7())
	_, err := s.Track(context.Background(), id)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Track() error = %v, want library.ErrNotFound", err)
	}
}

func TestLibraryStore_Album_NotFound(t *testing.T) {
	s := NewLibraryStore(newTestDB(t))

	id := uuid.Must(uuid.NewV7())
	_, err := s.Album(context.Background(), id)
	if !errors.Is(err, library.ErrNotFound) {
		t.Errorf("Album() error = %v, want library.ErrNotFound", err)
	}
}
