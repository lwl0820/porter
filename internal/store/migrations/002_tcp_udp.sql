CREATE TABLE rules_v2 (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK (protocol IN ('tcp', 'udp', 'tcp+udp')),
    listen_host TEXT NOT NULL,
    listen_port INTEGER NOT NULL CHECK (listen_port BETWEEN 1 AND 65535),
    target_host TEXT NOT NULL,
    target_port INTEGER NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    revision INTEGER NOT NULL CHECK (revision > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (protocol, listen_host, listen_port)
);
INSERT INTO rules_v2 (
    id, name, protocol, listen_host, listen_port, target_host,
    target_port, enabled, revision, created_at, updated_at
)
SELECT
    id, name, protocol, listen_host, listen_port, target_host,
    target_port, enabled, revision, created_at, updated_at
FROM rules;
DROP TABLE rules;
ALTER TABLE rules_v2 RENAME TO rules;
PRAGMA user_version = 2;
