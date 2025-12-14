# Multi-Tier Caching Strategy

> **How we achieve sub-millisecond redirects without hammering the database**

## Overview

Caching is often treated as an afterthought - "we'll add Redis if we need it." But in a URL shortener, caching isn't optional. It's **fundamental to the architecture**.

Here's why: Every redirect requires looking up `short_code → long_url`. At 10,000 redirects/second, that's **10,000 database queries/second**. Even with read replicas, PostgreSQL will struggle. Query times spike from 5ms to 50ms+. Users notice the delay.

The solution? **Multi-tier caching** - two layers of caching with different trade-offs:
- **L1 Cache**: In-memory LRU (10,000 entries, nanosecond access)
- **L2 Cache**: Redis (millions of entries, sub-millisecond access)
- **L3 "Cache"**: PostgreSQL replicas (source of truth, millisecond access)

In this document, we'll explore:
1. **Why cache?** - The latency numbers that drive this decision
2. **L1 Cache Deep Dive** - LRU eviction, thread safety, Go implementation
3. **L2 Cache Deep Dive** - Redis TTL, shared state across instances
4. **Multi-Tier Flow** - How L1 and L2 work together
5. **Cache Invalidation** - The hard problem and our solution
6. **Performance Analysis** - Real numbers: hit rates, latency, throughput

By the end, you'll understand not just *what* we cache, but *why* two tiers and how the implementation works line-by-line.

---

## Part 1: Why Cache? (The Latency Numbers)

### Latency Numbers Every Programmer Should Know

Updated for 2024 (courtesy of Jeff Dean, adapted):

| Operation | Latency |
|-----------|---------|
| L1 cache reference | 0.5 ns |
| L2 cache reference | 7 ns |
| Main memory reference | **100 ns** |
| Send 2KB over 1 Gbps network | 20 μs |
| Read 1MB sequentially from SSD | 1 ms |
| **Round trip within same datacenter** | **500 μs** |
| **Database query (simple SELECT)** | **1-10 ms** |
| Disk seek | 10 ms |
| Round trip CA to Netherlands | 150 ms |

**Key insights:**
- **Memory is 10,000x faster than SSD**
- **Network calls add 500μs minimum** (even in same datacenter)
- **Database queries take 1-10ms** (includes network + disk + processing)

### Our Redirect Latency Budget

Target: **< 50ms for 95th percentile redirects**

Without caching:
```
Network to server:          ~10ms (from user to server)
Database query (replica):   ~5-10ms (SELECT long_url)
HTTP redirect response:     ~1ms
Total:                      ~16-21ms ✓ (meets target, but...)
```

Under load (10,000 req/sec):
```
Network to server:          ~10ms
Database query (replica):   ~50-100ms (!!!) (saturated connections)
HTTP redirect response:     ~1ms
Total:                      ~61-111ms ✗ (fails target)
```

**Problem**: Database can't sustain 10,000 queries/sec per replica. Even with 3 replicas (3,333 queries/sec each), connection pools saturate, queries queue up.

With L2 cache (Redis):
```
Network to server:          ~10ms
Redis GET:                  ~1-2ms (network + in-memory lookup)
HTTP redirect response:     ~1ms
Total:                      ~12-13ms ✓ (great!)
```

With L1 cache (in-memory):
```
Network to server:          ~10ms
In-memory lookup:           ~0.0001ms (100 nanoseconds)
HTTP redirect response:     ~1ms
Total:                      ~11ms ✓ (even better!)
```

**Conclusion**: Caching reduces redirect latency from 16-100ms to 11-13ms AND reduces database load from 10,000 queries/sec to ~10 queries/sec (99.9% cache hit rate).

---

## Part 2: L1 Cache - In-Memory LRU

### What is LRU?

**LRU = Least Recently Used**

An eviction policy that removes the **least recently accessed** item when the cache is full.

**Why LRU?**

**Temporal locality**: If a URL was accessed recently, it's likely to be accessed again soon. Popular URLs (viral links, campaign links) get hit thousands of times. LRU keeps "hot" URLs in cache.

**Alternatives:**
| Policy | Evicts | Pros | Cons | Use Case |
|--------|--------|------|------|----------|
| **LRU** | Least recently used | Good for hot data | O(1) with extra space | **Our choice** |
| **LFU** | Least frequently used | Optimal for frequency | Complex, O(log n) | Analytics workloads |
| **FIFO** | First in, first out | Simple | Doesn't consider popularity | Simple queues |
| **Random** | Random item | Very simple | Unpredictable | Low-importance caches |

