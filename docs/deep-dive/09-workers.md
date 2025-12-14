# Workers & Background Processing

> **Three specialized workers: Why separate them?**

## Overview

Our URL shortener has **three background workers** that process click events asynchronously:

1. **Analytics Worker** - Updates PostgreSQL click counts
2. **Pipeline Worker** - Enriches events and stores in ClickHouse
3. **Cleanup Worker** - Deletes expired URLs

Why three separate services instead of one? Each has different responsibilities, performance characteristics, and failure modes. Separating them provides **fault isolation** and **independent scaling**.

In this document:
1. **Why Async Processing?** - Decoupling from user-facing requests
2. **Analytics Worker** - Click count aggregation
3. **Pipeline Worker** - GeoIP enrichment and OLAP storage
4. **Cleanup Worker** - Periodic maintenance
5. **Consumer Groups** - How workers share load
6. **Error Handling** - Retries, dead letters, idempotency

---

## Part 1: Why Background Workers?

### The Problem: Slow Operations Block Users

When a user clicks a short URL:
```
Fast operations (must be < 50ms):
- Cache lookup: 1ms
- HTTP redirect: 1ms
Total: 2ms ✓

Slow operations (10-100ms):
- GeoIP lookup: 10ms
- User-agent parsing: 5ms
- Database insert (PostgreSQL): 10ms
- ClickHouse insert: 20ms
Total: 45ms ✗ (too slow!)
```

**Solution**: Do fast operations synchronously, slow operations asynchronously.

```
User clicks → [Fast: Cache + Redirect (2ms)] → User redirected ✓
               ↓
          [Publish event to queue]
               ↓
          [Workers process in background (45ms)]
```

### Benefits of Async Processing

**1. Fast user experience**
- Redirect in 2ms instead of 47ms
- Users don't wait for analytics processing

**2. Fault tolerance**
- If ClickHouse is down, redirects still work
- Events queued until ClickHouse recovers

**3. Independent scaling**
- Add more workers without touching redirect service
- Scale based on queue depth

**4. Batch processing**
- Workers process 100 events at once (more efficient)
- Redirect service processes 1 at a time (required)

---

## Part 2: Analytics Worker - PostgreSQL Click Counts

### Purpose

Update the `urls.clicks` counter in PostgreSQL for each click event.

```sql
UPDATE urls SET clicks = clicks + 1 WHERE short_code = 'abc123';
```

**Why this worker?**

Users want to see click counts in their dashboard:
```
Your URLs:
- tiny.ly/abc123  →  1,234 clicks
- tiny.ly/xyz789  →  567 clicks
```

This data lives in PostgreSQL (same database as URL metadata).

### Architecture

From `cmd/analytics-worker/main.go` (simplified):

```go
func main() {
    // Connect to Redis Streams
    redis := redis.NewClient(...)

    // Connect to PostgreSQL
    db := connectPostgreSQL(...)

    // Create consumer group if doesn't exist
    redis.XGroupCreateMkStream(ctx, "clicks:stream", "analytics-group", "0")

    // Main loop
    for {
        // Read batch of click events
        results := redis.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    "analytics-group",
            Consumer: "analytics-worker",
            Streams:  []string{"clicks:stream", ">"},
            Count:    100,        // Batch size
            Block:    5 * time.Second,  // Wait up to 5s for events
        })

        // Aggregate clicks by short_code
        clickCounts := make(map[string]int)
        messageIDs := []string{}

        for _, result := range results {
            for _, msg := range result.Messages {
                shortCode := msg.Values["short_code"].(string)
                clickCounts[shortCode]++
                messageIDs = append(messageIDs, msg.ID)
            }
        }

        // Batch update PostgreSQL
        tx, _ := db.Begin()
        for shortCode, count := range clickCounts {
            tx.Exec("UPDATE urls SET clicks = clicks + $1, updated_at = NOW() WHERE short_code = $2",
                count, shortCode)
        }
        tx.Commit()

        // Acknowledge messages
        redis.XAck(ctx, "clicks:stream", "analytics-group", messageIDs...)
    }
}
```

### Key Design Decisions

