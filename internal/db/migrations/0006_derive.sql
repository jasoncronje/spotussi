-- M5: the derived layer (D3): plays merged from the raw sources (D8)
-- with the play rule applied (D7), and listening sessions (D14). Both are
-- rebuilt from raw + catalog + settings: a full rebuild fills new tables
-- and swaps them in within one transaction (internal/derive/store.go
-- repeats these definitions for the swap; a test keeps them in step), and
-- live plays are merged incrementally. Timestamps are epoch ms.

CREATE TABLE plays (
    id                  INTEGER PRIMARY KEY, -- start order at the last full rebuild; later plays append
    track_id            TEXT    NOT NULL,    -- Spotify track ID (or local-file URI)
    album_id            TEXT,                -- resolved album: Spotify ID, or a provisional al_ ID
    started_ms          INTEGER NOT NULL,
    ended_ms            INTEGER NOT NULL,
    local_day           TEXT    NOT NULL,    -- YYYY-MM-DD of the start in the settings timezone (D14)
    local_hour          INTEGER NOT NULL,    -- 0-23, same zone
    ms_played           INTEGER NOT NULL,
    ms_played_estimated INTEGER NOT NULL,    -- recently-played only: ms_played is the track length (D7)
    counts_as_play      INTEGER NOT NULL,    -- passes the play rule (D7)
    skipped             INTEGER NOT NULL,
    shuffle             INTEGER,
    platform            TEXT,                -- export
    device_name         TEXT,                -- poller
    device_type         TEXT,
    context_uri         TEXT,                -- poller, else recently-played
    country             TEXT,                -- export conn_country
    reason_start        TEXT,                -- export
    reason_end          TEXT,
    offline             INTEGER,
    sources             INTEGER NOT NULL,    -- bitmask: 1 export, 2 poller, 4 recently-played (D8)
    contradicted        INTEGER NOT NULL,    -- live play inside the export's span that the export lacks (D8)
    excluded            INTEGER NOT NULL,    -- hidden by an artist or track exclusion
    session_id          INTEGER,             -- sessions.id; null when excluded
    export_row_id       INTEGER,             -- raw_export_rows.id
    poller_play_id      INTEGER,             -- raw_poller_plays.id
    recent_played_at_ms INTEGER              -- raw_recent_plays.played_at_ms
) STRICT;
CREATE INDEX plays_started ON plays (started_ms);
CREATE INDEX plays_track ON plays (track_id, started_ms);

-- Runs of non-excluded plays where each starts within session_gap_minutes
-- of the end of the run so far. id is the first play's id.
CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY,
    started_ms INTEGER NOT NULL,
    ended_ms   INTEGER NOT NULL,
    rows       INTEGER NOT NULL,
    plays      INTEGER NOT NULL,
    ms_played  INTEGER NOT NULL,
    skips      INTEGER NOT NULL
) STRICT;
CREATE INDEX sessions_started ON sessions (started_ms);

-- Artists and tracks hidden from every stat (D14), applied by derivation.
-- name keeps a provisional artist excludable after hydration replaces its
-- ID with a Spotify one.
CREATE TABLE exclusions (
    kind       TEXT    NOT NULL CHECK (kind IN ('artist', 'track')),
    id         TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (kind, id)
) STRICT;

-- Bookkeeping of the last full rebuild (fingerprint of its inputs, time,
-- duration), so startup only rebuilds when something changed.
CREATE TABLE derive_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;
