package cases

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"fractal/api/internal/auth"
	"fractal/api/internal/telemetry"
)

type Handlers struct {
	store *Store
	// anomalyEnabled gates the admin-only anomaly field on GET. While false
	// (default) responses are identical to Phase 10 — no anomaly key at all.
	anomalyEnabled bool
}

func NewHandlers(store *Store, anomalyEnabled bool) *Handlers {
	return &Handlers{store: store, anomalyEnabled: anomalyEnabled}
}

// isAdmin reports whether the caller may see anomaly details: platform
// admins and merchant admins only. Customers and merchant staff
// (MERCHANT_STAFF) never receive the field.
func isAdmin(claims *auth.Claims) bool {
	return claims.Kind == "PLATFORM_ADMIN" || claims.Role == "MERCHANT_ADMIN"
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	cases, err := h.store.ListCases(r.Context(), claims)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, r, http.StatusOK, cases)
}

func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	caseID := r.PathValue("id")
	c, ok, err := h.store.GetCase(r.Context(), caseID, claims)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if !ok {
		// Non-party: 404 so IDs cannot be enumerated.
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	// Phase 11: attach the anomaly score for admins only, and only while the
	// model is enabled. A lookup failure never breaks the case view — it
	// just omits the field (fail-safe for availability, not authorization:
	// non-admins never reach this branch).
	if h.anomalyEnabled && isAdmin(claims) {
		info, err := h.store.LatestAnomalyScore(r.Context(), caseID, c.MerchantID)
		if err != nil {
			slog.Warn("anomaly score lookup failed", "case_id", caseID, "err", err)
		} else if info != nil {
			c.Anomaly = info
		}
	}
	writeJSON(w, r, http.StatusOK, c)
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	// Stub: full create with ownership proof comes in the next step.
	writeJSON(w, r, http.StatusNotImplemented, map[string]string{"error": "create not implemented yet"})
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", telemetry.RequestIDFromCtx(r.Context()))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
