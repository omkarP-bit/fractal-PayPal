-- AI Case Assistant: incident grouping, triage and drafted replies.
--
-- Incidents are merchant-scoped groupings of cases that share verified facts.
-- Drafts are generated from those facts, never from free-form model claims.
-- Messages sent from a draft keep the AI-assisted label and the approving user.

CREATE SEQUENCE IF NOT EXISTS incident_number_seq START 5000;

CREATE TABLE IF NOT EXISTS incidents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  incident_number text NOT NULL UNIQUE DEFAULT ('INC-' || nextval('incident_number_seq')),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  provider_account_id uuid REFERENCES provider_accounts(id),
  fingerprint text NOT NULL,
  root_cause_code text NOT NULL,
  root_cause_summary text NOT NULL,
  ai_summary text NOT NULL,
  next_step text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','INVESTIGATING','RESOLVED','CLOSED')),
  severity text NOT NULL DEFAULT 'MEDIUM' CHECK (severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
  confidence double precision NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
  priority_score int NOT NULL DEFAULT 0,
  case_count int NOT NULL DEFAULT 0,
  affected_amount_minor bigint NOT NULL DEFAULT 0,
  currency text NOT NULL DEFAULT '',
  first_seen_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  resolved_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_incidents_merchant_status
  ON incidents (merchant_id, status, priority_score DESC, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS ix_incidents_fingerprint
  ON incidents (merchant_id, fingerprint, last_seen_at DESC);

CREATE TABLE IF NOT EXISTS incident_cases (
  incident_id uuid NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  added_by text NOT NULL DEFAULT 'AI' CHECK (added_by IN ('AI','MERCHANT','SYSTEM')),
  confidence double precision NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (incident_id, case_id)
);
CREATE INDEX IF NOT EXISTS ix_incident_cases_case ON incident_cases (case_id);
CREATE INDEX IF NOT EXISTS ix_incident_cases_incident ON incident_cases (incident_id);

CREATE TABLE IF NOT EXISTS message_drafts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  incident_id uuid REFERENCES incidents(id) ON DELETE SET NULL,
  draft_type text NOT NULL DEFAULT 'STATUS_UPDATE'
    CHECK (draft_type IN ('STATUS_UPDATE','INFORMATION_REQUEST','RESOLUTION_NOTICE')),
  body text NOT NULL,
  status text NOT NULL DEFAULT 'DRAFTED' CHECK (status IN ('DRAFTED','APPROVED','SENT','DISCARDED')),
  evidence_refs uuid[] NOT NULL DEFAULT '{}',
  ai_generated boolean NOT NULL DEFAULT true,
  approved_by uuid REFERENCES users(id),
  approved_at timestamptz,
  sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_message_drafts_incident
  ON message_drafts (incident_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_message_drafts_case
  ON message_drafts (case_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_active_draft_per_case
  ON message_drafts (case_id, COALESCE(incident_id, '00000000-0000-0000-0000-000000000000'::uuid), draft_type)
  WHERE status IN ('DRAFTED','APPROVED');

ALTER TABLE case_messages
  ADD COLUMN IF NOT EXISTS ai_generated boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS approved_by uuid REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS sent_at timestamptz;

ALTER TABLE message_drafts
  ADD COLUMN IF NOT EXISTS guardrail_flag text NOT NULL DEFAULT '';

ALTER TABLE case_messages
  ADD COLUMN IF NOT EXISTS draft_id uuid REFERENCES message_drafts(id);

ALTER TABLE incidents
  ADD COLUMN IF NOT EXISTS open_case_count int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS draft_type text NOT NULL DEFAULT 'STATUS_UPDATE';

ALTER TABLE audit_events
  ADD COLUMN IF NOT EXISTS incident_id uuid REFERENCES incidents(id);

CREATE INDEX IF NOT EXISTS ix_audit_incident ON audit_events (incident_id, seq);
