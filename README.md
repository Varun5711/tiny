<p align="center">
  <h1 align="center">Tiny</h1>
  <p align="center">
    A production-grade, distributed URL shortener built with Go microservices.
    <br />
    <strong>10,000+ lines of Go</strong> &middot; <strong>8 microservices</strong> &middot; <strong>6 data stores</strong>
    <br />
    <br />
    <a href="#quick-start">Quick Start</a>
    &middot;
    <a href="#api-reference">API Docs</a>
    &middot;
    <a href="#architecture">Architecture</a>
    &middot;
    <a href="docs/deep-dive/">Deep Dive</a>
  </p>
</p>

<p align="center">
  <a href="https://github.com/Varun5711/tiny/actions/workflows/ci.yaml"><img src="https://github.com/Varun5711/tiny/actions/workflows/ci.yaml/badge.svg?branch=refactor" alt="CI"></a>
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white" alt="Go 1.25">
  <img src="https://img.shields.io/badge/gRPC-Protocol%20Buffers-244c5a?logo=google&logoColor=white" alt="gRPC">
  <img src="https://img.shields.io/badge/License-MIT-green.svg" alt="License: MIT">
</p>

---

## What is Tiny?

Tiny is a **full-stack URL shortener** designed as a real-world distributed systems project. It goes far beyond a simple redirect service -- it includes user authentication, custom aliases, QR code generation, real-time click analytics with geo/device enrichment, distributed tracing, full-text search, and a terminal UI client.

Every architectural decision maps to a production concern: read replicas for scale, Redis Streams for async event processing, ClickHouse materialized views for sub-second analytics, Snowflake IDs for conflict-free distributed ID generation, and distributed locks for custom alias reservation.

### Key Features

| Feature | Description |
|---------|-------------|
| **URL Shortening** | Auto-generated short codes via Snowflake ID + Base62 encoding |
| **Custom Aliases** | Reserve vanity URLs with distributed lock protection |
| **QR Codes** | Auto-generated QR code (Base64 PNG) for every short URL |
| **Click Analytics** | Real-time tracking: geo location, device, browser, OS, referrer |
| **User Accounts** | JWT authentication with registration, login, and profile management |
| **Full-Text Search** | Search URLs via Elasticsearch across long URLs and short codes |
| **TTL Expiration** | Configurable URL expiration with automated cleanup |
| **TUI Client** | Interactive terminal UI built with Bubble Tea |
| **Distributed Tracing** | End-to-end request tracing with Jaeger + OpenTelemetry |
| **Multi-Tier Cache** | L1 (in-memory LRU) + L2 (Redis) for sub-millisecond redirects |

---

## Architecture

### System Overview

