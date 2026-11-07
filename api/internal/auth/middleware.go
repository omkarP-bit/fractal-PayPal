package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"fractal/api/internal/telemetry"
)

type contextKey string

const claimsKey contextKey = "claims"

// FromContext returns the verified JWT claims from the request context.
func FromContext(r *http.Request) *Claims {
	if c, ok := r.Context().Value(claimsKey).(*Claims); ok {
		return c
	}
	return nil
}

// RequireAuth verifies the Authorization: Bearer <jwt> header and stores claims
// in the context. Missing/invalid token -> 401 with RFC 9457 problem JSON.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "missing or invalid Authorization header")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		claims, err := Verify(token)
		if err != nil {
			writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "token verification failed: "+err.Error())
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, typ, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Request-Id", telemetry.RequestIDFromCtx(r.Context()))
	w.WriteHeader(status)
	body := fmt.Sprintf(`{"type":"https://fractal.dev/problems/%s","title":"%s","status":%d,"detail":"%s","request_id":"%s"}`,
		typ, typ, status, detail, telemetry.RequestIDFromCtx(r.Context()))
	_, _ = w.Write([]byte(body))
}
