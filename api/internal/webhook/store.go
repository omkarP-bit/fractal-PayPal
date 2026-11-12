package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"fractal/api/internal/provider"
)

// PGStore persists webhook data in PostgreSQL through database/sql.
// Register a driver in main (for example pgx's stdlib package).
type PGStore struct{ DB *sql.DB }

// InsertVerified relies on UNIQUE (provider, provider_event_id): a replay inserts
// nothing and RETURNING yields no row.
func (s *PGStore) InsertVerified(ctx context.Context, accountID string, ev Event, raw []byte, sha string) (bool, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO webhook_events
		  (provider, provider_event_id, provider_account_id, event_type, resource_id,
		   signature_verified, payload, payload_sha256)
		VALUES ('PAYPAL', $1, $2, $3, NULLIF($4, ''), true, $5::jsonb, $6)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
		RETURNING id`,
		ev.ID, accountID, ev.EventType, ev.Resource.ID, string(raw), sha,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // duplicate
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RecordSecurityEvent stores rejected deliveries. They never enter webhook_events.
func (s *PGStore) RecordSecurityEvent(ctx context.Context, se SecurityEvent) error {
	detail, err := json.Marshal(se.Detail)
	if err != nil {
		detail = []byte("{}")
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO security_events (event_type, provider, source_ip, payload_sha256, detail)
		VALUES ($1, $2, NULLIF($3::text, '')::inet, $4, $5::jsonb)`,
		se.Type, se.Provider, se.SourceIP, se.PayloadSHA256, string(detail))
	return err
}

// AccountCredentials is the per-merchant verification context. The secret is
// decrypted in memory only and never leaves this struct or reaches a logger.
type AccountCredentials struct {
	WebhookID    string
	ClientID     string
	ClientSecret string
	BaseURL      string
	Environment  string
}

// PGAccounts resolves the webhook ID and, when onboarding has stored them, the
// per-account PayPal credentials.
type PGAccounts struct {
	DB *sql.DB
	// Decrypt opens provider_accounts.encrypted_secret. It is optional so the
	// legacy single-account deployment keeps working from .env.
	Decrypt func(string) (string, error)
}

func (a *PGAccounts) WebhookID(ctx context.Context, accountID string) (string, error) {
	account, err := a.Credentials(ctx, accountID)
	if err != nil {
		return "", err
	}
	return account.WebhookID, nil
}

// Credentials loads the account row and falls back to the legacy env
// credentials for accounts that predate merchant onboarding.
func (a *PGAccounts) Credentials(ctx context.Context, accountID string) (AccountCredentials, error) {
	var (
		webhookID   sql.NullString
		clientID    sql.NullString
		sealed      sql.NullString
		environment string
	)
	err := a.DB.QueryRowContext(ctx, `
		SELECT webhook_id, client_id, encrypted_secret, environment
		FROM provider_accounts
		WHERE id = $1 AND provider = 'PAYPAL' AND status = 'ACTIVE'`, accountID).
		Scan(&webhookID, &clientID, &sealed, &environment)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountCredentials{}, ErrAccountNotFound
	}
	if err != nil {
		return AccountCredentials{}, err
	}

	account := AccountCredentials{
		WebhookID:   webhookID.String,
		ClientID:    clientID.String,
		Environment: environment,
		BaseURL:     provider.BaseURLForEnvironment(environment),
	}
	if account.WebhookID == "" {
		account.WebhookID = envWebhookID()
	}

	if account.ClientID == "" || sealed.String == "" {
		// Legacy account: the deployment-level app credentials still apply.
		account.ClientID = os.Getenv("PAYPAL_CLIENT_ID")
		account.ClientSecret = os.Getenv("PAYPAL_CLIENT_SECRET")
		if base := os.Getenv("PAYPAL_BASE_URL"); base != "" {
			account.BaseURL = base
		}
	} else {
		if a.Decrypt == nil {
			return AccountCredentials{}, errors.New("provider account has an encrypted secret but no decryption key")
		}
		secret, err := a.Decrypt(sealed.String)
		if err != nil {
			return AccountCredentials{}, fmt.Errorf("decrypt provider account secret: %w", err)
		}
		account.ClientSecret = secret
	}

	if account.WebhookID == "" {
		return AccountCredentials{}, ErrAccountNotFound
	}
	if account.ClientID == "" || account.ClientSecret == "" {
		return AccountCredentials{}, errors.New("provider account has no usable PayPal credentials")
	}
	return account, nil
}

func envWebhookID() string {
	if envID := os.Getenv("PAYPAL_WEBHOOK_ID"); envID != "" && envID != "your_sandbox_webhook_id" {
		return envID
	}
	return ""
}
