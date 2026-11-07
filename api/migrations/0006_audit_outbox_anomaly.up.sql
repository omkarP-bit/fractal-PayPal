CREATE TABLE audit_events (
  seq bigserial PRIMARY KEY,
  case_id uuid REFERENCES exception_cases(id),
  actor_type text NOT NULL CHECK (actor_type IN ('CUSTOMER','MERCHANT_USER','SYSTEM','AGENT','POLICY','PROVIDER')),
  actor_id text,
  event_type text NOT NULL,
  visibility text NOT NULL CHECK (visibility IN ('CUSTOMER','MERCHANT','INTERNAL')),
  data jsonb NOT NULL DEFAULT '{}',
  prev_hash text,
  event_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_audit_case ON audit_events (case_id, seq);

CREATE TABLE outbox_events (
  id bigserial PRIMARY KEY,
  case_id uuid REFERENCES exception_cases(id),
  topic text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz
);

CREATE TABLE anomaly_scores (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  case_id uuid REFERENCES exception_cases(id),
  model_id text NOT NULL,
  score double precision NOT NULL,
  features jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
