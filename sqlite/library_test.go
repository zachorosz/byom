package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/metadata"
)

// seedDir inserts a location, one dir, and n files so library rows
// have valid foreign keys to point at.
func seedDir(t *testing.T, db *sql.DB, n int) (dirID uuid.UUID, fileIDs []uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	locID := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(ctx,
		`INSERT INTO locations (id, uri) VALUES (?, ?)`, locID, "file:///music/"+locID.String()); err != nil {
		t.Fatalf("seed location failed: %v", err)
	}

	dirID = uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(ctx,
		`INSERT INTO dirs (id, location_id, relpath, seen_generation) VALUES (?, ?, ?, 1)`,
		dirID, locID, "Album"); err != nil {
		t.Fatalf("seed dir failed: %v", err)
	}

	for i := range n {
		fileID := uuid.Must(uuid.NewV7())
		if _, err := db.ExecContext(ctx,
			`INSERT INTO files (id, dir_id, name, kind, size_bytes, mod_time)
			 VALUES (?, ?, ?, 'audio', 1, 1)`,
			fileID, dirID, string(rune('a'+i))+".flac"); err != nil {
			t.Fatalf("seed file %d failed: %v", i, err)
		}
		fileIDs = append(fileIDs, fileID)
	}
	return dirID, fileIDs
}

func insertTestArtist(t *testing.T, s *LibraryStore, name string) library.Artist {
	t.Helper()
	artist := library.Artist{ID: uuid.Must(uuid.NewV7()), Name: name, SortName: name}
	if err := s.InsertArtist(context.Background(), artist, library.NormalizeArtistName(name)); err != nil {
		t.Fatalf("InsertArtist(ctx, %+v) failed: %v", artist, err)
	}
	return artist
}

func TestLibraryStore_ArtistsByAlias(t *testing.T) {
	ctx := context.Background()
	s := NewLibraryStore(newTestDB(t))

	artist := insertTestArtist(t, s, "Artist A")

	norm := library.NormalizeArtistName("Artist A")
	got, err := s.ArtistsByAlias(ctx, norm)
	if err != nil {
		t.Fatalf("ArtistsByAlias(ctx, %q) failed: %v", norm, err)
	}
	want := []library.Artist{artist}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ArtistsByAlias(ctx, %q) mismatch (-want +got):\n%s", norm, diff)
	}

	got, err = s.ArtistsByAlias(ctx, "unknown")
	if err != nil {
		t.Fatalf(`ArtistsByAlias(ctx, "unknown") failed: %v`, err)
	}
	if len(got) != 0 {
		t.Errorf(`ArtistsByAlias(ctx, "unknown") = %v, want empty`, got)
	}
}

// testAlbum builds an ImportAlbum for dirID with one track per file.
func testAlbum(dirID uuid.UUID, artist library.Artist, title, groupKey string, fileIDs []uuid.UUID) metadata.ImportAlbum {
	albumID := uuid.Must(uuid.NewV7())
	al := metadata.ImportAlbum{
		Album: library.Album{
			ID:       albumID,
			DirID:    dirID,
			Type:     library.AlbumMain,
			Title:    title,
			GroupKey: groupKey,
			Artists: []library.AlbumArtist{
				{ArtistID: artist.ID, CreditedName: artist.Name},
			},
		},
	}
	for i, fileID := range fileIDs {
		al.Tracks = append(al.Tracks, library.Track{
			ID:          uuid.Must(uuid.NewV7()),
			AlbumID:     albumID,
			DiscNumber:  1,
			TrackNumber: i + 1,
			Title:       "Track",
			FileID:      fileID,
			Audio:       library.AudioProperties{Codec: "flac", SampleRate: 44100, Channels: 2, Bitrate: 1000},
			Credits: []library.TrackCredit{
				{ArtistID: artist.ID, CreditedName: artist.Name, Role: "Performer"},
			},
		})
	}
	return al
}

