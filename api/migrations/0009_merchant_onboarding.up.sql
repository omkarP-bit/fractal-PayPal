-- Merchant self-service onboarding and per-merchant PayPal credentials.
--
-- One merchant signs up, chooses a region (which fixes the transaction
-- currency), pastes PayPal sandbox credentials, and FRACTAL registers a
-- per-merchant webhook URL. Secrets are stored encrypted; the plaintext is
-- never written to the database, logs or API responses.

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS password_hash text;

ALTER TABLE merchants
  ADD COLUMN IF NOT EXISTS currency char(3),
  ADD COLUMN IF NOT EXISTS owner_user_id uuid REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS onboarding_status text NOT NULL DEFAULT 'PENDING_PAYPAL'
    CHECK (onboarding_status IN ('PENDING_PAYPAL','ACTIVE','SUSPENDED')),
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE merchants
  ALTER COLUMN integration_base_url SET DEFAULT '',
  ALTER COLUMN integration_key_ref SET DEFAULT '';

-- Existing seeded merchants predate the currency column. Backfill the known
-- regions so the NOT NULL constraint can be applied without guessing silently.
UPDATE merchants SET currency = CASE upper(trim(country))
  WHEN 'US' THEN 'USD'
  WHEN 'IN' THEN 'USD' -- PayPal India is cross-border only; export pricing in USD.
  WHEN 'GB' THEN 'GBP'
  WHEN 'CA' THEN 'CAD'
  WHEN 'AU' THEN 'AUD'
  WHEN 'NZ' THEN 'NZD'
  WHEN 'JP' THEN 'JPY'
  WHEN 'SG' THEN 'SGD'
  WHEN 'CH' THEN 'CHF'
  WHEN 'SE' THEN 'SEK'
  WHEN 'NO' THEN 'NOK'
  WHEN 'DK' THEN 'DKK'
  WHEN 'PL' THEN 'PLN'
  WHEN 'CZ' THEN 'CZK'
  WHEN 'HU' THEN 'HUF'
  WHEN 'IL' THEN 'ILS'
  WHEN 'MX' THEN 'MXN'
  WHEN 'BR' THEN 'BRL'
  WHEN 'ZA' THEN 'ZAR'
  WHEN 'AE' THEN 'AED'
  WHEN 'HK' THEN 'HKD'
  WHEN 'MY' THEN 'MYR'
  WHEN 'PH' THEN 'PHP'
  WHEN 'TH' THEN 'THB'
  ELSE 'USD'
END
WHERE currency IS NULL;

ALTER TABLE merchants
  ALTER COLUMN currency SET NOT NULL;

ALTER TABLE provider_accounts
  ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS client_id text,
  ADD COLUMN IF NOT EXISTS encrypted_secret text,
  ADD COLUMN IF NOT EXISTS webhook_url text,
  ADD COLUMN IF NOT EXISTS webhook_status text NOT NULL DEFAULT 'PENDING'
    CHECK (webhook_status IN ('PENDING','REGISTERING','REGISTERED','FAILED','DISABLED')),
  ADD COLUMN IF NOT EXISTS webhook_registered_at timestamptz,
  ADD COLUMN IF NOT EXISTS webhook_error text,
  ADD COLUMN IF NOT EXISTS last_checked_at timestamptz,
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS ix_provider_accounts_merchant
  ON provider_accounts (merchant_id, provider, status);

-- Complete any provider account that was created before webhook registration
-- existed. It becomes usable as soon as a webhook ID is stored.
UPDATE provider_accounts
SET webhook_status = 'REGISTERED', webhook_registered_at = COALESCE(webhook_registered_at, now())
WHERE webhook_id IS NOT NULL AND webhook_status = 'PENDING';
