-- +goose Up
CREATE TABLE artists (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    sort_name   TEXT NOT NULL
);

CREATE INDEX idx_artists_sort_name ON artists(sort_name);

CREATE TABLE artist_aliases (
    norm_name TEXT NOT NULL,
    artist_id TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    PRIMARY KEY(norm_name, artist_id)
);

CREATE INDEX artist_aliases_artist ON artist_aliases(artist_id);

CREATE TABLE albums (
    id                      TEXT    PRIMARY KEY,
    dir_id                  TEXT    NOT NULL REFERENCES dirs(id) ON DELETE CASCADE,

    title                   TEXT    NOT NULL,
    album_type              TEXT    NOT NULL DEFAULT '',
    release_date            TEXT    NOT NULL DEFAULT '',
    original_release_date   TEXT    NOT NULL DEFAULT '',
    release_country         TEXT    NOT NULL DEFAULT '',
    bootleg                 INTEGER NOT NULL DEFAULT 0,
    compilation             INTEGER NOT NULL DEFAULT 0,
    live                    INTEGER NOT NULL DEFAULT 0,

    group_key               TEXT    NOT NULL UNIQUE,
    version                 TEXT    NOT NULL DEFAULT '',
    primary_version         INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_albums_group ON albums(group_key);
-- Each album grouping may only have 1 primary version.
CREATE UNIQUE INDEX idx_albums_group_primary_version ON albums(group_key)
    WHERE primary_version = 1;

CREATE TABLE album_artists (
    album_id        TEXT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    artist_id       TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    credited_name   TEXT    NOT NULL,
    position        INTEGER NOT NULL,
    PRIMARY KEY (album_id, position),
    UNIQUE (album_id, artist_id)
);

CREATE INDEX idx_album_artists_artist ON album_artists(artist_id);

CREATE TABLE tracks (
    id                      TEXT    PRIMARY KEY,
    album_id                TEXT    NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    
    disc_number             INTEGER NOT NULL,
    disc_subtitle           TEXT    NOT NULL DEFAULT '',
    track_number            INTEGER NOT NULL,
    title                   TEXT    NOT NULL,
    release_date            TEXT    NOT NULL DEFAULT '',
    original_release_date   TEXT    NOT NULL DEFAULT '',

    file_id                 TEXT    NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    codec                   TEXT    NOT NULL,
    sample_rate             INTEGER NOT NULL,
    bit_depth               INTEGER NOT NULL DEFAULT 0,
    channels                INTEGER NOT NULL,
    bitrate                 INTEGER NOT NULL,
    duration_ns             INTEGER NOT NULL,
    start_offset_ns         INTEGER NOT NULL DEFAULT 0,

    -- One whole-file track per file (start_offset_ns=0), or many offset tracks
    -- per file (cue).
    UNIQUE(file_id, start_offset_ns)
);

CREATE INDEX idx_tracks_album ON tracks(album_id);

CREATE TABLE track_credits (
    track_id        TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    artist_id       TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    credited_name   TEXT NOT NULL,
    role            TEXT NOT NULL,

    PRIMARY KEY (track_id, artist_id, role)
);

CREATE INDEX idx_track_credits_artist ON track_credits(artist_id);

CREATE TABLE images (
    id              TEXT PRIMARY KEY,
    content_hash    TEXT NOT NULL UNIQUE,
    mime            TEXT NOT NULL,
    width           INTEGER NOT NULL DEFAULT 0,
    height          INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE album_images (
    album_id    TEXT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    image_id    TEXT NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    set_cover   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_album_images_image ON album_images(image_id);
CREATE UNIQUE INDEX idx_album_images_one_set_cover ON album_images(album_id)
    WHERE set_cover = 1;

-- +goose Down
DROP TABLE album_images;
DROP TABLE images;
DROP TABLE track_credits;
DROP TABLE tracks;
DROP TABLE album_artists;
DROP TABLE albums;
DROP TABLE artist_aliases;
DROP TABLE artists;
