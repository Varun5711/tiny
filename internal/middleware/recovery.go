package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/Varun5711/shorternit/internal/logger"
)

func Recovery(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error("Panic recovered: %v\nStack trace:\n%s", err, debug.Stack())
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
