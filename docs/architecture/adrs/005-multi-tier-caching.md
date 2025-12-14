# ADR-005: Multi-Tier Caching Strategy

## Status
Accepted

## Context

URL redirects are the highest-volume operation. Each redirect needs to:
1. Look up short_code → long_url mapping
2. Return as fast as possible (<50ms target)

Hot URLs (viral links) may receive thousands of requests per second. Database can't handle this directly.

Options considered:
1. **Database only** - Rely on PostgreSQL with connection pooling
2. **Single Redis cache** - All lookups go through Redis
3. **CDN caching** - Cache redirects at edge
4. **Multi-tier (L1 + L2)** - Local memory + Redis

## Decision

Implement two-tier caching: L1 (in-memory LRU) + L2 (Redis).

**Architecture:**
```
Request → L1 (Local LRU) → L2 (Redis) → Database
              ↓ hit            ↓ hit         ↓ hit
           Return          Return +      Return +
                          Populate L1   Populate L1 + L2
```

**L1 Cache (per instance):**
- Type: In-memory LRU
- Capacity: 10,000 entries
- TTL: None (evicted by LRU)
- Latency: <1ms

**L2 Cache (shared):**
- Type: Redis
- Capacity: Limited by Redis memory
- TTL: 1 hour
- Latency: 1-5ms

## Consequences

### Positive
- **Blazing fast hot URLs** - L1 hits under 1ms, no network
- **Reduced Redis load** - L1 absorbs repeated requests
- **Graceful degradation** - L1 works if Redis is down temporarily
- **Memory efficient** - Only hot URLs stay in L1

### Negative
- **Cache inconsistency** - L1 caches are per-instance, may diverge
- **Memory usage** - Each instance uses memory for L1
- **Complexity** - Two caches to reason about
- **Cold start** - New instances have empty L1

### Cache invalidation strategy
```
URL Created:
  → Write to Database
  → SET in Redis (L2)
  → L1 not populated (lazy)

URL Deleted:
  → Delete from Database
  → DEL from Redis (L2)
  → L1 entry expires naturally or on next miss

URL Expired:
  → Database query filters by expires_at
  → Redis TTL handles expiration
  → L1 may serve stale briefly (acceptable)
```

### Hit rate expectations
```
Typical traffic pattern (Zipf distribution):
  - 20% of URLs get 80% of traffic
  - Top 1000 URLs get 50% of traffic

With 10K entry L1 cache:
  - L1 hit rate: ~60-70%
  - L2 hit rate: ~25-35%
  - Database: ~5%

At 10K redirects/sec:
  - 6,500 L1 hits (0.5ms each)
  - 3,000 L2 hits (3ms each)
  - 500 DB hits (15ms each)
  - Average latency: ~4ms
```

### Configuration
```go
// L1 Cache
l1Cache := cache.NewLRUCache(10000)

// L2 Cache (Redis)
l2TTL := 1 * time.Hour

// Multi-tier wrapper
cache := cache.NewMultiTierCache(l1Cache, redisClient, l2TTL)
```

## References
- [Caching Strategies](https://codeahoy.com/2017/08/11/caching-strategies-and-how-to-choose-the-right-one/)
- [LRU Cache Implementation](https://en.wikipedia.org/wiki/Cache_replacement_policies#Least_recently_used_(LRU))
