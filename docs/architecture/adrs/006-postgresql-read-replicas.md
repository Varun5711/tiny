# ADR-006: PostgreSQL Read Replicas

## Status
Accepted

## Context

Database access patterns:
- **Writes:** URL creation, user registration, click counter updates
- **Reads:** URL lookups (cache miss), user authentication, URL listing

Read/write ratio is approximately 10:1 (reads dominate after cache misses).

Single PostgreSQL instance limitations:
- Connection limit (~100-500 connections)
- CPU saturation on read-heavy workloads
- Single point of failure

## Decision

Deploy PostgreSQL with 1 primary + 3 read replicas using streaming replication.

**Topology:**
```
                    ┌─────────────┐
                    │   Primary   │ ← All writes
                    │   :5432     │
                    └──────┬──────┘
                           │ WAL Streaming
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
   │  Replica 1  │  │  Replica 2  │  │  Replica 3  │
   │   :5433     │  │   :5434     │  │   :5435     │
   └─────────────┘  └─────────────┘  └─────────────┘
         ↑                ↑                ↑
         └────────────────┴────────────────┘
                    Read queries
```

**Routing logic:**
```go
// Writes always go to primary
db.Primary().Exec("INSERT INTO urls ...")

// Reads distributed across replicas
db.Replica().Query("SELECT * FROM urls WHERE ...")
```

## Consequences

### Positive
- **Read scalability** - 3x read capacity
- **High availability** - Promote replica if primary fails
- **Load distribution** - Spread connections across nodes
- **Zero data loss** - Synchronous replication option available

### Negative
- **Replication lag** - Replicas may be milliseconds behind
- **Operational complexity** - More nodes to monitor
- **Cost** - 4x database instances
- **Failover complexity** - Need orchestration for promotion

### Replication configuration
```
# Primary postgresql.conf
wal_level = replica
max_wal_senders = 10
max_replication_slots = 10
hot_standby = on
hot_standby_feedback = on

# Replication mode
synchronous_commit = on        # For strong consistency
# OR
synchronous_commit = local     # For better performance
```

### Read-after-write consistency
```
Problem:
  1. User creates URL (writes to primary)
  2. User immediately lists URLs (reads from replica)
  3. Replica hasn't received write yet → URL missing

Solutions:
  A. Read from primary after write (current approach)
     → Simple, guaranteed consistency
     → Slightly higher primary load

  B. Session-based routing
     → Track last write timestamp per session
     → Route to primary if within replication lag window

  C. Causal consistency tokens
     → Return LSN with write response
     → Wait for replica to reach LSN before reading

Current implementation: Option A for simplicity
```

### Connection pooling
```
Per-node limits:
  max_connections = 100

Application pool (per service instance):
  MaxConns = 25
  MinConns = 5
  MaxConnLifetime = 1h
  MaxConnIdleTime = 30m

Total connections (3 service instances):
  Primary: 75 connections
  Each Replica: 75 connections
```

### Monitoring queries
```sql
-- Check replication lag
SELECT
  client_addr,
  state,
  sent_lsn,
  write_lsn,
  flush_lsn,
  replay_lsn,
  pg_wal_lsn_diff(sent_lsn, replay_lsn) AS lag_bytes
FROM pg_stat_replication;

-- Check replica status
SELECT
  pg_is_in_recovery() AS is_replica,
  pg_last_wal_receive_lsn() AS receive_lsn,
  pg_last_wal_replay_lsn() AS replay_lsn,
  pg_last_xact_replay_timestamp() AS last_replay;
```

## References
- [PostgreSQL Streaming Replication](https://www.postgresql.org/docs/current/warm-standby.html)
- [High Availability PostgreSQL](https://www.postgresql.org/docs/current/high-availability.html)
