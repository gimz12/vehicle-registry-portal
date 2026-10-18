-- The insurers' internal claim files; none of these fields reach the ledger.
CREATE TABLE IF NOT EXISTS claim_record (
  record_id        TEXT PRIMARY KEY,
  vid              TEXT NOT NULL CHECK (vid ~ '^[0-9a-f]{64}$'),
  policy_no        TEXT,
  claim_file       TEXT,
  assessor_contact TEXT,
  reserve_lkr      NUMERIC(14, 2),
  stored_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS claim_record_vid ON claim_record (vid);
