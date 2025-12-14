# System Architecture

## Overview

Tiny URL Shortener is a distributed URL shortening service designed for high throughput and low latency. The system follows a microservices architecture with clear separation of concerns.

## System Diagram

```
                                    ┌─────────────────────────────────────────────────────────────┐
                                    │                        CLIENTS                              │
                                    │   Browser    Mobile App    CLI/TUI    Third-party APIs      │
                                    └─────────────────────────────┬───────────────────────────────┘
                                                                  │
                                                                  ▼
                                    ┌─────────────────────────────────────────────────────────────┐
                                    │                     LOAD BALANCER                           │
                                    │                    (nginx / HAProxy)                        │
                                    └──────────────────────────┬──────────────────────────────────┘
                                                               │
                         ┌─────────────────────────────────────┼─────────────────────────────────────┐
                         │                                     │                                     │
                         ▼                                     ▼                                     │
          ┌──────────────────────────────┐      ┌──────────────────────────────┐                    │
          │        API GATEWAY           │      │      REDIRECT SERVICE        │                    │
          │          :8080               │      │          :8081               │                    │
          │                              │      │                              │                    │
          │  • REST API endpoints        │      │  • URL redirection           │                    │
          │  • Authentication            │      │  • Click tracking            │                    │
          │  • Rate limiting             │      │  • Cache-first lookups       │                    │
          │  • Request validation        │      │  • Event publishing          │                    │
          └──────────────┬───────────────┘      └──────────────┬───────────────┘                    │
                         │                                     │                                     │
                         │ gRPC                                │ gRPC                                │
                         ▼                                     ▼                                     │
          ┌──────────────────────────────────────────────────────────────────┐                      │
          │                        SERVICE LAYER                             │                      │
          │  ┌────────────────────────┐    ┌────────────────────────┐        │                      │
          │  │     URL SERVICE        │    │     USER SERVICE       │        │                      │
          │  │     gRPC :50051        │    │     gRPC :50052        │        │                      │
          │  │                        │    │                        │        │                      │
          │  │  • CRUD operations     │    │  • Registration        │        │                      │
          │  │  • ID generation       │    │  • Authentication      │        │                      │
          │  │  • QR code generation  │    │  • JWT management      │        │                      │
          │  │  • Cache management    │    │  • Profile management  │        │                      │
          │  └────────────┬───────────┘    └───────────┬────────────┘        │                      │
          └───────────────┼────────────────────────────┼─────────────────────┘                      │
                          │                            │                                            │
          ┌───────────────┴────────────────────────────┴─────────────────────┐                      │
          │                                                                  │                      │
          ▼                                                                  ▼                      │
┌───────────────────────────────────────┐                 ┌──────────────────────────────┐          │
│           POSTGRESQL CLUSTER          │                 │            REDIS             │          │
│                                       │                 │           :6379              │          │
│  ┌─────────────┐                      │                 │                              │◄─────────┘
│  │   PRIMARY   │───► Writes           │                 │  • L2 Cache (URL mappings)  │
│  │    :5432    │                      │                 │  • Distributed locks        │
│  └──────┬──────┘                      │                 │  • Rate limit counters      │
│         │ Streaming Replication       │                 │  • Click event streams      │
│         ▼                             │                 │                              │
│  ┌─────────────┐ ┌─────────────┐      │                 └──────────────┬───────────────┘
│  │  REPLICA 1  │ │  REPLICA 2  │      │                                │
│  │    :5433    │ │    :5434    │      │                                │
│  └─────────────┘ └─────────────┘      │                                │
│         │              │              │                                │
│         └──────┬───────┘              │                                │
│                ▼                      │                                │
│  ┌─────────────────────────┐          │                                │
│  │       REPLICA 3         │◄─ Reads  │                                │
│  │         :5435           │          │                                │
│  └─────────────────────────┘          │                                │
└───────────────────────────────────────┘                                │
                                                                         │
                         ┌───────────────────────────────────────────────┘
                         │
                         ▼
          ┌──────────────────────────────────────────────────────────────────┐
          │                        WORKER LAYER                              │
          │                                                                  │
          │  ┌────────────────────┐  ┌────────────────────┐  ┌────────────┐  │
          │  │  ANALYTICS WORKER  │  │  PIPELINE WORKER   │  │  CLEANUP   │  │
          │  │                    │  │                    │  │  WORKER    │  │
          │  │  • Consume events  │  │  • Aggregate data  │  │            │  │
          │  │  • Geo enrichment  │  │  • Build reports   │  │  • Remove  │  │
          │  │  • Device parsing  │  │  • Rollup metrics  │  │    expired │  │
          │  │  • Store to CH     │  │                    │  │    URLs    │  │
          │  └─────────┬──────────┘  └─────────┬──────────┘  └─────┬──────┘  │
          └────────────┼───────────────────────┼──────────────────┼──────────┘
                       │                       │                  │
                       ▼                       ▼                  │
          ┌──────────────────────────────────────────────┐        │
          │               CLICKHOUSE                     │        │
          │              :8123 / :9000                   │        │
          │                                              │        │
          │  • Click events (billions of rows)          │        │
          │  • Time-series analytics                    │        │
          │  • Geographic aggregations                  │        │
          │  • Device/browser statistics                │        │
          └──────────────────────────────────────────────┘        │
                                                                  │
                       ┌──────────────────────────────────────────┘
                       │
                       ▼
          ┌──────────────────────────────────────────────┐
          │               POSTGRESQL                     │
          │              (via Primary)                   │
          │                                              │
          │  • Delete expired URLs                      │
          │  • Cleanup orphaned records                 │
          └──────────────────────────────────────────────┘
```