```mermaid
graph TB
    subgraph Clients
        USER["Browser / cURL"]
        TUI["TUI Client<br/>(Bubble Tea)"]
    end

    subgraph "API Layer"
        GW["API Gateway<br/>HTTP :8080"]
        RS["Redirect Service<br/>HTTP :8081"]
    end

    subgraph "gRPC Services"
        URL["URL Service<br/>gRPC :50051"]
        AUTH["User Service<br/>gRPC :50052"]
    end

    subgraph "Data Stores"
        PG_P[("PostgreSQL<br/>Primary")]
        PG_R1[("Replica 1")]
        PG_R2[("Replica 2")]
        PG_R3[("Replica 3")]
        REDIS[("Redis 7<br/>Cache + Streams<br/>+ Rate Limit")]
        CH[("ClickHouse<br/>Analytics OLAP")]
        ES[("Elasticsearch 8<br/>Full-Text Search")]
    end

    subgraph "Background Workers"
        PW["Pipeline Worker<br/>(GeoIP + UA Enrich)"]
        AW["Analytics Worker<br/>(Aggregate)"]
        CW["Cleanup Worker<br/>(TTL Sweep 24h)"]
    end

    subgraph "Observability"
        JAEGER["Jaeger<br/>(Trace Collector)"]
    end

    USER -->|"REST API"| GW
    USER -->|"GET /:code"| RS
    TUI -->|"gRPC"| URL
    TUI -->|"gRPC"| AUTH

    GW -->|"gRPC"| URL
    GW -->|"gRPC"| AUTH
    GW -->|"Query"| CH
    GW -->|"Search"| ES
    GW -->|"Read/Write"| PG_P
    GW -->|"Rate Limit"| REDIS

    RS -->|"L1 miss → L2 lookup"| REDIS
    RS -->|"L2 miss → gRPC"| URL
    RS -->|"Publish Click"| REDIS

    URL -->|"Write"| PG_P
    URL -->|"Read"| PG_R1
    URL -->|"Read"| PG_R2
    URL -->|"Read"| PG_R3
    URL -->|"Cache"| REDIS
    URL -->|"Index"| ES
    URL -->|"Dist. Lock"| REDIS

    AUTH -->|"Read/Write"| PG_P

    PW -->|"Consume Stream"| REDIS
    PW -->|"Batch Insert"| CH
    PW -->|"Index Events"| ES

    AW -->|"Consume Stream"| REDIS
    AW -->|"Aggregate"| PG_P

    CW -->|"Delete Expired"| PG_P

    PG_P -->|"WAL Replication"| PG_R1
    PG_P -->|"WAL Replication"| PG_R2
    PG_P -->|"WAL Replication"| PG_R3

    GW -.->|"OTel Traces"| JAEGER
    RS -.->|"OTel Traces"| JAEGER
    URL -.->|"OTel Traces"| JAEGER
    AUTH -.->|"OTel Traces"| JAEGER

    style GW fill:#4A90D9,stroke:#2C5F8A,color:#fff
    style RS fill:#4A90D9,stroke:#2C5F8A,color:#fff
    style URL fill:#6B8E23,stroke:#4A6B0F,color:#fff
    style AUTH fill:#6B8E23,stroke:#4A6B0F,color:#fff
    style PG_P fill:#336791,stroke:#1D3F5E,color:#fff
    style PG_R1 fill:#336791,stroke:#1D3F5E,color:#fff
    style PG_R2 fill:#336791,stroke:#1D3F5E,color:#fff
    style PG_R3 fill:#336791,stroke:#1D3F5E,color:#fff
    style REDIS fill:#DC382D,stroke:#A12920,color:#fff
    style CH fill:#FFCC00,stroke:#CC9900,color:#000
    style ES fill:#005571,stroke:#003B4F,color:#fff
    style PW fill:#8B5CF6,stroke:#6D28D9,color:#fff
    style AW fill:#8B5CF6,stroke:#6D28D9,color:#fff
    style CW fill:#8B5CF6,stroke:#6D28D9,color:#fff
    style JAEGER fill:#66CFE3,stroke:#3BA7BD,color:#000
    style USER fill:#F59E0B,stroke:#D97706,color:#000
    style TUI fill:#F59E0B,stroke:#D97706,color:#000
```

### Services

| Service | Type | Port | Description |
|---------|------|------|-------------|
| **api-gateway** | HTTP | `8080` | REST API, auth middleware, CORS, rate limiting, Swagger |
| **redirect-service** | HTTP | `8081` | Fast 302 redirects with cache-first lookups |
| **url-service** | gRPC | `50051` | URL CRUD, Snowflake ID generation, custom aliases |
| **user-service** | gRPC | `50052` | Registration, login, JWT token management |
| **analytics-worker** | Worker | -- | Aggregates click events from Redis Streams to PostgreSQL |
| **pipeline-worker** | Worker | -- | Enriches clicks (GeoIP, UA parsing) and stores to ClickHouse + Elasticsearch |
| **cleanup-worker** | Worker | -- | Periodic deletion of expired URLs (every 24h) |
| **tui** | CLI | -- | Interactive terminal client (Bubble Tea) |

### Redirect Flow (Hot Path)

