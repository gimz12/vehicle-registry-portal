-- Police custody: the Police key half only. One half alone opens nothing.
CREATE TABLE IF NOT EXISTS key_share (
  vid       TEXT PRIMARY KEY CHECK (vid ~ '^[0-9a-f]{64}$'),
  share     BYTEA NOT NULL CHECK (octet_length(share) = 32),
  stored_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- The owner record this half belongs to, checked against the case's owner anchor at release.
ALTER TABLE key_share ADD COLUMN IF NOT EXISTS anchor_sha256 TEXT CHECK (anchor_sha256 ~ '^[0-9a-f]{64}$');
CREATE TABLE IF NOT EXISTS release_log (
  id      BIGSERIAL PRIMARY KEY,
  at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  item    TEXT NOT NULL,
  vid     TEXT NOT NULL,
  case_id TEXT,
  outcome TEXT NOT NULL
);