func TestLibraryStore_ReplaceDirAlbums_KeepsIDs(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dirID, fileIDs := seedDir(t, db, 2)
	artist := insertTestArtist(t, s, "Artist A")

	first := testAlbum(dirID, artist, "First Title", "group-1", fileIDs)
	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{first}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dirID, first) failed: %v", err)
	}

	var gotAlbumID uuid.UUID
	var gotTitle string
	if err := db.QueryRowContext(ctx,
		`SELECT id, title FROM albums WHERE dir_id = ?`, dirID).Scan(&gotAlbumID, &gotTitle); err != nil {
		t.Fatalf("read album after first import failed: %v", err)
	}
	var firstTrackID uuid.UUID
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM tracks WHERE file_id = ?`, fileIDs[0]).Scan(&firstTrackID); err != nil {
		t.Fatalf("read track after first import failed: %v", err)
	}

	// Reimport with fresh minted IDs, a changed title, and the second
	// track dropped: matched rows must keep their IDs, the rest must go.
	second := testAlbum(dirID, artist, "Second Title", "group-1", fileIDs[:1])
	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{second}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dirID, second) failed: %v", err)
	}

	var gotAlbumID2 uuid.UUID
	if err := db.QueryRowContext(ctx,
		`SELECT id, title FROM albums WHERE dir_id = ?`, dirID).Scan(&gotAlbumID2, &gotTitle); err != nil {
		t.Fatalf("read album after reimport failed: %v", err)
	}
	if gotAlbumID2 != gotAlbumID {
		t.Errorf("album ID after reimport = %v, want original %v", gotAlbumID2, gotAlbumID)
	}
	if want := "Second Title"; gotTitle != want {
		t.Errorf("album title after reimport = %q, want %q", gotTitle, want)
	}

	var gotTrackID uuid.UUID
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM tracks WHERE file_id = ?`, fileIDs[0]).Scan(&gotTrackID); err != nil {
		t.Fatalf("read track after reimport failed: %v", err)
	}
	if gotTrackID != firstTrackID {
		t.Errorf("track ID after reimport = %v, want original %v", gotTrackID, firstTrackID)
	}

	var trackCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tracks WHERE album_id = ?`, gotAlbumID).Scan(&trackCount); err != nil {
		t.Fatalf("count tracks failed: %v", err)
	}
	if want := 1; trackCount != want {
		t.Errorf("tracks after reimport = %d, want %d", trackCount, want)
	}

	// An empty replacement clears the dir's catalog entirely.
	if err := s.ReplaceDirAlbums(ctx, dirID, nil); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dirID, nil) failed: %v", err)
	}
	var albumCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM albums WHERE dir_id = ?`, dirID).Scan(&albumCount); err != nil {
		t.Fatalf("count albums failed: %v", err)
	}
	if want := 0; albumCount != want {
		t.Errorf("albums after empty replace = %d, want %d", albumCount, want)
	}
}

func TestLibraryStore_ReplaceDirAlbums_OmitsDuplicateAlbumArtists(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dirID, fileIDs := seedDir(t, db, 1)
	artist := insertTestArtist(t, s, "Artist A")

	al := testAlbum(dirID, artist, "Album", "group-1", fileIDs)
	al.Album.Artists = append(al.Album.Artists,
		library.AlbumArtist{ArtistID: artist.ID, CreditedName: "A. Artist"})

	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dirID, album) with duplicate artist failed: %v", err)
	}

	var gotName string
	if err := db.QueryRowContext(ctx,
		`SELECT aa.credited_name
		 FROM album_artists aa
		 JOIN albums a ON a.id = aa.album_id
		 WHERE a.dir_id = ?`, dirID).Scan(&gotName); err != nil {
		t.Fatalf("read album artists failed: %v", err)
	}
	// The single-row Scan above doubles as the count check: a second
	// row would have failed the import's unique constraint instead.
	if want := "Artist A"; gotName != want {
		t.Errorf("album artist credited_name = %q, want first credit %q", gotName, want)
	}
}

