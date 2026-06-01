package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// RequestIDKey is the context key under which the per-request correlation ID
// is stored. It reuses the contextKey type defined in auth.go.
const RequestIDKey contextKey = "request_id"

// RequestID is middleware that ensures every request carries a unique
// correlation ID. If the incoming request already has an X-Request-ID header
// (e.g. set by an upstream load balancer or API gateway), that value is
// preserved for end-to-end tracing. Otherwise a new UUID v4 is generated.
// The ID is echoed back in the response header and stored in the context so
// downstream handlers and loggers can include it without manual plumbing.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Honor an existing request ID from upstream infrastructure to
		// support distributed trace correlation across service boundaries.
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Echo the ID in the response so clients can reference it in
		// support requests or bug reports.
		w.Header().Set("X-Request-ID", requestID)

		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the request correlation ID from the context. Returns
// an empty string if no request ID was set (e.g. the middleware was not applied).
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}