LRU strikes the best balance: O(1) operations, captures temporal locality.

### LRU Implementation Deep Dive

From `internal/cache/lru.go`:

#### Data Structure

```go
type LRUCache struct {
    capacity int                            // Max number of entries
    cache    map[string]*list.Element       // Hash map for O(1) lookup
    lruList  *list.List                     // Doubly-linked list for ordering
    mu       sync.RWMutex                   // Read-write lock
}

type entry struct {
    key   string
    value interface{}
}
```

**Why two data structures?**

**Hash map (`cache`)**:
- Fast lookup: O(1)
- Stores `key → list element` mapping

**Doubly-linked list (`lruList`)**:
- Tracks access order (most recent at front, least recent at back)
- O(1) move-to-front operation
- O(1) remove from back (eviction)

**Together**: O(1) get, O(1) set, O(1) evict!

**Visual representation:**
```
Hash Map:
  "abc123" → *Element1
  "xyz789" → *Element2
  "def456" → *Element3

Doubly-Linked List (most recent → least recent):
  [abc123] ← → [def456] ← → [xyz789]
  ^front                      ^back
```

When "xyz789" is accessed:
1. Look up in hash map: O(1)
2. Move element to front of list: O(1)

```
  [xyz789] ← → [abc123] ← → [def456]
  ^front                      ^back
```

When cache is full and we need to evict:
1. Remove element from back: O(1)
2. Delete from hash map: O(1)

#### Get Operation (lru.go:28)

```go
func (c *LRUCache) Get(key string) (interface{}, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()

    if elem, found := c.cache[key]; found {
        c.lruList.MoveToFront(elem)  // Mark as recently used
        return elem.Value.(*entry).value, true
    }
    return nil, false
}
```

**Line-by-line:**

1. **`c.mu.Lock()`**: Acquire write lock
   - Why write lock for read? Because we modify the list (MoveToFront)
   - Could optimize with `sync.RWMutex` and track if we need to move, but simpler this way

2. **`if elem, found := c.cache[key]`**: O(1) hash map lookup

3. **`c.lruList.MoveToFront(elem)`**: Update recency
   - Doubly-linked list makes this O(1)
   - `container/list` maintains prev/next pointers
   - Removes elem from current position and inserts at front

4. **`elem.Value.(*entry).value`**: Extract value from list element
   - List elements store `interface{}`, we stored `*entry`
   - Type assertion to get our entry back

**Thread safety**: `sync.RWMutex` ensures concurrent access is safe.

#### Set Operation (lru.go:39)

```go
func (c *LRUCache) Set(key string, value interface{}) {
    c.mu.Lock()
    defer c.mu.Unlock()

    // If key exists, update value and move to front
    if elem, found := c.cache[key]; found {
        c.lruList.MoveToFront(elem)
        elem.Value.(*entry).value = value
        return
    }

    // New key: add to front
    elem := c.lruList.PushFront(&entry{key, value})
    c.cache[key] = elem

    // Evict if over capacity
    if c.lruList.Len() > c.capacity {
        c.evict()
    }
}
```

**Two cases:**

**Case 1: Key exists** (update)
- Move to front (mark as recently used)
- Update value
- No eviction needed

**Case 2: New key** (insert)
- Create entry
- Add to front of list
- Add to hash map
- Check capacity → evict if needed

**Why add to front?**
- This item was just accessed (set counts as access)
- It's the most recently used item
- Shouldn't be evicted next

#### Evict Operation (lru.go:67)

```go
func (c *LRUCache) evict() {
    elem := c.lruList.Back()  // Get least recently used (end of list)
    if elem != nil {
        c.lruList.Remove(elem)         // Remove from list
        delete(c.cache, elem.Value.(*entry).key)  // Remove from map
    }
}
```

**Simple and efficient:**
1. Get last element (least recently used)
2. Remove from list: O(1)
3. Remove from map: O(1)

**Note**: Not called unless cache is full (lazy eviction).

### Why 10,000 Entry Capacity?

**Memory calculation:**