func TestLibraryStore_ReplaceDirAlbums_OmitsDuplicateTrackCredits(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dirID, fileIDs := seedDir(t, db, 1)
	artist := insertTestArtist(t, s, "Artist A")

	al := testAlbum(dirID, artist, "Album", "group-1", fileIDs)
	dup := al.Tracks[0].Credits[0]
	dup.CreditedName = "A. Artist"
	al.Tracks[0].Credits = append(al.Tracks[0].Credits, dup)

	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dirID, album) with duplicate credit failed: %v", err)
	}

	var gotCount int
	var gotName string
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), MIN(tc.credited_name)
		 FROM track_credits tc
		 JOIN tracks t ON t.id = tc.track_id
		 WHERE t.file_id = ?`, fileIDs[0]).Scan(&gotCount, &gotName); err != nil {
		t.Fatalf("read track credits failed: %v", err)
	}
	if want := 1; gotCount != want {
		t.Errorf("track credit rows = %d, want %d", gotCount, want)
	}
	if want := "Artist A"; gotName != want {
		t.Errorf("track credit credited_name = %q, want first credit %q", gotName, want)
	}
}

func TestLibraryStore_ReplaceDirAlbums_PrimaryVersion(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dir1, files1 := seedDir(t, db, 1)
	dir2, files2 := seedDir(t, db, 1)
	artist := insertTestArtist(t, s, "Artist A")

	if err := s.ReplaceDirAlbums(ctx, dir1,
		[]metadata.ImportAlbum{testAlbum(dir1, artist, "Album", "group-1", files1)}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dir1, album) failed: %v", err)
	}
	// A second version of the same group must not steal primary.
	if err := s.ReplaceDirAlbums(ctx, dir2,
		[]metadata.ImportAlbum{testAlbum(dir2, artist, "Album (Remaster)", "group-1", files2)}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dir2, album) failed: %v", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT dir_id, primary_version FROM albums WHERE group_key = 'group-1'`)
	if err != nil {
		t.Fatalf("read group albums failed: %v", err)
	}
	defer rows.Close()

	got := map[uuid.UUID]bool{}
	for rows.Next() {
		var dirID uuid.UUID
		var primary bool
		if err := rows.Scan(&dirID, &primary); err != nil {
			t.Fatalf("scan group album failed: %v", err)
		}
		got[dirID] = primary
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate group albums failed: %v", err)
	}

	want := map[uuid.UUID]bool{dir1: true, dir2: false}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("primary versions mismatch (-want +got):\n%s", diff)
	}
}

// insertSortedArtist inserts an artist whose sort name differs from its
// display name, so callers can tell the two apart.
func insertSortedArtist(t *testing.T, s *LibraryStore, name, sortName string) library.Artist {
	t.Helper()
	artist := library.Artist{ID: uuid.Must(uuid.NewV7()), Name: name, SortName: sortName}
	if err := s.InsertArtist(context.Background(), artist, library.NormalizeArtistName(name)); err != nil {
		t.Fatalf("InsertArtist(%+v) failed: %v", artist, err)
	}
	return artist
}

func albumArtistSort(t *testing.T, db *sql.DB, albumID uuid.UUID) string {
	t.Helper()
	var sort string
	if err := db.QueryRowContext(context.Background(),
		`SELECT artist_sort FROM albums WHERE id = ?`, albumID).Scan(&sort); err != nil {
		t.Fatalf("read artist_sort failed: %v", err)
	}
	return sort
}

func TestLibraryStore_ReplaceDirAlbums_DenormalizesArtistSort(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dirID, fileIDs := seedDir(t, db, 1)
	artist := insertSortedArtist(t, s, "The Murlocs", "Murlocs, The")

	al := testAlbum(dirID, artist, "Young Blindness", "group-1", fileIDs)
	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums() failed: %v", err)
	}

	// The sort name, not the credited name: "The Murlocs" files under M.
	if got, want := albumArtistSort(t, db, al.Album.ID), "Murlocs, The"; got != want {
		t.Errorf("artist_sort = %q, want %q", got, want)
	}
}

func TestLibraryStore_ReplaceDirAlbums_ArtistSortUsesFirstCredit(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dirID, fileIDs := seedDir(t, db, 1)
	gizzard := insertSortedArtist(t, s, "King Gizzard & the Lizard Wizard", "King Gizzard")
	osees := insertSortedArtist(t, s, "Osees", "Osees")

	// A split files under its first credit, keeping it beside the rest of
	// that artist's discography.
	al := testAlbum(dirID, gizzard, "Split", "group-1", fileIDs)
	al.Album.Artists = append(al.Album.Artists,
		library.AlbumArtist{ArtistID: osees.ID, CreditedName: osees.Name})
	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums() failed: %v", err)
	}

	if got, want := albumArtistSort(t, db, al.Album.ID), "King Gizzard"; got != want {
		t.Errorf("artist_sort = %q, want %q", got, want)
	}
}

