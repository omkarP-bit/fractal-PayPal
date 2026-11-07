//go:build dev

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: go run -tags dev ./scripts/replay <event_id> <receiver_url>")
		os.Exit(1)
	}
	eventID := os.Args[1]
	receiverURL := os.Args[2]

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

	var payload []byte
	err = pool.QueryRow(context.Background(), `SELECT payload FROM webhook_events WHERE provider_event_id = $1`, eventID).Scan(&payload)
	if err != nil {
		fmt.Println("fetch event:", err)
		os.Exit(1)
	}

	var pretty map[string]any
	_ = json.Unmarshal(payload, &pretty)
	_ = pretty

	resp, err := http.Post(receiverURL, "application/json", io.NopCloser(bytesReader(payload)))
	if err != nil {
		fmt.Println("post:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("status=%d body=%s\n", resp.StatusCode, string(body))
}

func bytesReader(b []byte) *stringReader { return &stringReader{s: string(b), i: 0} }

type stringReader struct {
	s string
	i int
}

func (r *stringReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