Per entry:
- Key (string): 10 bytes (short code)
- Value (string): 100 bytes average (URL)
- Map overhead: ~48 bytes (pointer, hash, etc.)
- List element: ~48 bytes (prev, next, value pointers)
- **Total: ~200 bytes per entry**

10,000 entries = **2 MB of RAM**

**Coverage analysis:**

If 10% of URLs get 90% of traffic (Pareto principle):
- 1 million URLs in database
- Top 100,000 get 90% of traffic
- Top 10,000 get ~70% of traffic

**L1 cache with 10,000 entries: ~70% hit rate** (estimated).

**Why not 100,000 entries?**
- Memory: 20 MB (acceptable)
- Eviction performance: Still O(1), no issue
- **Diminishing returns**: 70% → 85% hit rate for 10x memory

**Why not 1,000 entries?**
- Only ~30% hit rate (estimated)
- More L2 cache hits (adds 1-2ms per request)

**10,000 is the sweet spot**: Good hit rate, low memory, fast eviction.

---

## Part 3: L2 Cache - Redis

### Why Redis?

**Compared to L1 (in-memory):**
| Feature | L1 (In-Memory) | L2 (Redis) |
|---------|----------------|------------|
| Latency | ~100 ns | ~1-2 ms |
| Capacity | 10,000 entries (~2 MB) | Millions (100+ GB) |
| Shared | No (per-instance) | Yes (across all instances) |
| Persistence | No (lost on restart) | Optional (RDB/AOF) |
| Eviction | LRU | LRU/TTL |

**Why both?**
- **L1 is faster** but limited capacity and not shared
- **L2 is shared** across all redirect service instances (cache warming benefits all)
- **Two-tier**: 70% hits in L1 (no network), 29% hits in L2 (network but no DB), 1% misses (DB query)

### Redis TTL Strategy

From `internal/cache/cache.go:41`:

```go
func (c *Cache) Set(ctx context.Context, key string, value string) error {
    c.l1Cache.Set(key, value)
    return c.l2Cache.Set(ctx, key, value, c.l2TTL).Err()
}
```

**`c.l2TTL`**: Configured as 1 hour (3600 seconds)

**Why 1 hour?**

**Too short (e.g., 5 minutes):**
- Frequent cache misses
- More database queries
- Less benefit from caching

**Too long (e.g., 24 hours):**
- Stale data longer (if URL is updated)
- Memory usage (more keys in Redis)
- Harder to detect deleted URLs

**1 hour balances:**
- Most URLs don't change frequently
- Popular URLs stay cached during traffic spikes
- Updated URLs eventually propagate
- Reasonable memory usage

**TTL refresh on access:**

Redis automatically refreshes TTL when using `SET`:
```go
c.l2Cache.Set(ctx, key, value, 1*time.Hour)
```

Every time a cache miss occurs and we fetch from DB:
1. Query database
2. Write to L2 with fresh 1-hour TTL
3. Write to L1 (no TTL, evicted by LRU)

**Result**: Hot URLs stay cached indefinitely (TTL refreshed on each miss).

### Redis Memory Management

**Eviction policy**: Configured in Redis config:
```
maxmemory 1gb
maxmemory-policy allkeys-lru
```

- `maxmemory`: Limit Redis memory usage
- `allkeys-lru`: When memory is full, evict least recently used keys (including our URL cache)

**Why `allkeys-lru` vs `volatile-lru`?**
- `volatile-lru`: Only evicts keys with TTL set (our case: all keys)
- `allkeys-lru`: Evicts any key (safer, but we only store cache data anyway)

Both work for us; `allkeys-lru` is more common.

---

## Part 4: Multi-Tier Cache Flow

### Read Path (Cache Lookup)

From `internal/cache/cache.go:25`:

```go
func (c *Cache) Get(ctx context.Context, key string) (string, bool) {
    // Try L1 first
    if val, found := c.l1Cache.Get(key); found {
        return val.(string), true  // Fast path: 100ns
    }

    // L1 miss, try L2
    val, err := c.l2Cache.Get(ctx, key).Result()
    if err == nil {
        c.l1Cache.Set(key, val)  // Populate L1 for next time
        return val, true          // Slower: ~1-2ms
    }

    // L2 miss
    return "", false  // Caller must query database
}
```

**Flow diagram:**

