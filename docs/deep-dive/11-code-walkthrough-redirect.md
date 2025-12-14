# Code Walkthrough: Redirect & Click Tracking

> **Following a user click from HTTP GET to asynchronous analytics**

## Overview

When a user clicks `http://localhost:8081/4fR9KxY`, we need to:
1. **Look up the long URL** (cache → database)
2. **Redirect immediately** (< 50ms)
3. **Publish click event** (async, non-blocking)
4. **Process analytics** (background workers)

**The journey:**
```
User → Redirect Service → Cache (L1/L2) → PostgreSQL (if miss)
                       ↓
                  Redis Streams (async)
                       ↓
              ┌────────┴────────┐
              ↓                 ↓
      Analytics Worker    Pipeline Worker
              ↓                 ↓
        PostgreSQL          ClickHouse
```

**Timing goals:**
- Cache hit: 1-5ms
- Cache miss: 10-20ms
- Background processing: 100-500ms (async, doesn't block redirect)

---

## Step 1: User Clicks Short URL

**Browser navigation:**
```
User clicks: http://localhost:8081/4fR9KxY
Browser sends: GET /4fR9KxY
```

**HTTP Request:**
```http
GET /4fR9KxY HTTP/1.1
Host: localhost:8081
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36...
Referer: https://example.com/referring-page
X-Forwarded-For: 192.168.1.100
```

**Note**: X-Forwarded-For added by load balancer (contains real client IP).

---

## Step 2: Redirect Service Receives Request

**File**: `cmd/redirect-service/main.go`

**Router setup:**
```go
router := gin.Default()

// Health check
router.GET("/health", func(c *gin.Context) {
    c.JSON(200, gin.H{"status": "ok"})
})

// Main redirect handler (catches all paths)
router.GET("/:shortCode", handlers.HandleRedirect)

router.Run(":8081")
```

---

## Step 3: Extract Short Code

**File**: `internal/handlers/redirect_handler.go`

```go
func HandleRedirect(c *gin.Context) {
    // Extract short code from URL path
    shortCode := c.Param("shortCode")

    // Validate format (Base62: alphanumeric)
    if !isValidShortCode(shortCode) {
        c.JSON(404, gin.H{"error": "Invalid short code"})
        return
    }

    // Continue processing...
}

func isValidShortCode(code string) bool {
    // Base62: 0-9, A-Z, a-z
    matched, _ := regexp.MatchString(`^[a-zA-Z0-9]{6,12}$`, code)
    return matched
}
```

**Example:**
```
Input:  /4fR9KxY
Extract: shortCode = "4fR9KxY"
Valid:   ✓ (matches Base62 pattern)
```

---

## Step 4: Multi-Tier Cache Lookup

**Cache hierarchy:**
```
L1 (In-Memory LRU) → L2 (Redis) → PostgreSQL
```

**File**: `internal/handlers/redirect_handler.go`

```go
func HandleRedirect(c *gin.Context) {
    shortCode := c.Param("shortCode")

    // Try L1 cache (in-memory)
    if longURL, found := l1Cache.Get("url:" + shortCode); found {
        redirectAndTrack(c, shortCode, longURL.(string))
        return
    }

    // Try L2 cache (Redis)
    longURL, err := redis.Get(ctx, "url:"+shortCode).Result()
    if err == nil {
        // L2 hit: populate L1 for next time
        l1Cache.Set("url:"+shortCode, longURL)
        redirectAndTrack(c, shortCode, longURL)
        return
    }

    // L1 + L2 miss: query database
    longURL, err = fetchFromDatabase(ctx, shortCode)
    if err != nil {
        c.JSON(404, gin.H{"error": "URL not found"})
        return
    }

    // Populate caches
    l1Cache.Set("url:"+shortCode, longURL)
    redis.Set(ctx, "url:"+shortCode, longURL, 1*time.Hour)

    redirectAndTrack(c, shortCode, longURL)
}
```

**Path breakdown:**

**L1 Hit (70% of requests):**
```
GET /4fR9KxY
  ↓
L1.Get("url:4fR9KxY") → Found!
  ↓
Redirect (latency: ~0.1ms)
```

**L2 Hit (29% of requests):**
```
GET /4fR9KxY
  ↓
L1.Get() → Miss
  ↓
Redis.GET("url:4fR9KxY") → Found!
  ↓
L1.Set() (populate for next time)
  ↓
Redirect (latency: ~2ms)
```

**Database (1% of requests):**
```
GET /4fR9KxY
  ↓
L1.Get() → Miss
  ↓
Redis.GET() → Miss
  ↓
PostgreSQL.SELECT → Found!
  ↓
L1.Set() + Redis.SET (populate caches)
  ↓
Redirect (latency: ~10-15ms)
```

---

## Step 5: Database Query (Cache Miss Path)

**File**: `internal/storage/postgres.go`

```go
func (s *Storage) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
    query := `
        SELECT short_code, long_url, clicks, created_at, expires_at
        FROM urls
        WHERE short_code = $1
          AND (expires_at IS NULL OR expires_at > NOW())
    `

    // Use read replica (not primary)
    row := s.db.Read().QueryRow(ctx, query, shortCode)

    var url models.URL
    err := row.Scan(&url.ShortCode, &url.LongURL, &url.Clicks, &url.CreatedAt, &url.ExpiresAt)
    if err == pgx.ErrNoRows {
        return nil, ErrURLNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }

    return &url, nil
}
```

**SQL executed:**
```sql
SELECT short_code, long_url, clicks, created_at, expires_at
FROM urls
WHERE short_code = '4fR9KxY'
  AND (expires_at IS NULL OR expires_at > NOW());
```

**Key points:**
- Uses **read replica** (not primary) via `s.db.Read()`
- Checks expiry in query (`expires_at > NOW()`)
- Returns `ErrURLNotFound` if no match (404 to user)

---

## Step 6: HTTP 302 Redirect

```go
func redirectAndTrack(c *gin.Context, shortCode, longURL string) {
    // Publish click event (async, fire-and-forget)
    go publishClickEvent(shortCode, c.ClientIP(), c.Request.UserAgent(), c.Request.Referer())

    // Redirect user (don't wait for event publishing)
    c.Redirect(http.StatusFound, longURL)
}
```

**HTTP Response:**
```http
HTTP/1.1 302 Found
Location: https://example.com/very-long-product-page?utm_source=newsletter
Content-Length: 0
```

**Browser receives this and navigates to Location URL.**

**Critical**: Redirect sent **immediately**, doesn't wait for event publishing!

---

## Step 7: Publish Click Event (Async)

**File**: `internal/handlers/redirect_handler.go`

```go
func publishClickEvent(shortCode, ip, userAgent, referer string) {
    event := &events.ClickEvent{
        ShortCode:   shortCode,
        Timestamp:   time.Now().Unix(),
        IP:          ip,
        UserAgent:   userAgent,
        OriginalURL: "", // Filled by worker
        Referer:     referer,
        QueryParams: "", // Extracted from original request
    }

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    err := clickProducer.Publish(ctx, event)
    if err != nil {
        log.Error("Failed to publish click event: %v", err)
        // Don't fail redirect if publishing fails (graceful degradation)
    }
}
```

**Redis Streams command:**
```
XADD clicks:stream * short_code 4fR9KxY timestamp 1704067200 ip 192.168.1.100 user_agent "Mozilla..." referer "https://..."
```

**Time:** ~1-2ms (but goroutine runs in background)

**Graceful degradation:**
- If Redis is down, redirect still works
- Click event lost (acceptable trade-off for availability)

---

## Step 8: Extract Client IP

```go
func getClientIP(r *http.Request) string {
    // Behind load balancer/proxy
    forwarded := r.Header.Get("X-Forwarded-For")
    if forwarded != "" {
        // Format: "client, proxy1, proxy2"
        ips := strings.Split(forwarded, ",")
        return strings.TrimSpace(ips[0])  // First IP is client
    }

    // Direct connection
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    return host
}
```

**Example:**
```
X-Forwarded-For: 192.168.1.100, 10.0.0.5, 10.0.0.10
Extract: 192.168.1.100 (client's real IP)
```

**Why important:** Rate limiting and analytics need real client IP, not load balancer IP.

---

## Step 9: Analytics Worker Processes Event

**File**: `cmd/analytics-worker/main.go`

**Consumes from Redis Streams:**
```go
func processAnalytics() {
    for {
        // Read batch of 100 events
        results := redis.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    "analytics-group",
            Consumer: "analytics-worker-1",
            Streams:  []string{"clicks:stream", ">"},
            Count:    100,
            Block:    5 * time.Second,
        })

        // Aggregate by short_code
        clickCounts := make(map[string]int)
        for _, msg := range results[0].Messages {
            shortCode := msg.Values["short_code"].(string)
            clickCounts[shortCode]++
        }

        // Batch update PostgreSQL
        tx, _ := db.Begin()
        for shortCode, count := range clickCounts {
            tx.Exec("UPDATE urls SET clicks = clicks + $1, updated_at = NOW() WHERE short_code = $2", count, shortCode)
        }
        tx.Commit()

        // Acknowledge
        for _, msg := range results[0].Messages {
            redis.XAck(ctx, "clicks:stream", "analytics-group", msg.ID)
        }
    }
}
```

**SQL executed:**
```sql
UPDATE urls SET clicks = clicks + 1, updated_at = NOW() WHERE short_code = '4fR9KxY';
```

**Time:** ~20ms (for batch of 100)

**Latency:** 1-5 seconds (from click to database update)

---

## Step 10: Pipeline Worker Enriches & Stores

**File**: `cmd/pipeline-worker/main.go`

```go
func processPipeline() {
    geoIP, _ := geoip2.Open("GeoLite2-City.mmdb")

    for {
        results := redis.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    "analytics-group",
            Consumer: "pipeline-worker-1",
            Streams:  []string{"clicks:stream", ">"},
            Count:    100,
            Block:    5 * time.Second,
        })

        enrichedEvents := []ClickEvent{}

        for _, msg := range results[0].Messages {
            // Extract fields
            shortCode := msg.Values["short_code"].(string)
            ipStr := msg.Values["ip"].(string)
            userAgent := msg.Values["user_agent"].(string)

            // GeoIP lookup
            ip := net.ParseIP(ipStr)
            record, _ := geoIP.City(ip)

            // User-agent parsing
            ua := useragent.Parse(userAgent)

            enrichedEvents = append(enrichedEvents, ClickEvent{
                ShortCode:      shortCode,
                IP:             ipStr,
                Country:        record.Country.Names["en"],
                CountryCode:    record.Country.IsoCode,
                City:           record.City.Names["en"],
                Latitude:       record.Location.Latitude,
                Longitude:      record.Location.Longitude,
                Browser:        ua.Browser,
                BrowserVersion: ua.Version,
                OS:             ua.OS,
                DeviceType:     detectDevice(ua),
                ...
            })
        }

        // Batch insert to ClickHouse
        clickhouse.InsertBatch(enrichedEvents)

        // Acknowledge
        redis.XAck(...)
    }
}
```

**ClickHouse insert:**
```sql
INSERT INTO analytics.click_events (short_code, ip_address, country, city, browser, os, ...)
VALUES ('4fR9KxY', '192.168.1.100', 'United States', 'San Francisco', 'Chrome', 'Windows', ...);
```

**Time:** ~100ms (for batch of 100, includes enrichment)

**Latency:** 5-10 seconds (from click to ClickHouse)

---

## Complete Timing Breakdown

### Fast Path (L1 Cache Hit - 70% of requests)

| Step | Operation | Time |
|------|-----------|------|
| 1 | HTTP parsing | ~0.2ms |
| 2 | L1 cache lookup | ~0.0001ms |
| 3 | Goroutine spawn (event publishing) | ~0.001ms |
| 4 | HTTP 302 response | ~0.3ms |
| **Total (user-facing)** | | **~0.5ms** ✓ |
| 5 | Background: Publish to Redis | ~1ms (async) |

### Medium Path (L2 Cache Hit - 29% of requests)

| Step | Operation | Time |
|------|-----------|------|
| 1 | HTTP parsing | ~0.2ms |
| 2 | L1 cache miss | ~0.0001ms |
| 3 | Redis GET | ~1.5ms |
| 4 | L1 populate | ~0.0001ms |
| 5 | HTTP 302 response | ~0.3ms |
| **Total (user-facing)** | | **~2ms** ✓ |

### Slow Path (Database - 1% of requests)

| Step | Operation | Time |
|------|-----------|------|
| 1 | HTTP parsing | ~0.2ms |
| 2 | L1 cache miss | ~0.0001ms |
| 3 | L2 cache miss | ~1.5ms |
| 4 | PostgreSQL query (replica) | ~5ms |
| 5 | L1 + L2 populate | ~1.5ms |
| 6 | HTTP 302 response | ~0.3ms |
| **Total (user-facing)** | | **~8.5ms** ✓ |

**All paths meet <50ms target!**

### Background Processing (Async)

| Worker | Operation | Latency from Click |
|--------|-----------|-------------------|
| Analytics | PostgreSQL update | 1-5 seconds |
| Pipeline | GeoIP + ClickHouse insert | 5-10 seconds |

**Not user-facing** → acceptable delay.

---

## Error Handling

### 1. URL Not Found

```go
if err == ErrURLNotFound {
    c.JSON(404, gin.H{"error": "URL not found or expired"})
    return
}
```

**Response:**
```http
HTTP/1.1 404 Not Found
{"error": "URL not found or expired"}
```

### 2. Expired URL

Handled in SQL query:
```sql
WHERE short_code = $1 AND (expires_at IS NULL OR expires_at > NOW())
```

Returns no rows → 404

### 3. Database Unavailable

```go
longURL, err := fetchFromDatabase(ctx, shortCode)
if err != nil {
    log.Error("Database error: %v", err)
    c.JSON(503, gin.H{"error": "Service temporarily unavailable"})
    return
}
```

**Response:**
```http
HTTP/1.1 503 Service Unavailable
{"error": "Service temporarily unavailable"}
```

### 4. Redis Unavailable (Event Publishing)

```go
err := clickProducer.Publish(ctx, event)
if err != nil {
    log.Error("Failed to publish: %v", err)
    // Continue with redirect (graceful degradation)
}
```

**Redirect still works**, analytics lost temporarily.

---

## Summary

**Redirect flow:**
1. Extract short code from URL
2. Multi-tier cache lookup (L1 → L2 → DB)
3. Publish click event asynchronously
4. HTTP 302 redirect (immediate)

**Background processing:**
5. Analytics Worker updates click counts
6. Pipeline Worker enriches and stores in ClickHouse

**Key optimizations:**
- **Multi-tier caching**: 99% cache hit rate
- **Async event publishing**: Doesn't block redirect
- **Batch processing**: Workers process 100 events at once
- **Read replicas**: Database reads don't hit primary

**Latency achieved:**
- L1 hit: 0.5ms
- L2 hit: 2ms
- Database: 8.5ms
- **All under 10ms** (target: <50ms) ✓

**Graceful degradation:**
- Redis down: Redirects work, analytics lost
- ClickHouse down: Redirects work, detailed analytics lost
- Database down: 503 error (no way to get long URL)

---

**Up next**: [Scaling Strategy →](./12-scaling-strategy.md)

Learn how to scale each component horizontally, identify bottlenecks, and plan for 10x-100x growth.

---

**Word Count**: ~2,200 words
**Code References**: Multiple files referenced inline
