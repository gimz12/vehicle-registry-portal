-- Superseded by services/custody/sql/*.sql (one database per organisation); kept for the record.

-- Off-chain store: personal data lives here encrypted per record; only a SHA-256
-- anchor goes on-chain. No salt table: handles come from keyed PRFs. The two key
-- shares sit in separate schemas, so no single authority can de-anonymise an owner.

-- ---------------------------------------------------------------------------
-- Owner PII, encrypted per record
-- ---------------------------------------------------------------------------
CREATE TABLE owner_record (
    vid             TEXT PRIMARY KEY,          -- asset handle: HMAC(k_v, chassis)
    envelope_version SMALLINT    NOT NULL DEFAULT 1,
    ciphertext      BYTEA       NOT NULL,      -- AES-256-GCM(K_v, ownerPII, vid+version AAD)
    nonce           BYTEA       NOT NULL,
    anchor_sha256   TEXT        NOT NULL,      -- SHA-256(version||vid||nonce||ciphertext)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE owner_record IS
    'One encrypted owner record per vehicle. K_v = s_dmt XOR s_police, so combining
     both shares yields exactly one vehicle''s key — never a population-wide key.';

-- ---------------------------------------------------------------------------
-- Key shares, held in separate schemas to model separate logical custody.
-- ---------------------------------------------------------------------------
CREATE SCHEMA custody_dmt;
CREATE SCHEMA custody_police;

CREATE TABLE custody_dmt.key_share (
    vid        TEXT PRIMARY KEY REFERENCES owner_record(vid) ON DELETE CASCADE,
    share      BYTEA       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE custody_police.key_share (
    vid        TEXT PRIMARY KEY REFERENCES owner_record(vid) ON DELETE CASCADE,
    share      BYTEA       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Erasure (PDPA s.16) destroys the shares; the ciphertext becomes undecryptable
-- while the ledger record stays intact.
COMMENT ON TABLE custody_dmt.key_share IS
    'DMT half of each record key. Destroying a share is the erasure mechanism.';

-- ---------------------------------------------------------------------------
-- Read-side event index
-- ---------------------------------------------------------------------------
-- Mirror of committed ledger events. seq (block commit order) is the only
-- trustworthy ordering: Fabric transaction timestamps are client-set.
CREATE TABLE event_index (
    seq             BIGINT      PRIMARY KEY,
    tx_id           TEXT        NOT NULL UNIQUE,
    event_type      TEXT        NOT NULL,
    vid             TEXT        NOT NULL,
    pseudonym       TEXT,
    evidence_tier   SMALLINT    NOT NULL CHECK (evidence_tier BETWEEN 0 AND 2),
    attestor_id     TEXT,
    submitted_day   BIGINT,                    -- client-declared; interval arithmetic only
    odometer        BIGINT,
    idx_chassis     TEXT,
    idx_engine      TEXT,
    evidence_anchor TEXT,
    evidence_kind   TEXT,
    insurer_id      TEXT,
    payload         JSONB       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX event_index_vid_seq  ON event_index (vid, seq);
CREATE INDEX event_index_engine   ON event_index (idx_engine) WHERE idx_engine IS NOT NULL;
CREATE INDEX event_index_chassis  ON event_index (idx_chassis) WHERE idx_chassis IS NOT NULL;
CREATE INDEX event_index_attestor ON event_index (attestor_id) WHERE attestor_id IS NOT NULL;
CREATE INDEX event_index_anchor   ON event_index (evidence_anchor) WHERE evidence_anchor IS NOT NULL;

-- The attestor index supports revocation: verdicts resting on a corrupt attestor
-- are degraded at query time without mutating committed records.

-- ---------------------------------------------------------------------------
-- Reveal audit trail
-- ---------------------------------------------------------------------------
-- Mirrors the on-chain reveal log for reporting.
CREATE TABLE reveal_log (
    reveal_id    TEXT        PRIMARY KEY,
    vid          TEXT        NOT NULL,
    flag_id      TEXT,
    ground_code  TEXT        NOT NULL,
    requester    TEXT        NOT NULL,
    granted      BOOLEAN     NOT NULL,
    reason       TEXT,                          -- why refused, when not granted
    block_seq    BIGINT      NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX reveal_log_vid ON reveal_log (vid);