## Data Flow

### URL Creation Flow

```
Client                API Gateway           URL Service            PostgreSQL           Redis
  │                       │                      │                      │                 │
  │  POST /api/urls       │                      │                      │                 │
  │──────────────────────►│                      │                      │                 │
  │                       │                      │                      │                 │
  │                       │  CreateURL (gRPC)    │                      │                 │
  │                       │─────────────────────►│                      │                 │
  │                       │                      │                      │                 │
  │                       │                      │  Generate Snowflake ID                 │
  │                       │                      │─────────┐            │                 │
  │                       │                      │         │            │                 │
  │                       │                      │◄────────┘            │                 │
  │                       │                      │                      │                 │
  │                       │                      │  Encode to Base62    │                 │
  │                       │                      │─────────┐            │                 │
  │                       │                      │         │            │                 │
  │                       │                      │◄────────┘            │                 │
  │                       │                      │                      │                 │
  │                       │                      │  INSERT url          │                 │
  │                       │                      │─────────────────────►│                 │
  │                       │                      │                      │                 │
  │                       │                      │         OK           │                 │
  │                       │                      │◄─────────────────────│                 │
  │                       │                      │                      │                 │
  │                       │                      │  SET url:{code}      │                 │
  │                       │                      │────────────────────────────────────────►
  │                       │                      │                      │                 │
  │                       │     Response         │                      │                 │
  │                       │◄─────────────────────│                      │                 │
  │                       │                      │                      │                 │
  │    201 Created        │                      │                      │                 │
  │◄──────────────────────│                      │                      │                 │
  │                       │                      │                      │                 │
```

### Redirect Flow (Cache Hit)

```
Client              Redirect Service          Redis
  │                       │                     │
  │  GET /{shortcode}     │                     │
  │──────────────────────►│                     │
  │                       │                     │
  │                       │  GET url:{code}     │
  │                       │────────────────────►│
  │                       │                     │
  │                       │    long_url         │
  │                       │◄────────────────────│
  │                       │                     │
  │                       │  XADD clicks:stream │
  │                       │────────────────────►│
  │                       │                     │
  │   302 Redirect        │                     │
  │◄──────────────────────│                     │
  │                       │                     │
```

### Redirect Flow (Cache Miss)

```
Client              Redirect Service          Redis           URL Service         PostgreSQL
  │                       │                     │                   │                  │
  │  GET /{shortcode}     │                     │                   │                  │
  │──────────────────────►│                     │                   │                  │
  │                       │                     │                   │                  │
  │                       │  GET url:{code}     │                   │                  │
  │                       │────────────────────►│                   │                  │
  │                       │                     │                   │                  │
  │                       │      (nil)          │                   │                  │
  │                       │◄────────────────────│                   │                  │
  │                       │                     │                   │                  │
  │                       │  GetURL (gRPC)      │                   │                  │
  │                       │────────────────────────────────────────►│                  │
  │                       │                     │                   │                  │
  │                       │                     │                   │  SELECT          │
  │                       │                     │                   │─────────────────►│
  │                       │                     │                   │                  │
  │                       │                     │                   │    row           │
  │                       │                     │                   │◄─────────────────│
  │                       │                     │                   │                  │
  │                       │      Response       │                   │                  │
  │                       │◄────────────────────────────────────────│                  │
  │                       │                     │                   │                  │
  │                       │  SET url:{code}     │                   │                  │
  │                       │────────────────────►│                   │                  │
  │                       │                     │                   │                  │
  │                       │  XADD clicks:stream │                   │                  │
  │                       │────────────────────►│                   │                  │
  │                       │                     │                   │                  │
  │   302 Redirect        │                     │                   │                  │
  │◄──────────────────────│                     │                   │                  │
  │                       │                     │                   │                  │
```

