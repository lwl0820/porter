CREATE TABLE plans (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK(length(trim(name)) > 0),
    port_count INTEGER NOT NULL CHECK(port_count BETWEEN 1 AND 65535),
    traffic_limit_bytes INTEGER NOT NULL CHECK(traffic_limit_bytes > 0),
    accounting_mode TEXT NOT NULL CHECK(accounting_mode IN ('both','max','upload','download')),
    reset_mode TEXT NOT NULL CHECK(reset_mode IN ('calendar_month','anniversary','fixed_day')),
    reset_day INTEGER,
    created_at TEXT NOT NULL,
    CHECK((reset_mode='fixed_day' AND reset_day IS NOT NULL AND reset_day BETWEEN 1 AND 31)
       OR (reset_mode IN ('calendar_month','anniversary') AND reset_day IS NULL))
);
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL CHECK(role IN ('admin','user')),
    password_hash TEXT NOT NULL DEFAULT '',
    plan_id TEXT REFERENCES plans(id),
    port_start INTEGER NOT NULL DEFAULT 0,
    port_end INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    CHECK((role='admin' AND id='system-admin' AND plan_id IS NULL AND port_start=0 AND port_end=0 AND password_hash='')
       OR (role='user' AND id<>'system-admin' AND length(trim(username))>0 AND plan_id IS NOT NULL AND port_start BETWEEN 1 AND 65535 AND port_end BETWEEN port_start AND 65535 AND length(password_hash)>0))
);
INSERT INTO users(id,username,role,created_at) VALUES('system-admin','','admin',strftime('%Y-%m-%dT%H:%M:%fZ','now'));
CREATE TABLE system_settings (
    id INTEGER PRIMARY KEY CHECK(id=1),
    port_start INTEGER NOT NULL DEFAULT 0,
    port_end INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL DEFAULT 1 CHECK(revision>0),
    updated_at TEXT NOT NULL,
    CHECK((port_start=0 AND port_end=0) OR (port_start BETWEEN 1 AND 65535 AND port_end BETWEEN port_start AND 65535))
);
INSERT INTO system_settings(id,updated_at) VALUES(1,strftime('%Y-%m-%dT%H:%M:%fZ','now'));
CREATE TABLE user_ports (
    port INTEGER PRIMARY KEY CHECK(port BETWEEN 1 AND 65535),
    user_id TEXT NOT NULL REFERENCES users(id)
);
CREATE INDEX user_ports_user ON user_ports(user_id);
CREATE TABLE user_usage (
    user_id TEXT PRIMARY KEY REFERENCES users(id),
    cycle_start TEXT NOT NULL,
    cycle_end TEXT NOT NULL,
    upload_bytes INTEGER NOT NULL DEFAULT 0 CHECK(upload_bytes>=0),
    download_bytes INTEGER NOT NULL DEFAULT 0 CHECK(download_bytes>=0),
    version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),
    CHECK(cycle_end>cycle_start)
);
ALTER TABLE rules ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'system-admin';
CREATE INDEX rules_owner ON rules(owner_id);
CREATE TRIGGER rules_owner_insert BEFORE INSERT ON rules
WHEN NOT EXISTS(SELECT 1 FROM users WHERE id=NEW.owner_id)
BEGIN SELECT RAISE(ABORT,'规则所属用户不存在'); END;
CREATE TRIGGER rules_owner_update BEFORE UPDATE OF owner_id ON rules
WHEN NOT EXISTS(SELECT 1 FROM users WHERE id=NEW.owner_id)
BEGIN SELECT RAISE(ABORT,'规则所属用户不存在'); END;
PRAGMA user_version = 4;
