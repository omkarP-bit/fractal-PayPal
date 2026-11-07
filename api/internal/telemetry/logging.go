package telemetry

import "context"

type requestIDKey struct{}

// WithRequestID attaches a request ID to the context so every log/trace span
// can carry it through the call chain.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromCtx returns the request ID from context, or "" if unset.
func RequestIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}
