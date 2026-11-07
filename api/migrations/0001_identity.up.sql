CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email citext NOT NULL UNIQUE,
  display_name text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('CUSTOMER','MERCHANT_USER','PLATFORM_ADMIN','SERVICE')),
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED')),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE merchants (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  legal_name text NOT NULL,
  country char(2) NOT NULL,
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED')),
  integration_base_url text NOT NULL,
  integration_key_ref text NOT NULL,
  policy_config jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE merchant_members (
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  user_id uuid NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('MERCHANT_ADMIN','MERCHANT_STAFF')),
  permissions text[] NOT NULL DEFAULT '{}',
  PRIMARY KEY (merchant_id, user_id)
);

CREATE TABLE provider_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  provider text NOT NULL CHECK (provider IN ('PAYPAL')),
  environment text NOT NULL CHECK (environment IN ('SANDBOX','LIVE')),
  external_account_id text NOT NULL,
  webhook_id text,
  credentials_ref text NOT NULL,
  status text NOT NULL DEFAULT 'ACTIVE',
  UNIQUE (provider, environment, external_account_id)
);
