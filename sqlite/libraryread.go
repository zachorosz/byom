package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/page"
)

// This file holds LibraryStore's read surface: single-item lookups and
// keyset-paginated listings. See library.go for the write surface and
// type definition.

const artistColumns = `id, name, sort_name`

// Artist returns the artist with id, or library.ErrNotFound.
func (s *LibraryStore) Artist(ctx context.Context, id uuid.UUID) (library.Artist, error) {
	var a library.Artist
	err := s.db.QueryRowContext(ctx,
		`SELECT `+artistColumns+` FROM artists WHERE id = ?`, id).
		Scan(&a.ID, &a.Name, &a.SortName)
	if errors.Is(err, sql.ErrNoRows) {
		return library.Artist{}, fmt.Errorf("artist %s: %w", id, library.ErrNotFound)
	}
	if err != nil {
		return library.Artist{}, fmt.Errorf("get artist: %w", err)
	}
	return a, nil
}

type artistCursor struct {
	SortName string    `json:"sort_name"`
	ID       uuid.UUID `json:"id"`
}

// Artists returns a page of artists ordered by sort name, resuming
// after token. The returned token fetches the next page and is empty
// once the listing is exhausted.
func (s *LibraryStore) Artists(ctx context.Context, token string, limit int) ([]library.Artist, string, error) {
	limit = page.Size(limit)

	q := `SELECT ` + artistColumns + ` FROM artists`
	var args []any
	if token != "" {
		var cur artistCursor
		if err := page.Decode(token, &cur); err != nil {
			return nil, "", err
		}
		q += ` WHERE (sort_name, id) > (?, ?)`
		args = append(args, cur.SortName, cur.ID)
	}
	q += ` ORDER BY sort_name, id LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list artists: %w", err)
	}
	defer rows.Close()

	var artists []library.Artist
	for rows.Next() {
		var a library.Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.SortName); err != nil {
			return nil, "", fmt.Errorf("scan artist: %w", err)
		}
		artists = append(artists, a)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate artists: %w", err)
	}

	if len(artists) <= limit {
		return artists, "", nil
	}
	artists = artists[:limit]
	last := artists[limit-1]
	next, err := page.Encode(artistCursor{SortName: last.SortName, ID: last.ID})
	if err != nil {
		return nil, "", err
	}
	return artists, next, nil
}

const albumColumns = `a.id, a.dir_id, a.title, a.album_type, a.release_date,
	a.original_release_date, a.release_country, a.bootleg, a.compilation,
	a.live, a.group_key, a.version, a.primary_version, a.artist_sort,
	COALESCE(img.content_hash, '') AS cover_hash`

func scanAlbum(scanFn func(...any) error) (library.Album, error) {
	var al library.Album
	err := scanFn(&al.ID, &al.DirID, &al.Title, &al.Type, &al.ReleaseDate,
		&al.OriginalReleaseDate, &al.ReleaseCountry, &al.Bootleg, &al.Compilation,
		&al.Live, &al.GroupKey, &al.Version, &al.PrimaryVersion, &al.ArtistSort,
		&al.CoverHash)
	return al, err
}

// Album returns the album with id and its credited artists, or
// library.ErrNotFound.
func (s *LibraryStore) Album(ctx context.Context, id uuid.UUID) (library.Album, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+albumColumns+`
		 FROM albums AS a
		 LEFT JOIN album_images AS cov ON a.id = cov.album_id AND cov.set_cover = 1
		 LEFT JOIN images AS img ON img.id = cov.image_id
		 WHERE a.id = ?`, id)
	al, err := scanAlbum(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return library.Album{}, fmt.Errorf("album %s: %w", id, library.ErrNotFound)
	}
	if err != nil {
		return library.Album{}, fmt.Errorf("get album: %w", err)
	}

	byAlbum, err := s.albumArtists(ctx, []uuid.UUID{al.ID})
	if err != nil {
		return library.Album{}, err
	}
	al.Artists = byAlbum[al.ID]
	return al, nil
}

// unknownDate sorts after every real date, since a blank date means
// unknown rather than ancient.
const unknownDate = "9999"

const (
	releaseDateExpr  = `COALESCE(NULLIF(a.release_date, ''), '` + unknownDate + `')`
	originalDateExpr = `COALESCE(NULLIF(a.original_release_date, ''), '` + unknownDate + `')`
)

func sortableDate(date string) string {
	if date == "" {
		return unknownDate
	}
	return date
}

type albumCursor struct {
	Query        library.AlbumQuery `json:"query"`
	Title        string             `json:"title"`
	ArtistSort   string             `json:"artist_sort,omitempty"`
	ReleaseDate  string             `json:"release_date,omitempty"`
	OriginalDate string             `json:"original_date,omitempty"`
	ID           uuid.UUID          `json:"id"`
}

func newAlbumCursor(query library.AlbumQuery, al library.Album) albumCursor {
	return albumCursor{
		Query:        query,
		Title:        al.Title,
		ArtistSort:   al.ArtistSort,
		ReleaseDate:  sortableDate(al.ReleaseDate),
		OriginalDate: sortableDate(al.OriginalReleaseDate),
		ID:           al.ID,
	}
}

// albumSort returns the sort expressions for an ordering and cur's
// matching values, always ending with the album ID so that the
// ordering is total. An unrecognized order is an error rather than a
// silent fallback.
func albumSort(order library.AlbumOrder, cur albumCursor) ([]string, []any, error) {
	switch order {
	case library.AlbumOrderTitle:
		return []string{`a.title`, `a.id`},
			[]any{cur.Title, cur.ID}, nil
	case library.AlbumOrderArtist:
		return []string{`a.artist_sort`, originalDateExpr, `a.title`, `a.id`},
			[]any{cur.ArtistSort, cur.OriginalDate, cur.Title, cur.ID}, nil
	case library.AlbumOrderReleaseDate:
		return []string{releaseDateExpr, `a.title`, `a.id`},
			[]any{cur.ReleaseDate, cur.Title, cur.ID}, nil
	case library.AlbumOrderOriginalDate:
		return []string{originalDateExpr, `a.title`, `a.id`},
			[]any{cur.OriginalDate, cur.Title, cur.ID}, nil
	case library.AlbumOrderRecentlyAdded:
		return []string{`a.id`}, []any{cur.ID}, nil
	}
	return nil, nil, fmt.Errorf("unknown album order %q", order)
}

// Albums returns a page of albums narrowed and sorted by query,
// resuming after token. Each album carries its credited artists. The
// returned token fetches the next page and is empty once the listing is
// exhausted. Resuming with a query other than the one the token was
// issued for fails with page.ErrInvalidToken.
func (s *LibraryStore) Albums(ctx context.Context, query library.AlbumQuery, token string, limit int) ([]library.Album, string, error) {
	limit = page.Size(limit)

	q := `SELECT ` + albumColumns + `
		FROM albums AS a
		LEFT JOIN album_images AS cov ON a.id = cov.album_id AND cov.set_cover = 1
		LEFT JOIN images AS img ON img.id = cov.image_id
	`
	var args []any
	var where []string
	if query.ArtistID != uuid.Nil {
		q += ` JOIN album_artists aa ON aa.album_id = a.id`
		where = append(where, `aa.artist_id = ?`)
		args = append(args, query.ArtistID)
	}
	if !query.IncludeAllVersions {
		where = append(where, `a.primary_version = 1`)
	}
	if len(query.Types) > 0 {
		where = append(where, `a.album_type IN (`+placeholders(len(query.Types))+`)`)
		for _, t := range query.Types {
			args = append(args, string(t))
		}
	}
	switch query.Bootlegs {
	case library.BootlegsExclude:
		where = append(where, `a.bootleg = 0`)
	case library.BootlegsOnly:
		where = append(where, `a.bootleg = 1`)
	}

	var cur albumCursor
	if token != "" {
		if err := page.Decode(token, &cur); err != nil {
			return nil, "", err
		}
		if !cur.Query.Equal(query) {
			return nil, "", fmt.Errorf("%w: query changed mid-listing", page.ErrInvalidToken)
		}
	}
	sortExprs, cursorValues, err := albumSort(query.Order, cur)
	if err != nil {
		return nil, "", err
	}

	// Descending flips every key, so the keyset comparison stays a
	// single row-value comparison rather than an OR expansion.
	direction, compare := ` ASC`, `>`
	if query.Descending {
		direction, compare = ` DESC`, `<`
	}
	if token != "" {
		where = append(where, `(`+strings.Join(sortExprs, `, `)+`) `+compare+
			` (`+placeholders(len(sortExprs))+`)`)
		args = append(args, cursorValues...)
	}

	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	orderBy := make([]string, len(sortExprs))
	for i, expr := range sortExprs {
		orderBy[i] = expr + direction
	}
	q += ` ORDER BY ` + strings.Join(orderBy, `, `) + ` LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list albums: %w", err)
	}
	defer rows.Close()

	var albums []library.Album
	for rows.Next() {
		al, err := scanAlbum(rows.Scan)
		if err != nil {
			return nil, "", fmt.Errorf("scan album: %w", err)
		}
		albums = append(albums, al)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate albums: %w", err)
	}

	next := ""
	if len(albums) > limit {
		albums = albums[:limit]
		if next, err = page.Encode(newAlbumCursor(query, albums[limit-1])); err != nil {
			return nil, "", err
		}
	}

	ids := make([]uuid.UUID, len(albums))
	for i, al := range albums {
		ids[i] = al.ID
	}
	byAlbum, err := s.albumArtists(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	for i := range albums {
		albums[i].Artists = byAlbum[albums[i].ID]
	}
	return albums, next, nil
}

