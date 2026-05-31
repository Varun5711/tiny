package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

func Tracing(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		handler := otelhttp.NewHandler(next, serviceName)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(w, r)
			span := trace.SpanFromContext(r.Context())
			if span.SpanContext().HasTraceID() {
				w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
			}
		})
	}
}
