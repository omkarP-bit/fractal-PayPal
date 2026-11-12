DROP INDEX IF EXISTS ix_audit_incident;
ALTER TABLE audit_events DROP COLUMN IF EXISTS incident_id;

ALTER TABLE incidents
  DROP COLUMN IF EXISTS draft_type,
  DROP COLUMN IF EXISTS open_case_count;

ALTER TABLE case_messages
  DROP COLUMN IF EXISTS draft_id;

ALTER TABLE message_drafts DROP COLUMN IF EXISTS guardrail_flag;

DROP INDEX IF EXISTS ix_message_drafts_case;
DROP INDEX IF EXISTS ix_message_drafts_incident;
DROP INDEX IF EXISTS uq_active_draft_per_case;
DROP TABLE IF EXISTS message_drafts;

DROP INDEX IF EXISTS ix_incident_cases_incident;
DROP INDEX IF EXISTS ix_incident_cases_case;
DROP TABLE IF EXISTS incident_cases;

DROP INDEX IF EXISTS ix_incidents_fingerprint;
DROP INDEX IF EXISTS ix_incidents_merchant_status;
DROP TABLE IF EXISTS incidents;

DROP SEQUENCE IF EXISTS incident_number_seq;

ALTER TABLE case_messages
  DROP COLUMN IF EXISTS sent_at,
  DROP COLUMN IF EXISTS approved_by,
  DROP COLUMN IF EXISTS ai_generated;
