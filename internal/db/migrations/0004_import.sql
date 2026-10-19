-- M3: historical import of the Extended Streaming History export (D10,
-- D23) into the raw layer (D3). Timestamps are epoch ms.

-- One row per import job: an upload processed by the server (queued, then
-- run one at a time) or a `spotussi import` run. Progress columns are
-- updated in the same transaction as each batch of rows, so they always
-- describe what is committed.
CREATE TABLE imports (
    id             INTEGER PRIMARY KEY,
    filename       TEXT    NOT NULL,
    origin         TEXT    NOT NULL CHECK (origin IN ('upload', 'cli')),
    kind           TEXT    NOT NULL CHECK (kind IN ('zip', 'json', 'dir')),
    state          TEXT    NOT NULL CHECK (state IN ('queued', 'running', 'done', 'failed')),
    source_path    TEXT,             -- uploaded temp file while queued or running
    files          INTEGER NOT NULL DEFAULT 0, -- history files found
    files_done     INTEGER NOT NULL DEFAULT 0,
    bytes_total    INTEGER,          -- uncompressed JSON bytes, once known
    bytes_done     INTEGER NOT NULL DEFAULT 0,
    rows_total     INTEGER,          -- known when the job finishes
    rows_done      INTEGER NOT NULL DEFAULT 0,
    rows_new       INTEGER NOT NULL DEFAULT 0,
    rows_duplicate INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL,
    started_at     INTEGER,
    finished_at    INTEGER,
    error_code     TEXT,
    error          TEXT
) STRICT;
CREATE INDEX imports_state ON imports (state, id);

-- Every row of every Streaming_History_{Audio,Video}_*.json file, verbatim
-- (all content types, D11), plus extracted columns for merging and
-- querying. row_hash is sha256(ts | uri | ms_played), with the track,
-- artist and episode names added when every URI is null: re-imports and
-- later, overlapping exports insert nothing new. ip_addr and
-- incognito_mode stay in this layer only (DF4).
CREATE TABLE raw_export_rows (
    id           INTEGER PRIMARY KEY,
    import_id    INTEGER NOT NULL REFERENCES imports (id),
    row_hash     BLOB    NOT NULL UNIQUE,
    ts_ms        INTEGER NOT NULL, -- end of the stream (`ts`, UTC)
    ms_played    INTEGER NOT NULL,
    content_type TEXT    NOT NULL, -- track, episode, audiobook, unknown
    uri          TEXT,             -- first non-null track, episode, audiobook chapter URI
    track_name   TEXT,
    artist_name  TEXT,
    album_name   TEXT,
    reason_start TEXT,
    reason_end   TEXT,
    shuffle      INTEGER,
    skipped      INTEGER,
    offline      INTEGER,
    incognito    INTEGER,
    platform     TEXT,
    conn_country TEXT,
    video        INTEGER NOT NULL DEFAULT 0, -- from a Streaming_History_Video file
    json         TEXT    NOT NULL
) STRICT;
CREATE INDEX raw_export_rows_ts ON raw_export_rows (ts_ms);
CREATE INDEX raw_export_rows_uri_ts ON raw_export_rows (uri, ts_ms);
-- Provisional history (D23) pages music rows by start time, newest first.
-- The start index covers every column History filters on, so counting
-- matches never reads the (large) rows; artist and album filters use
-- the normalized-name keys of the provisional entities.
CREATE INDEX raw_export_rows_track_start
    ON raw_export_rows ((ts_ms - ms_played), id, ms_played, skipped, shuffle, platform)
    WHERE content_type = 'track';
CREATE INDEX raw_export_rows_track_artist
    ON raw_export_rows (lower(trim(artist_name)), (ts_ms - ms_played), id)
    WHERE content_type = 'track';
CREATE INDEX raw_export_rows_track_album
    ON raw_export_rows (lower(trim(album_name)), (ts_ms - ms_played), id)
    WHERE content_type = 'track';

CREATE TRIGGER raw_export_rows_no_update BEFORE UPDATE ON raw_export_rows
BEGIN SELECT RAISE(ABORT, 'raw_export_rows is append-only'); END;
CREATE TRIGGER raw_export_rows_no_delete BEFORE DELETE ON raw_export_rows
BEGIN SELECT RAISE(ABORT, 'raw_export_rows is append-only'); END;
