package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

// Tracing returns middleware that wraps each request in an OpenTelemetry span
// using the otelhttp instrumentation library. The serviceName parameter
// becomes the span name prefix, making it easy to identify the API gateway
// in a distributed trace viewer (e.g. Jaeger, Tempo).
//
// The middleware also exposes the trace ID to clients via the X-Trace-ID
// response header, enabling frontend applications and API consumers to
// correlate their requests with backend traces for debugging.
func Tracing(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// otelhttp.NewHandler automatically starts a span, records HTTP
		// metrics, and propagates the trace context to downstream services.
		handler := otelhttp.NewHandler(next, serviceName)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(w, r)
			// Extract the trace ID after the handler runs so it reflects
			// the span that was actually created for this request.
			span := trace.SpanFromContext(r.Context())
			if span.SpanContext().HasTraceID() {
				w.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
			}
		})
	}
}
