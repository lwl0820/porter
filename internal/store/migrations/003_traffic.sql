ALTER TABLE rules ADD COLUMN traffic_enabled INTEGER NOT NULL DEFAULT 0 CHECK (traffic_enabled IN (0, 1));
CREATE TABLE rule_traffic (
    rule_id TEXT PRIMARY KEY REFERENCES rules(id) ON DELETE CASCADE,
    upload_bytes INTEGER NOT NULL DEFAULT 0 CHECK (upload_bytes >= 0),
    download_bytes INTEGER NOT NULL DEFAULT 0 CHECK (download_bytes >= 0)
);
PRAGMA user_version = 3;
