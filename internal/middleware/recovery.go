package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/Varun5711/shorternit/internal/logger"
)

// Recovery returns middleware that catches panics in downstream handlers and
// converts them into 500 Internal Server Error responses instead of crashing
// the process. The full stack trace is logged for debugging. This should be
// the outermost middleware in the chain so it can recover panics from every
// layer, including other middleware.
func Recovery(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Log the panic value and stack trace so operators can
					// diagnose the root cause without losing the process.
					log.Error("Panic recovered: %v\nStack trace:\n%s", err, debug.Stack())
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
