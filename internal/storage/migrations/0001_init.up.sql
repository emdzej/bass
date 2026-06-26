CREATE TABLE apps (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL,
  origins       TEXT NOT NULL,             -- JSON array of allowed origins
  redirect_uris TEXT NOT NULL,             -- JSON array of allowed redirect URIs
  key_allowlist TEXT NOT NULL,             -- JSON array of glob patterns
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);

CREATE TABLE devices (
  id              TEXT PRIMARY KEY,
  user_sub        TEXT NOT NULL,
  app_id          TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  label           TEXT,
  sync_token_hash TEXT NOT NULL,
  refresh_hash    TEXT NOT NULL,
  token_expires   INTEGER NOT NULL,
  refresh_expires INTEGER NOT NULL,
  created_at      INTEGER NOT NULL,
  last_seen_at    INTEGER NOT NULL,
  revoked_at      INTEGER
);
CREATE INDEX idx_devices_user_app ON devices(user_sub, app_id);
CREATE INDEX idx_devices_sync_hash ON devices(sync_token_hash) WHERE revoked_at IS NULL;
CREATE INDEX idx_devices_refresh_hash ON devices(refresh_hash) WHERE revoked_at IS NULL;

CREATE TABLE items (
  user_sub    TEXT NOT NULL,
  app_id      TEXT NOT NULL,
  key         TEXT NOT NULL,
  value       BLOB,
  payload_ver INTEGER NOT NULL DEFAULT 1,
  version     INTEGER NOT NULL,
  deleted     INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL,
  updated_by  TEXT NOT NULL,
  PRIMARY KEY (user_sub, app_id, key)
);
CREATE INDEX idx_items_version ON items(user_sub, app_id, version);

CREATE TABLE version_counters (
  user_sub TEXT NOT NULL,
  app_id   TEXT NOT NULL,
  version  INTEGER NOT NULL,
  PRIMARY KEY (user_sub, app_id)
);
