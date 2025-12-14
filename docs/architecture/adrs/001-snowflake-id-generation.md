# ADR-001: Snowflake ID Generation

## Status
Accepted

## Context

We need to generate unique identifiers for shortened URLs. The requirements are:
- Globally unique across distributed instances
- Short when encoded (for URL friendliness)
- Sortable by time (newer URLs have higher IDs)
- High throughput (thousands per second per node)
- No coordination required between nodes

Options considered:
1. **UUID v4** - Random 128-bit identifiers
2. **Auto-increment** - Database sequence numbers
3. **Snowflake IDs** - Twitter's distributed ID algorithm
4. **ULID** - Universally Unique Lexicographically Sortable Identifier
5. **NanoID** - Random string generator

## Decision

Use Snowflake ID generation with Base62 encoding.

**Snowflake structure (64 bits):**
```
[41 bits: timestamp][5 bits: datacenter][5 bits: worker][12 bits: sequence]
```

**Encoding:**
- Snowflake ID → Base62 string (0-9, A-Z, a-z)
- Results in 7-11 character short codes

**Implementation:**
```go
// Generate ID
id, _ := generator.NextID()  // e.g., 7142351835967488

// Encode to short code
shortCode := idgen.Encode(id)  // e.g., "dK8Hp2"
```

## Consequences

### Positive
- **No database dependency** - IDs generated in application memory
- **Time-sortable** - Can determine creation order from ID
- **Short codes** - 7-11 chars vs 22 chars for Base64 UUID
- **High throughput** - 4,096 IDs/ms per worker without collision
- **Decentralized** - No coordination between nodes needed
- **Decodable** - Can extract timestamp from ID if needed

### Negative
- **Clock dependency** - Requires synchronized clocks (NTP)
- **Clock rollback** - Generator refuses to create IDs if clock moves backward
- **Limited workers** - Max 32 datacenters × 32 workers = 1,024 nodes
- **Predictable** - IDs are sequential, can estimate creation rate

### Trade-offs vs alternatives

| Criteria | Snowflake | UUID v4 | Auto-increment |
|----------|-----------|---------|----------------|
| Length (Base62) | 7-11 chars | 22 chars | 1-11 chars |
| Coordination | None | None | Database required |
| Sortable | Yes | No | Yes |
| Throughput | 4M/sec/cluster | Unlimited | DB limited |
| Predictability | Sequential | Random | Sequential |

## References
- [Twitter Snowflake](https://github.com/twitter-archive/snowflake)
- [Discord Snowflake](https://discord.com/developers/docs/reference#snowflakes)