### Analytics Pipeline

```
Redis Stream            Analytics Worker              ClickHouse
     │                         │                           │
     │  XREADGROUP             │                           │
     │◄────────────────────────│                           │
     │                         │                           │
     │   click events          │                           │
     │────────────────────────►│                           │
     │                         │                           │
     │                         │  Parse User-Agent         │
     │                         │─────────┐                 │
     │                         │         │                 │
     │                         │◄────────┘                 │
     │                         │                           │
     │                         │  GeoIP Lookup             │
     │                         │─────────┐                 │
     │                         │         │                 │
     │                         │◄────────┘                 │
     │                         │                           │
     │                         │  INSERT INTO clicks       │
     │                         │──────────────────────────►│
     │                         │                           │
     │  XACK                   │                           │
     │◄────────────────────────│                           │
     │                         │                           │
```

## Port Allocation

```
┌────────────────────────────────────────────────────────────┐
│                      PORT MAPPING                          │
├─────────────────────┬──────────┬───────────────────────────┤
│ Service             │ Port     │ Protocol                  │
├─────────────────────┼──────────┼───────────────────────────┤
│ API Gateway         │ 8080     │ HTTP/REST                 │
│ Redirect Service    │ 8081     │ HTTP                      │
│ URL Service         │ 50051    │ gRPC                      │
│ User Service        │ 50052    │ gRPC                      │
│ PostgreSQL Primary  │ 5432     │ PostgreSQL                │
│ PostgreSQL Replica1 │ 5433     │ PostgreSQL                │
│ PostgreSQL Replica2 │ 5434     │ PostgreSQL                │
│ PostgreSQL Replica3 │ 5435     │ PostgreSQL                │
│ Redis               │ 6379     │ RESP                      │
│ ClickHouse HTTP     │ 8123     │ HTTP                      │
│ ClickHouse Native   │ 9000     │ Native                    │
└─────────────────────┴──────────┴───────────────────────────┘
```