```mermaid
sequenceDiagram
    actor User
    participant RS as Redirect Service<br/>:8081
    participant L1 as L1 Cache<br/>(In-Memory LRU)
    participant L2 as L2 Cache<br/>(Redis)
    participant URL as URL Service<br/>(gRPC :50051)
    participant PG as PostgreSQL<br/>(Read Replica)
    participant Stream as Redis Stream
    participant PW as Pipeline Worker

    User->>RS: GET /abc123
    RS->>L1: Lookup "url:abc123"
    alt L1 Cache Hit
        L1-->>RS: https://example.com
    else L1 Cache Miss
        RS->>L2: GET url:abc123
        alt L2 Cache Hit
            L2-->>RS: https://example.com
            RS->>L1: Set (backfill)
        else L2 Cache Miss
            RS->>URL: GetURL(abc123) [gRPC]
            URL->>PG: SELECT long_url FROM urls
            PG-->>URL: https://example.com
            URL-->>RS: {long_url, found: true}
            RS->>L1: Set
            RS->>L2: SET url:abc123 (TTL 1h)
        end
    end

    RS->>Stream: XADD clicks:stream {short_code, ip, ua, referer, ...}
    RS-->>User: 302 Redirect → https://example.com

    Note over Stream,PW: Async Processing
    PW->>Stream: XREADGROUP (consumer group)
    PW->>PW: Enrich: GeoIP + UA Parse
    PW->>PW: Batch (configurable size)

    par Store Analytics
        PW->>PW: Insert → ClickHouse
    and Index for Search
        PW->>PW: Bulk Index → Elasticsearch
    end
    PW->>Stream: XACK (acknowledge)
```

### Create URL Flow

```mermaid
sequenceDiagram
    actor User
    participant GW as API Gateway<br/>:8080
    participant MW as Middleware<br/>(Auth + Rate Limit)
    participant URL as URL Service<br/>(gRPC :50051)
    participant SF as Snowflake ID<br/>Generator
    participant PG as PostgreSQL<br/>(Primary)
    participant Redis as Redis
    participant ES as Elasticsearch
    participant QR as QR Code<br/>Generator

    User->>GW: POST /api/urls {long_url}
    GW->>MW: Validate JWT Token
    MW->>MW: Check Rate Limit (Redis)
    MW-->>GW: Authorized (user_id)

    GW->>URL: CreateURL(long_url, user_id) [gRPC]
    URL->>SF: NextID()
    SF-->>URL: 1234567890 (int64)
    URL->>URL: Base62 Encode → "7Bx9kL"
    URL->>QR: Generate QR for short URL
    QR-->>URL: Base64 PNG

    URL->>PG: INSERT INTO urls (short_code, long_url, user_id, qr_code, ...)
    URL->>Redis: SET url:7Bx9kL → long_url
    URL->>ES: Index URL document (if enabled)

    URL-->>GW: {short_code, short_url, qr_code, ...}
    GW-->>User: 201 Created {short_code, short_url, qr_code}
```

### Custom Alias Flow (with Distributed Locking)

```mermaid
sequenceDiagram
    actor User
    participant GW as API Gateway
    participant URL as URL Service
    participant Redis as Redis
    participant PG as PostgreSQL<br/>(Primary)

    User->>GW: POST /api/urls/custom {alias: "my-brand", long_url}
    GW->>URL: CreateCustomURL("my-brand", long_url) [gRPC]

    URL->>URL: Validate alias (regex, reserved words)
    URL->>Redis: SET lock:alias:my-brand NX EX 5s
    alt Lock Acquired
        URL->>PG: SELECT EXISTS(short_code = 'my-brand')
        alt Alias Available
            PG-->>URL: false (not taken)
            URL->>PG: INSERT INTO urls (short_code='my-brand', ...)
            URL->>Redis: SET url:my-brand → long_url
            URL->>Redis: DEL lock:alias:my-brand (Lua script)
            URL-->>GW: {short_code: "my-brand", short_url, qr_code}
            GW-->>User: 201 Created
        else Alias Taken
            PG-->>URL: true (exists)
            URL->>URL: Generate 3 alternatives
            URL->>Redis: DEL lock:alias:my-brand
            URL-->>GW: Error: alias taken, try: my-brand1, my-brand2, my-brand-x
            GW-->>User: 409 Conflict
        end
    else Lock Not Acquired
        URL-->>GW: Error: alias being claimed, retry
        GW-->>User: 429 Retry Later
    end
```

### Analytics Pipeline