func TestLibraryStore_ReplaceDirAlbums_RefreshesArtistSortOnReparse(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dirID, fileIDs := seedDir(t, db, 1)
	murlocs := insertSortedArtist(t, s, "The Murlocs", "Murlocs, The")
	gizzard := insertSortedArtist(t, s, "King Gizzard & the Lizard Wizard", "King Gizzard")

	al := testAlbum(dirID, murlocs, "Young Blindness", "group-1", fileIDs)
	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums() failed: %v", err)
	}

	// Retagging the dir to a different artist must not leave the old
	// sort key behind on the update path.
	al.Album.Artists = []library.AlbumArtist{
		{ArtistID: gizzard.ID, CreditedName: gizzard.Name},
	}
	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums() reparse failed: %v", err)
	}

	if got, want := albumArtistSort(t, db, al.Album.ID), "King Gizzard"; got != want {
		t.Errorf("artist_sort = %q, want %q", got, want)
	}
}

func TestLibraryStore_ReplaceDirAlbums_ArtistSortWithoutCredits(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dirID, fileIDs := seedDir(t, db, 1)
	artist := insertTestArtist(t, s, "Artist A")

	al := testAlbum(dirID, artist, "Uncredited", "group-1", fileIDs)
	al.Album.Artists = nil
	if err := s.ReplaceDirAlbums(ctx, dirID, []metadata.ImportAlbum{al}); err != nil {
		t.Fatalf("ReplaceDirAlbums() failed: %v", err)
	}

	if got := albumArtistSort(t, db, al.Album.ID); got != "" {
		t.Errorf("artist_sort = %q, want empty", got)
	}
}

func TestLibraryStore_ReplaceDirAlbums_PromotesPrimaryAfterDelete(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dir1, files1 := seedDir(t, db, 1)
	dir2, files2 := seedDir(t, db, 1)
	artist := insertTestArtist(t, s, "Artist A")

	if err := s.ReplaceDirAlbums(ctx, dir1,
		[]metadata.ImportAlbum{testAlbum(dir1, artist, "Album", "group-1", files1)}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dir1, album) failed: %v", err)
	}
	remaster := testAlbum(dir2, artist, "Album (Remaster)", "group-1", files2)
	if err := s.ReplaceDirAlbums(ctx, dir2, []metadata.ImportAlbum{remaster}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dir2, album) failed: %v", err)
	}

	// dir1 held the group's primary; emptying it must hand primary to
	// the surviving version rather than leaving the group with none.
	if err := s.ReplaceDirAlbums(ctx, dir1, nil); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dir1, nil) failed: %v", err)
	}

	var gotPrimary bool
	if err := db.QueryRowContext(ctx,
		`SELECT primary_version FROM albums WHERE id = ?`, remaster.Album.ID).Scan(&gotPrimary); err != nil {
		t.Fatalf("read surviving album failed: %v", err)
	}
	if !gotPrimary {
		t.Errorf("surviving album primary_version = false, want true")
	}
}

func TestLibraryStore_ReplaceDirAlbums_KeepsPrimaryOnUnaffectedGroup(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewLibraryStore(db)

	dir1, files1 := seedDir(t, db, 1)
	dir2, files2 := seedDir(t, db, 1)
	artist := insertTestArtist(t, s, "Artist A")

	original := testAlbum(dir1, artist, "Album", "group-1", files1)
	if err := s.ReplaceDirAlbums(ctx, dir1, []metadata.ImportAlbum{original}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dir1, album) failed: %v", err)
	}
	remaster := testAlbum(dir2, artist, "Album (Remaster)", "group-1", files2)
	if err := s.ReplaceDirAlbums(ctx, dir2, []metadata.ImportAlbum{remaster}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dir2, album) failed: %v", err)
	}

	// Reimporting the non-primary dir must not shuffle primary around.
	if err := s.ReplaceDirAlbums(ctx, dir2, []metadata.ImportAlbum{remaster}); err != nil {
		t.Fatalf("ReplaceDirAlbums(ctx, dir2, album) reimport failed: %v", err)
	}

	var gotPrimaryID uuid.UUID
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM albums WHERE group_key = 'group-1' AND primary_version = 1`).
		Scan(&gotPrimaryID); err != nil {
		t.Fatalf("read group primary failed: %v", err)
	}
	if gotPrimaryID != original.Album.ID {
		t.Errorf("group primary = %v, want original album %v", gotPrimaryID, original.Album.ID)
	}
}
