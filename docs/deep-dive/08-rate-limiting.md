# Rate Limiting with Sliding Window

> **Protecting APIs from abuse using Redis sorted sets**

## Overview

Rate limiting is essential for any public API. Without it, a single client could overwhelm your servers, causing downtime for everyone.

Our URL shortener implements **sliding window rate limiting** using Redis sorted sets: **100 requests per minute per IP address**.

In this document:
1. **Why Rate Limit?** - Protection, fairness, cost control
2. **Algorithm Comparison** - Token bucket, leaky bucket, fixed window, sliding window
3. **Sliding Window Deep Dive** - How it works with Redis sorted sets
4. **Implementation** - Line-by-line code walkthrough
5. **Response Headers** - Communicating limits to clients

---

## Part 1: Why Rate Limit?

### 1. Prevent Abuse

**Scenario**: Malicious actor tries to overload your system.

Without rate limiting:
```
Attacker script:
while true; do
  curl http://api.example.com/urls
done

Result: 10,000+ requests/second → server crashes
```

With rate limiting (100 req/min):
```
Request 1-100: ✓ Allowed
Request 101:   ✗ Blocked (429 Too Many Requests)
Attacker gives up or is temp-banned
```

### 2. Fair Resource Allocation

**Problem**: One user's heavy usage affects everyone.

```
User A: 1,000 requests/second (scraping?)
User B: 10 requests/second (normal usage)

Without rate limiting:
- Database saturated by User A
- User B's requests timeout
```

With rate limiting:
- Each user limited to fair share (100 req/min)
- User A's excess requests rejected
- User B unaffected

### 3. Cost Control

**Cloud services charge per request:**
- Database queries cost money
- Bandwidth costs money
- CPU time costs money

Rate limiting prevents runaway costs from abuse or bugs.

### 4. API Stability

Graceful degradation:
```
System at capacity → Rate limit kicks in → Reject excess → System stable
```

Without rate limiting:
```
System at capacity → Keeps accepting requests → Queues fill → Crashes
```

---

## Part 2: Rate Limiting Algorithms

### Algorithm 1: Fixed Window

**Concept**: Allow N requests per fixed time window (e.g., per minute starting at :00).

```
Window: 10:00:00 - 10:00:59
Limit: 100 requests

Timestamp   Request Count   Allowed?
10:00:05    1               ✓
10:00:30    50              ✓
10:00:59    100             ✓ (last one)
10:01:00    1               ✓ (new window)
```

**Problem: Burst at window boundary**
```
10:00:59  →  100 requests  ✓ (allowed)
10:01:00  →  100 requests  ✓ (allowed)
Total: 200 requests in 2 seconds!
```

### Algorithm 2: Token Bucket

**Concept**: Tokens refill at constant rate. Each request consumes 1 token.

```
Bucket capacity: 100 tokens
Refill rate: 100 tokens/minute

Tokens  Action
100     Request → 99 tokens
99      Request → 98 tokens
0       Request → Rejected (no tokens)
        Wait 0.6 seconds → 1 token refilled
```

**Pros**: Smooth rate, allows bursts if bucket full
**Cons**: More complex, requires tracking refill time

### Algorithm 3: Leaky Bucket

**Concept**: Requests enter queue, processed at fixed rate.

```
Queue (10 max)  |  Processing (10/sec)
[req req req]  →→→  [out]
```

**Pros**: Perfectly smooth output rate
**Cons**: Adds latency (queueing), more complex

### Algorithm 4: Sliding Window (Our Choice)

**Concept**: Count requests in last N seconds (rolling window).

```
Current time: 10:00:30
Window: 60 seconds (look back to 9:59:30)

Timestamp   Count in last 60s   Allowed?
9:59:45     1                    ✓
10:00:15    50                   ✓
10:00:30    100                  ✗ (limit reached)
10:00:45    99 (9:59:45 fell out)  ✓ (under limit again)
```

**Pros:**
- No burst at boundaries (true sliding)
- Fair across time
- Simple with Redis sorted sets

**Cons:**
- Slightly more storage (track each request timestamp)

**Why we chose sliding window:**
- **Fairness**: No boundary exploits
- **Simple with Redis**: Sorted sets perfect for this
- **Accurate**: True rate over time, not approximation

---

## Part 3: Sliding Window with Redis Sorted Sets

### Redis Sorted Set Basics

A sorted set stores members with scores:
```
ZADD myset 100 "member1"
ZADD myset 200 "member2"
ZADD myset 150 "member3"

Sorted order (by score):
member1 (100)
member3 (150)
member2 (200)
```

