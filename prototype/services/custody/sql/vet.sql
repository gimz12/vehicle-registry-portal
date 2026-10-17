-- The emission-test centre's internal records; none of these fields reach the ledger.
CREATE TABLE IF NOT EXISTS test_record (
  record_id      TEXT PRIMARY KEY,
  vid            TEXT NOT NULL CHECK (vid ~ '^[0-9a-f]{64}$'),
  technician     TEXT,
  certificate_no TEXT,
  stored_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS test_record_vid ON test_record (vid);