```mermaid
graph LR
    subgraph "Click Event Source"
        RS["Redirect Service"]
    end

    subgraph "Message Queue"
        STREAM["Redis Stream<br/>clicks:stream"]
    end

    subgraph "Pipeline Worker"
        READ["XREADGROUP<br/>(Consumer Group)"]
        GEO["GeoIP Enrichment<br/>Country, City, Lat/Lng"]
        UA["User-Agent Parse<br/>Browser, OS, Device"]
        BATCH["Batch Builder"]
    end

    subgraph "Storage"
        CH[("ClickHouse<br/>(Raw Events)")]
        MV1["daily_clicks_by_url<br/>(Materialized View)"]
        MV2["clicks_by_country<br/>(Materialized View)"]
        MV3["clicks_by_device<br/>(Materialized View)"]
        MV4["hourly_clicks<br/>(Materialized View)"]
        ES[("Elasticsearch<br/>(Click Search Index)")]
    end

    subgraph "Analytics Worker"
        AW["Aggregate to<br/>PostgreSQL"]
    end

    RS -->|"XADD"| STREAM
    STREAM --> READ
    READ --> GEO --> UA --> BATCH
    BATCH -->|"Batch Insert"| CH
    BATCH -->|"Bulk Index"| ES
    CH --> MV1
    CH --> MV2
    CH --> MV3
    CH --> MV4
    STREAM -->|"XREADGROUP"| AW

    style RS fill:#4A90D9,stroke:#2C5F8A,color:#fff
    style STREAM fill:#DC382D,stroke:#A12920,color:#fff
    style CH fill:#FFCC00,stroke:#CC9900,color:#000
    style ES fill:#005571,stroke:#003B4F,color:#fff
    style MV1 fill:#FFF3CD,stroke:#CC9900,color:#000
    style MV2 fill:#FFF3CD,stroke:#CC9900,color:#000
    style MV3 fill:#FFF3CD,stroke:#CC9900,color:#000
    style MV4 fill:#FFF3CD,stroke:#CC9900,color:#000
```

---

## Tech Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Language** | Go 1.25 | All services |
| **RPC** | gRPC + Protobuf | Inter-service communication |
| **HTTP** | `net/http` | API Gateway + Redirect Service |
| **Primary DB** | PostgreSQL 16 (TimescaleDB) | URLs, users, 1 primary + 3 read replicas |
| **Analytics DB** | ClickHouse | Click events, materialized views for aggregations |
| **Cache + Queue** | Redis 7 | Multi-tier cache (L1/L2), Streams for async events, rate limiting, distributed locks |
| **Search** | Elasticsearch 8 | Full-text URL search, click event search, log shipping |
| **Tracing** | Jaeger + OpenTelemetry | Distributed request tracing across all services |
| **Logging** | Zap | Structured JSON logging with optional ES shipping |
| **DI Framework** | Uber FX | Dependency injection, lifecycle management, graceful shutdown |
| **Auth** | JWT (golang-jwt/v5) | Token-based authentication |
| **ID Generation** | Snowflake + Base62 | Globally unique, time-sortable, URL-safe short codes |
| **QR Codes** | go-qrcode | PNG QR code generation (Base64-encoded) |
| **GeoIP** | MaxMind GeoLite2 | IP-to-location enrichment |
| **UA Parsing** | mssola/user_agent | Browser, OS, device detection |
| **TUI** | Bubble Tea (charmbracelet) | Interactive terminal UI |
| **Containers** | Docker + BuildKit | Multi-stage builds, layer caching |
| **Orchestration** | Kubernetes | Deployments, StatefulSets, HPAs, NetworkPolicies |
| **CI/CD** | GitHub Actions | Lint, test, vuln scan, Docker build, GHCR push |

---

## Quick Start

### Prerequisites

- **Go 1.25+**
- **Docker** & **Docker Compose**
- **Make** (optional, for convenience commands)

### 1. Clone and configure

```bash
git clone https://github.com/Varun5711/tiny.git
cd tiny
cp .env.example .env
```

### 2. Start infrastructure

