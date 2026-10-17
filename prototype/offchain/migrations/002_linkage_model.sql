-- Upgrade an existing development volume created before the corrected linkage
-- model. New databases already receive these fields through schema.sql.
ALTER TABLE owner_record
    ADD COLUMN IF NOT EXISTS envelope_version SMALLINT NOT NULL DEFAULT 1;

ALTER TABLE event_index
    ADD COLUMN IF NOT EXISTS idx_chassis TEXT,
    ADD COLUMN IF NOT EXISTS evidence_kind TEXT;

CREATE INDEX IF NOT EXISTS event_index_chassis
    ON event_index (idx_chassis) WHERE idx_chassis IS NOT NULL;
