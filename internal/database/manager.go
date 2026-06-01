// Package database provides a PostgreSQL connection pool manager that supports
// read-write splitting across a primary instance and multiple read replicas.
// This is a common pattern for URL shorteners where read traffic (redirect
// lookups) vastly exceeds write traffic (URL creation), allowing horizontal
// read scaling without application-level sharding.
package database

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBManager manages a pool of PostgreSQL connections split between a single
// primary (read-write) instance and zero or more read replicas. Read queries
// are distributed across replicas using lock-free atomic round-robin, while
// all write queries are routed to the primary.
//
// If no replicas are configured, Read() transparently falls back to the
// primary, making the manager safe to use in single-node development setups.
type DBManager struct {
	// primary is the connection pool for the read-write PostgreSQL instance.
	// All INSERT, UPDATE, and DELETE queries must go through this pool.
	primary *pgxpool.Pool

	// replicas holds connection pools for read-only PostgreSQL replicas.
	// May be empty if no replicas are configured, in which case Read()
	// falls back to the primary pool.
	replicas []*pgxpool.Pool

	// replicaIndex is an atomically incremented counter used to distribute
	// read queries across replicas in a round-robin fashion. Using uint32
	// with atomic.AddUint32 avoids mutex contention on the hot read path.
	// The counter is allowed to wrap around naturally; the modulo operation
	// in Read() handles the wrap correctly.
	replicaIndex uint32
}

// Config holds the connection parameters for initializing a DBManager.
// The pool tuning parameters (MaxConns, MinConns, lifetimes) are applied
// uniformly to both the primary and all replica pools.
type Config struct {
	// PrimaryDSN is the PostgreSQL connection string for the primary
	// read-write instance (e.g., "postgres://user:pass@primary:5432/tiny").
	PrimaryDSN string

	// ReplicaDSNs is a list of connection strings for read replicas.
	// Pass an empty slice for single-node setups.
	ReplicaDSNs []string

	// MaxConns is the maximum number of connections per pool. Should not
	// exceed PostgreSQL's max_connections divided by the number of
	// application instances.
	MaxConns int32

	// MinConns is the minimum number of idle connections maintained per
	// pool to avoid cold-start latency on traffic spikes.
	MinConns int32

	// MaxConnLifetime is the maximum total lifetime of a connection.
	// Recycling connections periodically helps rebalance load after
	// replica failovers or DNS changes.
	MaxConnLifetime time.Duration

	// MaxConnIdleTime is the maximum time a connection can remain idle
	// before being closed, preventing resource waste during low traffic.
	MaxConnIdleTime time.Duration
}

// NewDBManager creates a DBManager by establishing connection pools to the
// primary instance and all configured replicas. Each pool is verified with
// a Ping before being accepted. If any connection fails, all previously
// opened pools are closed to prevent resource leaks, and an error is returned.
//
// The context controls the timeout for the initial connection and ping to
// each database instance.
func NewDBManager(ctx context.Context, cfg Config) (*DBManager, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.PrimaryDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse primary DSN: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime

	primaryPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse primary dsn: %w", err)
	}

	// Verify the primary is reachable before proceeding. A failed ping
	// here prevents the application from starting with a bad DSN.
	if err := primaryPool.Ping(ctx); err != nil {
		primaryPool.Close()
		return nil, fmt.Errorf("failed to ping primary: %w", err)
	}

	replicas := make([]*pgxpool.Pool, 0, len(cfg.ReplicaDSNs))
	for i, dsn := range cfg.ReplicaDSNs {
		replicaConfig, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			// Clean up all previously opened pools on failure.
			primaryPool.Close()
			closeReplicas(replicas)
			return nil, fmt.Errorf("failed to parse replica %d DSN: %w", i, err)
		}

		replicaConfig.MaxConns = cfg.MaxConns
		replicaConfig.MinConns = cfg.MinConns
		replicaConfig.MaxConnLifetime = cfg.MaxConnLifetime
		replicaConfig.MaxConnIdleTime = cfg.MaxConnIdleTime

		replicaPool, err := pgxpool.NewWithConfig(ctx, replicaConfig)
		if err != nil {
			primaryPool.Close()
			closeReplicas(replicas)
			return nil, fmt.Errorf("failed to connect to replica %d: %w", i, err)
		}

		if err := replicaPool.Ping(ctx); err != nil {
			replicaPool.Close()
			primaryPool.Close()
			closeReplicas(replicas)
			return nil, fmt.Errorf("failed to ping replica %d: %w", i, err)
		}

		replicas = append(replicas, replicaPool)
	}

	return &DBManager{
		primary:      primaryPool,
		replicas:     replicas,
		replicaIndex: 0,
	}, nil
}

