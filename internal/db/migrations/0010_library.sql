-- Library and playlist sync (D36, DF9). Spotify exposes only the current
-- state of a library, so sync records what it saw and when: one row per
-- sync run, one per list walked or checked in it, and one span per
-- stretch of membership of an item in a list. Raw observations are kept
-- forever (D3): spans are closed, never deleted. Timestamps are epoch ms.

CREATE TABLE library_syncs (
    id          INTEGER PRIMARY KEY,
    kind        TEXT    NOT NULL CHECK (kind IN ('quick', 'full', 'manual')),
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    requests    INTEGER NOT NULL DEFAULT 0,
    error       TEXT
) STRICT;

-- What a sync did with one list ('tracks', 'albums', 'playlists' or a
-- playlist ID): checked its first page or walked all of it. Only a
-- complete walk can prove that an item is gone.
CREATE TABLE library_walks (
    sync_id  INTEGER NOT NULL,
    list     TEXT    NOT NULL,
    mode     TEXT    NOT NULL CHECK (mode IN ('check', 'walk')),
    complete INTEGER NOT NULL,
    total    INTEGER,
    changed  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (sync_id, list)
) STRICT, WITHOUT ROWID;

-- One stretch of membership of an item in a list: saved tracks
-- ('tracks'), saved albums ('albums') or a playlist's items (the
-- playlist ID). An item removed and added again is a new span (Spotify
-- gives it a new added_at). json is the verbatim item at first sight.
CREATE TABLE library_spans (
    id              INTEGER PRIMARY KEY,
    list            TEXT    NOT NULL,
    item_uri        TEXT    NOT NULL,
    added_at        INTEGER,          -- null: Spotify gave none (very old playlists)
    added_by        TEXT,             -- playlist items: the user who added it
    position        INTEGER,          -- in the list at the last walk that saw it
    first_seen_sync INTEGER NOT NULL,
    last_seen_sync  INTEGER NOT NULL, -- last complete walk that saw it
    gone_sync       INTEGER,          -- first complete walk without it; null while present
    json            TEXT    NOT NULL
) STRICT;
CREATE INDEX library_spans_present ON library_spans (list, added_at) WHERE gone_sync IS NULL;
CREATE INDEX library_spans_item ON library_spans (item_uri);

-- Every playlist the user has (own, collaborative, followed), as last
-- listed. Followed playlists have no items for a development-mode app
-- (Feb 2026); items_snapshot_id is the snapshot of the last complete walk
-- of the items, so an unchanged playlist is not walked again.
CREATE TABLE library_playlists (
    id                TEXT    PRIMARY KEY,
    name              TEXT    NOT NULL,
    owner_id          TEXT,
    owner_name        TEXT,
    relation          TEXT    NOT NULL CHECK (relation IN ('own', 'collaborative', 'followed')),
    collaborative     INTEGER NOT NULL DEFAULT 0,
    public            INTEGER,
    image_url         TEXT,
    snapshot_id       TEXT,
    items_total       INTEGER,
    items_available   INTEGER NOT NULL DEFAULT 0,
    items_snapshot_id TEXT,
    position          INTEGER,
    first_seen_sync   INTEGER NOT NULL,
    last_seen_sync    INTEGER NOT NULL,
    gone_sync         INTEGER,
    json              TEXT    NOT NULL
) STRICT;