```bash
# Start PostgreSQL (primary + 3 replicas), Redis, and ClickHouse
docker compose -f deployments/docker/docker-compose.yml up -d \
  postgres-primary postgres-replica1 postgres-replica2 postgres-replica3 \
  redis clickhouse
```

### 3. Start services

```bash
# Option A: Run all services with Docker Compose
docker compose -f deployments/docker/docker-compose.yml up --build

# Option B: Run services locally (requires Go 1.25+)
go run ./cmd/url-service &
go run ./cmd/user-service &
go run ./cmd/redirect-service &
go run ./cmd/api-gateway &
go run ./cmd/pipeline-worker &
go run ./cmd/analytics-worker &
go run ./cmd/cleanup-worker &
```

### 4. Try it out

```bash
# Register a user
curl -s -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"secret123","name":"Test User"}'

# Save the token from the response
TOKEN="<token-from-response>"

# Shorten a URL
curl -s -X POST http://localhost:8080/api/urls \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"long_url":"https://github.com/Varun5711/tiny"}'

# Visit the short URL
curl -v http://localhost:8081/<short_code>
# → 302 redirect to https://github.com/Varun5711/tiny
```

### 5. Launch the TUI

```bash
go run ./cmd/tui
```

---

## API Reference

### Authentication

#### Register
```http
POST /api/auth/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "securepassword",
  "name": "John Doe"
}
```

**Response** `200 OK`
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "name": "John Doe",
  "token": "eyJhbGciOiJIUzI1NiIs..."
}
```

#### Login
```http
POST /api/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "securepassword"
}
```

#### Get Profile
```http
GET /api/auth/profile
Authorization: Bearer <token>
```

---

### URLs

> All URL endpoints require `Authorization: Bearer <token>` header.

#### Create Short URL
```http
POST /api/urls
Content-Type: application/json

{
  "long_url": "https://example.com/very/long/path",
  "expires_at": 1735689600       // optional, unix timestamp
}
```

**Response** `201 Created`
```json
{
  "short_code": "7Bx9kL",
  "short_url": "http://localhost:8081/7Bx9kL",
  "long_url": "https://example.com/very/long/path",
  "created_at": 1704067200,
  "expires_at": 1735689600,
  "qr_code": "data:image/png;base64,iVBOR..."
}
```

#### Create Custom Alias
```http
POST /api/urls/custom
Content-Type: application/json

