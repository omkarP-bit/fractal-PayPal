package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
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

// PGAccounts resolves the webhook ID registered for a provider account.
type PGAccounts struct{ DB *sql.DB }

func (a *PGAccounts) WebhookID(ctx context.Context, accountID string) (string, error) {
	var id sql.NullString
	err := a.DB.QueryRowContext(ctx, `
		SELECT webhook_id FROM provider_accounts
		WHERE id = $1 AND provider = 'PAYPAL' AND status = 'ACTIVE'`, accountID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAccountNotFound
	}
	if err != nil {
		return "", err
	}
	// Prefer the DB-stored webhook ID; fall back to PAYPAL_WEBHOOK_ID from .env.
	if id.Valid && id.String != "" {
		return id.String, nil
	}
	if envID := os.Getenv("PAYPAL_WEBHOOK_ID"); envID != "" && envID != "your_sandbox_webhook_id" {
		return envID, nil
	}
	return "", ErrAccountNotFound
}
