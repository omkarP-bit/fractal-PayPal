package webapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"fractal/api/internal/evidence"
	"fractal/api/internal/provider"
)

// providerFetcherForCase returns the PayPal client belonging to the merchant
// that owns the case. Legacy deployments fall back to the shared client when the
// account has no stored credentials.
func (h *Handlers) providerFetcherForCase(ctx context.Context, caseID string) (evidence.ProviderFetcher, error) {
	if h.PayPal == nil {
		return nil, errors.New("paypal client is not configured")
	}

	var (
		environment string
		clientID    sql.NullString
		sealed      sql.NullString
	)
	err := h.DB.QueryRow(ctx, `
		SELECT pa.environment, pa.client_id, pa.encrypted_secret
		FROM exception_cases c
		JOIN provider_accounts pa ON pa.merchant_id = c.merchant_id
		WHERE c.id = $1 AND pa.provider = 'PAYPAL' AND pa.status = 'ACTIVE'
		ORDER BY pa.created_at ASC
		LIMIT 1`, caseID).Scan(&environment, &clientID, &sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return h.PayPal, nil
	}
	if err != nil {
		return nil, err
	}
	if !clientID.Valid || !sealed.Valid || clientID.String == "" || sealed.String == "" {
		return h.PayPal, nil
	}
	if h.Secrets == nil {
		return nil, errors.New("provider credentials are encrypted but no decryption key is configured")
	}
	secret, err := h.Secrets.Decrypt(sealed.String)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider credentials: %w", err)
	}
	return provider.NewPayPal(provider.BaseURLForEnvironment(environment), clientID.String, secret, h.Log), nil
}
