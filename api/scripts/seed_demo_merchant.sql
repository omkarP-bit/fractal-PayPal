-- Demo merchant and its PayPal Sandbox provider account (follows Spec Part B).
--
-- The provider account UUID is part of FRACTAL's webhook URL, so seed first,
-- register the webhook second, then store the webhook ID:
--
--   psql "$DATABASE_URL" -f scripts/seed_demo_merchant.sql
--   ./scripts/paypal_webhooks.sh add https://<tunnel>/api/v1/webhooks/paypal/00000000-0000-0000-0000-0000000000b1
--   psql "$DATABASE_URL" -c "UPDATE provider_accounts SET webhook_id = 'WEBHOOK_ID_HERE' WHERE id = '00000000-0000-0000-0000-0000000000b1'"

INSERT INTO merchants (id, legal_name, country, currency, onboarding_status, integration_base_url, integration_key_ref)
VALUES ('00000000-0000-0000-0000-0000000000a1', 'Acme Demo Store', 'US', 'USD', 'ACTIVE',
        'http://merchant-store:8090', 'env:MERCHANT_SIGNING_SECRET')
ON CONFLICT (id) DO NOTHING;

INSERT INTO provider_accounts (id, merchant_id, provider, environment, external_account_id, webhook_id, credentials_ref)
VALUES ('00000000-0000-0000-0000-0000000000b1', '00000000-0000-0000-0000-0000000000a1',
        'PAYPAL', 'SANDBOX', 'SANDBOX-DEMO-APP', NULL, 'env:PAYPAL_CLIENT_ID')
ON CONFLICT (id) DO NOTHING;
