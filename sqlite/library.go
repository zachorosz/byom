package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zachorosz/byom/library"
	"github.com/zachorosz/byom/metadata"
)

// LibraryStore persists and serves the music library: catalog writes
// from imports (this file) and single-item lookups plus keyset-paginated
// listings (libraryread.go).
type LibraryStore struct {
	db *sql.DB
}

func NewLibraryStore(db *sql.DB) *LibraryStore {
	return &LibraryStore{db: db}
}

// ArtistsByAlias returns the artists whose alias matches normName,
// oldest first.
func (s *LibraryStore) ArtistsByAlias(ctx context.Context, normName string) ([]library.Artist, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.name, a.sort_name
		 FROM artist_aliases aa
		 JOIN artists a ON a.id = aa.artist_id
		 WHERE aa.norm_name = ?
		 ORDER BY a.id`, normName)
	if err != nil {
		return nil, fmt.Errorf("lookup artists by alias: %w", err)
	}
	defer rows.Close()

	var artists []library.Artist
	for rows.Next() {
		var a library.Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.SortName); err != nil {
			return nil, fmt.Errorf("scan artist: %w", err)
		}
		artists = append(artists, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artists: %w", err)
	}
	return artists, nil
}

// InsertArtist inserts artist and its alias rows in one tx.
func (s *LibraryStore) InsertArtist(ctx context.Context, artist library.Artist, aliases ...string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO artists (id, name, sort_name) VALUES (?, ?, ?)`,
		artist.ID, artist.Name, artist.SortName); err != nil {
		return fmt.Errorf("insert artist: %w", err)
	}
	for _, alias := range aliases {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO artist_aliases (norm_name, artist_id) VALUES (?, ?)`,
			alias, artist.ID); err != nil {
			return fmt.Errorf("insert artist alias: %w", err)
		}
	}
	return tx.Commit()
}

// ReplaceDirAlbums replaces dirID's albums with albums in one tx.
// Albums match existing rows on (dir_id, group_key) and tracks on
// (file_id, start_offset_ns); matches keep their row IDs so external
// references survive reparses. A track whose file moved dirs is
// adopted by the new dir's album rather than recreated. Everything
// else previously imported for the dir is deleted. A version group
// that loses its primary to that delete promotes its oldest survivor.
func (s *LibraryStore) ReplaceDirAlbums(ctx context.Context, dirID uuid.UUID, albums []metadata.ImportAlbum) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	keptAlbums := make([]uuid.UUID, 0, len(albums))
	for _, al := range albums {
		albumID, err := upsertAlbum(ctx, tx, al.Album)
		if err != nil {
			return err
		}
		keptAlbums = append(keptAlbums, albumID)

		if err := replaceAlbumArtists(ctx, tx, albumID, al.Album.Artists); err != nil {
			return err
		}

		if err := replaceAlbumImages(ctx, tx, albumID, al.Album.Images); err != nil {
			return err
		}

		keptTracks := make([]uuid.UUID, 0, len(al.Tracks))
		for _, t := range al.Tracks {
			t.AlbumID = albumID // the upsert may have kept an existing album ID
			trackID, err := upsertTrack(ctx, tx, t)
			if err != nil {
				return err
			}
			keptTracks = append(keptTracks, trackID)

			if err := replaceTrackCredits(ctx, tx, trackID, t.Credits); err != nil {
				return err
			}
		}
		if err := deleteStale(ctx, tx, "tracks", "id", "album_id", albumID, keptTracks); err != nil {
			return err
		}
	}

	// Read before the delete: afterwards the groups whose primary this
	// dir owned are no longer discoverable from dirID.
	groupKeys, err := dirPrimaryGroups(ctx, tx, dirID)
	if err != nil {
		return err
	}

	if err := deleteStale(ctx, tx, "albums", "id", "dir_id", dirID, keptAlbums); err != nil {
		return err
	}

	if err := promoteOrphanedPrimaries(ctx, tx, groupKeys); err != nil {
		return err
	}

	return tx.Commit()
}

// dirPrimaryGroups returns the group keys of the albums under dirID
// that currently hold their group's primary version.
func dirPrimaryGroups(ctx context.Context, tx *sql.Tx, dirID uuid.UUID) ([]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT group_key FROM albums WHERE dir_id = ? AND primary_version = 1`, dirID)
	if err != nil {
		return nil, fmt.Errorf("list dir primary groups: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan dir primary group: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dir primary groups: %w", err)
	}
	return keys, nil
}

// promoteOrphanedPrimaries restores the one-primary-per-group
// invariant for each group in groupKeys that no longer has a primary
// version, promoting its oldest surviving album. Groups that kept a
// primary, and groups whose albums are all gone, are left untouched.
func promoteOrphanedPrimaries(ctx context.Context, tx *sql.Tx, groupKeys []string) error {
	for _, key := range groupKeys {
		// IDs are UUIDv7, so MIN picks the earliest imported version.
		if _, err := tx.ExecContext(ctx,
			`UPDATE albums SET primary_version = 1
			 WHERE id = (SELECT MIN(id) FROM albums WHERE group_key = ?)
			   AND NOT EXISTS (
				SELECT 1 FROM albums WHERE group_key = ? AND primary_version = 1)`,
			key, key); err != nil {
			return fmt.Errorf("promote group primary: %w", err)
		}
	}
	return nil
}

