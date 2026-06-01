// Package redis provides a thin wrapper around the go-redis client, centralising
// connection configuration, health checks, and pool statistics in one place.
//
// The wrapper exists so that the rest of the codebase does not depend on
// go-redis configuration details directly. It also makes it straightforward to
// swap in a cluster client or add middleware (e.g., tracing, metrics) later
// without touching every call site.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps a go-redis Client with opinionated defaults for timeouts
// and connection pooling. It is the single point of contact for all Redis I/O
// in the application -- the multi-tier cache (L2), distributed locks, and
// event streams all receive their *redis.Client from here.
type RedisClient struct {
	client *redis.Client
}

// Config holds the parameters needed to connect to a Redis instance.
type Config struct {
	Addr     string // host:port of the Redis server
	Password string // AUTH password (empty for no auth)
	DB       int    // database number (0-15)
}

// NewRedisClient creates a RedisClient and validates the connection with a
// PING. If the server is unreachable or authentication fails the partially
// opened client is closed and a descriptive error is returned.
//
// The connection pool is configured with sensible defaults:
//   - DialTimeout 5s   -- fail fast if the server is down at startup.
//   - Read/WriteTimeout 3s -- avoid blocking the hot redirect path on a slow Redis.
//   - PoolSize 10, MinIdleConns 2 -- keep a small warm pool to avoid connection
//     setup latency on the first burst of traffic after an idle period.
func NewRedisClient(ctx context.Context, cfg Config) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{
		client: rdb,
	}, nil
}

// GetClient returns the underlying go-redis Client. Callers that need direct
// access -- such as the distributed lock or event stream packages -- use this
// rather than re-creating their own connections.
func (r *RedisClient) GetClient() *redis.Client {
	return r.client
}

// Ping sends a Redis PING and returns any error. It is used by health-check
// endpoints to verify that the Redis connection is still alive.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Close gracefully shuts down the connection pool. It should be called during
// application shutdown (typically via defer) to release file descriptors.
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Stats returns a snapshot of the connection pool metrics. This is useful for
// exposing pool health in a /debug or /metrics endpoint so operators can tune
// PoolSize and MinIdleConns based on real traffic patterns.
func (r *RedisClient) Stats() map[string]interface{} {
	stats := r.client.PoolStats()
	return map[string]interface{}{
		"hits":        stats.Hits,
		"misses":      stats.Misses,
		"timeouts":    stats.Timeouts,
		"total_conns": stats.TotalConns,
		"idle_conns":  stats.IdleConns,
		"stale_conns": stats.StaleConns,
	}
}
