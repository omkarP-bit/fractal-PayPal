// Command setpassword sets a password for an existing user.
//
// Seeded demo accounts have no password hash, so they are normally used with
// the local-only dev-login shortcut. A deployed demo has that shortcut
// disabled, so give the accounts you need a real password:
//
//	DATABASE_URL=postgres://... go run ./cmd/setpassword owner@example.com 'a-long-password'
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fractal/api/internal/auth"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: setpassword <email> <password>")
		os.Exit(2)
	}

	email := strings.ToLower(strings.TrimSpace(os.Args[1]))
	password := os.Args[2]

	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash password: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	tag, err := pool.Exec(ctx, "UPDATE users SET password_hash = $2 WHERE email = $1", email, hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update password: %v\n", err)
		os.Exit(1)
	}
	if tag.RowsAffected() == 0 {
		fmt.Fprintf(os.Stderr, "no user with email %q\n", email)
		os.Exit(1)
	}

	fmt.Printf("password updated for %s\n", email)
}