**Key operations:**
- `ZADD`: Add member with score
- `ZREMRANGEBYSCORE`: Remove members in score range
- `ZCARD`: Count members
- `ZRANGE`: Get members by rank

### Our Data Structure

```
Key: "ratelimit:192.168.1.1"
Score: Request timestamp (nanoseconds)
Member: Request ID (also timestamp)

Example:
ratelimit:192.168.1.1 → {
  1704067200000000000: "1704067200000000000",
  1704067201000000000: "1704067201000000000",
  1704067202000000000: "1704067202000000000",
  ...
}
```

**Why timestamp as both score and member?**
- **Score**: For range queries (remove old requests)
- **Member**: Must be unique (same timestamp possible, so we use nanoseconds)

### Algorithm Steps

From `internal/middleware/ratelimit.go:49-96`:

```go
func (rl *RateLimiter) allowRequest(ctx context.Context, key string) (bool, int, time.Time) {
    now := time.Now()
    windowStart := now.Add(-rl.window)  // 60 seconds ago

    pipe := rl.redis.Pipeline()  // Batch commands

    // Step 1: Remove old requests (outside window)
    pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

    // Step 2: Count remaining requests
    zcard := pipe.ZCard(ctx, key)

    // Step 3: Add current request
    pipe.ZAdd(ctx, key, redis.Z{
        Score:  float64(now.UnixNano()),
        Member: fmt.Sprintf("%d", now.UnixNano()),
    })

    // Step 4: Set TTL (cleanup old keys)
    pipe.Expire(ctx, key, rl.window)

    // Execute all commands atomically
    _, err := pipe.Exec(ctx)

    count := int(zcard.Val())

    // Step 5: Check if over limit
    if count >= rl.limit {
        return false, 0, resetTime  // Reject
    }

    return true, remaining, resetTime  // Allow
}
```

**Visual walkthrough:**

```
Current time: 10:00:30
Window: 60 seconds
Limit: 100 requests/minute

Step 1: Remove requests older than 9:59:30
  Before: [9:59:20, 9:59:25, 9:59:35, 10:00:10, 10:00:20]
  After:  [9:59:35, 10:00:10, 10:00:20]  (removed 9:59:20, 9:59:25)

Step 2: Count remaining = 3

Step 3: Add current request
  After:  [9:59:35, 10:00:10, 10:00:20, 10:00:30]

Step 4: Set TTL = 60 seconds
  (Key auto-deleted if no requests for 60s)

Step 5: Check limit
  count = 3 < 100 → Allow request
```

**Key insight**: We remove old requests before counting. This ensures we only count requests in the current window.

### Why Pipeline?

```go
pipe := rl.redis.Pipeline()
pipe.ZRemRangeByScore(...)
pipe.ZCard(...)
pipe.ZAdd(...)
pipe.Expire(...)
pipe.Exec()  // Execute all at once
```

**Without pipeline** (4 network round-trips):
```
Client → Redis: ZREMRANGEBYSCORE → Client
Client → Redis: ZCARD → Client
Client → Redis: ZADD → Client
Client → Redis: EXPIRE → Client
Total: ~4ms (1ms per round-trip)
```

**With pipeline** (1 network round-trip):
```
Client → Redis: [ZREMRANGEBYSCORE, ZCARD, ZADD, EXPIRE] → Client
Total: ~1ms
```

**4x faster!** Critical for rate limiting (must be fast).

---

## Part 4: Implementation Details

### Getting Client IP

From `ratelimit.go:98`:

```go
func getClientIP(r *http.Request) string {
    // Check X-Forwarded-For (behind proxy/load balancer)
    forwarded := r.Header.Get("X-Forwarded-For")
    if forwarded != "" {
        // Format: "client, proxy1, proxy2"
        // Take first IP (client's real IP)
        return strings.Split(forwarded, ",")[0]
    }

    // Direct connection
    return r.RemoteAddr
}
```

**Why X-Forwarded-For?**

When behind a load balancer:
```
Client (1.2.3.4) → Load Balancer → Server

r.RemoteAddr = load balancer IP (not useful!)
X-Forwarded-For = 1.2.3.4 (client's real IP)
```

**Security concern**: X-Forwarded-For can be spoofed! Trust only if behind a trusted proxy.

### Rate Limit Headers

From `ratelimit.go:35-37`:

```go
w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.limit))
w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))
```

