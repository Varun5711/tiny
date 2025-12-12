package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	redis       *redis.Client
	limit       int
	window      time.Duration
	keyPrefix   string
}

func NewRateLimiter(redisClient *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		redis:     redisClient,
		limit:     limit,
		window:    window,
		keyPrefix: "ratelimit:",
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)
		key := rl.keyPrefix + clientIP

		allowed, remaining, resetTime := rl.allowRequest(r.Context(), key)

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

func (rl *RateLimiter) allowRequest(ctx context.Context, key string) (bool, int, time.Time) {
	now := time.Now()
	windowStart := now.Add(-rl.window)

	pipe := rl.redis.Pipeline()

	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	zcard := pipe.ZCard(ctx, key)

	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	pipe.Expire(ctx, key, rl.window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return true, rl.limit, now.Add(rl.window)
	}

	count := int(zcard.Val())

	if count >= rl.limit {
		oldestKey := key
		results, _ := rl.redis.ZRange(ctx, oldestKey, 0, 0).Result()
		var resetTime time.Time
		if len(results) > 0 {
			var oldestTimestamp int64
			fmt.Sscanf(results[0], "%d", &oldestTimestamp)
			resetTime = time.Unix(0, oldestTimestamp).Add(rl.window)
		} else {
			resetTime = now.Add(rl.window)
		}

		return false, 0, resetTime
	}

	remaining := rl.limit - count - 1
	if remaining < 0 {
		remaining = 0
	}

	resetTime := now.Add(rl.window)

	return true, remaining, resetTime
}

func getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return forwarded
	}

	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	return r.RemoteAddr
}
