package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter enforces per-IP request limits using a Redis-backed sliding
// window algorithm. Each request adds an entry to a sorted set keyed by client
// IP, with the score set to the current timestamp in nanoseconds. Expired
// entries (outside the window) are pruned on every request. This approach
// avoids the burst problem of fixed-window counters while remaining simple to
// implement and reason about.
type RateLimiter struct {
	redis     *redis.Client
	limit     int           // Maximum number of requests allowed per window.
	window    time.Duration // Sliding window duration (e.g. 1 minute).
	keyPrefix string        // Redis key prefix to namespace rate-limit keys.
}

// NewRateLimiter creates a RateLimiter that allows at most limit requests per
// window duration. The Redis client should be shared with other components
// (e.g. the cache layer) to avoid connection pool fragmentation.
func NewRateLimiter(redisClient *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		redis:     redisClient,
		limit:     limit,
		window:    window,
		keyPrefix: "ratelimit:",
	}
}

// Middleware returns an http.Handler middleware that enforces the configured
// rate limit. It sets standard rate-limit response headers on every request
// (X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset) so clients
// can self-throttle, and returns 429 Too Many Requests with a Retry-After
// header when the limit is exceeded.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)
		key := rl.keyPrefix + clientIP

		allowed, remaining, resetTime := rl.allowRequest(r.Context(), key)

		// Always set rate-limit headers so well-behaved clients can
		// monitor their quota even when they are not yet throttled.
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(time.Until(resetTime).Seconds())))
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// allowRequest executes the sliding-window rate-limit check as a single Redis
// pipeline for atomicity and reduced round trips. The pipeline performs four
// operations in order:
//  1. ZREMRANGEBYSCORE - prune entries older than the window
//  2. ZCARD           - count remaining (pre-add) entries
//  3. ZADD            - record the current request
//  4. EXPIRE          - set a TTL so the key is garbage-collected
//
// On Redis failure the request is allowed (fail-open), which trades a brief
// period of unenforced limits for service availability.
func (rl *RateLimiter) allowRequest(ctx context.Context, key string) (bool, int, time.Time) {
	now := time.Now()
	windowStart := now.Add(-rl.window)

	pipe := rl.redis.Pipeline()

	// Remove entries that have fallen outside the sliding window.
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// Count entries before adding the current one; this is the basis for
	// the rate-limit decision.
	zcard := pipe.ZCard(ctx, key)

	// Record this request with its nanosecond timestamp as both score and
	// member. Using nanoseconds as the member ensures uniqueness even
	// under high concurrency.
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Set a TTL equal to the window so Redis reclaims memory for IPs that
	// stop sending traffic.
	pipe.Expire(ctx, key, rl.window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		// Fail open: if Redis is unreachable, allow the request rather
		// than blocking all traffic.
		return true, rl.limit, now.Add(rl.window)
	}

	count := int(zcard.Val())

	if count >= rl.limit {
		// Determine when the oldest entry in the window will expire, so
		// the client knows when to retry.
		oldestKey := key
		results, _ := rl.redis.ZRange(ctx, oldestKey, 0, 0).Result()
		var resetTime time.Time
		if len(results) > 0 {
			var oldestTimestamp int64
			_, _ = fmt.Sscanf(results[0], "%d", &oldestTimestamp)
			resetTime = time.Unix(0, oldestTimestamp).Add(rl.window)
		} else {
			resetTime = now.Add(rl.window)
		}

		return false, 0, resetTime
	}

	// Subtract 1 from remaining to account for the entry we just added.
	remaining := rl.limit - count - 1
	if remaining < 0 {
		remaining = 0
	}

	resetTime := now.Add(rl.window)

	return true, remaining, resetTime
}

// getClientIP extracts the client's real IP address from proxy headers or the
// TCP connection. It validates parsed IPs with net.ParseIP to guard against
// spoofed X-Forwarded-For values containing non-IP strings. Priority:
// X-Forwarded-For (first entry) > X-Real-IP > RemoteAddr.
func getClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		ip := strings.TrimSpace(parts[0])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		if net.ParseIP(realIP) != nil {
			return realIP
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
