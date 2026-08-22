package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
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

// Artists returns a page of artists ordered by sort name, resuming
// after token. The returned token fetches the next page and is empty
// once the listing is exhausted.
func (s *LibraryStore) Artists(ctx context.Context, token string, limit int) ([]library.Artist, string, error) {
	limit = page.Size(limit)

	q := `SELECT ` + artistColumns + ` FROM artists`
	var args []any
	if token != "" {
		cur, err := page.DecodeToken(token, 2)
		if err != nil {
			return nil, "", err
		}
		q += ` WHERE (sort_name, id) > (?, ?)`
		args = append(args, cur[0], cur[1])
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
	return artists, page.EncodeToken(last.SortName, last.ID.String()), nil
}

const albumColumns = `a.id, a.dir_id, a.title, a.album_type, a.release_date,
	a.original_release_date, a.release_country, a.bootleg, a.compilation,
	a.live, a.group_key, a.version, a.primary_version`

func scanAlbum(dst func(...any) error) (library.Album, error) {
	var al library.Album
	err := dst(&al.ID, &al.DirID, &al.Title, &al.Type, &al.ReleaseDate,
		&al.OriginalReleaseDate, &al.ReleaseCountry, &al.Bootleg, &al.Compilation,
		&al.Live, &al.GroupKey, &al.Version, &al.PrimaryVersion)
	return al, err
}

// Album returns the album with id and its credited artists, or
// library.ErrNotFound.
func (s *LibraryStore) Album(ctx context.Context, id uuid.UUID) (library.Album, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+albumColumns+` FROM albums a WHERE a.id = ?`, id)
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

// Albums returns a page of albums ordered by title, resuming after
// token and restricted to artistID when it is not uuid.Nil. Each album
// carries its credited artists. The returned token fetches the next
// page and is empty once the listing is exhausted.
func (s *LibraryStore) Albums(ctx context.Context, artistID uuid.UUID, token string, limit int) ([]library.Album, string, error) {
	limit = page.Size(limit)

	q := `SELECT ` + albumColumns + ` FROM albums a`
	var args []any
	var where []string
	if artistID != uuid.Nil {
		q += ` JOIN album_artists aa ON aa.album_id = a.id`
		where = append(where, `aa.artist_id = ?`)
		args = append(args, artistID)
	}
	if token != "" {
		cur, err := page.DecodeToken(token, 2)
		if err != nil {
			return nil, "", err
		}
		where = append(where, `(a.title, a.id) > (?, ?)`)
		args = append(args, cur[0], cur[1])
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY a.title, a.id LIMIT ?`
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
		last := albums[limit-1]
		next = page.EncodeToken(last.Title, last.ID.String())
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

// Tracks returns a page of tracks in disc and track order, resuming
// after token and restricted to albumID when it is not uuid.Nil. Each
// track carries its credits. The returned token fetches the next page
// and is empty once the listing is exhausted.
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
		cur, err := page.DecodeToken(token, 3)
		if err != nil {
			return nil, "", err
		}
		disc, trackNo, err := parseTrackCursor(cur)
		if err != nil {
			return nil, "", err
		}
		where = append(where, `(disc_number, track_number, id) > (?, ?, ?)`)
		args = append(args, disc, trackNo, cur[2])
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
		next = page.EncodeToken(
			strconv.Itoa(last.DiscNumber), strconv.Itoa(last.TrackNumber), last.ID.String())
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

func parseTrackCursor(cur []string) (disc, trackNo int, err error) {
	if disc, err = strconv.Atoi(cur[0]); err != nil {
		return 0, 0, fmt.Errorf("%w: disc number: %v", page.ErrInvalidToken, err)
	}
	if trackNo, err = strconv.Atoi(cur[1]); err != nil {
		return 0, 0, fmt.Errorf("%w: track number: %v", page.ErrInvalidToken, err)
	}
	return disc, trackNo, nil
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
