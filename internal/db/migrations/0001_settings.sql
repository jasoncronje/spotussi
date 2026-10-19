-- Instance settings as key/value rows (play rule, timezone, theme, ...).
-- Values are JSON-encoded scalars so one table holds every setting type;
-- defaults live in code, so a missing row means "default".
CREATE TABLE settings (
    key        TEXT    PRIMARY KEY,
    value      TEXT    NOT NULL,
    updated_at INTEGER NOT NULL -- epoch ms
) STRICT;