```
Request → L1 Cache?
          ↓ NO
          L2 Cache (Redis)?
          ↓ NO
          Database (PostgreSQL replica)
          ↓
          Populate L2 ← Set TTL = 1h
          ↓
          Populate L1 ← No TTL, LRU eviction
          ↓
          Return to caller
```

**Latency by path:**

| Path | Operations | Latency | Probability (estimated) |
|------|------------|---------|-------------------------|
| **L1 Hit** | In-memory lookup | ~0.0001 ms | ~70% |
| **L2 Hit** | Network + Redis GET | ~1-2 ms | ~29% |
| **L3 Miss** | Network + DB query | ~5-10 ms | ~1% |

**Expected redirect latency:**
```
0.70 × 0.0001ms + 0.29 × 1.5ms + 0.01 × 7.5ms
= 0.00007ms + 0.435ms + 0.075ms
= 0.51ms (just for cache/DB lookup)
```

Add network to user (~10ms) and response (~1ms): **~11.5ms total** ✓

### Write Path (Cache Population)

From `internal/cache/cache.go:39`:

```go
func (c *Cache) Set(ctx context.Context, key string, value string) error {
    c.l1Cache.Set(key, value)  // Write-through to L1
    return c.l2Cache.Set(ctx, key, value, c.l2TTL).Err()  // Write-through to L2
}
```

**Write-through strategy**: Always write to both caches when updating.

**Why write-through vs write-back?**

**Write-through** (our choice):
- Pros: L1 and L2 always consistent, simpler
- Cons: Slightly slower writes (wait for Redis SET)

