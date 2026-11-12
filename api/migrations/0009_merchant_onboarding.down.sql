DROP INDEX IF EXISTS ix_provider_accounts_merchant;

ALTER TABLE provider_accounts
  DROP COLUMN IF EXISTS created_at,
  DROP COLUMN IF EXISTS last_checked_at,
  DROP COLUMN IF EXISTS webhook_error,
  DROP COLUMN IF EXISTS webhook_registered_at,
  DROP COLUMN IF EXISTS webhook_status,
  DROP COLUMN IF EXISTS webhook_url,
  DROP COLUMN IF EXISTS encrypted_secret,
  DROP COLUMN IF EXISTS client_id,
  DROP COLUMN IF EXISTS updated_at;

ALTER TABLE merchants
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS onboarding_status,
  DROP COLUMN IF EXISTS owner_user_id,
  DROP COLUMN IF EXISTS currency;

ALTER TABLE users
  DROP COLUMN IF EXISTS password_hash;
