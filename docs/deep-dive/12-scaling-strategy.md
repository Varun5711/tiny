# Scaling Strategy

> **Planning for 10x, 100x, and 1000x growth**

## Overview

Our URL shortener currently handles 1,000-10,000 requests/second. What happens at 100,000? 1,000,000?

This document explores:
1. **Current Capacity** - What we can handle today
2. **Bottlenecks** - Where we'll hit limits first
3. **Horizontal Scaling** - Adding more instances
4. **Database Scaling** - Sharding, read replicas, caching
5. **Cost Analysis** - Infrastructure costs at each scale
6. **When to Upgrade** - Redis Streams → Kafka, monolith → microservices

---

## Part 1: Current Capacity Analysis

### Single Instance Limits

**Redirect Service (1 instance):**
- Throughput: ~5,000 req/sec
- Bottleneck: CPU (cache lookups, HTTP parsing)
- Memory: 2GB (L1 cache + overhead)

**URL Service (1 instance):**
- Throughput: ~1,000 req/sec
- Bottleneck: Database writes
- Memory: 1GB

**Workers:**
- Analytics Worker: 10,000 events/sec
- Pipeline Worker: 1,000 events/sec (GeoIP is slow)

### Database Limits

**PostgreSQL (1 primary + 3 replicas):**
- Writes (primary): 5,000-10,000 queries/sec
- Reads (replicas): 10,000-15,000 queries/sec total
- Bottleneck: Disk I/O, connection pool saturation

**ClickHouse (1 node):**
- Inserts: 50,000 rows/sec (batched)
- Queries: Depends on complexity (100-1000 queries/sec)
- Bottleneck: Disk I/O for inserts

**Redis (1 instance):**
- Operations: 100,000 ops/sec
- Memory: 4GB (current), can scale to 100GB+
- Bottleneck: Network bandwidth (1 Gbps = 125 MB/s)

### Current Total Capacity

**Assumptions:**
- 90% cache hit rate
- 10% database queries

**Redirect throughput:** 5,000 req/sec per instance
- With 10 instances: **50,000 req/sec**
- Database load: 50,000 × 10% = 5,000 queries/sec ✓ (within limits)

**URL creation:** 1,000 req/sec per instance
- With 5 instances: **5,000 req/sec**
- Database writes: 5,000 writes/sec ✓ (near limit)

**Current system capacity: ~50,000 redirects/sec + 5,000 creates/sec**

---

## Part 2: Bottleneck Identification

### Bottleneck 1: Database Writes (Primary)

**Problem:**
- All writes go to single primary
- Connection pool: 25 connections
- At 10,000 writes/sec, connections saturate

**Solutions:**

**A. Increase connection pool:**
```go
MaxConns: 100  // was 25
```
**Gains:** 2-3x throughput
**Limits:** Database can't handle 100 connections efficiently (context switching)

**B. Use connection pooler (PgBouncer):**
- Multiplexes connections
- 1000 client connections → 25 database connections
- Better resource utilization

**C. Shard database:**
- Split URLs across multiple databases
- Hash short_code to determine shard
- Gains: Linear scaling (2 shards = 2x writes)

### Bottleneck 2: Pipeline Worker (GeoIP Enrichment)

**Problem:**
- GeoIP lookup: ~5-10ms per event
- 1 worker: ~100-200 events/sec
- 10,000 events/sec needs 50-100 workers!

**Solutions:**

**A. Cache GeoIP results:**
```go
geoIPCache := make(map[string]GeoIPResult)
if result, found := geoIPCache[ip]; found {
    return result  // 0.001ms instead of 5ms!
}
```
**Gains:** 10-100x speedup for repeat IPs

**B. Use faster GeoIP library:**
- MaxMind in-memory: ~5-10ms
- Alternative: IP2Location binary search: ~1-2ms
- Gains: 2-5x throughput

**C. Horizontal scaling:**
- Add more Pipeline Worker instances
- Consumer groups distribute load
- Gains: Linear scaling

### Bottleneck 3: Redis Network Bandwidth

**Problem:**
- 1 Gbps NIC = 125 MB/s
- Each cache operation: ~500 bytes
- Max: 250,000 ops/sec

**At 100,000 req/sec:**
- Cache operations: 100,000 × 2 (GET + SET) = 200,000 ops/sec
- Bandwidth: 200,000 × 500 bytes = 100 MB/s ✓ (within limits)

**At 500,000 req/sec:**
- Bandwidth: 500 MB/s (exceeds 1 Gbps limit!) ✗

**Solutions:**

**A. Redis Cluster (sharding):**
```
3 Redis nodes:
- Node 1: Keys A-M
- Node 2: Keys N-Z
- Node 3: Keys 0-9
```
**Gains:** 3x bandwidth (375 MB/s)

**B. Upgrade network (10 Gbps):**
**Gains:** 10x bandwidth (1.25 GB/s)

**C. Compress cache values:**
```go
compressed := gzip.Compress(longURL)
redis.Set(key, compressed)
```
**Gains:** 3-5x smaller payloads