func upsertAlbum(ctx context.Context, tx *sql.Tx, al library.Album) (uuid.UUID, error) {
	// A new album becomes its group's primary version only when the
	// group has none yet; the update path keeps the stored value.
	var hasPrimary bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM albums WHERE group_key = ? AND primary_version = 1)`,
		al.GroupKey).Scan(&hasPrimary); err != nil {
		return uuid.Nil, fmt.Errorf("check group primary: %w", err)
	}

	var id uuid.UUID
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO albums (id, dir_id, title, album_type, release_date,
			original_release_date, release_country, bootleg, compilation,
			live, group_key, version, primary_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (dir_id, group_key) DO UPDATE SET
			title = excluded.title,
			album_type = excluded.album_type,
			release_date = excluded.release_date,
			original_release_date = excluded.original_release_date,
			release_country = excluded.release_country,
			bootleg = excluded.bootleg,
			compilation = excluded.compilation,
			live = excluded.live,
			version = excluded.version
		 RETURNING id`,
		al.ID, al.DirID, al.Title, al.Type, al.ReleaseDate,
		al.OriginalReleaseDate, al.ReleaseCountry, al.Bootleg, al.Compilation,
		al.Live, al.GroupKey, al.Version, !hasPrimary,
	).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("upsert album: %w", err)
	}
	return id, nil
}

func upsertTrack(ctx context.Context, tx *sql.Tx, t library.Track) (uuid.UUID, error) {
	var id uuid.UUID
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO tracks (id, album_id, disc_number, disc_subtitle,
			track_number, title, release_date, original_release_date,
			file_id, codec, sample_rate, bit_depth, channels, bitrate,
			duration_ns, start_offset_ns)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (file_id, start_offset_ns) DO UPDATE SET
			album_id = excluded.album_id,
			disc_number = excluded.disc_number,
			disc_subtitle = excluded.disc_subtitle,
			track_number = excluded.track_number,
			title = excluded.title,
			release_date = excluded.release_date,
			original_release_date = excluded.original_release_date,
			codec = excluded.codec,
			sample_rate = excluded.sample_rate,
			bit_depth = excluded.bit_depth,
			channels = excluded.channels,
			bitrate = excluded.bitrate,
			duration_ns = excluded.duration_ns
		 RETURNING id`,
		t.ID, t.AlbumID, t.DiscNumber, t.DiscSubtitle,
		t.TrackNumber, t.Title, t.ReleaseDate, t.OriginalReleaseDate,
		t.FileID, t.Audio.Codec, t.Audio.SampleRate, t.Audio.BitDepth,
		t.Audio.Channels, t.Audio.Bitrate, int64(t.Duration), int64(t.StartOffset),
	).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("upsert track: %w", err)
	}
	return id, nil
}

func replaceAlbumArtists(ctx context.Context, tx *sql.Tx, albumID uuid.UUID, artists []library.AlbumArtist) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM album_artists WHERE album_id = ?`, albumID); err != nil {
		return fmt.Errorf("clear album artists: %w", err)
	}
	for i, aa := range artists {
		// A duplicate artist is omitted (first credit wins), leaving a
		// gap at its position rather than failing the whole tx.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO album_artists (album_id, artist_id, credited_name, position)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT DO NOTHING`,
			albumID, aa.ArtistID, aa.CreditedName, i); err != nil {
			return fmt.Errorf("insert album artist: %w", err)
		}
	}
	return nil
}

// replaceAlbumImages syncs an album's image links: stale links are
// deleted first, then incoming links are upserted. The update path
// never touches set_cover, and the payload's cover default applies
// only when no cover survived the delete — a cover choice already in
// the DB (e.g. the user's) is never replaced by a scan.
func replaceAlbumImages(ctx context.Context, tx *sql.Tx, albumID uuid.UUID, images []library.AlbumImage) error {
	imageIDs := make([]uuid.UUID, 0, len(images))
	for _, img := range images {
		imageIDs = append(imageIDs, img.ImageID)
	}
	if err := deleteStale(ctx, tx, "album_images", "image_id", "album_id", albumID, imageIDs); err != nil {
		return err
	}

	var hasCover bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM album_images WHERE album_id = ? AND set_cover = 1)`,
		albumID).Scan(&hasCover); err != nil {
		return fmt.Errorf("check album cover: %w", err)
	}

	for _, img := range images {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO album_images (album_id, image_id, kind, set_cover)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT (album_id, image_id) DO UPDATE SET
				kind = excluded.kind`,
			albumID, img.ImageID, img.Kind, img.SetCover && !hasCover); err != nil {
			return fmt.Errorf("upsert album image: %w", err)
		}
	}
	return nil
}

func replaceTrackCredits(ctx context.Context, tx *sql.Tx, trackID uuid.UUID, credits []library.TrackCredit) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM track_credits WHERE track_id = ?`, trackID); err != nil {
		return fmt.Errorf("clear track credits: %w", err)
	}
	for _, c := range credits {
		// A duplicate (artist, role) credit is omitted (first wins)
		// rather than failing the whole tx.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO track_credits (track_id, artist_id, credited_name, role)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT DO NOTHING`,
			trackID, c.ArtistID, c.CreditedName, c.Role); err != nil {
			return fmt.Errorf("insert track credit: %w", err)
		}
	}
	return nil
}

// deleteStale deletes table rows owned by owner whose idCol is not in
// keep. table, idCol, and ownerCol are trusted literals from callers,
// never user input.
func deleteStale(ctx context.Context, tx *sql.Tx, table, idCol, ownerCol string, owner uuid.UUID, keep []uuid.UUID) error {
	args := []any{owner}
	q := `DELETE FROM ` + table + ` WHERE ` + ownerCol + ` = ?`
	if len(keep) > 0 {
		ph := make([]string, len(keep))
		for i, id := range keep {
			ph[i] = "?"
			args = append(args, id)
		}
		q += ` AND ` + idCol + ` NOT IN (` + strings.Join(ph, ",") + `)`
	}
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("delete stale %s: %w", table, err)
	}
	return nil
}