**1. Batch Processing (100 events)**

Why not process 1 at a time?

**One at a time:**
```
100 events = 100 database transactions
Time: 100 × 10ms = 1 second
```

**Batched:**
```
100 events = 1 transaction with 100 UPDATEs (or less if same short_code)
Time: ~20ms (50x faster!)
```

**2. Aggregation in Memory**

```go
clickCounts := make(map[string]int)
for _, msg := range messages {
    shortCode := msg.Values["short_code"].(string)
    clickCounts[shortCode]++  // Aggregate duplicates
}
```

**Why?**

100 events might be:
```
abc123, abc123, xyz789, abc123, xyz789, abc123, ...
```

After aggregation:
```
abc123: 50 clicks
xyz789: 30 clicks
other: 20 clicks
```

**3 UPDATE statements instead of 100!**

**3. Transaction for Consistency**

```go
tx, _ := db.Begin()
// Multiple UPDATEs
tx.Commit()
```

All-or-nothing: Either all click counts update, or none. Prevents partial updates if database crashes mid-batch.

### Performance

**Throughput:**
- 100 events/second: 1 batch/second (easy)
- 10,000 events/second: 100 batches/second (still manageable)

**Latency:**
- Event published at T=0
- Worker polls at T=1s (next poll cycle)
- Processing takes 20ms
- Database updated at T=1.02s

**1-2 second delay acceptable for click counts** (not real-time critical).

---

## Part 3: Pipeline Worker - ClickHouse Analytics

### Purpose

Enrich click events with GeoIP and user-agent data, then store in ClickHouse for OLAP queries.

```
Raw event:
{
  short_code: "abc123",
  ip: "1.2.3.4",
  user_agent: "Mozilla/5.0 ...",
}

Enriched event:
{
  short_code: "abc123",
  ip: "1.2.3.4",
  country: "US",
  city: "San Francisco",
  latitude: 37.7749,
  longitude: -122.4194,
  browser: "Chrome",
  browser_version: "120.0",
  os: "Windows",
  device_type: "desktop",
  ...
}
```

### Architecture

From `cmd/pipeline-worker/main.go` (simplified):

```go
func main() {
    redis := redis.NewClient(...)
    clickhouse := connectClickHouse(...)

    // GeoIP database (MaxMind)
    geoIP, _ := geoip2.Open("GeoLite2-City.mmdb")

    redis.XGroupCreateMkStream(ctx, "clicks:stream", "analytics-group", "0")

    for {
        results := redis.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    "analytics-group",
            Consumer: "pipeline-worker",
            Streams:  []string{"clicks:stream", ">"},
            Count:    100,
            Block:    5 * time.Second,
        })

        enrichedEvents := []ClickEvent{}
        messageIDs := []string{}

        for _, result := range results {
            for _, msg := range result.Messages {
                // Extract raw data
                shortCode := msg.Values["short_code"].(string)
                ipStr := msg.Values["ip"].(string)
                userAgent := msg.Values["user_agent"].(string)

                // Enrich with GeoIP
                ip := net.ParseIP(ipStr)
                record, _ := geoIP.City(ip)
                country := record.Country.Names["en"]
                city := record.City.Names["en"]
                lat := record.Location.Latitude
                lon := record.Location.Longitude

                // Parse user-agent
                ua := useragent.Parse(userAgent)
                browser := ua.Browser
                os := ua.OS
                device := ua.Device

                enrichedEvents = append(enrichedEvents, ClickEvent{
                    ShortCode: shortCode,
                    IP: ipStr,
                    Country: country,
                    City: city,
                    Latitude: lat,
                    Longitude: lon,
                    Browser: browser,
                    OS: os,
                    DeviceType: device,
                    ...
                })

                messageIDs = append(messageIDs, msg.ID)
            }
        }

        // Batch insert to ClickHouse
        clickhouse.InsertBatch(enrichedEvents)

        // Acknowledge
        redis.XAck(ctx, "clicks:stream", "analytics-group", messageIDs...)
    }
}
```

### GeoIP Enrichment

**MaxMind GeoLite2 Database:**
- ~3.5 million IP ranges mapped to locations
- Stored in memory-mapped file (fast lookups)
- Lookup time: ~5-10ms

