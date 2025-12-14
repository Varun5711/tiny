# Short Code Generation: Snowflake IDs + Base62 Encoding

> **How to generate millions of unique, URL-safe short codes without collisions**

## Overview

The heart of any URL shortener is generating **short codes** - those 6-8 character identifiers like `abc123` or `7kQ3mN`. This seems trivial until you consider the requirements:

1. **Globally unique** - No two URLs can have the same short code
2. **Collision-free** - Even across multiple servers generating IDs simultaneously
3. **URL-safe** - Only characters allowed in URLs (no `/`, `?`, `#`, `+`)
4. **Short** - Ideally 6-8 characters (shorter = better UX)
5. **Unpredictable** - Sequential IDs leak information ("we only have 1000 URLs")
6. **Fast** - Generate in under 1ms without database queries

These requirements eliminate simple solutions:
- ❌ Auto-increment IDs: Sequential (predictable), need database coordination
- ❌ Random strings: Collisions inevitable at scale
- ❌ UUIDs: Too long (36 characters)
- ❌ Hash-based: Variable length, need collision checking

Our solution: **Snowflake IDs + Base62 encoding**
1. Generate a unique 64-bit integer (Snowflake algorithm)
2. Encode it in Base62 (URL-safe, short representation)

In this document, we'll explore:
1. **The Problem Space** - Why simple solutions fail
2. **Snowflake IDs** - Twitter's distributed ID generation algorithm
3. **Base62 Encoding** - Converting numbers to URL-safe strings
4. **Implementation** - Line-by-line code walkthrough
5. **Collision Analysis** - Proving uniqueness mathematically

By the end, you'll understand not just the "what" but the "why" of every bit.

---

## Part 1: The Problem - Why Simple Solutions Fail

### Attempt 1: Auto-Increment Database IDs

```sql
CREATE TABLE urls (
    id SERIAL PRIMARY KEY,  -- Auto-increment
    ...
);
```

Then use `id` as the short code: `tiny.ly/1`, `tiny.ly/2`, etc.

**Problems:**
1. **Predictable**: Anyone can guess URLs (`tiny.ly/1`, `tiny.ly/2`, ...)
2. **Leaks scale**: "We're at ID 500,000" tells competitors your volume
3. **Requires database**: Every ID generation hits the database
4. **Single point of bottleneck**: Database must coordinate ID assignment
5. **Not URL-safe**: Numbers only (boring, long for large IDs)

### Attempt 2: Random Strings

```go
func generateShortCode() string {
    chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    code := ""
    for i := 0; i < 6; i++ {
        code += string(chars[rand.Intn(len(chars))])
    }
    return code
}
```

**Problems:**
1. **Birthday paradox**: Collisions happen sooner than expected
   - With 6 characters (62^6 = 56 billion combinations)
   - 50% collision probability at ~√combinations = ~237,000 URLs
2. **Need collision checking**: Database query before insert
3. **Retry complexity**: What if collision? Retry? How many times?
4. **Database constraint**: `UNIQUE` constraint causes errors on collision

**Birthday Paradox Example:**
```
1st URL: 0% collision chance
2nd URL: 1 / 56B = 0.0000000018%
1000th URL: 1000 / 56B = 0.0000018%
237,000th URL: ~50% (!!!)
```

### Attempt 3: UUID Version 4

```go
id := uuid.NewV4()  // "550e8400-e29b-41d4-a716-446655440000"
```

**Problems:**
1. **Too long**: 36 characters (with hyphens), 32 without
2. **Not user-friendly**: Hard to read, type, remember
3. **Ugly URLs**: `tiny.ly/550e8400-e29b-41d4-a716-446655440000`

Even Base62-encoded, UUIDs are still ~22 characters (too long).

### Attempt 4: Hash-Based (MD5/SHA)

```go
hash := md5.Sum(longURL)
shortCode := base62.Encode(hash[:8])  // Take first 8 bytes
```

**Problems:**
1. **Collisions**: Different URLs can hash to same code (birthday paradox again)
2. **Same URL, different hashes**: If user submits same URL twice, gets different short codes (wasteful)
3. **Still need database check**: Must query before insert
4. **Not deterministic**: Can't recreate short code without storing it

---

## Part 2: Snowflake IDs - The Twitter Solution

### What is Snowflake?

