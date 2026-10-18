-- Customs' internal import records; none of these fields reach the ledger.
CREATE TABLE IF NOT EXISTS import_record (
  record_id      TEXT PRIMARY KEY,
  vid            TEXT NOT NULL CHECK (vid ~ '^[0-9a-f]{64}$'),
  declarant      TEXT,
  cusdec_no      TEXT,
  valuation_lkr  NUMERIC(14, 2),
  stored_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS import_record_vid ON import_record (vid);