**Write-back**:
- Pros: Faster writes (don't wait for Redis)
- Cons: L1 and L2 can diverge, complex error handling

For our use case, writes are rare (URL updates/deletes are infrequent vs reads). Write-through simplicity wins.

---

## Part 5: Cache Invalidation - The Hard Problem

> "There are only two hard things in Computer Science: cache invalidation and naming things." — Phil Karlton

### The Problem

Scenario:
1. URL "abc123" cached everywhere (L1 on all instances, L2 in Redis)
2. User updates URL: "abc123" now points to a different long URL
3. Old cached value is wrong!

**How to invalidate?**

### Our Solution: Explicit Invalidation

When URL is updated/deleted:

```go
// In URL Service (internal/service/url_service.go)
func (s *Service) UpdateURL(ctx context.Context, shortCode, newLongURL string) error {
    // Update database
    err := s.storage.Update(ctx, shortCode, newLongURL)
    if err != nil {
        return err
    }

    // Invalidate cache
    s.cache.Delete(ctx, "url:"+shortCode)

    return nil
}
```

**Cache Delete** (cache.go:44):
```go
func (c *Cache) Delete(ctx context.Context, key string) error {
    c.l1Cache.Delete(key)  // Remove from local L1
    return c.l2Cache.Del(ctx, key).Err()  // Remove from shared L2
}
```

**Problem**: This only invalidates L1 on the **current instance**. Other redirect service instances still have stale L1 cache!

### Solution: Cache Invalidation Events

**Approach**: Publish invalidation event to all instances.

**Implementation (not shown in current code, but recommended)**:

1. **Redis Pub/Sub** for invalidation events:

```go
// Publisher (URL Service)
func (s *Service) UpdateURL(ctx context.Context, shortCode, newLongURL string) error {
    // Update DB
    err := s.storage.Update(ctx, shortCode, newLongURL)

    // Publish invalidation event
    s.redis.Publish(ctx, "cache:invalidate", shortCode)

    // Delete from local cache
    s.cache.Delete(ctx, "url:"+shortCode)

    return err
}

// Subscriber (Redirect Service)
func (s *RedirectService) SubscribeCacheInvalidations() {
    pubsub := s.redis.Subscribe(ctx, "cache:invalidate")
    for msg := range pubsub.Channel() {
        shortCode := msg.Payload
        s.cache.l1Cache.Delete("url:" + shortCode)  // Invalidate L1
    }
}
```

**Result**: When any instance updates a URL, all instances invalidate their L1 cache.

### Alternative: TTL Reliance

**Current approach** (simpler):
- Rely on 1-hour TTL to eventually fix staleness
- Updates are rare, so 1-hour staleness is acceptable
- Trade-off: Simplicity vs perfect consistency

**When explicit invalidation matters:**
- Frequent updates (e.g., e-commerce prices)
- Critical consistency requirements (financial data)
- User-visible staleness causes support issues

For a URL shortener, 1-hour eventual consistency is fine.

---

## Part 6: Performance Analysis

### Cache Hit Rate Simulation

**Assumptions:**
- 1 million URLs in database
- Pareto distribution (80/20 rule)
- Top 10% of URLs get 90% of traffic

**L1 Cache (10,000 entries, LRU):**

- Top 10,000 URLs (1% of total) get ~70% of traffic
- **L1 hit rate: ~70%**

**L2 Cache (1 million entries, effectively unlimited):**

- After L1 miss (30% of traffic)
- L2 stores all accessed URLs
- L2 hit rate of remaining traffic: ~97%
- **Combined L2 hit rate: 30% × 97% = ~29%**

**Database Queries (L1 + L2 miss):**

- Remaining: 30% × 3% = ~1% of traffic

**Summary:**
| Tier | Hit Rate | Latency | Database Queries Avoided |
|------|----------|---------|--------------------------|
| L1 | 70% | ~0.0001 ms | 70% |
| L2 | 29% | ~1-2 ms | 29% |
| DB | 1% | ~5-10 ms | 0% |

**Result**: **99% cache hit rate** (L1 + L2 combined)

### Throughput Impact

**Without caching:**
- 10,000 redirects/sec = 10,000 database queries/sec
- Each replica handles 3,333 queries/sec (3 replicas)
- Near capacity limit → high latency

**With multi-tier caching:**
- 10,000 redirects/sec
- 99% cache hit rate
- Database queries: 10,000 × 1% = **100 queries/sec**
- Each replica: 33 queries/sec (under capacity → low latency)

**Database load reduced by 100x** 🚀

### Memory Usage

**L1 (per instance):**
- 10,000 entries × 200 bytes = 2 MB per instance
- 10 redirect instances = 20 MB total

**L2 (Redis):**
- 1 million entries × 150 bytes (Redis overhead lower) = 150 MB
- Overhead + metadata: ~300 MB total

**Total memory cost: 320 MB** for 99% cache hit rate. Excellent ROI.

---

## Summary

**What we covered:**

**Why Cache:**
- Database queries take 5-10ms, memory access takes 0.0001ms
- At 10,000 req/sec, database can't sustain load
- Caching reduces latency from 16-100ms to 11-13ms

**L1 Cache (In-Memory LRU):**
- LRU eviction policy: Least Recently Used
- Doubly-linked list + hash map = O(1) operations
- 10,000 entry capacity = 2 MB RAM
- 70% hit rate (estimated)
- Thread-safe with sync.RWMutex

**L2 Cache (Redis):**
- Shared across all instances
- 1-hour TTL balances freshness vs performance
- Millions of entries possible
- 29% hit rate (of remaining traffic after L1)

**Multi-Tier Flow:**
- L1 → L2 → Database (waterfall lookup)
- Write-through strategy (update both caches on write)
- Combined 99% hit rate
- Database load reduced 100x

**Cache Invalidation:**
- Explicit delete on updates
- TTL provides eventual consistency
- Optional: Redis Pub/Sub for immediate invalidation across instances
- Trade-off: Simplicity vs perfect consistency

**Performance:**
- Expected redirect latency: ~11.5ms
- Database queries: 100/sec (vs 10,000/sec without cache)
- Memory cost: 320 MB for 99% hit rate

**Key Insight:**
Multi-tier caching isn't just about performance - it's about **scalability**. Without caching, you'd need 10x more database replicas. With caching, you need 100x fewer queries. The memory cost (320 MB) is trivial compared to database scaling costs.

**When to use single-tier:**
- Traffic < 100 req/sec (database can handle it)
- Cost-sensitive (save Redis hosting)
- Simpler ops (one less system)

**When multi-tier shines:**
- Traffic > 1,000 req/sec
- Need sub-10ms response times
- Want to scale horizontally (shared L2 cache benefits all instances)

---

**Up next**: [Short Code Generation (Snowflake + Base62) →](./05-short-code-generation.md)

Learn how we generate unique, URL-safe short codes using Snowflake IDs and Base62 encoding, and why this approach guarantees no collisions across distributed servers.

---

**Word Count**: ~3,100 words
**Reading Time**: ~15 minutes
**Code References**:
- `internal/cache/cache.go`
- `internal/cache/lru.go`