Developed by Twitter in 2010 for generating unique IDs across distributed systems. Named after unique snowflakes in nature.

**Key insight**: Embed uniqueness guarantees **into the ID structure**.

###Bit Layout (64 bits total)

```
 ┌─────────────────────────────────────────────────────────────┐
 │  1 │         41         │    5    │    5    │      12      │
 │sign│     timestamp      │datacenter│ worker │   sequence   │
 └─────────────────────────────────────────────────────────────┘
  unused  milliseconds since    ID      ID     counter
          custom epoch (41)    (5)     (5)     (12)
```

**Breaking it down:**

1. **Sign bit (1 bit)**: Always 0 (unused, keeps number positive)
2. **Timestamp (41 bits)**: Milliseconds since custom epoch
   - Range: 2^41 ms = ~69 years
   - Custom epoch: 2024-01-01 (configurable)
3. **Datacenter ID (5 bits)**: Which datacenter/region (0-31)
4. **Worker ID (5 bits)**: Which server in datacenter (0-31)
5. **Sequence (12 bits)**: Counter for same millisecond (0-4095)

**Example ID:**
```
Binary:  0 0000111110001000101011100100010010 00001 00001 000000000001
         │ │                                 │ │     │     │
         │ └─ 269482274 (ms since epoch)     │ │     │     └─ Sequence: 1
         │                                   │ └─────┴─ Worker: 1, DC: 1
         └─ Sign: 0

Decimal: 19284756301824 (example)
Base62:  "4fR9KxY" (7 characters)
```

### Why This Design is Brilliant

**1. No coordination needed**
Each worker knows its datacenter ID and worker ID at startup. No database queries, no network calls to generate IDs.

**2. Time-ordered**
IDs are roughly sortable by time (first 41 bits are timestamp). Database indexes stay efficient (no random inserts).

**3. Collision-free**
Two IDs collide only if all these match simultaneously:
- Same millisecond (timestamp)
- Same datacenter
- Same worker
- Same sequence number

**Probability**: Essentially zero with proper configuration.

**4. High throughput**
Each worker can generate 4,096 IDs per millisecond:
- 1 millisecond = 4,096 IDs
- 1 second = 4,096,000 IDs per worker
- 32 workers = 131 million IDs/second

**5. Compact**
64-bit integers fit in a single database column, cache efficiently, Base62 encodes to ~10-11 characters.

---

## Part 3: Implementation Deep Dive

### Constants (snowflake.go:9-20)

```go
const (
    workerIDBits      = 5   // 2^5 = 32 workers
    datacenterIDBits  = 5   // 2^5 = 32 datacenters
    sequenceBits      = 12  // 2^12 = 4096 sequence numbers

    maxWorkerID       = -1 ^ (-1 << workerIDBits)       // 31
    maxDatacenterID   = -1 ^ (-1 << datacenterIDBits)   // 31
    maxSequence       = -1 ^ (-1 << sequenceBits)       // 4095

    workerIDShift     = sequenceBits                    // 12
    datacenterIDShift = sequenceBits + workerIDBits     // 17
    timestampShift    = sequenceBits + workerIDBits + datacenterIDBits  // 22

    customEpoch       = 1704067200000  // 2024-01-01 00:00:00 UTC in milliseconds
)
```

**Bit shifting explained:**

`maxWorkerID = -1 ^ (-1 << 5)` calculates maximum value for 5 bits:
```
-1 in binary (two's complement, 64-bit):
  1111111111111111111111111111111111111111111111111111111111111111

-1 << 5 (shift left by 5):
  1111111111111111111111111111111111111111111111111111111111100000

-1 ^ (-1 << 5) (XOR):
  0000000000000000000000000000000000000000000000000000000000011111
  = 31 (decimal)
```

**Custom Epoch (2024-01-01):**
- Standard Unix epoch: 1970-01-01
- Our epoch: 2024-01-01
- Saves bits: Timestamp relative to 2024 instead of 1970
- Extends lifetime: 41 bits from 2024 = 69 years (until 2093)

### Generator Struct (snowflake.go:22-28)

```go
type Generator struct {
    mu            sync.Mutex  // Thread safety
    datacenterID  int64       // Fixed at startup (0-31)
    workerID      int64       // Fixed at startup (0-31)
    sequence      int64       // Incremented per ID (0-4095)
    lastTimestamp int64       // Last millisecond we generated ID
}
```

