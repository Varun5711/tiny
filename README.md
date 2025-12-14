# Tiny URL Shortener

A high-performance, production-ready URL shortening service built with Go. Features microservices architecture, PostgreSQL with read replicas, Redis caching, ClickHouse analytics, and gRPC inter-service communication.

## Features

- **URL Shortening** - Generate short URLs with custom aliases or auto-generated IDs using Snowflake algorithm
- **High Performance** - Redis caching with read replicas for fast redirects
- **Real-time Analytics** - Click tracking with ClickHouse OLAP for analytics queries
- **User Management** - JWT-based authentication with user accounts
- **QR Code Generation** - Generate QR codes for shortened URLs
- **TTL Support** - Configurable expiration for URLs
- **TUI Client** - Terminal-based user interface for managing URLs

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   API Gateway   │────▶│   URL Service   │────▶│   PostgreSQL    │
│   (HTTP :8080)  │     │   (gRPC :50051) │     │   (Primary +    │
└─────────────────┘     └─────────────────┘     │   3 Replicas)   │
         │                       │              └─────────────────┘
         │                       │
         ▼                       ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│    Redirect     │────▶│     Redis       │     │   ClickHouse    │
│ Service (:8081) │     │   (Cache +      │     │   (Analytics)   │
└─────────────────┘     │    Streams)     │     └─────────────────┘
         │              └─────────────────┘              ▲
         │                       │                       │
         ▼                       ▼                       │
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Click Event    │────▶│   Analytics     │────▶│    Pipeline     │
│   (to Stream)   │     │    Worker       │     │    Worker       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Services

| Service | Port | Description |
|---------|------|-------------|
| `api-gateway` | 8080 | HTTP REST API for clients |
| `redirect-service` | 8081 | Fast URL redirection service |
| `url-service` | 50051 | gRPC service for URL CRUD operations |
| `user-service` | - | User authentication and management |
| `analytics-worker` | - | Processes click events from Redis streams |
| `pipeline-worker` | - | Aggregates analytics data to ClickHouse |
| `cleanup-worker` | - | Removes expired URLs |
| `tui` | - | Terminal UI client |

## Tech Stack

- **Language**: Go 1.25
- **Database**: PostgreSQL 16 (TimescaleDB) with streaming replication
- **Cache**: Redis 7
- **Analytics**: ClickHouse
- **Communication**: gRPC + Protocol Buffers
- **Authentication**: JWT
- **ID Generation**: Snowflake algorithm

## Getting Started

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- [Overmind](https://github.com/DarthSim/overmind) (process manager)

### Installation

```bash
# Clone the repository
git clone https://github.com/Varun5711/shorternit.git
cd shorternit

# Install dependencies
make install

# Start infrastructure (PostgreSQL, Redis, ClickHouse)
make db-up

# Start all services
make dev
```

### Configuration

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

Key environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_PRIMARY_DSN` | PostgreSQL primary connection | `postgres://...` |
| `DB_REPLICA*_DSN` | PostgreSQL replica connections | `postgres://...` |
| `REDIS_ADDR` | Redis server address | `localhost:6379` |
| `API_GATEWAY_PORT` | HTTP API port | `8080` |
| `REDIRECT_SERVICE_PORT` | Redirect service port | `8081` |
| `BASE_URL` | Base URL for short links | `http://localhost:8081` |
| `DEFAULT_URL_TTL` | Default URL expiration | `72h` |

## Project Structure

```
.
├── cmd/                    # Application entry points
│   ├── analytics-worker/   # Click analytics processor
│   ├── api-gateway/        # HTTP REST API
│   ├── cleanup-worker/     # Expired URL cleanup
│   ├── pipeline-worker/    # Analytics aggregation
│   ├── redirect-service/   # URL redirect handler
│   ├── tui/                # Terminal UI client
│   ├── url-service/        # Core URL gRPC service
│   └── user-service/       # User management
├── internal/               # Private application code
│   ├── analytics/          # Analytics processing
│   ├── auth/               # JWT authentication
│   ├── cache/              # Redis caching layer
│   ├── clickhouse/         # ClickHouse client
│   ├── config/             # Configuration loading
│   ├── database/           # PostgreSQL connection
│   ├── enrichment/         # Click data enrichment
│   ├── events/             # Event definitions
│   ├── grpc/               # gRPC client/server
│   ├── handlers/           # HTTP handlers
│   ├── idgen/              # Snowflake ID generator
│   ├── lock/               # Distributed locking
│   ├── logger/             # Structured logging
│   ├── middleware/         # HTTP middleware
│   ├── models/             # Data models
│   ├── qrcode/             # QR code generation
│   ├── redis/              # Redis client
│   ├── service/            # Business logic
│   ├── storage/            # Data persistence
│   └── validation/         # Input validation
├── proto/                  # Protocol Buffer definitions
├── api/                    # API specifications
├── deployments/            # Deployment configurations
│   ├── docker-compose/     # Docker Compose files
│   ├── kubernetes/         # Kubernetes manifests
│   └── terraform/          # Terraform configs
├── migrations/             # Database migrations
├── scripts/                # Utility scripts
├── test/                   # Test files
├── docs/                   # Documentation
├── Makefile                # Build automation
├── Procfile                # Process definitions
└── go.mod                  # Go module definition
```

## API Reference

### Create Short URL

```bash
POST /api/v1/urls
Content-Type: application/json

{
  "original_url": "https://example.com/very/long/url",
  "custom_alias": "my-link",  // optional
  "ttl": "168h"               // optional
}
```

Response:
```json
{
  "short_code": "my-link",
  "short_url": "http://localhost:8081/my-link",
  "original_url": "https://example.com/very/long/url",
  "expires_at": "2024-01-15T00:00:00Z"
}
```

### Redirect

```bash
GET /{short_code}
# Returns 301 redirect to original URL
```

### Get URL Info

```bash
GET /api/v1/urls/{short_code}
```

### Get Analytics

```bash
GET /api/v1/urls/{short_code}/analytics
```

## Development

### Build All Services

```bash
make build
```

### Run Tests

```bash
make test
```

### Database Management

```bash
# Start databases
make db-up

# Stop databases
make db-down

# Reset databases (WARNING: destroys data)
make db-reset
```

### Clean Build Artifacts

```bash
make clean
```

## Deployment

### Docker Compose (Development)

```bash
cd deployments/docker-compose
docker-compose up
```

### Kubernetes

See `deployments/kubernetes/` for Kubernetes manifests.

### Terraform

See `deployments/terraform/` for infrastructure-as-code.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