### Bottleneck 4: ClickHouse Inserts

**Problem:**
- Single node: 50,000 inserts/sec
- At 100,000 clicks/sec: Overloaded

**Solutions:**

**A. Batch larger:**
- Current: 100 rows per batch
- Larger: 1,000 rows per batch
- **Gains:** 2-3x throughput (fewer network round-trips)

**B. Distributed ClickHouse:**
```
3 nodes:
- Shard 1: short_codes A-M
- Shard 2: short_codes N-Z
- Shard 3: short_codes 0-9
```
**Gains:** 3x write capacity

**C. Sample events:**
- Store 10% of clicks (random sample)
- Still statistically accurate analytics
- **Gains:** 10x capacity headroom

---

## Part 3: Horizontal Scaling

### Stateless Services (Easy to Scale)

**Redirect Service:**
```
Load Balancer (HAProxy/nginx)
   ↓
[Redirect-1] [Redirect-2] [Redirect-3] ... [Redirect-N]
```

**No coordination needed!** Each instance:
- Has own L1 cache (may have different entries)
- Shares L2 cache (Redis)
- Reads from any PostgreSQL replica

**Add instance:**
```bash
docker-compose up --scale redirect-service=10
```

**URL Service:**
```
API Gateway
   ↓
[URL-1] [URL-2] [URL-3] ... [URL-N]
```

**Also stateless:**
- Each generates unique Snowflake IDs (different worker IDs)
- All write to same database
- Share Redis cache

### Stateful Components (Harder to Scale)

**PostgreSQL Primary:**
- Can't horizontally scale writes (single primary)
- Options:
  1. Vertical scaling (bigger machine)
  2. Sharding (split data across databases)
  3. Read-write splitting (already done!)

**Redis (single instance):**
- Options:
  1. Redis Sentinel (high availability)
  2. Redis Cluster (sharding)

**ClickHouse (single node):**
- Options:
  1. Distributed tables (sharding)
  2. Replication (availability)

---

## Part 4: Database Sharding Strategy

### Problem: Single Primary Can't Scale Writes

At 50,000 writes/sec, single PostgreSQL primary is overloaded.

### Solution: Shard by Short Code

**Shard function:**
```go
func getShardID(shortCode string) int {
    hash := crc32.ChecksumIEEE([]byte(shortCode))
    return int(hash % numShards)
}
```

**Example with 4 shards:**
```
short_code "abc123" → hash → shard 0
short_code "xyz789" → hash → shard 2
short_code "def456" → hash → shard 1
```

**Architecture:**
```
Application
   ↓
[DBManager]
   ├─ Shard 0 (1 primary + 3 replicas)
   ├─ Shard 1 (1 primary + 3 replicas)
   ├─ Shard 2 (1 primary + 3 replicas)
   └─ Shard 3 (1 primary + 3 replicas)
```

**Write capacity:** 4 shards × 10,000 writes/sec = **40,000 writes/sec**

**Trade-offs:**
- **Pro:** Linear scaling (N shards = N× writes)
- **Con:** More operational complexity (4× databases to manage)
- **Con:** Can't join across shards (rarely needed for URL shortener)

### When to Shard?

**Current load:** 5,000 writes/sec
**Single primary capacity:** 10,000 writes/sec

**Threshold:** Shard when consistently hitting 70-80% capacity (7,000-8,000 writes/sec).

**Timeline:**
- Today: Single primary ✓
- 2x growth: Still single primary ✓
- 3x growth: Consider sharding
- 5x growth: Must shard

---

## Part 5: Redis Scaling

### Redis Sentinel (High Availability)

**Problem:** Single Redis instance is a single point of failure.

**Solution:** Redis Sentinel (automatic failover)

```
[Redis Primary]
   ↓ replication
[Redis Replica 1] [Redis Replica 2]

[Sentinel 1] [Sentinel 2] [Sentinel 3]
```

**How it works:**
- Sentinels monitor primary
- If primary fails, elect new primary from replicas
- Clients automatically reconnect to new primary

**Gains:**
- High availability (automatic failover)
- No downtime during primary failure

**Cost:** 3× Redis instances (primary + 2 replicas + 3 sentinels)

### Redis Cluster (Sharding)

**Problem:** Single Redis instance has limited:
- Memory (maxes out ~100GB)
- Throughput (100,000 ops/sec)

**Solution:** Redis Cluster (multiple nodes)

```
[Redis Node 1] - Slots 0-5460
[Redis Node 2] - Slots 5461-10922
[Redis Node 3] - Slots 10923-16383
```

**Sharding by key:**
```
CRC16(key) % 16384 → slot number → node
```

**Gains:**
- 3× memory (3 nodes × 100GB = 300GB)
- 3× throughput (300,000 ops/sec)

**Trade-offs:**
- Can't do multi-key operations (MGET across nodes)
- More complex failover

### When to Upgrade?

**Current:** Single Redis
**Upgrade to Sentinel:** When availability is critical (99.9% SLA)
**Upgrade to Cluster:** When >100GB memory or >100K ops/sec