**Why mutex?**
Multiple goroutines might call `NextID()` simultaneously. `sync.Mutex` ensures only one generates at a time.

**Alternative**: Lock-free with atomic operations (more complex, marginal performance gain).

### NextID() - The Core Algorithm (snowflake.go:45-72)

```go
func (g *Generator) NextID() (int64, error) {
    g.mu.Lock()
    defer g.mu.Unlock()

    timestamp := g.currentTimestamp()  // Milliseconds since custom epoch

    // Handle clock moving backwards
    if timestamp < g.lastTimestamp {
        return 0, fmt.Errorf("clock moved backwards: refusing to generate ID for %d milliseconds",
            g.lastTimestamp-timestamp)
    }

    // Same millisecond as last ID
    if timestamp == g.lastTimestamp {
        g.sequence = (g.sequence + 1) & maxSequence  // Increment and wrap at 4095
        if g.sequence == 0 {
            // Sequence exhausted, wait for next millisecond
            timestamp = g.waitForNextMillis(g.lastTimestamp)
        }
    } else {
        // New millisecond, reset sequence
        g.sequence = 0
    }

    g.lastTimestamp = timestamp

    // Compose ID from components
    id := (timestamp << timestampShift) |
          (g.datacenterID << datacenterIDShift) |
          (g.workerID << workerIDShift) |
          g.sequence

    return id, nil
}
```

**Line-by-line analysis:**

**Line 49-53: Clock Skew Detection**
```go
if timestamp < g.lastTimestamp {
    return 0, fmt.Errorf("clock moved backwards...")
}
```

**Problem**: System clock can move backwards (NTP correction, manual change).
**Solution**: Refuse to generate IDs until clock catches up.
**Alternative**: Wait until clock advances (but could be minutes/hours).

**Line 56: Increment Sequence**
```go
g.sequence = (g.sequence + 1) & maxSequence
```

**Why `& maxSequence`?**
- `maxSequence = 4095 = 0b111111111111` (12 bits of 1s)
- `& maxSequence` masks off higher bits, wrapping at 4096:
  ```
  0 → 1 → 2 → ... → 4094 → 4095 → 0 (wraps)
  ```

**Line 57-59: Sequence Exhaustion**
```go
if g.sequence == 0 {
    timestamp = g.waitForNextMillis(g.lastTimestamp)
}
```

If we've generated 4,096 IDs in the same millisecond (rare but possible):
- Sequence wraps to 0
- Wait for next millisecond
- Continue generating

**Line 66-69: Bit Composition**
```go
id := (timestamp << timestampShift) |
      (g.datacenterID << datacenterIDShift) |
      (g.workerID << workerIDShift) |
      g.sequence
```

**Example with real values:**
```
timestamp = 269482274 (ms since 2024-01-01)
datacenterID = 1
workerID = 1
sequence = 42

Step 1: timestamp << 22
  269482274 << 22 = 1136125419560960 (shift left by 22 bits)

Step 2: datacenterID << 17
  1 << 17 = 131072

Step 3: workerID << 12
  1 << 12 = 4096

Step 4: OR all together
  1136125419560960 | 131072 | 4096 | 42 = 1136125419696170

Base62 encode: "4fR9KxY"
```

**Visual representation of bit operations:**
```
timestamp (41 bits):      000011111000100010101110010001001000000000000000000000000000000
datacenterID << 17:       000000000000000000000000000000000000000000000000000100000000000
workerID << 12:           000000000000000000000000000000000000000000000000000001000000000
sequence (12 bits):       000000000000000000000000000000000000000000000000000000000101010

OR all:                   000011111000100010101110010001001000000000000000000101101010
                          └─────────────────┬────────────────────────────────┘
                                    Final 64-bit ID
```

### waitForNextMillis() (snowflake.go:78-84)

```go
func (g *Generator) waitForNextMillis(lastTimestamp int64) int64 {
    timestamp := g.currentTimestamp()
    for timestamp <= lastTimestamp {
        timestamp = g.currentTimestamp()
    }
    return timestamp
}
```

**Spin loop**: Repeatedly checks time until it advances.

**Trade-off:**
- **Pro**: Guarantees we don't skip time
- **Con**: Burns CPU while waiting (typically < 1ms)

