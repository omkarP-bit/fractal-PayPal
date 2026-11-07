//go:build seed

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.Default()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		logger.Error("DATABASE_URL not set")
		os.Exit(1)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		logger.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// users
	must := func(sql string, args ...any) {
		if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
			logger.Error("seed sql", "sql", sql, "err", err)
			os.Exit(1)
		}
	}

	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	tokenHash := hex.EncodeToString(randBytes)

	must(`INSERT INTO users (email, display_name, kind) VALUES
		('buyer@example.com', 'Buyer Customer', 'CUSTOMER'),
		('ops@acme.com', 'Ops Merchant Admin', 'MERCHANT_USER'),
		('staff@acme.com', 'Staff Merchant User', 'MERCHANT_USER'),
		('admin@fractal.dev', 'Platform Admin', 'PLATFORM_ADMIN')
		ON CONFLICT (email) DO NOTHING`)

	must(`INSERT INTO merchants (legal_name, country, integration_base_url, integration_key_ref)
		VALUES ('Acme Store', 'US', 'http://merchant-store:8090', 'ref:acme:dev-key')
		ON CONFLICT (id) DO NOTHING`)

	must(`INSERT INTO provider_accounts (merchant_id, provider, environment, external_account_id, credentials_ref, webhook_id)
		SELECT m.id, 'PAYPAL', 'SANDBOX', 'PAYPAL-MERCHANT-123', 'ref:paypal:sandbox-secret', 'WH-SBX-1'
		FROM merchants m WHERE m.legal_name = 'Acme Store'
		ON CONFLICT (provider, environment, external_account_id) DO NOTHING`)

	logger.Info("seed complete", "claim_token_hash", tokenHash)
}