---

## Part 6: Cost Analysis

### Current Scale (10,000 req/sec)

| Component | Instances | Cost/mo (AWS) |
|-----------|-----------|---------------|
| Redirect Service | 2× t3.medium | $60 |
| URL Service | 2× t3.medium | $60 |
| API Gateway | 1× t3.small | $15 |
| PostgreSQL (primary) | 1× db.t3.medium | $60 |
| PostgreSQL (replicas) | 3× db.t3.micro | $30 |
| ClickHouse | 1× c5.large | $70 |
| Redis | 1× cache.t3.micro | $15 |
| Workers | 3× t3.micro | $30 |
| **Total** | | **$340/month** |

### 10x Scale (100,000 req/sec)

| Component | Instances | Cost/mo |
|-----------|-----------|---------|
| Redirect Service | 20× t3.medium | $600 |
| URL Service | 10× t3.medium | $300 |
| API Gateway | 3× t3.small | $45 |
| PostgreSQL (primary) | 1× db.r5.xlarge | $350 |
| PostgreSQL (replicas) | 6× db.t3.small | $120 |
| ClickHouse | 3× c5.2xlarge (cluster) | $600 |
| Redis | 3× cache.m5.large (cluster) | $300 |
| Workers | 10× t3.small | $200 |
| **Total** | | **$2,515/month** |

**10x traffic = 7.4x cost** (economies of scale kick in)

### 100x Scale (1,000,000 req/sec)

| Component | Instances | Cost/mo |
|-----------|-----------|---------|
| Redirect Service | 200× t3.large | $12,000 |
| URL Service | 100× t3.large | $6,000 |
| API Gateway | 10× t3.medium | $300 |
| PostgreSQL | 4 shards × $1,000 | $4,000 |
| ClickHouse | 10 nodes × $600 | $6,000 |
| Redis | Redis Cluster × 10 nodes | $2,000 |
| Workers | 100× t3.medium | $4,000 |
| **Total** | | **$34,300/month** |

**100x traffic = 100x cost** (linear scaling)

---

## Part 7: When to Switch Technologies

### Redis Streams → Kafka

**Current:** Redis Streams (100,000 msgs/sec capacity)

**Switch to Kafka when:**
- Throughput: >100,000 msgs/sec consistently
- Retention: Need months/years of events
- Multiple consumers: Need complex routing (topics, partitions)
- Multi-datacenter: Need cross-region replication

**Cost:**
- Redis: $15/month (cache.t3.micro)
- Kafka: $500/month (3× kafka.m5.large)

**Don't switch if:** Redis Streams meets your needs. **Kafka is overkill for most use cases.**

### Monolith → Microservices

**Current:** Microservices (8 services)

**Alternative:** Combine into monolith
```
One app:
- REST API (external)
- gRPC services (internal)
- Background workers (goroutines)
```

**When microservices make sense:**
- Team >10 engineers (can own separate services)
- Need independent scaling (redirect vs URL creation)
- Different tech stacks (Go + Python for ML)

**When monolith is better:**
- Team <5 engineers
- Simpler ops (1 deployment vs 8)
- Lower latency (function calls vs gRPC)

---

## Summary

**Current capacity:** 50,000 redirects/sec + 5,000 creates/sec

**Bottlenecks at scale:**
1. Database writes (10,000/sec per primary)
2. Pipeline Worker enrichment (100 events/sec per worker)
3. Redis network bandwidth (250,000 ops/sec per instance)
4. ClickHouse inserts (50,000/sec per node)

**Scaling strategies:**
- **Stateless services:** Horizontal scaling (load balancer + N instances)
- **PostgreSQL:** Sharding by short_code (4 shards = 40,000 writes/sec)
- **Redis:** Cluster mode (3 nodes = 3× capacity)
- **ClickHouse:** Distributed tables (10 nodes = 10× writes)
- **Workers:** Add more instances (consumer groups distribute load)

**Cost:**
- 10K req/sec: $340/month
- 100K req/sec: $2,515/month (7x)
- 1M req/sec: $34,300/month (100x)

**Technology upgrades:**
- Redis Streams → Kafka: When >100K msgs/sec or need long retention
- Single Redis → Redis Cluster: When >100GB memory or >100K ops/sec
- Microservices → Monolith: For small teams (<5), simpler ops

**Key Insight:**
Start simple, scale incrementally. Our current architecture handles 50,000 req/sec. That's enough for 99% of URL shorteners. Only scale when you hit actual bottlenecks, not anticipated ones. Premature optimization is the root of all evil.

**When in doubt:**
1. Add more instances (horizontal scaling)
2. Monitor bottlenecks (database CPU, Redis memory, etc.)
3. Optimize hot paths (caching, indexing)
4. Only after exhausting above: Re-architect (sharding, clustering)

---

**Series Complete!**

You've now explored every aspect of the URL shortener architecture, from Snowflake IDs to database sharding. Ready to build your own or optimize an existing system.

---

**Word Count**: ~2,500 words
**Total Series**: ~35,700 words across 12 documents