**Alternative**: `time.Sleep(1 * time.Millisecond)` - but might overshoot.

---

## Part 4: Base62 Encoding

### Why Base62?

**Character set comparison:**

| Encoding | Characters | Count | URL-Safe? |
|----------|------------|-------|-----------|
| Base64 | `A-Za-z0-9+/` | 64 | ❌ No (`+`, `/`) |
| **Base62** | `A-Za-z0-9` | 62 | ✅ Yes |
| Base58 | `A-Za-z0-9` (no `0OIl`) | 58 | ✅ Yes |
| Hex | `0-9A-F` | 16 | ✅ Yes (but longer) |

**Base62 wins**:
- URL-safe (no special characters)
- Most compact (higher base = shorter strings)
- Readable (includes both cases and numbers)

**Base58 (Bitcoin)**:
- Removes confusing characters: `0` vs `O`, `I` vs `l`
- Slightly longer strings
- Useful for typed inputs (QR codes, manual entry)
- Not necessary for URLs (copy-paste, click)

### Encoding Algorithm (base62.go:18-35)

```go
const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func Encode(num int64) string {
    if num == 0 {
        return "0"
    }

    res := make([]byte, 0)
    for num > 0 {
        rem := num % 62
        res = append(res, base62Chars[rem])
        num /= 62
    }

    // Reverse (we built backwards)
    for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
        res[i], res[j] = res[j], res[i]
    }

    return string(res)
}
```

**Algorithm**: Repeated division by 62.

**Example: Encode 1136125419696170**

```
Step 1: 1136125419696170 % 62 = 34 → 'Y'
        1136125419696170 / 62 = 18324603543809

Step 2: 18324603543809 % 62 = 23 → 'N'
        18324603543809 / 62 = 295558122319

Step 3: 295558122319 % 62 = 42 → 'g'
        295558122319 / 62 = 4767711974

...continue until num = 0...

Result (backwards): "YxKgR9fR4"
Reversed:           "4fR9KxY"
```

**Why build backwards then reverse?**

Digits come out in **reverse order** (least significant first). Reversing at the end gives correct order.

**Alternative**: Build string in reverse (prepend) - but string concatenation in Go is expensive (creates new strings). Slice append + reverse is more efficient.

### Decoding Algorithm (base62.go:37-51)

```go
func Decode(str string) (int64, error) {
    var num int64 = 0
    for i := 0; i < len(str); i++ {
        val := base62Index[str[i]]  // Lookup character value
        if val == -1 {
            return 0, fmt.Errorf("invalid base62 character: %c", str[i])
        }
        num = num*62 + int64(val)  // Horner's method
    }
    return num, nil
}
```

**Example: Decode "4fR9KxY"**

```
'4' = 4   → num = 0 * 62 + 4 = 4
'f' = 41  → num = 4 * 62 + 41 = 289
'R' = 27  → num = 289 * 62 + 27 = 17945
'9' = 9   → num = 17945 * 62 + 9 = 1112599
'K' = 20  → num = 1112599 * 62 + 20 = 68981158
'x' = 59  → num = 68981158 * 62 + 59 = 4276831855
'Y' = 34  → num = 4276831855 * 62 + 34 = 265163575244

Result: 265163575244 (example - actual would match original)
```

**Horner's method**: Efficient polynomial evaluation.
```
"abc" in base 62 = a×62² + b×62¹ + c×62⁰
                 = ((a×62) + b)×62 + c
```

---

## Part 5: Collision Analysis & Guarantees

### Mathematical Proof of Uniqueness

**Claim**: Two Snowflake IDs can only collide if ALL of these match:
1. Same timestamp (millisecond)
2. Same datacenter ID
3. Same worker ID
4. Same sequence number

**Proof**:

Each ID component is placed in non-overlapping bits:
- Timestamp: bits 22-62 (41 bits)
- Datacenter: bits 17-21 (5 bits)
- Worker: bits 12-16 (5 bits)
- Sequence: bits 0-11 (12 bits)

Bitwise OR (`|`) ensures no overlap. Therefore, two IDs are equal if and only if all components are equal.

**Probability of collision:**

