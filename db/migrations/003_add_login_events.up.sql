CREATE TABLE IF NOT EXISTS login_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NULL,
    username    TEXT,
    success     BOOLEAN NOT NULL,
    reason      TEXT,
    ip          TEXT,
    user_agent  TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);


