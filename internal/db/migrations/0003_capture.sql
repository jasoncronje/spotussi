-- M2: live capture (D2) into the raw layer (D3). Raw tables are
-- append-only: rows are never updated or deleted (enforced by triggers),
-- and derived plays (M5) are rebuilt from them. Timestamps are epoch ms.

-- One row per completed play observed by the player poller, with the
-- last player snapshot verbatim. Extracted columns describe the play as a
-- whole (shuffle, context and device as first observed; device is the
-- one that played the most of it).
CREATE TABLE raw_poller_plays (
    id               INTEGER PRIMARY KEY,
    track_id         TEXT    NOT NULL, -- Spotify ID, or the URI for local files
    track_uri        TEXT    NOT NULL,
    duration_ms      INTEGER NOT NULL,
    started_ms       INTEGER NOT NULL, -- estimated start (first sighting minus progress)
    ended_ms         INTEGER NOT NULL, -- estimated end
    ms_played        INTEGER NOT NULL, -- measured listening time (D2)
    skipped          INTEGER NOT NULL, -- ended well before duration_ms by changing track
    end_reason       TEXT    NOT NULL, -- track_change, restart, stopped, gap
    shuffle          INTEGER,
    repeat           TEXT,             -- off, track, context
    device_name      TEXT,
    device_type      TEXT,
    context_uri      TEXT,
    private_session  INTEGER NOT NULL DEFAULT 0,
    seeks            INTEGER NOT NULL DEFAULT 0,
    pauses           INTEGER NOT NULL DEFAULT 0,
    device_switches  INTEGER NOT NULL DEFAULT 0,
    observations     INTEGER NOT NULL, -- player snapshots that saw this play
    first_seen_ms    INTEGER NOT NULL,
    last_seen_ms     INTEGER NOT NULL,
    json             TEXT    NOT NULL, -- last /me/player response for this play
    created_at       INTEGER NOT NULL,
    UNIQUE (track_id, started_ms)
) STRICT;
CREATE INDEX raw_poller_plays_started ON raw_poller_plays (started_ms);
CREATE INDEX raw_poller_plays_ended ON raw_poller_plays (ended_ms);

-- Verbatim items from /me/player/recently-played (the backstop).
CREATE TABLE raw_recent_plays (
    played_at_ms INTEGER NOT NULL,
    track_id     TEXT    NOT NULL,
    duration_ms  INTEGER,
    context_uri  TEXT,
    json         TEXT    NOT NULL,
    fetched_at   INTEGER NOT NULL,
    PRIMARY KEY (played_at_ms, track_id)
) STRICT, WITHOUT ROWID;

CREATE TRIGGER raw_poller_plays_no_update BEFORE UPDATE ON raw_poller_plays
BEGIN SELECT RAISE(ABORT, 'raw_poller_plays is append-only'); END;
CREATE TRIGGER raw_poller_plays_no_delete BEFORE DELETE ON raw_poller_plays
BEGIN SELECT RAISE(ABORT, 'raw_poller_plays is append-only'); END;
CREATE TRIGGER raw_recent_plays_no_update BEFORE UPDATE ON raw_recent_plays
BEGIN SELECT RAISE(ABORT, 'raw_recent_plays is append-only'); END;
CREATE TRIGGER raw_recent_plays_no_delete BEFORE DELETE ON raw_recent_plays
BEGIN SELECT RAISE(ABORT, 'raw_recent_plays is append-only'); END;

-- The poller's in-flight state (the play being listened to and the last
-- snapshot), written in the same transaction as each completed play so a
-- restart mid-track neither loses nor duplicates it.
CREATE TABLE capture_state (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    state      TEXT    NOT NULL, -- JSON (capture.MachineState)
    updated_at INTEGER NOT NULL
) STRICT;

-- played_at calibration (D8): for each recently-played row matched to a
-- poller play, the offsets of played_at from the poller's measured start
-- and end. Derived diagnostics, recomputable from the raw tables.
CREATE TABLE capture_calibration (
    played_at_ms    INTEGER NOT NULL,
    track_id        TEXT    NOT NULL,
    poller_play_id  INTEGER NOT NULL REFERENCES raw_poller_plays (id),
    start_offset_ms INTEGER NOT NULL, -- played_at - started_ms
    end_offset_ms   INTEGER NOT NULL, -- played_at - ended_ms
    ms_played       INTEGER NOT NULL,
    duration_ms     INTEGER NOT NULL,
    computed_at     INTEGER NOT NULL,
    PRIMARY KEY (played_at_ms, track_id)
) STRICT, WITHOUT ROWID;
