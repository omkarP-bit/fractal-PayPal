CREATE TABLE IF NOT EXISTS payment_transactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  provider_account_id uuid NOT NULL REFERENCES provider_accounts(id),
  provider text NOT NULL,
  provider_order_id text NOT NULL,
  capture_id text,
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency char(3) NOT NULL,
  provider_status text NOT NULL,
  merchant_order_ref text,
  last_fetched_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (provider, provider_order_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_capture ON payment_transactions (provider, capture_id) WHERE capture_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS order_snapshots (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  merchant_order_ref text NOT NULL,
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency char(3) NOT NULL,
  order_status text NOT NULL,
  fetched_at timestamptz NOT NULL,
  payload_sha256 text NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_order_snapshots_latest ON order_snapshots (merchant_id, merchant_order_ref, fetched_at DESC);

CREATE TABLE IF NOT EXISTS order_claim_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  merchant_order_ref text NOT NULL,
  customer_email citext NOT NULL,
  token_hash text NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  UNIQUE (merchant_id, merchant_order_ref, token_hash)
);
