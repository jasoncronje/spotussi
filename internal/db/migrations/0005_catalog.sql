-- Catalog (D9, D17): Spotify metadata for the tracks, albums, artists and
-- play contexts seen in the raw layer. Shared contract between hydration
-- (M4, which fills it) and derivation (M5, which reads it). Only real
-- Spotify entities live here, keyed by Spotify ID; tracks known only from
-- export rows stay provisional (name-keyed) in derivation until hydrated.
-- Rows are upserted as better data arrives, unlike the raw layer.
-- Timestamps are epoch ms.

CREATE TABLE tracks (
    id           TEXT    PRIMARY KEY, -- Spotify track ID
    name         TEXT    NOT NULL,
    duration_ms  INTEGER,
    explicit     INTEGER,
    isrc         TEXT,
    album_id     TEXT,                -- albums.id
    disc_number  INTEGER,
    track_number INTEGER,
    is_local     INTEGER NOT NULL DEFAULT 0,
    linked_from  TEXT,                -- original ID when Spotify relinked it
    source       TEXT    NOT NULL,    -- api, poller, recent: who wrote it last
    hydrated_at  INTEGER              -- null: seen but not fetched in full
) STRICT;
CREATE INDEX tracks_album ON tracks (album_id);

CREATE TABLE albums (
    id                     TEXT    PRIMARY KEY,
    name                   TEXT    NOT NULL,
    album_type             TEXT,    -- album, single, compilation
    release_date           TEXT,    -- as given: YYYY, YYYY-MM or YYYY-MM-DD
    release_date_precision TEXT,    -- year, month, day
    total_tracks           INTEGER,
    image_url              TEXT,    -- largest image
    image_url_small        TEXT,    -- smallest image of at least 64 px
    hydrated_at            INTEGER
) STRICT;

CREATE TABLE artists (
    id              TEXT    PRIMARY KEY,
    name            TEXT    NOT NULL,
    image_url       TEXT,
    image_url_small TEXT,
    hydrated_at     INTEGER -- null: known only as a track's simplified artist
) STRICT;

-- Track and album credits, in Spotify's order (position 0 is the main
-- artist).
CREATE TABLE track_artists (
    track_id  TEXT    NOT NULL,
    artist_id TEXT    NOT NULL,
    position  INTEGER NOT NULL,
    PRIMARY KEY (track_id, position)
) STRICT, WITHOUT ROWID;
CREATE INDEX track_artists_artist ON track_artists (artist_id, track_id);

CREATE TABLE album_artists (
    album_id  TEXT    NOT NULL,
    artist_id TEXT    NOT NULL,
    position  INTEGER NOT NULL,
    PRIMARY KEY (album_id, position)
) STRICT, WITHOUT ROWID;

CREATE TABLE artist_genres (
    artist_id TEXT NOT NULL,
    genre     TEXT NOT NULL,
    PRIMARY KEY (artist_id, genre)
) STRICT, WITHOUT ROWID;
CREATE INDEX artist_genres_genre ON artist_genres (genre, artist_id);

-- Play contexts (D17): playlists, albums, artists, shows. name_source says
-- where the name came from; Spotify-owned playlists can't be read by new
-- apps and stay unresolved until named by hand.
CREATE TABLE contexts (
    uri         TEXT    PRIMARY KEY,
    type        TEXT    NOT NULL, -- playlist, album, artist, show, collection, other
    name        TEXT,
    name_source TEXT    NOT NULL CHECK (name_source IN ('api', 'manual', 'unresolved')),
    owner_id    TEXT,
    image_url   TEXT,
    resolved_at INTEGER
) STRICT;

-- Hydration work queue (D9): highest priority first (most-played), with
-- backoff on failure.
CREATE TABLE hydration_queue (
    entity_type     TEXT    NOT NULL CHECK (entity_type IN ('track', 'album', 'artist', 'context')),
    id              TEXT    NOT NULL,
    priority        INTEGER NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    PRIMARY KEY (entity_type, id)
) STRICT, WITHOUT ROWID;
CREATE INDEX hydration_queue_next ON hydration_queue (next_attempt_at, priority DESC);
