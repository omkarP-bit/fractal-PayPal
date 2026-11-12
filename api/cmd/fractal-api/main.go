package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"fractal/api/internal/anomaly"
	"fractal/api/internal/auth"
	"fractal/api/internal/cases"
	"fractal/api/internal/policy"
	"fractal/api/internal/provider"
	"fractal/api/internal/reconciler"
	"fractal/api/internal/secrets"
	"fractal/api/internal/telemetry"
	"fractal/api/internal/telemetry/endpoints"
	"fractal/api/internal/webapi"
	"fractal/api/internal/webhook"
)

type webhookProcessor struct {
	db       *pgxpool.Pool
	paypal   *provider.PayPal
	accounts *webhook.PGAccounts
	log      *slog.Logger
	web      *webapi.Handlers
}

func (p *webhookProcessor) Process(ctx context.Context, ev webhook.EventRow) error {
	var payload struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
		Resource  struct {
			ID                string `json:"id"`
			SupplementaryData struct {
				RelatedIDs struct {
					OrderID string `json:"order_id"`
				} `json:"related_ids"`
			} `json:"supplementary_data"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("parse webhook payload: %w", err)
	}

	providerOrderID := payload.Resource.SupplementaryData.RelatedIDs.OrderID
	if providerOrderID == "" {
		providerOrderID = payload.Resource.ID
	}

	// Fetch fresh provider state from PayPal using this merchant's credentials.
	client := p.paypal
	if p.accounts != nil {
		account, err := p.accounts.Credentials(ctx, ev.ProviderAccountID)
		if err != nil {
			return fmt.Errorf("resolve provider credentials: %w", err)
		}
		client = provider.NewPayPal(account.BaseURL, account.ClientID, account.ClientSecret, p.log)
	}
	tx, err := client.GetTransaction(ctx, providerOrderID)
	if err != nil {
		return fmt.Errorf("fetch paypal transaction: %w", err)
	}

	var merchantID string
	if err := p.db.QueryRow(ctx, `
		SELECT merchant_id::text FROM provider_accounts WHERE id = $1`,
		ev.ProviderAccountID).Scan(&merchantID); err != nil {
		return fmt.Errorf("resolve provider account %s: %w", ev.ProviderAccountID, err)
	}

	// Upsert payment_transactions.
	_, err = p.db.Exec(ctx, `
		INSERT INTO payment_transactions (merchant_id, provider_account_id, provider, provider_order_id, capture_id, amount_minor, currency, provider_status, merchant_order_ref, last_fetched_at)
		VALUES ($1, $2, 'PAYPAL', $3, NULLIF($4, ''), $5, $6, $7, $8, $9)
		ON CONFLICT (provider, provider_order_id) DO UPDATE
			SET capture_id = COALESCE(EXCLUDED.capture_id, payment_transactions.capture_id),
			    amount_minor = EXCLUDED.amount_minor,
			    currency = EXCLUDED.currency,
			    provider_status = EXCLUDED.provider_status,
			    merchant_order_ref = EXCLUDED.merchant_order_ref,
			    last_fetched_at = EXCLUDED.last_fetched_at`,
		merchantID, ev.ProviderAccountID, tx.ProviderOrderID, tx.CaptureID,
		tx.AmountMinor, tx.Currency, tx.Status, tx.MerchantOrderRef, tx.FetchedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert payment_transaction: %w", err)
	}

	// Find matching cases for this payment and mark them for re-investigation.
	// A case opened before the payment was known is linked here, so a late
	// webhook still reaches the case and the incident grouping.
	rows, err := p.db.Query(ctx, `
		UPDATE exception_cases
		SET payment_transaction_id = COALESCE(payment_transaction_id, pt.id),
		    status = 'COLLECTING_EVIDENCE', version = version + 1, updated_at = now()
		FROM payment_transactions pt
		WHERE pt.provider = 'PAYPAL' AND pt.provider_order_id = $1
		  AND exception_cases.merchant_id = pt.merchant_id
		  AND (
			exception_cases.payment_transaction_id = pt.id
			OR (exception_cases.payment_transaction_id IS NULL
			    AND exception_cases.merchant_order_ref = pt.merchant_order_ref)
		  )
		  AND exception_cases.status NOT IN ('RESOLVED', 'REJECTED', 'CLOSED')
		RETURNING exception_cases.id::text`,
		tx.ProviderOrderID,
	)
	if err != nil {
		return fmt.Errorf("update cases: %w", err)
	}
	var affected []string
	for rows.Next() {
		var caseID string
		if err := rows.Scan(&caseID); err != nil {
			rows.Close()
			return fmt.Errorf("read affected case: %w", err)
		}
		affected = append(affected, caseID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read affected cases: %w", err)
	}
	if p.web != nil {
		for _, caseID := range affected {
			go p.web.ReinvestigateCase(context.WithoutCancel(ctx), caseID)
		}
		// A provider-side fix changes the verified facts, so the incident
		// grouping is re-checked for the merchant.
		if p.web.Assistant != nil && len(affected) > 0 {
			go func() {
				groupCtx := context.WithoutCancel(ctx)
				if err := p.web.Assistant.RegroupMerchant(groupCtx, merchantID); err != nil {
					p.log.Warn("incident regroup failed", "merchant_id", merchantID, "err", err)
				}
			}()
		}
	}

	p.log.Info("webhook event processed", "event", ev.ID, "type", ev.EventType, "capture", payload.Resource.ID)
	return nil
}

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

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		logger.Error("open sql db", "err", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	caseStore := cases.NewStore(pool)

	anomalyEnabled := false
	switch os.Getenv("FRACTAL_ANOMALY_ENABLED") {
	case "true", "1", "yes":
		anomalyEnabled = true
	}
	scorer, err := anomaly.NewScorer(anomalyEnabled)
	if err != nil {
		slog.Error("anomaly scorer unavailable; anomaly_score stays null", "err", err)
		anomalyEnabled = false
	} else {
		baselineVer, iforestVer := scorer.ModelVersions()
		slog.Info("anomaly scorer ready", "enabled", anomalyEnabled,
			"baseline_model", baselineVer, "iforest_model", iforestVer)
	}

	caseHandlers := cases.NewHandlers(caseStore, anomalyEnabled)

	if tp, err := telemetry.InitTracer(os.Getenv("OTEL_ENDPOINT")); err != nil {
		slog.Warn("tracer init failed", "err", err)
	} else {
		defer tp.Shutdown(context.Background())
	}

	clientID := os.Getenv("PAYPAL_CLIENT_ID")
	clientSecret := os.Getenv("PAYPAL_CLIENT_SECRET")
	baseURL := os.Getenv("PAYPAL_BASE_URL")
	if baseURL == "" {
		baseURL = provider.PayPalSandboxBaseURL
	}

	paypal := provider.NewPayPal(baseURL, clientID, clientSecret, logger)

	cipher, err := secrets.NewFromEnv()
	if err != nil {
		logger.Error("credential encryption unavailable", "err", err)
		os.Exit(1)
	}

	opaClient := policy.NewClient(os.Getenv("OPA_URL"))
	web := webapi.New(pool, logger, opaClient, paypal, anomalyEnabled)
	web.Secrets = cipher
	web.PublicBaseURL = os.Getenv("FRACTAL_PUBLIC_URL")
	web.KibanaURL = os.Getenv("KIBANA_URL")

	wake, wakeCh := webhook.NewWaker()
	accounts := &webhook.PGAccounts{DB: sqlDB, Decrypt: cipher.Decrypt}
	h := &webhook.Handler{
		Provider: "PAYPAL",
		Accounts: accounts,
		Verifier: paypal,
		VerifierFor: func(account webhook.AccountCredentials) provider.WebhookVerifier {
			return provider.NewPayPal(account.BaseURL, account.ClientID, account.ClientSecret, logger)
		},
		Store: &webhook.PGStore{DB: sqlDB},
		Wake:  wake,
		Log:   logger,
	}

	// Real webhook processor: updates payment_transactions from verified events.
	go (&webhook.Worker{
		DB:   sqlDB,
		Proc: &webhookProcessor{db: pool, paypal: paypal, accounts: accounts, log: logger, web: web},
		Wake: wakeCh,
		Log:  logger,
	}).Run(context.Background())

	reconcilerEnabled := false
	switch os.Getenv("FRACTAL_RECONCILER_ENABLED") {
	case "true", "1", "yes":
		reconcilerEnabled = true
	}
	if reconcilerEnabled {
		systemCustomerID := os.Getenv("FRACTAL_SYSTEM_CUSTOMER_ID")
		if systemCustomerID == "" {
			var foundID string
			if err := pool.QueryRow(context.Background(), `
				SELECT id::text FROM users WHERE kind = 'CUSTOMER' AND status = 'ACTIVE'
				ORDER BY created_at ASC LIMIT 1`).Scan(&foundID); err == nil {
				systemCustomerID = foundID
			}
		}
		staleAfter := 15 * time.Minute
		if d, err := time.ParseDuration(os.Getenv("FRACTAL_RECONCILER_STALE_AFTER")); err == nil && d > 0 {
			staleAfter = d
		}
		recJob := reconciler.New(pool, logger, reconciler.Config{
			StaleAfter:       staleAfter,
			SystemCustomerID: systemCustomerID,
		})
		go recJob.RunLoop(context.Background())
		slog.Info("proactive reconciler enabled",
			"stale_after", staleAfter, "system_customer_id", systemCustomerID)
	}

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

	mux.Handle("POST /api/v1/auth/dev-login", middleware(http.HandlerFunc(web.DevLogin)))
	mux.Handle("POST /api/v1/auth/login", middleware(http.HandlerFunc(web.PasswordLogin)))

	// Merchant onboarding: signup, region choice and Connect PayPal.
	mux.Handle("GET /api/v1/onboarding/regions", middleware(http.HandlerFunc(web.Regions)))
	mux.Handle("POST /api/v1/onboarding/signup", middleware(http.HandlerFunc(web.Signup)))
	mux.Handle("GET /api/v1/onboarding/status", middleware(auth.RequireAuth(http.HandlerFunc(web.OnboardingStatus))))
	mux.Handle("POST /api/v1/onboarding/paypal", middleware(auth.RequireAuth(http.HandlerFunc(web.ConnectPayPal))))
	mux.Handle("POST /api/v1/onboarding/paypal/managed", middleware(auth.RequireAuth(http.HandlerFunc(web.ConnectManagedPayPal))))
	mux.Handle("POST /api/v1/onboarding/paypal/re-register", middleware(auth.RequireAuth(http.HandlerFunc(web.ReRegisterWebhook))))
	mux.Handle("GET /api/v1/observability/summary", middleware(auth.RequireAuth(http.HandlerFunc(web.ObservabilitySummary))))

	mux.Handle("GET /api/v1/cases", middleware(auth.RequireAuth(http.HandlerFunc(web.ListCases))))
	mux.Handle("POST /api/v1/cases", middleware(auth.RequireAuth(http.HandlerFunc(web.CreateCase))))
	mux.Handle("GET /api/v1/cases/{id}", middleware(auth.RequireAuth(http.HandlerFunc(web.GetCase))))
	mux.Handle("GET /api/v1/cases/{id}/timeline", middleware(auth.RequireAuth(http.HandlerFunc(web.Timeline))))
	mux.Handle("GET /api/v1/cases/{id}/messages", middleware(auth.RequireAuth(http.HandlerFunc(web.Messages))))
	mux.Handle("POST /api/v1/cases/{id}/messages", middleware(auth.RequireAuth(http.HandlerFunc(web.PostMessage))))
	mux.Handle("POST /api/v1/cases/{id}/actions", middleware(auth.RequireAuth(http.HandlerFunc(web.PostAction))))
	mux.Handle("POST /api/v1/cases/{id}/approvals/{approvalID}", middleware(auth.RequireAuth(http.HandlerFunc(web.DecideApproval))))

	// AI Case Assistant: incident grouping, triage inbox and drafted replies.
	mux.Handle("GET /api/v1/assistant/incidents", middleware(auth.RequireAuth(http.HandlerFunc(web.ListIncidents))))
	mux.Handle("GET /api/v1/assistant/incidents/{id}", middleware(auth.RequireAuth(http.HandlerFunc(web.GetIncident))))
	mux.Handle("POST /api/v1/assistant/incidents/{id}/drafts", middleware(auth.RequireAuth(http.HandlerFunc(web.DraftIncident))))
	mux.Handle("POST /api/v1/assistant/incidents/{id}/drafts/approve", middleware(auth.RequireAuth(http.HandlerFunc(web.ApproveIncidentDrafts))))
	mux.Handle("POST /api/v1/assistant/incidents/{id}/drafts/send", middleware(auth.RequireAuth(http.HandlerFunc(web.SendIncidentDrafts))))
	mux.Handle("POST /api/v1/assistant/incidents/{id}/cases/{caseID}", middleware(auth.RequireAuth(http.HandlerFunc(web.AddIncidentCase))))
	mux.Handle("DELETE /api/v1/assistant/incidents/{id}/cases/{caseID}", middleware(auth.RequireAuth(http.HandlerFunc(web.RemoveIncidentCase))))
	mux.Handle("POST /api/v1/assistant/incidents/{id}/fanout", middleware(auth.RequireAuth(http.HandlerFunc(web.FanOutIncident))))
	mux.Handle("POST /api/v1/assistant/regroup", middleware(auth.RequireAuth(http.HandlerFunc(web.RegroupMerchant))))
	mux.Handle("POST /api/v1/assistant/drafts/{id}", middleware(auth.RequireAuth(http.HandlerFunc(web.UpdateDraft))))
	mux.Handle("POST /api/v1/assistant/drafts/{id}/approve", middleware(auth.RequireAuth(http.HandlerFunc(web.ApproveDraft))))
	mux.Handle("POST /api/v1/assistant/drafts/{id}/send", middleware(auth.RequireAuth(http.HandlerFunc(web.SendDraft))))
	mux.Handle("POST /api/v1/assistant/drafts/{id}/discard", middleware(auth.RequireAuth(http.HandlerFunc(web.DiscardDraft))))
	mux.Handle("POST /api/v1/cases/{id}/group", middleware(auth.RequireAuth(http.HandlerFunc(web.GroupCase))))
	mux.Handle("POST /api/v1/cases/{id}/drafts", middleware(auth.RequireAuth(http.HandlerFunc(web.DraftCase))))

	mux.Handle("POST /api/v1/webhooks/paypal/{provider_account_id}", middleware(h))
	// Per-merchant onboarding registers the shorter public URL. Both paths use
	// the same receiver so existing tunnelled webhooks keep working.
	mux.Handle("POST /webhooks/paypal/{provider_account_id}", middleware(h))

	mux.Handle("POST /internal/v1/claim-tokens", middleware(http.HandlerFunc(web.RegisterClaimToken)))

	mux.Handle("GET /internal/v1/telemetry/logs", middleware(http.HandlerFunc(endpoints.LogsByCase)))
	mux.Handle("GET /internal/v1/telemetry/traces/{trace_id}", middleware(http.HandlerFunc(endpoints.TraceByID)))
	mux.Handle("GET /internal/v1/telemetry/handler-error-rate", middleware(http.HandlerFunc(endpoints.HandlerErrorRate)))

	mux.Handle("GET /internal/v1/cases/{id}/evidence", middleware(http.HandlerFunc(web.InternalCaseEvidence)))
	mux.Handle("GET /internal/v1/cases/{id}/webhook-events", middleware(http.HandlerFunc(web.InternalCaseWebhookEvents)))

	policyHandler := &policy.Handler{
		Client:         opaClient,
		AnomalyEnabled: anomalyEnabled,
		Log:            logger,
	}
	mux.Handle("POST /internal/v1/policy/evaluate", middleware(http.HandlerFunc(policyHandler.Evaluate)))

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
		Handler:           webapi.CORS(mux),
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