**Example response:**
```http
HTTP/1.1 200 OK
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 42
X-RateLimit-Reset: 1704067260

...response body...
```

**Client benefits:**
- Know how many requests left
- Know when limit resets
- Can pace requests intelligently

### 429 Response (Rate Limited)

From `ratelimit.go:39-43`:

```go
if !allowed {
    w.Header().Set("Retry-After", fmt.Sprintf("%d", int(time.Until(resetTime).Seconds())))
    http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
    return
}
```

**Example:**
```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1704067260
Retry-After: 30

Rate limit exceeded
```

**`Retry-After: 30`** tells client: "Wait 30 seconds before trying again."

**Standard HTTP status codes:**
- `200 OK`: Request allowed
- `429 Too Many Requests`: Rate limited
- `503 Service Unavailable`: Server overloaded (different from rate limit)

---

## Part 5: Configuration

### Our Limits

```go
limit := 100            // requests
window := 1 * time.Minute  // per minute
```

**100 requests/minute = 1.67 requests/second**

**Why 100/minute?**

**Too low (e.g., 10/minute):**
- Legitimate users hit limit
- Poor UX (constant 429 errors)

**Too high (e.g., 10,000/minute):**
- Doesn't prevent abuse
- Servers still overloaded

**100/minute is reasonable:**
- Normal users: 1-10 requests/minute (well under limit)
- Aggressive scrapers: Blocked quickly
- API explorers: Can test without hitting limit

### Per-User vs Per-IP

**Our choice**: Per-IP (simpler)

**Alternative**: Per-user (requires authentication)
```go
// Instead of IP
key := "ratelimit:user:" + userID
```

**Pros:**
- Can't bypass by changing IP
- More accurate (one user, one limit)

**Cons:**
- Requires authentication on all endpoints
- Shared IPs (offices, schools) share limit

**Hybrid approach** (best of both):
- Anonymous users: Per-IP (100/min)
- Authenticated users: Per-user (1000/min, higher trust)

---

## Part 6: Edge Cases

### 1. Redis Down

```go
_, err := pipe.Exec(ctx)
if err != nil {
    // Fail open: allow request
    return true, rl.limit, now.Add(rl.window)
}
```

**Philosophy**: Fail open (allow requests) rather than fail closed (reject all).

**Trade-off:**
- **Availability**: Service stays up
- **Security**: Brief window without rate limiting

**Alternative**: Fail closed (safer but worse UX if Redis has issues).

### 2. Clock Skew

If server clock jumps backwards:
```
Request 1: Timestamp 10:00:30 (now)
*Clock jumps back 10 seconds*
Request 2: Timestamp 10:00:20 (now?!)
```

**Impact**: Request 2 timestamp older than Request 1, but added later. Sorted set order slightly off.

**Mitigation**: Use NTP to sync clocks. Small skew (<1 second) is harmless.

### 3. Memory Usage

Per IP:
```
100 requests × 8 bytes (nanosecond timestamp) = 800 bytes
1,000 IPs × 800 bytes = 800 KB
```

**With TTL**: Old keys auto-deleted after 60 seconds of inactivity.

**Max memory**: Even with 100,000 active IPs: 80 MB (acceptable).

---

## Summary

**Sliding Window Algorithm:**
- Count requests in last N seconds (rolling window)
- Fair, no boundary bursts
- Implemented with Redis sorted sets

**Implementation:**
- Score: Request timestamp (nanoseconds)
- Remove old requests before counting
- Redis pipeline for performance (4x faster)
- Fail open if Redis unavailable

**Configuration:**
- 100 requests/minute per IP
- Rate limit headers inform clients
- 429 status code with Retry-After

**Key Advantages:**
- **Accurate**: True rate over sliding window
- **Fast**: O(log N) Redis operations, pipelined
- **Simple**: ~100 lines of code
- **Standard**: HTTP headers match industry practice

**Trade-offs:**
- **Storage**: 8 bytes per request (vs token bucket: 1 counter)
- **Fairness**: Perfect rate limiting vs simpler algorithms

**Key Insight:**
Redis sorted sets are perfect for sliding window rate limiting. The score (timestamp) naturally expires old requests, and ZCARD gives us the count. This wouldn't be possible with simple key-value stores.

---

**Up next**: [Workers & Background Processing →](./09-workers.md)

Learn how Analytics Worker, Pipeline Worker, and Cleanup Worker process events asynchronously, and why we separated them into distinct services.

---

**Word Count**: ~2,300 words
**Code References**: `internal/middleware/ratelimit.go`
