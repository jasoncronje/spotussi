-- Fallback metadata (D27): what free public sources (Spotify oEmbed,
-- Deezer, MusicBrainz) found for catalog entries Spotify has not
-- hydrated yet, with per-entity and per-field provenance.
--
-- Spotify stays canonical: tracks, albums and artists hold Spotify IDs
-- only. Fallbacks write there only into rows Spotify has not hydrated
-- (tracks: hydrated_at null, source 'fallback'; artists: the image of an
-- artist known by name only), and a Spotify write replaces every field
-- a fallback wrote. Artists and albums known only from export names live
-- in fallback_artists and fallback_albums, keyed by derivation's
-- provisional IDs (D20): ar_ / al_ plus base64url of the normalized name.
-- Their genres go into artist_genres under the same provisional ID.
-- Timestamps are epoch ms.

CREATE TABLE fallback_artists (
    id              TEXT    PRIMARY KEY, -- ar_ + base64url(normalized name)
    name            TEXT    NOT NULL,    -- as the export spells it
    image_url       TEXT,
    image_url_small TEXT,
    deezer_id       TEXT,
    mbid            TEXT,                -- MusicBrainz artist ID
    updated_at      INTEGER NOT NULL
) STRICT;

CREATE TABLE fallback_albums (
    id                     TEXT    PRIMARY KEY, -- al_ + base64url(artist \0 album), normalized
    name                   TEXT    NOT NULL,
    artist_name            TEXT    NOT NULL,
    album_type             TEXT,    -- album, single, compilation (Spotify's terms)
    release_date           TEXT,    -- YYYY, YYYY-MM or YYYY-MM-DD
    release_date_precision TEXT,    -- year, month, day
    total_tracks           INTEGER,
    image_url              TEXT,    -- largest image
    image_url_small        TEXT,    -- smallest image of at least 64 px
    deezer_id              TEXT,
    updated_at             INTEGER NOT NULL
) STRICT;

-- Per-entity, per-source outcome. id is a Spotify track or artist ID,
-- or a provisional album or artist ID. A failed entry is retried at
-- next_attempt_at; matched and no_match entries are final until the
-- entity's export names change.
CREATE TABLE fallback_state (
    source          TEXT    NOT NULL CHECK (source IN ('oembed', 'deezer', 'musicbrainz')),
    entity_type     TEXT    NOT NULL CHECK (entity_type IN ('track', 'album', 'artist')),
    id              TEXT    NOT NULL,
    status          TEXT    NOT NULL CHECK (status IN ('matched', 'no_match', 'failed')),
    external_id     TEXT,             -- Deezer ID, MusicBrainz ID
    confidence      REAL,             -- 0-1, matched only
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    updated_at      INTEGER NOT NULL,
    PRIMARY KEY (source, entity_type, id)
) STRICT, WITHOUT ROWID;

-- Per-field provenance of everything a fallback wrote. entity_id is
-- tracks.id or artists.id (Spotify IDs) or fallback_albums.id /
-- fallback_artists.id; field is the column name, or 'genres'. A catalog
-- field with no row here came from Spotify (tracks.source says which
-- Spotify source).
CREATE TABLE catalog_provenance (
    entity_type TEXT    NOT NULL CHECK (entity_type IN ('track', 'album', 'artist')),
    entity_id   TEXT    NOT NULL,
    field       TEXT    NOT NULL,
    source      TEXT    NOT NULL CHECK (source IN ('oembed', 'deezer', 'musicbrainz')),
    confidence  REAL    NOT NULL,
    updated_at  INTEGER NOT NULL,
    PRIMARY KEY (entity_type, entity_id, field)
) STRICT, WITHOUT ROWID;
CREATE INDEX catalog_provenance_source ON catalog_provenance (source, entity_type);
