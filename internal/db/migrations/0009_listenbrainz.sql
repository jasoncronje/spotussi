-- ListenBrainz fallback stage (D37): a first pass before MusicBrainz that
-- finds artist MBIDs and MusicBrainz genres through ListenBrainz's batch
-- lookups. Its per-entity outcomes and the genres it writes need the new
-- source; SQLite cannot alter a CHECK, so fallback_state and
-- catalog_provenance are rebuilt with it.
--
-- fallback_artists.spotify_id holds the Spotify artist ID of the artist's
-- one open.spotify.com/artist/ link on MusicBrainz (url-rels, fetched
-- with the genres at no extra request). It is stored only: nothing reads
-- it yet (linking provisional artists to it is an open decision).

CREATE TABLE fallback_state_new (
    source          TEXT    NOT NULL CHECK (source IN ('oembed', 'deezer', 'listenbrainz', 'musicbrainz')),
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
INSERT INTO fallback_state_new SELECT source, entity_type, id, status, external_id, confidence, attempts,
    next_attempt_at, last_error, updated_at FROM fallback_state;
DROP TABLE fallback_state;
ALTER TABLE fallback_state_new RENAME TO fallback_state;

CREATE TABLE catalog_provenance_new (
    entity_type TEXT    NOT NULL CHECK (entity_type IN ('track', 'album', 'artist')),
    entity_id   TEXT    NOT NULL,
    field       TEXT    NOT NULL,
    source      TEXT    NOT NULL CHECK (source IN ('oembed', 'deezer', 'listenbrainz', 'musicbrainz', 'borrowed')),
    confidence  REAL    NOT NULL,
    updated_at  INTEGER NOT NULL,
    PRIMARY KEY (entity_type, entity_id, field)
) STRICT, WITHOUT ROWID;
INSERT INTO catalog_provenance_new SELECT entity_type, entity_id, field, source, confidence, updated_at FROM catalog_provenance;
DROP TABLE catalog_provenance;
ALTER TABLE catalog_provenance_new RENAME TO catalog_provenance;
CREATE INDEX catalog_provenance_source ON catalog_provenance (source, entity_type);

ALTER TABLE fallback_artists ADD COLUMN spotify_id TEXT; -- MusicBrainz url-rels; stored only (D37)