// AlbumVersions returns every album sharing id's version group,
// including id itself, with the primary version first and the rest by
// release date. It returns library.ErrNotFound if no album has id.
func (s *LibraryStore) AlbumVersions(ctx context.Context, id uuid.UUID) ([]library.Album, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+albumColumns+`
		 FROM albums AS a
		 LEFT JOIN album_images AS cov ON a.id = cov.album_id AND cov.set_cover = 1
		 LEFT JOIN images AS img ON img.id = cov.image_id
		 WHERE a.group_key = (SELECT group_key FROM albums WHERE id = ?)
		 ORDER BY a.primary_version DESC, a.release_date, a.id`, id)
	if err != nil {
		return nil, fmt.Errorf("list album versions: %w", err)
	}
	defer rows.Close()

	var albums []library.Album
	for rows.Next() {
		al, err := scanAlbum(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan album: %w", err)
		}
		albums = append(albums, al)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate album versions: %w", err)
	}
	// The album belongs to its own group, so an empty group means the
	// album does not exist.
	if len(albums) == 0 {
		return nil, fmt.Errorf("album %s: %w", id, library.ErrNotFound)
	}

	ids := make([]uuid.UUID, len(albums))
	for i, al := range albums {
		ids[i] = al.ID
	}
	byAlbum, err := s.albumArtists(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range albums {
		albums[i].Artists = byAlbum[albums[i].ID]
	}
	return albums, nil
}