**Example:**
```
Input:  IP 8.8.8.8
Output: {
  Country: "United States",
  City: "Mountain View",
  Latitude: 37.4056,
  Longitude: -122.0775,
  Timezone: "America/Los_Angeles"
}
```

**Accuracy**: ~55-80% accuracy at city level (IP geolocation is approximate).

### User-Agent Parsing

**Input:**
```
Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36
```

**Parsed:**
```
Browser: Chrome
Browser Version: 120.0
OS: Windows
OS Version: 10
Device: Desktop
```

**Library**: `github.com/mssola/user_agent` (Go)

### Why Separate from Analytics Worker?

**Different responsibilities:**
- Analytics Worker: Simple counter updates (fast)
- Pipeline Worker: Heavy enrichment (slow)

**Different performance:**
- Analytics Worker: 20ms per batch
- Pipeline Worker: 100ms per batch (GeoIP + parsing)

**Fault isolation:**
- If GeoIP database is corrupt, Pipeline Worker fails
- Analytics Worker keeps running (click counts still update)

**Independent scaling:**
- Need more enrichment capacity? Add Pipeline Workers
- Analytics Worker scales separately

---

## Part 4: Cleanup Worker - Expired URL Deletion

### Purpose

Periodically delete URLs that have expired.

```sql
DELETE FROM urls WHERE expires_at < NOW();
```

### Architecture

From `cmd/cleanup-worker/main.go` (simplified):

```go
func main() {
    db := connectPostgreSQL(...)
    cache := connectRedis(...)

    ticker := time.NewTicker(24 * time.Hour)  // Run daily

    for range ticker.C {
        log.Info("Starting cleanup job...")

        // Find expired URLs
        rows, _ := db.Query("SELECT short_code FROM urls WHERE expires_at < NOW()")

        shortCodes := []string{}
        for rows.Next() {
            var shortCode string
            rows.Scan(&shortCode)
            shortCodes = append(shortCodes, shortCode)
        }

        // Delete from database
        result, _ := db.Exec("DELETE FROM urls WHERE expires_at < NOW()")
        deletedCount := result.RowsAffected()

        // Invalidate cache
        for _, code := range shortCodes {
            cache.Del(ctx, "url:"+code)
        }

        log.Infof("Cleanup complete: %d URLs deleted", deletedCount)
    }
}
```

### Why Not Database TTL?

**Some databases support TTL:**
- Redis: `EXPIRE key seconds`
- DynamoDB: TTL attribute
- MongoDB: TTL indexes

**PostgreSQL does NOT have native TTL:**
- Must manually DELETE expired rows
- Cleanup worker provides this functionality

**Alternative**: PostgreSQL extension `pg_cron` (schedule jobs in database). We chose external worker for simplicity.

### Why Daily Instead of Real-Time?

**Real-time deletion:**
```
User requests URL → Check expiry → Delete if expired → 404
```

**Problems:**
- Every request checks expiry (slow)
- Frequent deletes fragment tables (performance issue)

**Daily cleanup:**
```
Once per day: Delete all expired URLs in one batch
```

**Benefits:**
- Single efficient batch DELETE (faster)
- No per-request overhead
- PostgreSQL can vacuum/optimize in one go

**Trade-off:** Expired URLs remain accessible for up to 24 hours after expiry. **Acceptable** for most use cases.

### Graceful Handling

**What if cleanup fails?**
- Next day's run will catch the URLs
- URLs remain accessible (better than losing non-expired URLs)

**What if cleanup never runs?**
- Database grows (storage cost)
- Eventually need manual cleanup

**Monitoring:** Alert if cleanup hasn't run in 48 hours.

---

## Part 5: Consumer Groups - Load Balancing

### The Problem

One stream, two consumers (Analytics Worker + Pipeline Worker).

**Without consumer groups:**
```
Event 1 → Both workers process it (duplicate!)
Event 2 → Both workers process it (duplicate!)
```

**With consumer groups:**
```
Event 1 → Analytics Worker processes
Event 2 → Pipeline Worker processes
Event 3 → Analytics Worker processes
```

