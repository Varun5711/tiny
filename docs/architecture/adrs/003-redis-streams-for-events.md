# ADR-003: Redis Streams for Event Processing

## Status
Accepted

## Context

When a URL is clicked, we need to:
1. Redirect the user immediately (<50ms)
2. Record analytics asynchronously

This requires decoupling the redirect path from analytics processing. We need a message queue/stream for click events.

Options considered:
1. **Apache Kafka** - Distributed streaming platform
2. **RabbitMQ** - Traditional message broker
3. **Redis Streams** - Redis native streaming
4. **AWS SQS** - Managed queue service
5. **Direct database writes** - Synchronous insert

## Decision

Use Redis Streams for click event processing.

**Why Redis Streams:**
- Already using Redis for caching
- No additional infrastructure
- Built-in consumer groups for horizontal scaling
- Persistence with AOF
- Simple operations (XADD, XREADGROUP, XACK)

**Stream structure:**
```
Stream: clicks:stream
Entry:  {
  short_code: "abc123",
  timestamp: 1702500000,
  ip: "192.168.1.1",
  user_agent: "Mozilla/5.0...",
  referer: "https://google.com",
  original_url: "https://example.com/..."
}
```

**Consumer pattern:**
```
Redirect Service → XADD clicks:stream → Analytics Worker (XREADGROUP)
                                                ↓
                                         Process & Enrich
                                                ↓
                                         XACK clicks:stream
```

## Consequences

### Positive
- **Zero additional infrastructure** - Redis already in stack
- **Low latency** - XADD is O(1), doesn't block redirect
- **At-least-once delivery** - Consumer groups with acknowledgment
- **Horizontal scaling** - Multiple workers share load via consumer groups
- **Backpressure handling** - Stream has max length, old entries trimmed
- **Replay capability** - Can reprocess events from any point

### Negative
- **Not as durable as Kafka** - Redis is primarily in-memory
- **Limited retention** - Practical limit ~millions of entries
- **Single point of failure** - Unless using Redis Cluster/Sentinel
- **No schema enforcement** - Events are schemaless hashes

### Configuration
```
Stream:         clicks:stream
Consumer Group: analytics-group
Max Length:     1,000,000 entries (MAXLEN ~1000000)
Block Time:     5 seconds
Batch Size:     100 events
```

### Failure handling
```
1. Worker crashes before XACK
   → Event stays in pending list
   → Another worker claims it after timeout

2. Redis crashes
   → AOF recovery (fsync everysec)
   → Lose max 1 second of events

3. Worker can't process event
   → Move to dead-letter stream after N retries
   → Alert for manual inspection
```

## References
- [Redis Streams Introduction](https://redis.io/docs/data-types/streams/)
- [Redis Streams vs Kafka](https://redis.com/blog/kafka-vs-redis-streams/)