// albumArtists returns the credited artists of each album in ids,
// keyed by album ID and in credited order.
func (s *LibraryStore) albumArtists(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]library.AlbumArtist, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT album_id, artist_id, credited_name FROM album_artists
		 WHERE album_id IN (`+placeholders(len(ids))+`)
		 ORDER BY album_id, position`, args...)
	if err != nil {
		return nil, fmt.Errorf("list album artists: %w", err)
	}
	defer rows.Close()

	byAlbum := make(map[uuid.UUID][]library.AlbumArtist, len(ids))
	for rows.Next() {
		var albumID uuid.UUID
		var aa library.AlbumArtist
		if err := rows.Scan(&albumID, &aa.ArtistID, &aa.CreditedName); err != nil {
			return nil, fmt.Errorf("scan album artist: %w", err)
		}
		byAlbum[albumID] = append(byAlbum[albumID], aa)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate album artists: %w", err)
	}
	return byAlbum, nil
}

const trackColumns = `id, album_id, disc_number, disc_subtitle, track_number,
	title, release_date, original_release_date, file_id, codec, sample_rate,
	bit_depth, channels, bitrate, duration_ns, start_offset_ns`

func scanTrack(dst func(...any) error) (library.Track, error) {
	var t library.Track
	var durationNS, startOffsetNS int64
	err := dst(&t.ID, &t.AlbumID, &t.DiscNumber, &t.DiscSubtitle, &t.TrackNumber,
		&t.Title, &t.ReleaseDate, &t.OriginalReleaseDate, &t.FileID,
		&t.Audio.Codec, &t.Audio.SampleRate, &t.Audio.BitDepth, &t.Audio.Channels,
		&t.Audio.Bitrate, &durationNS, &startOffsetNS)
	t.Duration = time.Duration(durationNS)
	t.StartOffset = time.Duration(startOffsetNS)
	return t, err
}

// Track returns the track with id and its credits, or
// library.ErrNotFound.
func (s *LibraryStore) Track(ctx context.Context, id uuid.UUID) (library.Track, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+trackColumns+` FROM tracks WHERE id = ?`, id)
	t, err := scanTrack(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return library.Track{}, fmt.Errorf("track %s: %w", id, library.ErrNotFound)
	}
	if err != nil {
		return library.Track{}, fmt.Errorf("get track: %w", err)
	}

	byTrack, err := s.trackCredits(ctx, []uuid.UUID{t.ID})
	if err != nil {
		return library.Track{}, err
	}
	t.Credits = byTrack[t.ID]
	return t, nil
}

type trackCursor struct {
	AlbumID     uuid.UUID `json:"album_id"`
	DiscNumber  int       `json:"disc_number"`
	TrackNumber int       `json:"track_number"`
	ID          uuid.UUID `json:"id"`
}

// Tracks returns a page of tracks in disc and track order, resuming
// after token and restricted to albumID when it is not uuid.Nil. Each
// track carries its credits. The returned token fetches the next page
// and is empty once the listing is exhausted. Resuming with an albumID
// other than the one the token was issued for fails with
// page.ErrInvalidToken.
func (s *LibraryStore) Tracks(ctx context.Context, albumID uuid.UUID, token string, limit int) ([]library.Track, string, error) {
	limit = page.Size(limit)

	q := `SELECT ` + trackColumns + ` FROM tracks`
	var args []any
	var where []string
	if albumID != uuid.Nil {
		where = append(where, `album_id = ?`)
		args = append(args, albumID)
	}
	if token != "" {
		var cur trackCursor
		if err := page.Decode(token, &cur); err != nil {
			return nil, "", err
		}
		if cur.AlbumID != albumID {
			return nil, "", fmt.Errorf("%w: album changed mid-listing", page.ErrInvalidToken)
		}
		where = append(where, `(disc_number, track_number, id) > (?, ?, ?)`)
		args = append(args, cur.DiscNumber, cur.TrackNumber, cur.ID)
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY disc_number, track_number, id LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list tracks: %w", err)
	}
	defer rows.Close()

	var tracks []library.Track
	for rows.Next() {
		t, err := scanTrack(rows.Scan)
		if err != nil {
			return nil, "", fmt.Errorf("scan track: %w", err)
		}
		tracks = append(tracks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate tracks: %w", err)
	}

	next := ""
	if len(tracks) > limit {
		tracks = tracks[:limit]
		last := tracks[limit-1]
		cur := trackCursor{
			AlbumID:     albumID,
			DiscNumber:  last.DiscNumber,
			TrackNumber: last.TrackNumber,
			ID:          last.ID,
		}
		var err error
		if next, err = page.Encode(cur); err != nil {
			return nil, "", err
		}
	}

	ids := make([]uuid.UUID, len(tracks))
	for i, t := range tracks {
		ids[i] = t.ID
	}
	byTrack, err := s.trackCredits(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	for i := range tracks {
		tracks[i].Credits = byTrack[tracks[i].ID]
	}
	return tracks, next, nil
}

// trackCredits returns the credits of each track in ids, keyed by
// track ID.
func (s *LibraryStore) trackCredits(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]library.TrackCredit, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT track_id, artist_id, credited_name, role FROM track_credits
		 WHERE track_id IN (`+placeholders(len(ids))+`)
		 ORDER BY track_id, role, artist_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list track credits: %w", err)
	}
	defer rows.Close()

	byTrack := make(map[uuid.UUID][]library.TrackCredit, len(ids))
	for rows.Next() {
		var trackID uuid.UUID
		var c library.TrackCredit
		if err := rows.Scan(&trackID, &c.ArtistID, &c.CreditedName, &c.Role); err != nil {
			return nil, fmt.Errorf("scan track credit: %w", err)
		}
		byTrack[trackID] = append(byTrack[trackID], c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate track credits: %w", err)
	}
	return byTrack, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
