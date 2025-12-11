CREATE TABLE IF NOT EXISTS users
(
    id         INTEGER     NOT NULL PRIMARY KEY,
    status     INTEGER     NOT NULL DEFAULT 0,
    username   VARCHAR(32) NOT NULL DEFAULT '',
    first_name VARCHAR(64) NOT NULL DEFAULT '',
    last_name  VARCHAR(64) NOT NULL DEFAULT '',
    created    DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated    DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_users_approved ON users (status, updated);
-- status: 0 - pending, 1 - approved, 2 - rejected

CREATE TABLE IF NOT EXISTS events
(
    timestamp DATETIME NOT NULL PRIMARY KEY,
    load      INTEGER  NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_events_load ON events (load);

CREATE TABLE IF NOT EXISTS holidays
(
    day     DATE         NOT NULL PRIMARY KEY,
    title   VARCHAR(255) NOT NULL,
    created DATETIME DEFAULT '1970-01-01 00:00:00'
);


-- Migrations
-- 2025-12-06 14:04:33 UTC
-- ALTER TABLE holidays ADD COLUMN created DATETIME DEFAULT '1970-01-01 00:00:00';
--- 2025-12-09 14:07:47
-- DROP TABLE IF EXISTS users;
