//go:build mintclaim

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run -tags mintclaim ./scripts/mintclaim <order_ref> [email] [merchant_id]")
		os.Exit(1)
	}
	orderRef := os.Args[1]
	email := "buyer@example.com"
	if len(os.Args) > 2 {
		email = os.Args[2]
	}
	merchantID := "00000000-0000-0000-0000-0000000000c1"
	if len(os.Args) > 3 {
		merchantID = os.Args[3]
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Println("DATABASE_URL not set")
		os.Exit(1)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		fmt.Println("connect db:", err)
		os.Exit(1)
	}
	defer pool.Close()

	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		fmt.Println("random:", err)
		os.Exit(1)
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))

	tag, err := pool.Exec(context.Background(), `
		INSERT INTO order_claim_tokens
			(merchant_id, merchant_order_ref, customer_email, token_hash, expires_at)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (SELECT 1 FROM merchants WHERE id = $1)
		ON CONFLICT (merchant_id, merchant_order_ref, token_hash)
		DO UPDATE SET expires_at = EXCLUDED.expires_at, consumed_at = NULL`,
		merchantID, orderRef, email, hex.EncodeToString(sum[:]), time.Now().UTC().Add(24*time.Hour))
	if err != nil {
		fmt.Println("insert claim token:", err)
		os.Exit(1)
	}
	if tag.RowsAffected() == 0 {
		fmt.Println("merchant not found:", merchantID)
		os.Exit(1)
	}

	fmt.Printf("order_ref=%s\nemail=%s\nclaim_code=%s\n", orderRef, email, token)
}