// Write returns the primary connection pool for write operations (INSERT,
// UPDATE, DELETE). All mutations must use this pool to ensure they hit the
// primary PostgreSQL instance.
func (m *DBManager) Write() *pgxpool.Pool {
	return m.primary
}

// Read returns a connection pool for read-only queries. If replicas are
// configured, it selects one using atomic round-robin to distribute load
// evenly. If no replicas exist, it falls back to the primary pool.
//
// The round-robin uses atomic.AddUint32 to avoid mutex contention, making
// this method safe and efficient for high-throughput concurrent access.
// The uint32 counter wraps naturally at 2^32; the modulo ensures the index
// always lands within the replica slice bounds.
func (m *DBManager) Read() *pgxpool.Pool {
	if len(m.replicas) == 0 {
		return m.primary
	}

	idx := atomic.AddUint32(&m.replicaIndex, 1) % uint32(len(m.replicas))
	return m.replicas[idx]
}

// closeReplicas is a helper that closes all replica pools in the slice.
// It is called during cleanup when NewDBManager encounters an error after
// some replicas have already been connected. Nil-safe for each pool entry.
func closeReplicas(replicas []*pgxpool.Pool) {
	for _, pool := range replicas {
		if pool != nil {
			pool.Close()
		}
	}
}

// Primary returns the primary connection pool directly. This is an alias
// for Write() provided for callers that need explicit access to the primary
// for operations like schema migrations or advisory locks.
func (m *DBManager) Primary() *pgxpool.Pool {
	return m.primary
}

// Close gracefully shuts down all connection pools (primary and replicas),
// releasing database connections back to PostgreSQL. This should be called
// during application shutdown, typically deferred after NewDBManager returns.
func (m *DBManager) Close() {
	if m.primary != nil {
		m.primary.Close()
	}
	closeReplicas(m.replicas)
}

// Stats returns a snapshot of connection pool statistics for the primary and
// all replicas. This is intended for health-check endpoints and monitoring
// dashboards. The returned map contains "primary" (a single stats object)
// and "replicas" (a slice of stats objects, one per replica).
//
// Each stats object includes:
//   - total_conns: total number of connections in the pool
//   - idle_conns: number of idle (available) connections
//   - acquired_conns: number of connections currently in use by queries
func (m *DBManager) Stats() map[string]interface{} {
	stats := make(map[string]interface{})

	if m.primary != nil {
		primaryStat := m.primary.Stat()
		stats["primary"] = map[string]interface{}{
			"total_conns":    primaryStat.TotalConns(),
			"idle_conns":     primaryStat.IdleConns(),
			"acquired_conns": primaryStat.AcquiredConns(),
		}
	}

	replicaStats := make([]map[string]interface{}, len(m.replicas))
	for i, replica := range m.replicas {
		replicaStat := replica.Stat()
		replicaStats[i] = map[string]interface{}{
			"total_conns":    replicaStat.TotalConns(),
			"idle_conns":     replicaStat.IdleConns(),
			"acquired_conns": replicaStat.AcquiredConns(),
		}
	}
	stats["replicas"] = replicaStats

	return stats
}
