-- M1: instance access control (D13) and the Spotify connection (D12).

-- Small secrets keyed by name: the owner's argon2id password hash. Values
-- that need confidentiality (refresh token) are encrypted by the caller.
CREATE TABLE secrets (
    name       TEXT    PRIMARY KEY,
    value      BLOB    NOT NULL,
    updated_at INTEGER NOT NULL -- epoch ms
) STRICT;

-- Browser login sessions (password mode). The cookie holds a random token;
-- only its SHA-256 is stored, so a database copy cannot be replayed.
CREATE TABLE auth_sessions (
    token_hash   TEXT    PRIMARY KEY,
    csrf_token   TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL
) STRICT;
CREATE INDEX auth_sessions_expires ON auth_sessions (expires_at);

-- Read-only API/MCP tokens (D13, D16), stored as SHA-256 of the secret.
-- Revoking deletes the row.
CREATE TABLE api_tokens (
    id           TEXT    PRIMARY KEY,
    name         TEXT    NOT NULL,
    token_hash   TEXT    NOT NULL UNIQUE,
    prefix       TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER
) STRICT;

-- In-flight OAuth authorizations: PKCE verifier per state, single use,
-- short-lived.
CREATE TABLE spotify_auth_requests (
    state        TEXT    PRIMARY KEY,
    verifier     TEXT    NOT NULL,
    client_id    TEXT    NOT NULL,
    redirect_uri TEXT    NOT NULL,
    return_to    TEXT    NOT NULL,
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL
) STRICT;

-- The one connected Spotify account (D1). refresh_token is AES-GCM
-- ciphertext under DATA_DIR/key.
CREATE TABLE spotify_connection (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    client_id       TEXT    NOT NULL,
    account_id      TEXT    NOT NULL,
    account_name    TEXT,
    scopes          TEXT    NOT NULL,
    refresh_token   BLOB    NOT NULL,
    connected_at    INTEGER NOT NULL,
    refreshed_at    INTEGER,
    needs_reconnect INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT
) STRICT;
