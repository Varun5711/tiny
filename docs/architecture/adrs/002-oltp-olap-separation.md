# ADR-002: OLTP/OLAP Database Separation

## Status
Accepted

## Context

The system has two distinct data access patterns:

**OLTP (Online Transaction Processing):**
- URL creation, lookup, user management
- Simple queries: INSERT, SELECT by primary key
- Low latency required (<10ms)
- Moderate write volume

**OLAP (Online Analytical Processing):**
- Click analytics, aggregations, time-series
- Complex queries: GROUP BY, COUNT, date ranges
- Higher latency acceptable (100ms-1s)
- High write volume (millions of events/day)

Options considered:
1. **Single PostgreSQL** - Store everything in one database
2. **PostgreSQL + TimescaleDB** - Time-series extension for analytics
3. **PostgreSQL + ClickHouse** - Separate OLAP database
4. **PostgreSQL + Elasticsearch** - Search-optimized analytics

## Decision

Use PostgreSQL for OLTP and ClickHouse for OLAP.

**PostgreSQL handles:**
- URL records (short_code, long_url, clicks, expires_at)
- User accounts (credentials, profiles)
- Transactional consistency

**ClickHouse handles:**
- Click events (billions of rows)
- Geographic aggregations
- Time-series analytics
- Device/browser statistics

**Data flow:**
```
Click → Redis Stream → Analytics Worker → ClickHouse
                                      ↓
                              Increment counter → PostgreSQL
```

## Consequences

### Positive
- **Query performance** - ClickHouse handles 100M+ rows aggregations in <1s
- **Storage efficiency** - ClickHouse compresses analytics data 10-20x
- **PostgreSQL stays fast** - No bloat from high-volume analytics writes
- **Independent scaling** - Scale analytics without affecting core OLTP
- **Specialized features** - ClickHouse has built-in time-series functions

### Negative
- **Operational complexity** - Two database systems to manage
- **Data consistency** - Analytics data is eventually consistent
- **Learning curve** - Team needs to know ClickHouse SQL dialect
- **Infrastructure cost** - Additional database server(s)

### Data consistency model
```
URL click happens
     ↓
[Immediate] Redis cache hit count increment (approximate)
[Immediate] Event published to Redis Stream
[~1-5 sec]  Analytics worker processes event
[~1-5 sec]  ClickHouse insert
[~1-5 sec]  PostgreSQL click counter increment

Dashboard shows: ClickHouse data (1-5 sec delay)
URL list shows:  PostgreSQL counter (1-5 sec delay)
```

## References
- [ClickHouse vs PostgreSQL Benchmarks](https://clickhouse.com/docs/en/introduction/distinctive-features)
- [OLTP vs OLAP](https://www.ibm.com/cloud/blog/olap-vs-oltp)
