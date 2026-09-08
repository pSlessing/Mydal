package httpx

import "context"

type requestIDKey struct{}

// WithRequestID returns a context carrying id, retrievable with
// RequestIDFromContext. The RequestID middleware sets this once per request
// so error logging at the edge (WriteError) can carry the same id the access
// log line does, letting the two be correlated.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the id assigned to this request, if any.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