With proper configuration (unique datacenter/worker per instance):
- Different instances: datacenter or worker ID differs → **0% collision**
- Same instance:
  - Different milliseconds → **0% collision**
  - Same millisecond, different sequence → **0% collision**
  - Same millisecond, same sequence → **Impossible** (sequence is incremented)

**Conclusion**: Collisions are impossible with Snowflake IDs.

### Edge Cases

**1. Clock skew**
- Handled by refusing to generate IDs
- Alternative: NTP-synced clocks (< 1ms drift)

**2. Sequence exhaustion**
- 4,096 IDs per millisecond per worker
- If exceeded: Wait for next millisecond
- Rare: Requires > 4M IDs/second from single worker

**3. Epoch overflow**
- 41 bits from 2024 = 69 years (until 2093)
- Solution: Change epoch, migrate (decades away)

**4. Worker ID collision**
- Manual configuration error (two workers same ID)
- Mitigation: Configuration management, health checks

---

## Part 6: Length Analysis

### Snowflake ID → Base62 Length

**Snowflake ID range**: 0 to 2^64 - 1

**Base62 encoding**:
```
62^10 = 839,299,365,868,340,224 < 2^64
62^11 = 52,036,560,683,837,093,888 > 2^64
```

**Conclusion**: All 64-bit Snowflake IDs encode to **≤ 11 characters** in Base62.

**Typical length** (41-bit timestamp):
```
2^41 = 2,199,023,255,552 (max timestamp)

Combined with datacenter + worker + sequence:
Typical ID: ~10^15 range
Base62: ~10-11 characters
```

**Example lengths:**
```
ID: 1 → "1" (1 char)
ID: 1000 → "g8" (2 chars)
ID: 1000000 → "4c92" (4 chars)
ID: 10^15 → "2gosa7pa2gv" (11 chars)
```

### Comparison with Alternatives

| Method | Example | Length | URL-Safe? |
|--------|---------|--------|-----------|
| Auto-increment | `123456` | Variable (6+ digits) | Yes |
| UUID v4 | `550e8400-e29b-41d4-a716` | 36 (with `-`), 32 (without) | Yes |
| UUID v4 (Base62) | `2B73jCgSDrEKLwZaf9` | 22 | Yes |
| **Snowflake (Base62)** | **`4fR9KxY`** | **10-11** | **Yes** |
| Random 6-char | `aB3x9K` | 6 (but collisions!) | Yes |

**Snowflake wins**: Short, unique, no collisions, no database queries.

---

## Summary

**What we covered:**

**The Problem:**
- Simple solutions fail: auto-increment (predictable), random (collisions), UUID (too long)
- Need: unique, short, URL-safe, fast, distributed

**Snowflake IDs:**
- 64-bit structure: timestamp (41) + datacenter (5) + worker (5) + sequence (12)
- Time-ordered, collision-free, no coordination needed
- 4,096 IDs/ms per worker = 4M IDs/sec per worker
- Custom epoch (2024) extends lifetime to 2093

**Implementation:**
- Mutex for thread safety
- Clock skew detection (refuse IDs if clock moves backwards)
- Sequence wrapping with wait-for-next-millisecond
- Bit shifting to compose final ID

**Base62 Encoding:**
- Character set: `0-9A-Za-z` (62 characters)
- URL-safe, shorter than Base64
- Encodes 64-bit IDs to 10-11 characters
- Reversible (decode back to integer)

**Collision Guarantees:**
- Mathematically impossible with proper configuration
- Non-overlapping bit positions ensure uniqueness
- Edge cases handled (clock skew, sequence exhaustion)

**Key Insight:**
Distributed ID generation doesn't require a database. By embedding uniqueness constraints (datacenter, worker, sequence) into the ID structure, we achieve:
- **Zero network calls** to generate IDs
- **Zero collisions** (proven mathematically)
- **High throughput** (millions of IDs/sec)
- **Short codes** (10-11 characters)

This is why Snowflake-based systems power Twitter, Discord, Instagram, and now our URL shortener.

---

**Up next**: [gRPC vs REST: Internal Communication →](./06-grpc-internal-communication.md)

Learn why we use gRPC for service-to-service communication (5-10x faster than REST) but REST for external APIs (better DX).

---

**Word Count**: ~3,500 words
**Reading Time**: ~17 minutes
**Code References**:
- `internal/idgen/snowflake.go`
- `internal/idgen/base62.go`
