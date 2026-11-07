package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"fractal/api/internal/auth"
	"fractal/api/internal/cases"
	"fractal/api/internal/provider"
	"fractal/api/internal/telemetry"
	"fractal/api/internal/telemetry/endpoints"
	"fractal/api/internal/webhook"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

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

	// Share the same database via database/sql for the webhook store/account resolver.
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		logger.Error("open sql db", "err", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	caseStore := cases.NewStore(pool)
	caseHandlers := cases.NewHandlers(caseStore)

	// OpenTelemetry tracer (OTLP -> Elastic).
	if tp, err := telemetry.InitTracer(os.Getenv("OTEL_ENDPOINT")); err != nil {
		slog.Warn("tracer init failed", "err", err)
	} else {
		defer tp.Shutdown(context.Background())
	}

	// ---- PayPal provider + webhook handler ----
	clientID := os.Getenv("PAYPAL_CLIENT_ID")
	clientSecret := os.Getenv("PAYPAL_CLIENT_SECRET")
	baseURL := os.Getenv("PAYPAL_BASE_URL")
	if baseURL == "" {
		baseURL = provider.PayPalSandboxBaseURL
	}

	paypal := provider.NewPayPal(baseURL, clientID, clientSecret, logger)

	wake, wakeCh := webhook.NewWaker()
	h := &webhook.Handler{
		Provider: "PAYPAL",
		Accounts: &webhook.PGAccounts{DB: sqlDB},
		Verifier: paypal,
		Store:    &webhook.PGStore{DB: sqlDB},
		Wake:     wake,
		Log:      logger,
	}

	// Background worker: drains RECEIVED webhook_events and processes them.
	go (&webhook.Worker{
		DB:   sqlDB,
		Proc: &noopProcessor{},
		Wake: wakeCh,
		Log:  logger,
	}).Run(context.Background())

	mux := http.NewServeMux()

	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-Id")
			if reqID == "" {
				reqID = "req_" + time.Now().UTC().Format("150405.000000")
			}
			w.Header().Set("X-Request-Id", reqID)
			ctx := telemetry.WithRequestID(r.Context(), reqID)
			_, span := telemetry.StartRequestSpan(ctx, r.Method, r.URL.Path, reqID, "")
			defer telemetry.EndSpan(span, nil)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux.Handle("GET /healthz", middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
		slog.Info("health check", "request_id", telemetry.RequestIDFromCtx(r.Context()), "path", r.URL.Path)
	})))

	mux.Handle("GET /cases", middleware(auth.RequireAuth(http.HandlerFunc(caseHandlers.List))))
	mux.Handle("GET /cases/{id}", middleware(auth.RequireAuth(http.HandlerFunc(caseHandlers.Get))))
	mux.Handle("POST /cases", middleware(auth.RequireAuth(http.HandlerFunc(caseHandlers.Create))))

	mux.Handle("POST /api/v1/webhooks/paypal/{provider_account_id}", middleware(h))

	// Internal read-only telemetry endpoints (agent tools, Spec C7).
	mux.Handle("GET /internal/v1/telemetry/logs", middleware(http.HandlerFunc(endpoints.LogsByCase)))
	mux.Handle("GET /internal/v1/telemetry/traces/{trace_id}", middleware(http.HandlerFunc(endpoints.TraceByID)))
	mux.Handle("GET /internal/v1/telemetry/handler-error-rate", middleware(http.HandlerFunc(endpoints.HandlerErrorRate)))

	mux.Handle("/", middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})))

	addr := os.Getenv("FRACTAL_PORT")
	if addr == "" {
		addr = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("fractal-api listening", "port", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "err", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	slog.Info("fractal-api stopped")
}

// noopProcessor is a placeholder Processor that acknowledges events without
// side effects. Real processing (payment_transactions, evidence, cases) is
// wired in later phases.
type noopProcessor struct{}

func (p *noopProcessor) Process(ctx context.Context, ev webhook.EventRow) error {
	slog.Info("webhook event processed (noop)", "event", ev.ID, "type", ev.EventType)
	return nil
}
