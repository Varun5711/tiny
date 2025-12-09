package database

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DBManager struct {
	primary      *pgxpool.Pool
	replicas     []*pgxpool.Pool
	replicaIndex uint32
}

type Config struct {
	PrimaryDSN  string
	ReplicaDSNs []string

	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

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

	if err := primaryPool.Ping(ctx); err != nil {
		primaryPool.Close()
		return nil, fmt.Errorf("failed to ping primary: %w", err)
	}

	replicas := make([]*pgxpool.Pool, 0, len(cfg.ReplicaDSNs))
	for i, dsn := range cfg.ReplicaDSNs {
		replicaConfig, err := pgxpool.ParseConfig(dsn)
		if err != nil {
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

func (m *DBManager) Write() *pgxpool.Pool {
	return m.primary
}

func (m *DBManager) Read() *pgxpool.Pool {
	if len(m.replicas) == 0 {
		return m.primary
	}

	idx := atomic.AddUint32(&m.replicaIndex, 1) % uint32(len(m.replicas))
	return m.replicas[idx]
}
func closeReplicas(replicas []*pgxpool.Pool) {
	for _, pool := range replicas {
		if pool != nil {
			pool.Close()
		}
	}
}

func (m *DBManager) Primary() *pgxpool.Pool {
	return m.primary
}

func (m *DBManager) Close() {
	if m.primary != nil {
		m.primary.Close()
	}
	closeReplicas(m.replicas)
}

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
