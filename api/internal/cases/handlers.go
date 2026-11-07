package cases

import (
	"encoding/json"
	"net/http"

	"fractal/api/internal/auth"
	"fractal/api/internal/telemetry"
)

type Handlers struct {
	store *Store
}

func NewHandlers(store *Store) *Handlers { return &Handlers{store: store} }

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