{
  "alias": "my-brand",
  "long_url": "https://example.com",
  "expires_at": 1735689600       // optional
}
```

#### List URLs
```http
GET /api/urls?limit=20&offset=0
Authorization: Bearer <token>
```

**Response** `200 OK`
```json
{
  "urls": [
    {
      "short_code": "7Bx9kL",
      "short_url": "http://localhost:8081/7Bx9kL",
      "long_url": "https://example.com",
      "clicks": 42,
      "created_at": 1704067200,
      "expires_at": 1735689600,
      "is_active": true
    }
  ],
  "total": 1,
  "has_more": false
}
```

#### Delete URL
```http
DELETE /api/urls/{short_code}
Authorization: Bearer <token>
```

#### Redirect
```http
GET http://localhost:8081/{short_code}
→ 302 Found (Location: https://original-url.com)
```

---

### Analytics

#### Get URL Stats
```http
GET /api/analytics/{short_code}/stats
```
Returns total clicks, unique visitors, last clicked timestamp.

#### Get Click Timeline
```http
GET /api/analytics/{short_code}/timeline?period=7d
```
Returns hourly/daily click counts with unique visitor breakdowns.

#### Get Geo Stats
```http
GET /api/analytics/{short_code}/geo
```
Returns click distribution by country with percentages.

#### Get Device Stats
```http
GET /api/analytics/{short_code}/devices
```
Returns breakdown by device type, browser, and OS.

#### Get Top Referrers
```http
GET /api/analytics/{short_code}/referrers
```
Returns ranked list of referrer URLs by click count.

#### Get Raw Click Events
```http
GET /api/analytics/clicks?short_code={code}&limit=50&offset=0
Authorization: Bearer <token>
```

---

### Search

#### Full-Text Search
```http
GET /api/search?q=example&limit=10&offset=0
```
Searches across long URLs and short codes via Elasticsearch.

---

### Health

```http
GET /health
→ 200 OK    (PostgreSQL + Redis reachable)
→ 503        (dependency unavailable)
```

---

## Configuration

All configuration is via environment variables (loaded from `.env` in development):

### Database
| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PRIMARY_DSN` | -- | PostgreSQL primary connection string |
| `DB_REPLICA1_DSN` | -- | Read replica 1 |
| `DB_REPLICA2_DSN` | -- | Read replica 2 |
| `DB_REPLICA3_DSN` | -- | Read replica 3 |
| `DB_MAX_CONNS` | `25` | Max connections per pool |
| `DB_MIN_CONNS` | `5` | Min idle connections |

### Redis
| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | -- | Redis password |
| `REDIS_STREAM_NAME` | `clicks:stream` | Stream name for click events |

### ClickHouse
| Variable | Default | Description |
|----------|---------|-------------|
| `CLICKHOUSE_ADDR` | `localhost:9000` | ClickHouse native protocol address |
| `CLICKHOUSE_DATABASE` | `analytics` | Database name |
| `CLICKHOUSE_USERNAME` | `clickhouse` | Username |

### Services
| Variable | Default | Description |
|----------|---------|-------------|
| `API_GATEWAY_PORT` | `8080` | API Gateway HTTP port |
| `REDIRECT_SERVICE_PORT` | `8081` | Redirect service HTTP port |
| `BASE_URL` | `http://localhost:8081` | Base URL for generated short links |
| `DEFAULT_URL_TTL` | `72h` | Default URL expiration |
| `JWT_SECRET` | -- | **Required.** Secret key for JWT signing |

### Elasticsearch
| Variable | Default | Description |
|----------|---------|-------------|
| `ES_ENABLED` | `false` | Enable Elasticsearch integration |
| `ES_ADDRESSES` | `http://localhost:9200` | Comma-separated ES addresses |
| `ES_INDEX_PREFIX` | `shorternit` | Index name prefix |

### Tracing
| Variable | Default | Description |
|----------|---------|-------------|
| `TRACING_ENABLED` | `false` | Enable OpenTelemetry tracing |
| `JAEGER_ENDPOINT` | `http://localhost:4318` | Jaeger OTLP endpoint |
| `TRACING_SAMPLE_RATE` | `1.0` | Sampling rate (0.0 to 1.0) |

### Rate Limiting
| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_REQUESTS` | `100` | Max requests per window |
| `RATE_LIMIT_WINDOW` | `1m` | Rate limit window duration |

### Cache
| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_L1_CAPACITY` | `10000` | In-memory LRU cache size |
| `CACHE_L2_TTL` | `1h` | Redis cache entry TTL |

---

## Project Structure

```
tiny/
├── cmd/                          # Service entry points
│   ├── api-gateway/              # HTTP REST API (Uber FX)
│   ├── redirect-service/         # Fast URL redirect (Uber FX)
│   ├── url-service/              # URL CRUD gRPC server (Uber FX)
│   ├── user-service/             # Auth gRPC server (Uber FX)
│   ├── analytics-worker/         # Redis Stream → PostgreSQL (Uber FX)
│   ├── pipeline-worker/          # Redis Stream → ClickHouse + ES (Uber FX)
│   ├── cleanup-worker/           # Expired URL deletion (Uber FX)
│   └── tui/                      # Terminal UI (Bubble Tea)
│
├── internal/                     # Private application packages
│   ├── analytics/                # Analytics aggregation service
│   ├── auth/                     # JWT manager + bcrypt passwords
│   ├── cache/                    # Multi-tier cache (LRU + Redis)
│   ├── clickhouse/               # ClickHouse client + analytics queries
│   ├── config/                   # Env-based configuration loader
│   ├── database/                 # PostgreSQL connection pool manager
│   ├── elasticsearch/            # ES client: URL index, click index, log shipping
│   ├── enrichment/               # GeoIP lookup + User-Agent parsing
│   ├── events/                   # Click event model + Redis Stream producer
│   ├── grpc/                     # gRPC client factory (with OTel instrumentation)
│   ├── handlers/                 # HTTP handlers (URL, Auth, Analytics, Swagger, Redirect)
│   ├── idgen/                    # Snowflake ID generator + Base62 encoder
│   ├── lock/                     # Redis-backed distributed lock (Lua script)
│   ├── logger/                   # Zap structured logging (JSON + ES syncer)
│   ├── middleware/               # CORS, rate limit, auth, recovery, tracing, request ID
│   ├── models/                   # Domain models (URL, User, errors)
│   ├── qrcode/                   # QR code PNG generation
│   ├── redis/                    # Redis client wrapper
│   ├── service/                  # Business logic (URL service, User service)
│   ├── storage/                  # PostgreSQL storage layer (CRUD, pagination, filters)
│   ├── tracing/                  # OpenTelemetry tracer provider setup
│   └── validation/               # Alias validation + alternative suggestions
│
├── proto/                        # Protobuf definitions
│   ├── url/                      # URL service (CreateURL, GetURL, ListURLs, DeleteURL, etc.)
│   ├── user/                     # User service (Register, Login, ValidateToken, etc.)
│   └── analytics/                # Analytics service
│
├── build/docker/                 # Multi-stage Dockerfiles (8 services)
├── deployments/
│   ├── docker/                   # docker-compose.yml (full stack)
│   ├── k8s/                      # Kubernetes manifests (base + overlays)
│   │   ├── base/                 # Deployments, Services, HPAs, NetworkPolicies
│   │   └── overlays/             # staging / production kustomizations
│   └── terraform/                # Infrastructure-as-code (placeholder)
│
├── scripts/
│   ├── databases/                # SQL schemas (PostgreSQL + ClickHouse)
│   ├── migrations/               # Database migration scripts
│   └── install.sh                # CLI installer (curl | bash)
│
├── docs/
│   ├── api/                      # gRPC API docs + examples
│   ├── architecture/             # System design + ADRs
│   └── deep-dive/                # 12-chapter technical deep dive
│
├── test/integration/             # End-to-end integration tests
├── api/openapi/                  # OpenAPI/Swagger specification
├── .github/workflows/ci.yaml    # CI pipeline (lint, test, build, Docker)
├── .env.example                  # Environment variable template
├── go.mod                        # Go module (30+ dependencies)
└── Makefile                      # Build automation
```

---

## Database Schema

### PostgreSQL

```sql
-- Users table
CREATE TABLE users (
    id            VARCHAR(50) PRIMARY KEY,
    email         VARCHAR(255) UNIQUE NOT NULL,
    name          VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);

-- URLs table
CREATE TABLE urls (
    short_code  VARCHAR(20) PRIMARY KEY,
    long_url    TEXT NOT NULL,
    user_id     VARCHAR(50),
    clicks      BIGINT DEFAULT 0,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,
    qr_code     TEXT
);
```

### ClickHouse

```sql
CREATE TABLE analytics.click_events (
    event_id String, short_code String, original_url String,
    clicked_at DateTime64(3), ip_address String,
    country String, country_code String, region String, city String,
    latitude Float64, longitude Float64, timezone String,
    user_agent String, browser String, browser_version String,
    os String, os_version String, device_type String,
    device_brand String, device_model String,
    is_mobile UInt8, is_tablet UInt8, is_desktop UInt8, is_bot UInt8,
    referer String, query_params String
) ENGINE = MergeTree()
  PARTITION BY toYYYYMM(clicked_date)
  ORDER BY (short_code, clicked_at)
  TTL clicked_date + INTERVAL 180 DAY;

-- Pre-aggregated materialized views
-- daily_clicks_by_url, clicks_by_country, clicks_by_device, hourly_clicks
```

---

## Deployment

### Docker Compose (Development)

```bash
# Full stack: databases + all 7 services
docker compose -f deployments/docker/docker-compose.yml up --build

# Infrastructure only (bring your own services)
docker compose -f deployments/docker/docker-compose.yml up -d \
  postgres-primary postgres-replica1 postgres-replica2 postgres-replica3 \
  redis clickhouse
```

### Kubernetes (Production)

```bash
# Apply base manifests
kubectl apply -k deployments/k8s/base/

# Or use overlays
kubectl apply -k deployments/k8s/overlays/production/
```

Includes:
- **Deployments** with security contexts (`runAsNonRoot`, `readOnlyRootFilesystem`, `drop ALL`)
- **StatefulSets** for PostgreSQL, Redis, ClickHouse, Elasticsearch
- **Horizontal Pod Autoscalers** for all services
- **NetworkPolicies** restricting traffic between services
- **Pod Disruption Budgets** (production overlay)
- **CronJob** for cleanup-worker
- **Ingress** with path-based routing

### CI/CD

The GitHub Actions pipeline runs on every push to `main` and `refactor`:

| Job | What it does |
|-----|-------------|
| **Lint** | `golangci-lint` (errcheck, staticcheck, govet, etc.) |
| **Test** | `go test -race` with coverage |
| **Vuln** | `govulncheck` (informational, non-blocking) |
| **Build** | `go build ./cmd/...` |
| **Docker** | Build all 7 Docker images, push to GHCR on main |

---

## Development

### Build all services

```bash
go build ./cmd/...
```

### Run tests

```bash
# Unit tests
go test ./...

# With race detector
go test -race ./...

# Integration tests (requires running infrastructure)
INTEGRATION_TEST=true go test ./test/integration/ -v
```

### Lint

```bash
golangci-lint run ./...
```

### Generate Protobuf

```bash
protoc --go_out=. --go-grpc_out=. proto/**/*.proto
```

---

## Design Decisions

| Decision | Choice | Why |
|----------|--------|-----|
| **ID generation** | Snowflake + Base62 | Time-sortable, no coordination needed, 7-char codes |
| **Inter-service comm** | gRPC | Type safety, streaming, smaller payload than JSON |
| **Analytics store** | ClickHouse | Column-oriented, materialized views, 100x faster than PostgreSQL for aggregations |
| **Event pipeline** | Redis Streams | Built-in consumer groups, at-least-once delivery, no Kafka overhead |
| **Custom alias locking** | Redis distributed lock | Prevents race conditions; Lua-script-based release for safety |
| **Caching strategy** | L1 (LRU) + L2 (Redis) | Sub-millisecond L1 hits; L2 survives restarts |
| **DI framework** | Uber FX | Lifecycle hooks solve graceful shutdown; constructor injection catches missing deps at startup |
| **Database replication** | 1 primary + 3 replicas | Writes to primary, reads distributed across replicas |

---

## Documentation

The `docs/deep-dive/` directory contains a 12-chapter technical walkthrough:

1. [Big Picture](docs/deep-dive/01-big-picture.md) -- System overview
2. [Database Architecture](docs/deep-dive/02-database-architecture.md) -- PostgreSQL replication, ClickHouse schema
3. [Messaging & Queuing](docs/deep-dive/03-messaging-queuing.md) -- Redis Streams pipeline
4. [Caching Strategy](docs/deep-dive/04-caching-strategy.md) -- Multi-tier cache design
5. [Short Code Generation](docs/deep-dive/05-short-code-generation.md) -- Snowflake + Base62
6. [gRPC Communication](docs/deep-dive/06-grpc-internal-communication.md) -- Service-to-service calls
7. [Authentication & JWT](docs/deep-dive/07-authentication-jwt.md) -- Auth flow
8. [Rate Limiting](docs/deep-dive/08-rate-limiting.md) -- Redis-based sliding window
9. [Background Workers](docs/deep-dive/09-workers.md) -- Pipeline, analytics, cleanup
10. [Code Walkthrough: Create URL](docs/deep-dive/10-code-walkthrough-create-url.md)
11. [Code Walkthrough: Redirect](docs/deep-dive/11-code-walkthrough-redirect.md)
12. [Scaling Strategy](docs/deep-dive/12-scaling-strategy.md) -- Horizontal scaling plan

---

## License

MIT License -- see [LICENSE](LICENSE) for details.

---

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Run tests (`go test -race ./...`)
4. Run linter (`golangci-lint run ./...`)
5. Commit your changes
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

---

<p align="center">
  Built by <a href="https://github.com/Varun5711">Varun Hotani</a>
</p>
