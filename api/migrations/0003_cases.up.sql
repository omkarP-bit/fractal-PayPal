DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relkind = 'S' AND relname = 'case_number_seq') THEN
    CREATE SEQUENCE case_number_seq START 10000;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS exception_cases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_number text NOT NULL UNIQUE DEFAULT ('FR-' || nextval('case_number_seq')),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  customer_id uuid NOT NULL REFERENCES users(id),
  payment_transaction_id uuid REFERENCES payment_transactions(id),
  merchant_order_ref text NOT NULL,
  case_type text NOT NULL CHECK (case_type IN
    ('PROVIDER_MERCHANT_STATE_MISMATCH','AMOUNT_MISMATCH','DUPLICATE_CHARGE','OTHER')),
  status text NOT NULL DEFAULT 'OPEN' CHECK (status IN
    ('OPEN','COLLECTING_EVIDENCE','INVESTIGATING','AWAITING_MERCHANT_APPROVAL',
     'AWAITING_CUSTOMER_EVIDENCE','ACTION_PENDING','RESOLVED','ESCALATED','REJECTED','CLOSED')),
  opened_via text NOT NULL CHECK (opened_via IN ('CUSTOMER','MERCHANT','AUTO_RECONCILER')),
  resolution_code text,
  version int NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_live_case ON exception_cases (payment_transaction_id, case_type)
  WHERE status NOT IN ('RESOLVED','REJECTED','CLOSED');
CREATE INDEX IF NOT EXISTS ix_cases_merchant ON exception_cases (merchant_id, status);
CREATE INDEX IF NOT EXISTS ix_cases_customer ON exception_cases (customer_id, status);

CREATE TABLE IF NOT EXISTS case_messages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  author_id uuid REFERENCES users(id),
  author_type text NOT NULL CHECK (author_type IN ('CUSTOMER','MERCHANT','SYSTEM','AGENT')),
  visibility text NOT NULL DEFAULT 'SHARED' CHECK (visibility IN ('SHARED','MERCHANT_ONLY','INTERNAL')),
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_messages_case ON case_messages (case_id, created_at);
