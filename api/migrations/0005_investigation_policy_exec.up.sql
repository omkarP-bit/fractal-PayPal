CREATE TABLE IF NOT EXISTS investigations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  run_number int NOT NULL,
  agent_version text NOT NULL,
  model_id text NOT NULL,
  findings jsonb NOT NULL,
  recommendation text,
  suggested_severity text CHECK (suggested_severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
  evidence_ids uuid[] NOT NULL DEFAULT '{}',
  validation_status text NOT NULL CHECK (validation_status IN ('SCHEMA_VALID','REFS_VERIFIED','REJECTED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (case_id, run_number)
);

CREATE TABLE IF NOT EXISTS policy_bundles (
  revision text PRIMARY KEY,
  bundle_sha256 text NOT NULL,
  status text NOT NULL CHECK (status IN ('STAGED','ACTIVE','RETIRED')),
  activated_at timestamptz,
  activated_by uuid REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS policy_decisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  evaluation_id uuid NOT NULL UNIQUE,
  action_type text NOT NULL,
  params_sha256 text NOT NULL,
  input jsonb NOT NULL,
  input_sha256 text NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('ALLOW','REQUIRE_APPROVAL','WAIT','BLOCK')),
  findings jsonb NOT NULL,
  policy_revision text NOT NULL,
  evaluation_error text,
  requested_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_decisions_case ON policy_decisions (case_id, created_at);

CREATE TABLE IF NOT EXISTS approvals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  policy_decision_id uuid NOT NULL REFERENCES policy_decisions(id),
  action_type text NOT NULL,
  params_sha256 text NOT NULL,
  requested_role text NOT NULL,
  approver_id uuid REFERENCES users(id),
  decision text NOT NULL DEFAULT 'PENDING' CHECK (decision IN ('PENDING','APPROVED','REJECTED')),
  reason text,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  decided_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS action_executions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  policy_decision_id uuid NOT NULL REFERENCES policy_decisions(id),
  action_type text NOT NULL,
  params jsonb NOT NULL,
  params_sha256 text NOT NULL,
  idempotency_key text NOT NULL UNIQUE,
  status text NOT NULL DEFAULT 'IN_PROGRESS' CHECK (status IN ('IN_PROGRESS','SUCCEEDED','FAILED')),
  response jsonb,
  error text,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_action_once ON action_executions (case_id, action_type, params_sha256)
  WHERE status IN ('IN_PROGRESS','SUCCEEDED');
