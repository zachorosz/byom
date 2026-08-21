-- +goose Up
CREATE TABLE locations (
    id              TEXT    PRIMARY KEY,
    uri             TEXT    NOT NULL UNIQUE,
    scan_generation INTEGER NOT NULL DEFAULT 0,
    available       INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE scans (
    id              TEXT        PRIMARY KEY,
    location_id     TEXT        NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    generation      INTEGER     NOT NULL,
    state           TEXT        NOT NULL DEFAULT 'running'
                                CHECK (state IN ('running', 'done', 'failed', 'aborted')),
    start_time      TIMESTAMP   NOT NULL,
    finish_time     TIMESTAMP,
    error           TEXT,
    dirs_seen       INTEGER     NOT NULL DEFAULT 0,
    dirs_missing    INTEGER     NOT NULL DEFAULT 0,
    files_seen      INTEGER     NOT NULL DEFAULT 0,
    files_missing   INTEGER     NOT NULL DEFAULT 0
);

CREATE TABLE dirs (
    id                  TEXT    PRIMARY KEY,
    location_id         TEXT    NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    relpath             TEXT    NOT NULL,
    seen_generation     INTEGER NOT NULL,
    missing             INTEGER NOT NULL DEFAULT 0,
    -- dirty:               1 = needs (re)parsing. Set by SyncDir, cleared only
    --                      by the claim step in DirtyDirs. 
    -- locked_generation:   NULL = not currently being parsed.
    --                      NOT NULL = a parser has claimed this row and is 
    --                      working against the seen_generation value
    --                      captured at claim time. Written only by claim/release.
    dirty               INTEGER NOT NULL DEFAULT 0,
    locked_generation   INTEGER,

    UNIQUE (location_id, relpath)
);

-- Sweep query: active dirs not seen in the current generation.
CREATE INDEX idx_dirs_sweep ON dirs(location_id, seen_generation)
    WHERE missing = 0;

CREATE INDEX idx_dirs_parse_queue ON dirs(id) 
    WHERE dirty = 1 AND locked_generation IS NULL;

CREATE TABLE files (
    id              TEXT        PRIMARY KEY,
    dir_id          TEXT        NOT NULL REFERENCES dirs(id) ON DELETE CASCADE,
    name            TEXT        NOT NULL,
    kind            TEXT        NOT NULL,
    size_bytes      INTEGER     NOT NULL,
    mod_time        TIMESTAMP   NOT NULL,
    missing         INTEGER     NOT NULL DEFAULT 0,

    UNIQUE (dir_id, name)
);

-- +goose Down
DROP TABLE files;
DROP TABLE dirs;
DROP TABLE scans;
DROP TABLE locations;
