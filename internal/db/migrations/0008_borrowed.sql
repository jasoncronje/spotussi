-- Borrowed fallback metadata (D27): a real Spotify artist that is not
-- hydrated yet and provably the same artist as a provisional one (the
-- only real and the only provisional artist of that normalized name,
-- sharing a track) takes the provisional artist's image and genres. Its
-- provenance rows say 'borrowed', with the lender's confidence; a Spotify
-- hydration replaces them like any fallback field. SQLite cannot alter a
-- CHECK, so catalog_provenance is rebuilt with the new source.

CREATE TABLE catalog_provenance_new (
    entity_type TEXT    NOT NULL CHECK (entity_type IN ('track', 'album', 'artist')),
    entity_id   TEXT    NOT NULL,
    field       TEXT    NOT NULL,
    source      TEXT    NOT NULL CHECK (source IN ('oembed', 'deezer', 'musicbrainz', 'borrowed')),
    confidence  REAL    NOT NULL,
    updated_at  INTEGER NOT NULL,
    PRIMARY KEY (entity_type, entity_id, field)
) STRICT, WITHOUT ROWID;
INSERT INTO catalog_provenance_new SELECT entity_type, entity_id, field, source, confidence, updated_at FROM catalog_provenance;
DROP TABLE catalog_provenance;
ALTER TABLE catalog_provenance_new RENAME TO catalog_provenance;
CREATE INDEX catalog_provenance_source ON catalog_provenance (source, entity_type);