## Caching Strategy

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         MULTI-TIER CACHE                                │
│                                                                         │
│   ┌─────────────────────┐                                               │
│   │      L1 CACHE       │  In-memory LRU cache (per instance)           │
│   │    (Local LRU)      │  • Capacity: 10,000 entries                   │
│   │                     │  • Latency: <1ms                              │
│   │   Hit ──► Return    │  • No network overhead                        │
│   │   Miss ─┐           │                                               │
│   └─────────┼───────────┘                                               │
│             │                                                           │
│             ▼                                                           │
│   ┌─────────────────────┐                                               │
│   │      L2 CACHE       │  Redis distributed cache                      │
│   │      (Redis)        │  • TTL: 1 hour                                │
│   │                     │  • Latency: 1-5ms                             │
│   │   Hit ──► Return    │  • Shared across instances                    │
│   │         + Promote   │                                               │
│   │           to L1     │                                               │
│   │   Miss ─┐           │                                               │
│   └─────────┼───────────┘                                               │
│             │                                                           │
│             ▼                                                           │
│   ┌─────────────────────┐                                               │
│   │      DATABASE       │  PostgreSQL (read replicas)                   │
│   │    (PostgreSQL)     │  • Latency: 5-20ms                            │
│   │                     │  • Populate L1 + L2 on read                   │
│   └─────────────────────┘                                               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## ID Generation (Snowflake)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      SNOWFLAKE ID STRUCTURE                             │
│                          (64 bits total)                                │
│                                                                         │
│   ┌─────────────────┬───────────┬───────────┬─────────────────────┐     │
│   │   Timestamp     │Datacenter │  Worker   │      Sequence       │     │
│   │   (41 bits)     │ (5 bits)  │ (5 bits)  │     (12 bits)       │     │
│   └─────────────────┴───────────┴───────────┴─────────────────────┘     │
│                                                                         │
│   Timestamp:   Milliseconds since custom epoch (2024-01-01)             │
│   Datacenter:  0-31 (supports 32 datacenters)                           │
│   Worker:      0-31 (supports 32 workers per datacenter)                │
│   Sequence:    0-4095 (4096 IDs per millisecond per worker)             │
│                                                                         │
│   Max throughput: 4096 * 32 * 32 = 4,194,304 IDs/ms per cluster         │
│                                                                         │
│   Example:                                                              │
│   ID: 7142351835967488 ──► Base62: "dK8Hp2"                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Database Schema

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         POSTGRESQL TABLES                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  urls                                users                              │
│  ─────────────────────────          ─────────────────────────           │
│  short_code    VARCHAR(50) PK       id             UUID PK              │
│  long_url      TEXT NOT NULL        email          VARCHAR(255) UNIQUE  │
│  clicks        BIGINT DEFAULT 0     password_hash  VARCHAR(255)         │
│  created_at    TIMESTAMPTZ          name           VARCHAR(255)         │
│  expires_at    TIMESTAMPTZ          created_at     TIMESTAMPTZ          │
│  qr_code       TEXT                 updated_at     TIMESTAMPTZ          │
│  user_id       UUID FK ─────────────────────────────┘                   │
│                                                                         │
│  Indexes:                                                               │
│  • urls_short_code_idx (PRIMARY)                                        │
│  • urls_user_id_idx                                                     │
│  • urls_expires_at_idx (for cleanup worker)                             │
│  • users_email_idx (UNIQUE)                                             │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                         CLICKHOUSE TABLES                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  click_events                                                           │
│  ─────────────────────────────────────────────────────                  │
│  event_id        UUID                                                   │
│  short_code      String                                                 │
│  original_url    String                                                 │
│  clicked_at      DateTime                                               │
│  ip_address      IPv4                                                   │
│  country         LowCardinality(String)                                 │
│  region          LowCardinality(String)                                 │
│  city            LowCardinality(String)                                 │
│  browser         LowCardinality(String)                                 │
│  browser_version String                                                 │
│  os              LowCardinality(String)                                 │
│  os_version      String                                                 │
│  device_type     Enum('desktop', 'mobile', 'tablet', 'bot')             │
│  referer         String                                                 │
│  user_agent      String                                                 │
│                                                                         │
│  Engine: MergeTree()                                                    │
│  PARTITION BY toYYYYMM(clicked_at)                                      │
│  ORDER BY (short_code, clicked_at)                                      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Scaling Considerations

```
                        HORIZONTAL SCALING STRATEGY
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                         │
│  STATELESS SERVICES (scale horizontally)                                │
│  ───────────────────────────────────────                                │
│  • API Gateway         → Scale based on request rate                    │
│  • Redirect Service    → Scale based on redirect QPS                    │
│  • URL Service         → Scale based on creation rate                   │
│  • User Service        → Scale based on auth requests                   │
│  • Analytics Worker    → Scale based on stream lag                      │
│                                                                         │
│  STATEFUL SERVICES (scale vertically first, then shard)                 │
│  ──────────────────────────────────────────────────────                 │
│  • PostgreSQL          → Read replicas for reads, consider sharding     │
│                          by short_code prefix for writes at scale       │
│  • Redis               → Redis Cluster for >100GB cache                 │
│  • ClickHouse          → Distributed tables for >1TB analytics          │
│                                                                         │
│  BOTTLENECK ANALYSIS                                                    │
│  ───────────────────                                                    │
│  1. First bottleneck:  PostgreSQL writes → Add connection pooling       │
│  2. Second bottleneck: Redis memory → Tune TTLs, add nodes              │
│  3. Third bottleneck:  ClickHouse queries → Add more replicas           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

## Security Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         SECURITY LAYERS                                 │
│                                                                         │
│  NETWORK LAYER                                                          │
│  ─────────────                                                          │
│  • TLS termination at load balancer                                     │
│  • Internal services communicate over private network                   │
│  • Database ports not exposed publicly                                  │
│                                                                         │
│  APPLICATION LAYER                                                      │
│  ─────────────────                                                      │
│  • JWT authentication (HS256, 24h expiry)                               │
│  • Rate limiting per IP (100 req/min default)                           │
│  • Input validation on all endpoints                                    │
│  • URL validation (scheme, host required)                               │
│  • Alias validation (profanity filter, reserved words)                  │
│                                                                         │
│  DATA LAYER                                                             │
│  ──────────                                                             │
│  • Passwords hashed with bcrypt (cost=10)                               │
│  • Database connections use SSL                                         │
│  • Credentials stored in environment variables                          │
│  • No sensitive data in logs                                            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```