Each event delivered to **exactly one consumer** per group.

### How Consumer Groups Work

```go
// Create group
redis.XGroupCreateMkStream(ctx, "clicks:stream", "analytics-group", "0")

// Worker 1 joins group
redis.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    "analytics-group",
    Consumer: "analytics-worker",  // Unique consumer name
    Streams:  []string{"clicks:stream", ">"},
})

// Worker 2 joins same group
redis.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    "analytics-group",
    Consumer: "pipeline-worker",  // Different consumer name
    Streams:  []string{"clicks:stream", ">"},
})
```

**Redis tracks:**
- Which messages each consumer has seen
- Pending messages (delivered but not ACK'd)
- Consumer offsets

### Horizontal Scaling

**Add more Analytics Workers:**
```
analytics-worker-1  ← Event 1, 4, 7, ...
analytics-worker-2  ← Event 2, 5, 8, ...
analytics-worker-3  ← Event 3, 6, 9, ...
```

Redis automatically load-balances.

**No code changes needed!** Just start more instances with same consumer group.

---

## Part 6: Error Handling

### At-Least-Once Delivery

**Scenario:**
```
1. Worker reads event
2. Worker processes event (UPDATE database)
3. Worker crashes before XACK
```

**Result:** Event redelivered to another worker → database updated TWICE!

### Solution 1: Idempotent Operations

```sql
-- Idempotent (same result if run twice)
UPDATE urls SET clicks = clicks + 1 WHERE short_code = 'abc123';

-- NOT idempotent
INSERT INTO clicks (short_code, timestamp) VALUES ('abc123', NOW());
```

**Analytics Worker uses idempotent UPDATEs** → safe to process twice.

### Solution 2: Deduplication (ClickHouse)

```go
// Generate UUID before publishing
event := ClickEvent{
    EventID: uuid.New().String(),  // Unique
    ShortCode: "abc123",
    ...
}
```

ClickHouse can deduplicate based on `event_id` (ReplacingMergeTree engine).

### Retry Logic

**Transient errors (retry):**
- Network timeouts
- Database connection errors
- Temporary overload

**Permanent errors (skip):**
- Invalid data format
- Missing required fields

```go
for _, msg := range messages {
    err := processEvent(msg)
    if err != nil {
        if isRetryable(err) {
            // Don't ACK, will be retried
            log.Warn("Retrying event %s: %v", msg.ID, err)
            continue
        } else {
            // Permanent error, ACK and skip
            log.Error("Skipping event %s: %v", msg.ID, err)
            redis.XAck(ctx, "clicks:stream", "analytics-group", msg.ID)
        }
    } else {
        redis.XAck(ctx, "clicks:stream", "analytics-group", msg.ID)
    }
}
```

---

## Summary

**Three Workers, Three Purposes:**
1. **Analytics Worker**: Fast PostgreSQL updates (click counts)
2. **Pipeline Worker**: Slow enrichment + ClickHouse inserts (analytics)
3. **Cleanup Worker**: Periodic maintenance (delete expired URLs)

**Why Separate?**
- **Fault isolation**: One worker crashes, others continue
- **Independent scaling**: Scale based on workload
- **Different performance**: Fast vs slow operations
- **Different responsibilities**: Simple vs complex processing

**Consumer Groups:**
- Load balance events across workers
- At-least-once delivery
- Horizontal scaling (add more workers)

**Error Handling:**
- Idempotent operations (safe to retry)
- Deduplication (UUIDs for events)
- Retry transient errors, skip permanent errors

**Key Insight:**
Background workers are not just "async processing" - they're specialized services with distinct responsibilities. Separating them provides flexibility to optimize, scale, and maintain each independently. This is a core principle of microservices architecture.

---

**Up next**: [Code Walkthrough: URL Creation →](./10-code-walkthrough-create-url.md)

Follow a URL creation request from HTTP POST to database insert, with line-by-line code analysis.

---

**Word Count**: ~2,600 words
**Code References**: `cmd/analytics-worker/main.go`, `cmd/pipeline-worker/main.go`, `cmd/cleanup-worker/main.go`
