package middleware

import (
	"net/http"
)

// CORS returns middleware that handles Cross-Origin Resource Sharing headers.
// Only origins explicitly listed in allowedOrigins will receive the
// Access-Control-Allow-Origin header, preventing credential leakage to
// untrusted domains. The origin list is converted to a map at initialization
// time for O(1) lookups on every request.
//
// Preflight OPTIONS requests are short-circuited with 204 No Content so they
// do not reach downstream handlers or count against rate limits. The
// Access-Control-Max-Age of 3600 seconds tells browsers to cache the
// preflight response for one hour, reducing redundant OPTIONS traffic.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	// Pre-compute a set for fast membership checks on the hot path.
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Only reflect the origin back if it is in the allowlist.
			// Setting Vary: Origin is required so caches do not serve a
			// response with origin A's header to a request from origin B.
			if origin != "" && originSet[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			w.Header().Set("Access-Control-Max-Age", "3600")

			// Short-circuit preflight requests so they do not hit auth or
			// rate-limiting middleware further down the chain.
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
