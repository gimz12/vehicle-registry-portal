-- DMT custody: sealed owner records, the DMT key half and the owner index.
-- Runs at service start; idempotent.
CREATE TABLE IF NOT EXISTS owner_record (
  vid              TEXT PRIMARY KEY CHECK (vid ~ '^[0-9a-f]{64}$'),
  envelope_version SMALLINT NOT NULL,
  ciphertext       BYTEA NOT NULL,                       -- AES-256-GCM ciphertext followed by its tag
  nonce            BYTEA NOT NULL CHECK (octet_length(nonce) = 12),
  anchor_sha256    TEXT NOT NULL CHECK (anchor_sha256 ~ '^[0-9a-f]{64}$'),
  stored_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS key_share (
  vid   TEXT PRIMARY KEY REFERENCES owner_record(vid) ON DELETE CASCADE,
  share BYTEA NOT NULL CHECK (octet_length(share) = 32)
);
-- owner_tag = HMAC(k_own, "owner", NIC); never leaves this database.
CREATE TABLE IF NOT EXISTS owner_index (
  vid       TEXT PRIMARY KEY REFERENCES owner_record(vid) ON DELETE CASCADE,
  owner_tag TEXT NOT NULL CHECK (owner_tag ~ '^[0-9a-f]{64}$')
);
CREATE INDEX IF NOT EXISTS owner_index_tag ON owner_index (owner_tag);
-- Every release or refusal of a record, a key half or a fleet, with the case it was asked under.
CREATE TABLE IF NOT EXISTS release_log (
  id      BIGSERIAL PRIMARY KEY,
  at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  item    TEXT NOT NULL,
  vid     TEXT NOT NULL,
  case_id TEXT,
  outcome TEXT NOT NULL
);
-- Vehicle description; only its keyed anchor is on the ledger. Colour is not
-- anchored, since a car may be repainted.
CREATE TABLE IF NOT EXISTS vehicle_description (
  vid         TEXT PRIMARY KEY CHECK (vid ~ '^[0-9a-f]{64}$'),
  make        TEXT NOT NULL,
  model       TEXT NOT NULL,
  year        SMALLINT NOT NULL CHECK (year BETWEEN 1950 AND 2100),
  engine_cc   INTEGER NOT NULL CHECK (engine_cc BETWEEN 0 AND 20000),
  fuel        TEXT NOT NULL,
  colour      TEXT NOT NULL,
  desc_anchor TEXT CHECK (desc_anchor ~ '^[0-9a-f]{64}$'),
  stored_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Plate history: one open row (to_day IS NULL) per plate, so two live vehicles
-- can never carry the same plate. Rows are closed, never deleted.
CREATE TABLE IF NOT EXISTS plate_history (
  id        BIGSERIAL PRIMARY KEY,
  plate     TEXT NOT NULL CHECK (plate ~ '^[A-Z0-9]{3,12}$'),
  vid       TEXT NOT NULL CHECK (vid ~ '^[0-9a-f]{64}$'),
  from_day  INTEGER NOT NULL DEFAULT 0,
  to_day    INTEGER,
  reason    TEXT NOT NULL DEFAULT 'registration',
  stored_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS plate_open ON plate_history (plate) WHERE to_day IS NULL;
CREATE INDEX IF NOT EXISTS plate_history_vid ON plate_history (vid);
