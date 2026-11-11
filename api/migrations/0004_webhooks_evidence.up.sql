CREATE TABLE IF NOT EXISTS webhook_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider text NOT NULL,
  provider_event_id text NOT NULL,
  provider_account_id uuid REFERENCES provider_accounts(id),
  event_type text NOT NULL,
  resource_id text,
  signature_verified boolean NOT NULL CHECK (signature_verified),
  payload jsonb NOT NULL,
  payload_sha256 text NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  processing_status text NOT NULL DEFAULT 'RECEIVED'
    CHECK (processing_status IN ('RECEIVED','PROCESSED','FAILED')),
  processed_at timestamptz,
  UNIQUE (provider, provider_event_id)
);
CREATE INDEX IF NOT EXISTS ix_webhook_resource ON webhook_events (provider, resource_id);

CREATE TABLE IF NOT EXISTS security_events (
  id bigserial PRIMARY KEY,
  event_type text NOT NULL,
  provider text,
  source_ip inet,
  payload_sha256 text,
  detail jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS case_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  evidence_type text NOT NULL CHECK (evidence_type IN
    ('PROVIDER_TRANSACTION','ORDER_SNAPSHOT','WEBHOOK_EVENT','TELEMETRY_LOG','TELEMETRY_TRACE','CUSTOMER_UPLOAD')),
  source_system text NOT NULL CHECK (source_system IN ('PAYPAL_API','MERCHANT_API','FRACTAL_DB','ELASTIC','CUSTOMER')),
  source_ref text NOT NULL,
  fetched_at timestamptz NOT NULL,
  payload jsonb NOT NULL,
  payload_sha256 text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_evidence_case ON case_evidence (case_id, fetched_at);
