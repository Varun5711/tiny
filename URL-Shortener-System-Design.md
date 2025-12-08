# URL Shortener - System Design & Implementation Plan (Resume/Learning Project)

## Project Overview
Building a **production-ready URL shortener in Go** demonstrating system design principles with:
- Microservices architecture (Docker Compose for local, K8s-ready)
- Cost-effective multi-database strategy (PostgreSQL, Redis, ClickHouse)
- Full feature set: Analytics, Custom URLs, Auth, Advanced Features
- **Budget: Under $50/month** (using free tiers + minimal cloud resources)
- **Goal: Portfolio project showcasing enterprise patterns at startup scale**

---

## Architecture Overview

### Project Structure
```
url-shortener/
├── services/                 # Backend microservices
│   ├── api-gateway/          # HTTP entry point, REST API
│   ├── url-service/          # Core URL shortening (gRPC)
│   ├── redirect-service/     # High-performance redirect handler
│   ├── analytics-service/    # Click tracking ingestion (gRPC)
│   ├── user-service/         # Authentication & user management (gRPC)
│   ├── custom-url-service/   # Custom alias management (gRPC)
│   └── reporting-service/    # Analytics queries & dashboards (gRPC)
├── clients/                  # Frontend applications
│   ├── tui-app/              # Terminal UI client (Go + Bubble Tea/tview)
│   │   ├── views/
│   │   ├── components/
│   │   ├── services/         # gRPC client for backend
│   │   └── main.go
│   └── web-dashboard/        # Optional: Web UI (React/Next.js)
├── workers/
│   ├── analytics-worker/     # Batch processing of click events
│   └── cleanup-worker/       # Expired link removal
├── proto/                    # gRPC Protocol Buffers
│   ├── url/url.proto
│   ├── analytics/analytics.proto
│   ├── user/user.proto
│   └── custom/custom.proto
├── shared/
│   ├── pkg/                  # Shared libraries
│   │   ├── id-generator/     # Snowflake ID generation
│   │   ├── cache/            # Redis client wrapper
│   │   ├── middleware/       # Auth, logging, tracing
│   │   ├── metrics/          # Prometheus metrics
│   │   ├── database/         # DB connection pools (read/write split)
│   │   └── grpc/             # gRPC client/server helpers
├── infrastructure/
│   ├── docker/               # Docker configurations
│   ├── k8s/                  # Kubernetes manifests
│   ├── helm/                 # Helm charts
│   ├── terraform/            # Infrastructure as Code
│   └── monitoring/           # Prometheus, Grafana configs
└── scripts/
    ├── migration/            # Database migrations
    ├── proto-gen/            # Protobuf code generation
    └── load-testing/         # k6 scripts
```

### Communication Architecture

**External (Client → API Gateway):**
- REST API (HTTP/JSON) for public/web clients
- gRPC for TUI client (more efficient, binary protocol)

**Internal (Service → Service):**
- gRPC for all inter-service communication
- Benefits: Type safety, performance, streaming support, HTTP/2 multiplexing

---

## System Design Deep Dive

### 1. Database Architecture: Read/Write Split

**PostgreSQL Primary-Replica Setup:**
```
┌─────────────┐
│  Write Path │
└─────────────┘
      ↓
┌─────────────────┐
│ PostgreSQL      │
│ PRIMARY         │ ← Writes (URL creation, updates, deletes)
│ (Write Queries) │
└────────┬────────┘
         │ Streaming Replication (WAL)
         ├─────────────┬──────────────┐
         ↓             ↓              ↓
┌────────────┐  ┌────────────┐  ┌────────────┐
│ REPLICA 1  │  │ REPLICA 2  │  │ REPLICA 3  │
│ (Reads)    │  │ (Reads)    │  │ (Reads)    │
└────────────┘  └────────────┘  └────────────┘
         ↑             ↑              ↑
         └─────────────┴──────────────┘
                Read Load Balancer
```

**Read/Write Split Strategy:**

1. **Primary Database (Write):**
   - All INSERT, UPDATE, DELETE operations
   - URL creation, user registration
   - Custom alias claims
   - Link status updates

2. **Replica Databases (Read):**
   - URL lookups for redirects
   - Analytics queries
   - User profile reads
   - Dashboard data
   - Load balanced round-robin

3. **Database Client Implementation:**
```go
type DBManager struct {
    primary  *pgxpool.Pool  // Write connection
    replicas []*pgxpool.Pool // Read connections (load balanced)
}

// Write operations go to primary
func (db *DBManager) CreateURL(ctx context.Context, url *URL) error {
    return db.primary.QueryRow(ctx, "INSERT INTO urls...")
}

// Read operations go to replicas (round-robin)
func (db *DBManager) GetURL(ctx context.Context, shortCode string) (*URL, error) {
    replica := db.selectReplica() // Round-robin selection
    return replica.QueryRow(ctx, "SELECT * FROM urls WHERE short_code = $1", shortCode)
}
```

4. **Replication Lag Handling:**
   - Typical lag: <100ms
   - For critical reads-after-writes: Read from primary
   - Cache writes immediately (Redis)
   - Show "processing" state in GUI if needed

---

### 2. URL Shortening Algorithm & Uniqueness Guarantees

**ID Generation Strategy: Snowflake-based distributed ID**
- 64-bit unique IDs: `[timestamp:41][datacenter:5][worker:5][sequence:12]`
- Guarantees uniqueness across distributed systems
- Time-ordered for better DB performance
- Convert to Base62 for short URLs (7-8 characters)

**Uniqueness Handling - Three Approaches:**

**Approach 1: Database Unique Constraint (Recommended)**
```sql
CREATE TABLE urls (
    id BIGINT PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,  -- Database enforces uniqueness
    long_url TEXT NOT NULL,
    user_id BIGINT REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Unique index for fast lookups
CREATE UNIQUE INDEX idx_urls_short_code ON urls(short_code);
```

**Application Code:**
```go
func (s *URLService) CreateShortURL(longURL string) (string, error) {
    for attempt := 0; attempt < 3; attempt++ {
        // Generate Snowflake ID
        id := s.idGen.NextID()
        shortCode := base62.Encode(id)

        // Try to insert (database will reject if duplicate)
        err := s.db.Insert(ctx, shortCode, longURL)
        if err == nil {
            // Success - cache it
            s.cache.Set(shortCode, longURL)
            return shortCode, nil
        }

        // Check if error is uniqueness violation
        if isUniqueViolation(err) {
            // Collision detected, retry with new ID
            continue
        }

        return "", err  // Other error
    }
    return "", errors.New("failed to generate unique code")
}
```

**Approach 2: Distributed Lock (For Custom Aliases)**
```go
func (s *URLService) CreateCustomAlias(alias, longURL string) error {
    // Use Redis distributed lock
    lock := s.redis.Lock(fmt.Sprintf("lock:alias:%s", alias), 5*time.Second)
    acquired, err := lock.Acquire()
    if !acquired {
        return errors.New("alias already claimed")
    }
    defer lock.Release()

    // Double-check availability
    exists := s.db.ExistsInPrimary(alias)  // MUST check primary, not replica
    if exists {
        return errors.New("alias already taken")
    }

    // Insert
    return s.db.Insert(ctx, alias, longURL)
}
```

**Approach 3: Pre-check + Retry (Optimistic)**
```go
func (s *URLService) CreateShortURL(longURL string) (string, error) {
    for attempt := 0; attempt < 3; attempt++ {
        shortCode := s.generateCode()

        // Check Redis cache first (fast path)
        if s.cache.Exists(shortCode) {
            continue // Collision, retry
        }

        // Check database (READ from PRIMARY for latest data)
        exists := s.db.ExistsPrimary(shortCode)
        if exists {
            continue // Collision, retry
        }

        // Try to insert
        err := s.db.Insert(ctx, shortCode, longURL)
        if err == nil {
            s.cache.Set(shortCode, longURL)
            return shortCode, nil
        }

        if isUniqueViolation(err) {
            continue
        }
        return "", err
    }
    return "", errors.New("failed after retries")
}
```

**Why Snowflake IDs Minimize Collisions:**
- 64-bit space: 18.4 quintillion possibilities
- Time-ordered: Old IDs never generated again
- Worker ID: Different services generate different IDs
- Collision probability: ~0% for reasonable scale
- Expected collisions: 0 in first 10 billion URLs

**Custom URL Collision Handling:**
- Must check PRIMARY database (not replicas - stale data risk)
- Use distributed lock (Redis) for critical section
- Return friendly error: "Alias already taken, try: url1, url2, url3"

### 2. Database Schema Design

#### PostgreSQL (Metadata & User Data)
```sql
-- Users table
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    api_key UUID UNIQUE NOT NULL,
    tier VARCHAR(50) DEFAULT 'free',
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_users_api_key ON users(api_key);

-- URL Mappings (hot data only, recent links)
CREATE TABLE urls (
    id BIGINT PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    long_url TEXT NOT NULL,
    user_id BIGINT REFERENCES users(id),
    custom_alias VARCHAR(50),
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    is_active BOOLEAN DEFAULT TRUE
);
CREATE INDEX idx_urls_short_code ON urls(short_code);
CREATE INDEX idx_urls_user_id ON urls(user_id);
CREATE INDEX idx_urls_expires_at ON urls(expires_at) WHERE expires_at IS NOT NULL;

-- Custom domains
CREATE TABLE custom_domains (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id),
    domain VARCHAR(255) UNIQUE NOT NULL,
    verified BOOLEAN DEFAULT FALSE
);
```

#### TimescaleDB Extension on PostgreSQL (Time-Series Analytics)
**Why not Cassandra?** Cost - we'll use PostgreSQL with TimescaleDB extension (free) for time-series data
```sql
-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Click events hypertable (auto-partitioned by time)
CREATE TABLE click_events (
    click_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_code VARCHAR(10) NOT NULL,
    clicked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT,
    referer TEXT,
    country VARCHAR(2),
    city VARCHAR(100),
    device_type VARCHAR(50)
);

-- Convert to hypertable (auto-partitioning by time)
SELECT create_hypertable('click_events', 'clicked_at');

-- Create indexes
CREATE INDEX idx_click_events_short_code ON click_events(short_code, clicked_at DESC);
CREATE INDEX idx_click_events_clicked_at ON click_events(clicked_at DESC);

-- Continuous aggregates for fast queries (materialized views)
CREATE MATERIALIZED VIEW daily_stats
WITH (timescaledb.continuous) AS
SELECT
    short_code,
    time_bucket('1 day', clicked_at) AS day,
    COUNT(*) as total_clicks,
    COUNT(DISTINCT ip_address) as unique_visitors
FROM click_events
GROUP BY short_code, day;

-- Refresh policy (auto-update every hour)
SELECT add_continuous_aggregate_policy('daily_stats',
    start_offset => INTERVAL '3 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');

-- Data retention policy (auto-delete after 90 days)
SELECT add_retention_policy('click_events', INTERVAL '90 days');
```

#### ClickHouse (OLAP Analytics) - Optional for Advanced Queries
**Deployment:** Run on cheapest VPS ($5-10/month) or locally for demo
```sql
-- Analytical queries, aggregations
CREATE TABLE click_analytics (
    short_code String,
    long_url String,
    clicked_at DateTime,
    ip_address String,
    country String,
    city String,
    referer String,
    user_agent String,
    device_type String,
    os String,
    browser String
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(clicked_at)
ORDER BY (short_code, clicked_at);

-- For learning: Can run ClickHouse locally via Docker
-- For resume demo: Deploy to Hetzner/DigitalOcean ($5-10/month)
```

#### Redis (Caching Strategy)
```
Key Patterns:
- url:short:{code} → long_url (TTL: 1 hour)
- url:meta:{code} → JSON metadata (TTL: 10 min)
- user:ratelimit:{api_key}:{window} → counter (TTL: 1 min)
- stats:clicks:{code}:{date} → counter (TTL: 7 days)
- cache:hot:urls → Sorted Set (top 1M URLs by access frequency)
```

### 3. gRPC Service Definitions

**Proto Files Structure:**

```protobuf
// proto/url/url.proto
syntax = "proto3";
package url;
option go_package = "github.com/yourusername/url-shortener/proto/url";

service URLService {
  rpc CreateShortURL(CreateShortURLRequest) returns (CreateShortURLResponse);
  rpc CreateCustomURL(CreateCustomURLRequest) returns (CreateCustomURLResponse);
  rpc GetURL(GetURLRequest) returns (GetURLResponse);
  rpc UpdateURL(UpdateURLRequest) returns (UpdateURLResponse);
  rpc DeleteURL(DeleteURLRequest) returns (DeleteURLResponse);
  rpc ListUserURLs(ListUserURLsRequest) returns (stream URLResponse);
}

message CreateShortURLRequest {
  string long_url = 1;
  int64 user_id = 2;
  optional int64 expires_at = 3;
}

message CreateShortURLResponse {
  string short_code = 1;
  string short_url = 2;
  int64 created_at = 3;
}

// ... other messages
```

```protobuf
// proto/analytics/analytics.proto
syntax = "proto3";
package analytics;

service AnalyticsService {
  rpc TrackClick(TrackClickRequest) returns (TrackClickResponse);
  rpc GetURLStats(GetURLStatsRequest) returns (GetURLStatsResponse);
  rpc GetTimeSeries(GetTimeSeriesRequest) returns (GetTimeSeriesResponse);
  rpc GetGeoStats(GetGeoStatsRequest) returns (GetGeoStatsResponse);
}

message TrackClickRequest {
  string short_code = 1;
  string ip_address = 2;
  string user_agent = 3;
  string referer = 4;
}
```

```protobuf
// proto/user/user.proto
syntax = "proto3";
package user;

service UserService {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc GetProfile(GetProfileRequest) returns (GetProfileResponse);
  rpc GenerateAPIKey(GenerateAPIKeyRequest) returns (GenerateAPIKeyResponse);
}
```

**Code Generation:**
```bash
# scripts/proto-gen/generate.sh
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/**/*.proto
```

---

### 4. Terminal UI Client (Go + Bubble Tea/tview)

**Technology Choice:**

**Option 1: Bubble Tea (Recommended - Modern, Elm-inspired)**
- Built by Charm.sh team
- Functional, reactive architecture
- Beautiful, modern TUI components
- Active development & ecosystem
- Great for complex interactive apps

**Option 2: tview**
- More traditional, imperative style
- Rich widget library out of the box
- Easier for beginners
- Table, form, list widgets included

**TUI Client Structure:**
```
clients/tui-app/
├── main.go                    # Entry point
├── app.go                     # Main application state
├── views/
│   ├── dashboard.go           # Main dashboard view
│   ├── create_url.go          # URL creation form
│   ├── url_list.go            # List of URLs
│   ├── analytics.go           # Analytics view
│   └── settings.go            # Settings view
├── components/
│   ├── table.go               # URL table component
│   ├── form.go                # Input form component
│   ├── chart.go               # ASCII bar/line charts
│   ├── spinner.go             # Loading spinner
│   └── statusbar.go           # Bottom status bar
├── services/
│   ├── grpc_client.go         # gRPC connection manager
│   ├── url_client.go          # URL service client
│   ├── analytics_client.go    # Analytics service client
│   └── auth_client.go         # User service client
├── models/
│   ├── url.go                 # Data models
│   └── state.go               # Application state
├── styles/
│   └── theme.go               # Color schemes
└── config/
    └── config.go              # App configuration
```

**TUI Features:**

1. **Main Dashboard (Tab 1):**
   ```
   ┌─ URL Shortener Dashboard ─────────────────────────────────┐
   │ [Create] [Refresh] [Settings]          User: john@doe.com │
   ├────────────────────────────────────────────────────────────┤
   │ Short Code  │ Long URL            │ Clicks │ Created      │
   ├─────────────┼─────────────────────┼────────┼──────────────┤
   │ abc123      │ https://google.com  │ 1,234  │ 2 hours ago  │
   │ xyz789      │ https://github.com  │ 567    │ 1 day ago    │
   │ custom-url  │ https://example.com │ 89     │ 3 days ago   │
   └────────────────────────────────────────────────────────────┘
   [Tab] Switch View  [Enter] Details  [d] Delete  [q] Quit
   ```

2. **URL Creation Form (Tab 2):**
   ```
   ┌─ Create Short URL ─────────────────────────────────────────┐
   │                                                             │
   │  Long URL:                                                  │
   │  ┌─────────────────────────────────────────────────────┐   │
   │  │ https://your-long-url.com/path                      │   │
   │  └─────────────────────────────────────────────────────┘   │
   │                                                             │
   │  Custom Alias (optional):                                   │
   │  ┌─────────────────────────────────────────────────────┐   │
   │  │ my-custom-alias                                     │   │
   │  └─────────────────────────────────────────────────────┘   │
   │                                                             │
   │  Expiration: [○] Never  [●] 1 day  [○] 7 days  [○] Custom │
   │                                                             │
   │                                      [Create] [Cancel]     │
   └────────────────────────────────────────────────────────────┘
   ```

3. **Analytics View (Tab 3):**
   ```
   ┌─ Analytics: abc123 ────────────────────────────────────────┐
   │ Short URL: short.ly/abc123                                 │
   │ Total Clicks: 1,234        Unique Visitors: 892            │
   │                                                             │
   │ Clicks Over Time (Last 7 Days):                            │
   │ ▁▃▅██▇▅                                                    │
   │                                                             │
   │ Top Countries:                    Top Referrers:           │
   │  1. United States    45%           1. Google      32%      │
   │  2. United Kingdom   22%           2. Direct      28%      │
   │  3. Canada          15%           3. Twitter     18%      │
   │                                                             │
   │ Device Breakdown:                                           │
   │ Mobile: ████████░░ 80%                                     │
   │ Desktop: ██░░░░░░░░ 20%                                    │
   └────────────────────────────────────────────────────────────┘
   ```

**Sample Bubble Tea Code:**
```go
// clients/tui-app/main.go
package main

import (
    "fmt"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
    pb "github.com/yourusername/url-shortener/proto/url"
)

type model struct {
    grpcClient  *GRPCManager
    urlClient   pb.URLServiceClient
    urls        []*URL
    cursor      int
    currentView string  // "dashboard", "create", "analytics"
    loading     bool
}

type URLListMsg []*URL

func (m model) Init() tea.Cmd {
    return fetchURLs(m.urlClient)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit

        case "tab":
            // Switch between views
            views := []string{"dashboard", "create", "analytics"}
            currentIdx := indexOf(m.currentView, views)
            m.currentView = views[(currentIdx+1)%len(views)]
            return m, nil

        case "enter":
            if m.currentView == "dashboard" {
                // Open detailed view for selected URL
                m.currentView = "analytics"
                return m, nil
            }

        case "up", "k":
            if m.cursor > 0 {
                m.cursor--
            }

        case "down", "j":
            if m.cursor < len(m.urls)-1 {
                m.cursor++
            }

        case "n":
            // New URL - switch to create view
            m.currentView = "create"
            return m, nil
        }

    case URLListMsg:
        m.urls = msg
        m.loading = false
        return m, nil
    }

    return m, nil
}

func (m model) View() string {
    switch m.currentView {
    case "dashboard":
        return m.renderDashboard()
    case "create":
        return m.renderCreateForm()
    case "analytics":
        return m.renderAnalytics()
    default:
        return "Unknown view"
    }
}

func (m model) renderDashboard() string {
    var s strings.Builder

    // Header
    headerStyle := lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("#FAFAFA")).
        Background(lipgloss.Color("#7D56F4")).
        Padding(0, 1)

    s.WriteString(headerStyle.Render("URL Shortener Dashboard"))
    s.WriteString("\n\n")

    // URL table
    tableStyle := lipgloss.NewStyle().
        BorderStyle(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("#874BFD"))

    table := "Short Code │ Long URL │ Clicks\n"
    table += "───────────┼──────────┼────────\n"

    for i, url := range m.urls {
        cursor := " "
        if m.cursor == i {
            cursor = ">"
        }
        table += fmt.Sprintf("%s %s │ %s │ %d\n",
            cursor, url.ShortCode, truncate(url.LongURL, 30), url.Clicks)
    }

    s.WriteString(tableStyle.Render(table))
    s.WriteString("\n\n")

    // Footer
    s.WriteString("Tab: Switch view | n: New URL | Enter: Details | q: Quit\n")

    return s.String()
}

func fetchURLs(client pb.URLServiceClient) tea.Cmd {
    return func() tea.Msg {
        // gRPC call to fetch URLs
        stream, err := client.ListUserURLs(context.Background(),
            &pb.ListUserURLsRequest{UserId: 1})
        if err != nil {
            return errMsg{err}
        }

        var urls []*URL
        for {
            resp, err := stream.Recv()
            if err == io.EOF {
                break
            }
            if err != nil {
                return errMsg{err}
            }
            urls = append(urls, &URL{
                ShortCode: resp.ShortCode,
                LongURL:   resp.LongUrl,
                Clicks:    resp.ClickCount,
            })
        }

        return URLListMsg(urls)
    }
}

func main() {
    // Connect to gRPC backend
    grpcClient, err := NewGRPCManager("localhost:50051")
    if err != nil {
        fmt.Printf("Error connecting to backend: %v\n", err)
        os.Exit(1)
    }
    defer grpcClient.Close()

    // Initialize model
    m := model{
        grpcClient:  grpcClient,
        urlClient:   grpcClient.URLClient(),
        currentView: "dashboard",
        urls:        []*URL{},
    }

    // Run TUI
    p := tea.NewProgram(m, tea.WithAltScreen())
    if err := p.Start(); err != nil {
        fmt.Printf("Error running TUI: %v\n", err)
        os.Exit(1)
    }
}
```

**gRPC Client Connection:**
```go
// clients/gui-app/services/grpc_client.go
package services

import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

type GRPCManager struct {
    conn *grpc.ClientConn
}

func NewGRPCManager(address string) (*GRPCManager, error) {
    conn, err := grpc.Dial(address,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(10 * 1024 * 1024)),
    )
    if err != nil {
        return nil, err
    }

    return &GRPCManager{conn: conn}, nil
}

func (g *GRPCManager) Close() {
    g.conn.Close()
}

func (g *GRPCManager) URLClient() pb.URLServiceClient {
    return pb.NewURLServiceClient(g.conn)
}

func (g *GRPCManager) AnalyticsClient() pb.AnalyticsServiceClient {
    return pb.NewAnalyticsServiceClient(g.conn)
}
```

---

### 5. Service Breakdown

#### API Gateway Service
**Responsibilities:**
- Request routing to microservices
- Authentication/Authorization (JWT validation)
- Rate limiting (Redis-based sliding window)
- Request/response logging
- Circuit breaker for downstream services

**Tech Stack:** Go + Chi Router + go-redis + prometheus client

**Key Features:**
- Dynamic rate limiting based on user tier
- API key validation middleware
- Request ID generation for distributed tracing
- CORS handling for web clients

---

#### Redirect Service (Critical Path - Ultra-Optimized)
**Responsibilities:**
- Handle GET /{shortCode} redirects
- Minimal latency (target: <10ms p99)
- High throughput (millions RPS)

**Optimization Strategies:**
1. **Multi-tier caching:**
   - L1: In-memory LRU cache (10K hottest URLs)
   - L2: Redis cluster (distributed cache)
   - L3: PostgreSQL read replicas

2. **Async click tracking:**
   - Fire-and-forget event to Kafka
   - Non-blocking response to user

3. **Connection pooling:**
   - Persistent DB connections
   - HTTP/2 for service-to-service

**Pseudo-code flow:**
```
1. Check L1 cache → return if hit
2. Check Redis → update L1, return if hit
3. Query PostgreSQL → update L2+L1, return
4. Publish click event to Kafka (async)
5. HTTP 301/302 redirect
```

---

#### URL Service
**Responsibilities:**
- Create short URLs (POST /api/v1/urls)
- Retrieve URL metadata (GET /api/v1/urls/{code})
- Update URL settings
- Delete/deactivate URLs

**Key Operations:**
1. **Create URL:**
   - Generate Snowflake ID
   - Convert to Base62
   - Check uniqueness (Redis + DB)
   - Store in PostgreSQL
   - Warm Redis cache
   - Return short URL

2. **Custom URL creation:**
   - Validate alias availability
   - Check against reserved words
   - Atomic insert with conflict handling

**Database strategy:**
- Write to PostgreSQL primary
- Replicate to read replicas
- Invalidate Redis cache on updates

---

#### Analytics Service
**Responsibilities:**
- Ingest click events from Kafka
- Enrich with geolocation (MaxMind GeoIP2)
- Parse User-Agent (ua-parser)
- Write to Cassandra + ClickHouse

**Data Pipeline:**
```
RabbitMQ Queue (clicks)
  → Analytics Service (Consumer)
  → Batch processing (50-100 events/batch)
  → Enrich data (GeoIP, UA parsing)
  → Write to PostgreSQL/TimescaleDB (time-series)
  → Optional: Write to ClickHouse (batch insert for OLAP)
```

**Event Schema (RabbitMQ Message):**
```json
{
  "click_id": "uuid",
  "short_code": "abc123",
  "timestamp": "2025-12-08T10:30:00Z",
  "ip": "1.2.3.4",
  "user_agent": "Mozilla/5.0...",
  "referer": "https://google.com"
}
```

---

#### User Service
**Responsibilities:**
- User registration/login
- API key generation & rotation
- Tier management (free/pro/enterprise)
- OAuth integration (Google, GitHub)

**Authentication Flow:**
- JWT tokens (15min expiry)
- Refresh tokens (30 days, stored in Redis)
- API keys for programmatic access
- RBAC for enterprise users

---

#### Custom URL Service
**Responsibilities:**
- Validate custom aliases
- Manage custom domains
- QR code generation
- Link preview metadata extraction

**Features:**
- Reserved word blacklist
- Profanity filter
- Domain verification (TXT record)
- QR code generation using go-qrcode library

---

#### Reporting Service
**Responsibilities:**
- Query ClickHouse for analytics
- Generate reports & dashboards
- Export data (CSV, JSON)
- Real-time metrics via WebSocket

**Analytics Endpoints:**
- GET /api/v1/analytics/{code}/summary
- GET /api/v1/analytics/{code}/timeseries
- GET /api/v1/analytics/{code}/geo
- GET /api/v1/analytics/{code}/devices
- GET /api/v1/analytics/{code}/referrers

---

### 4. Scaling Strategies

#### Horizontal Scaling (Kubernetes HPA)
```yaml
# Redirect service - autoscale based on CPU + Custom Metrics
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: redirect-service-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: redirect-service
  minReplicas: 10
  maxReplicas: 100
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Pods
    pods:
      metric:
        name: http_requests_per_second
      target:
        type: AverageValue
        averageValue: "1000"
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 100
        periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
```

#### Database Scaling
1. **PostgreSQL:**
   - Primary-replica setup (1 primary, 5+ read replicas)
   - Connection pooling via PgBouncer
   - Partition old data by time range
   - Archive to S3 after 90 days

2. **Cassandra:**
   - 6+ node cluster (RF=3)
   - Multi-datacenter replication
   - Compaction strategy: TimeWindowCompactionStrategy

3. **Redis:**
   - Redis Cluster (6 nodes, 3 primaries)
   - Sentinel for HA
   - Eviction policy: allkeys-lru for cache keys

4. **ClickHouse:**
   - Distributed tables across 3+ shards
   - 2x replication per shard
   - Materialized views for pre-aggregation

#### Load Balancing
- **External:** Cloud Load Balancer (AWS ALB / GCP GLB)
- **Internal:** Kubernetes Service (ClusterIP)
- **Service Mesh:** Istio for advanced traffic management
  - Circuit breaking
  - Retry logic
  - Canary deployments

---

### 5. Observability & Monitoring

#### Metrics (Prometheus + Grafana)
**Key Metrics:**
- Request rate (req/s per service)
- Latency percentiles (p50, p95, p99)
- Error rate (4xx, 5xx)
- Cache hit ratio
- Database connection pool saturation
- Kafka consumer lag

**Service-level dashboards:**
- Redirect service: latency heatmap, cache efficiency
- Analytics service: ingestion rate, processing lag
- Database: query performance, replication lag

#### Logging (ELK Stack or Loki)
- Structured JSON logging
- Correlation IDs for request tracing
- Log aggregation across all pods
- Alert on error rate spikes

#### Tracing (Jaeger or Tempo)
- Distributed tracing across microservices
- OpenTelemetry instrumentation
- Trace sampling (1% of requests)

#### Alerting (Alertmanager)
- P99 latency > 50ms
- Error rate > 1%
- Pod crash loops
- Database replication lag > 5s
- Kafka consumer lag > 10k messages

---

### 6. High Availability & Disaster Recovery

#### Multi-Region Deployment
- **Active-Active:** Deploy to 3+ regions (US, EU, ASIA)
- **GeoDNS routing:** Route to nearest region
- **Cross-region replication:** Cassandra DC replication

#### Backup Strategy
- PostgreSQL: Continuous WAL archiving to S3
- Cassandra: Daily snapshots to object storage
- Redis: RDB snapshots every 5 minutes
- ClickHouse: Incremental backups

#### Disaster Recovery
- RTO: 15 minutes
- RPO: 5 minutes
- Automated failover for databases
- Runbooks for region failure scenarios

---

### 7. Security Considerations

#### API Security
- Rate limiting per API key & IP
- JWT token expiration & rotation
- API key scoping (read/write permissions)
- HTTPS only (TLS 1.3)

#### Data Protection
- Encryption at rest (DB-level)
- Encryption in transit (mTLS between services)
- PII anonymization in analytics
- GDPR compliance (data deletion API)

#### DDoS Protection
- Cloud-native DDoS mitigation (Cloudflare/AWS Shield)
- Application-level rate limiting
- Captcha for suspicious patterns
- IP blacklisting/whitelisting

#### Malicious URL Detection
- URL reputation check (Google Safe Browsing API)
- Spam/phishing detection
- User reporting mechanism
- Admin dashboard for moderation

---

### 8. Performance Targets

| Metric | Target | Strategy |
|--------|--------|----------|
| Redirect latency (p99) | <10ms | Multi-tier caching, optimized redirect service |
| URL creation latency | <100ms | Async operations, connection pooling |
| Throughput | 100K req/s | Horizontal scaling, load balancing |
| Cache hit rate | >95% | Intelligent caching, pre-warming |
| Database write latency | <50ms | Batch inserts, optimized schema |
| Availability | 99.99% | Multi-region, auto-failover |

---

## Resume Impact Strategy

### What Makes This Project Stand Out

**1. Demonstrates Enterprise Thinking at Startup Budget:**
- You understand both "ideal architecture" and "pragmatic architecture"
- Show ability to make cost/performance tradeoffs
- Document WHY you chose each technology (interviewers love this)

**2. Full-Stack System Design:**
- Backend microservices (Go)
- Database design (SQL optimization, indexing, partitioning)
- Caching strategies (multi-tier)
- Message queues (async processing)
- Infrastructure as Code (Docker Compose + K8s manifests)
- Monitoring (Prometheus, Grafana)

**3. Production-Ready Code:**
- Unit tests + integration tests (80%+ coverage)
- API documentation (OpenAPI/Swagger)
- README with architecture diagrams
- CI/CD pipelines (GitHub Actions)
- Proper error handling & logging
- Security best practices

**4. Live Demo + Documentation:**
- Working URL: `https://short.yourdomain.com`
- GitHub repo with detailed README
- Architecture diagram (draw.io or Excalidraw)
- Blog post explaining design decisions
- Load testing results (k6 scripts included)

### Talking Points for Interviews

**When asked "Tell me about a project you built":**
> "I built a production-grade URL shortener demonstrating distributed systems design. It handles thousands of requests per second using Go microservices, implements multi-tier caching for sub-10ms redirects, and uses TimescaleDB for time-series analytics. I deployed it on a $30/month VPS but designed it to scale horizontally—all the Kubernetes manifests are ready. The interesting challenge was optimizing the redirect path since every millisecond matters..."

**System Design Questions:**
- "How would you design a URL shortener?" → You've already done it!
- "How do you handle high traffic?" → Explain your caching strategy
- "How do you track analytics at scale?" → TimescaleDB partitioning + async processing
- "How would you scale this to millions of users?" → K8s HPA, database sharding strategy

### GitHub README Template
```markdown
# URL Shortener - Production-Grade System Design

![Architecture Diagram](./docs/architecture.png)

## 🚀 Features
- ⚡ Sub-10ms redirects with multi-tier caching
- 📊 Real-time analytics (clicks, geo, devices)
- 🔐 JWT authentication + API keys
- 🎨 Custom short URLs & branded domains
- 📱 QR code generation
- ⏱️ Link expiration & scheduling
- 🌍 Multi-region ready architecture

## 🏗️ Architecture
- **Microservices:** 7 services in Go
- **Databases:** PostgreSQL + TimescaleDB (time-series), Redis (cache)
- **Message Queue:** RabbitMQ (async click tracking)
- **Deployment:** Docker Compose (dev), Kubernetes (prod-ready)
- **Monitoring:** Prometheus + Grafana + Jaeger tracing

## 📈 Performance
- **Throughput:** 10K req/s (single VPS)
- **Latency:** P99 < 10ms (cached), P99 < 50ms (uncached)
- **Cache Hit Rate:** 95%+
- **Availability:** 99.9% (tested over 30 days)

## 🎯 System Design Highlights
[Explain your key decisions here]

## 📊 Load Testing Results
[Include k6 graphs/results]

## 💰 Cost Optimization
Built for learning/portfolio - runs on $30/month VPS but designed for enterprise scale.
```

---

## Implementation Phases

### Phase 1: Core Foundation (Week 1-2)
**Goal:** Working URL shortener with basic features

**Deliverables:**
- Project structure & shared libraries
- ID generation service (Snowflake algorithm)
- Redirect service (optimized for speed)
- URL service (create, retrieve, update, delete)
- PostgreSQL schema & migrations (with proper indexes)
- Redis caching layer (multi-tier)
- Docker Compose for local development
- Basic unit tests

**Files to create:**
```
url-shortener/
├── cmd/
│   ├── redirect-service/main.go
│   ├── url-service/main.go
│   └── api-gateway/main.go
├── internal/
│   ├── idgen/
│   │   ├── snowflake.go
│   │   └── snowflake_test.go
│   ├── cache/
│   │   ├── redis.go
│   │   ├── memory.go (LRU)
│   │   └── multi_tier.go
│   ├── database/
│   │   ├── postgres.go
│   │   └── migrations/001_init.sql
│   └── models/
│       └── url.go
├── pkg/
│   ├── middleware/
│   │   ├── logging.go
│   │   ├── recovery.go
│   │   └── cors.go
│   └── response/
│       └── json.go
├── docker-compose.yml
├── go.mod
└── README.md
```

**Key Implementation Details:**
- Snowflake ID: 41-bit timestamp + 10-bit machine ID + 12-bit sequence
- Base62 encoding for short URLs (a-z, A-Z, 0-9)
- Multi-tier cache: L1 (in-memory LRU) → L2 (Redis) → L3 (PostgreSQL)
- Connection pooling: pgxpool with min=5, max=25

---

### Phase 2: Analytics Pipeline (Week 3)
**Goal:** Track and analyze click events asynchronously

**Deliverables:**
- RabbitMQ setup & producers
- Analytics service (consumer with batch processing)
- TimescaleDB hypertables for click events
- Basic analytics endpoints (summary, time-series)
- GeoIP enrichment (MaxMind free DB)
- User-Agent parsing

**Files to create:**
```
├── cmd/
│   ├── analytics-service/main.go
│   └── analytics-worker/main.go
├── internal/
│   ├── queue/
│   │   ├── rabbitmq.go
│   │   └── producer.go
│   ├── analytics/
│   │   ├── enricher.go (GeoIP + UA parsing)
│   │   ├── batch_writer.go
│   │   └── aggregator.go
│   └── database/migrations/
│       └── 002_timescaledb.sql
├── scripts/
│   └── setup-geoip.sh (download MaxMind DB)
```

**Data Flow:**
1. Redirect service publishes click event to RabbitMQ (non-blocking)
2. Analytics worker consumes in batches (50-100 events)
3. Enrich with GeoIP + UA parsing
4. Batch insert to TimescaleDB
5. Continuous aggregates auto-refresh

---

### Phase 3: Authentication & User Management (Week 4)
**Goal:** Multi-tenant system with API keys and rate limiting

**Deliverables:**
- User service (register, login, profile)
- JWT authentication (access + refresh tokens)
- API key generation & management
- Rate limiting (Redis-based sliding window)
- API Gateway with auth middleware

**Files to create:**
```
├── cmd/
│   └── user-service/main.go
├── internal/
│   ├── auth/
│   │   ├── jwt.go
│   │   ├── apikey.go
│   │   └── password.go (bcrypt)
│   ├── middleware/
│   │   ├── auth.go
│   │   └── ratelimit.go
│   └── database/migrations/
│       └── 003_users.sql
```

**Rate Limiting Strategy:**
- Free tier: 100 req/hour per API key
- Pro tier: 10K req/hour
- Anonymous: 10 req/hour per IP
- Redis sliding window counters

---

### Phase 4: Advanced Features (Week 5)
**Goal:** Custom URLs, QR codes, link expiration

**Deliverables:**
- Custom alias validation & reservation
- Custom domain support (with DNS verification)
- QR code generation endpoint
- Link expiration (TTL)
- Link scheduling (publish at future date)
- URL preview/metadata extraction
- Cleanup worker (expired links)

**Files to create:**
```
├── cmd/
│   └── cleanup-worker/main.go
├── internal/
│   ├── custom/
│   │   ├── validator.go (reserved words, profanity)
│   │   ├── domain.go (DNS verification)
│   │   └── qrcode.go
│   ├── expiry/
│   │   └── scheduler.go
│   └── database/migrations/
│       └── 004_custom_features.sql
```

---

### Phase 5: Deployment & Infrastructure (Week 6)
**Goal:** Production-ready deployment configuration

**Deliverables:**
- Docker multi-stage builds (optimized images)
- Docker Compose production config
- Kubernetes manifests (K8s-ready, not deployed)
- HPA configurations
- Nginx reverse proxy config
- SSL/TLS setup (Let's Encrypt)
- CI/CD pipeline (GitHub Actions)

**Files to create:**
```
├── deployments/
│   ├── docker/
│   │   ├── Dockerfile.redirect
│   │   ├── Dockerfile.url-service
│   │   └── Dockerfile.analytics
│   ├── docker-compose.prod.yml
│   ├── k8s/
│   │   ├── namespace.yaml
│   │   ├── deployments/
│   │   │   ├── redirect-service.yaml
│   │   │   ├── url-service.yaml
│   │   │   └── analytics-service.yaml
│   │   ├── services/
│   │   ├── configmaps/
│   │   ├── secrets/
│   │   ├── hpa/
│   │   └── ingress.yaml
│   ├── nginx/
│   │   └── nginx.conf
│   └── scripts/
│       └── deploy-vps.sh
├── .github/
│   └── workflows/
│       ├── ci.yml (tests + build)
│       └── cd.yml (deploy)
```

**Deployment Strategy:**
- Local dev: Docker Compose (all services)
- Production: Single VPS via Docker Compose
- K8s manifests: Ready for scaling (portfolio showcase)

---

### Phase 6: Observability & Monitoring (Week 7)
**Goal:** Full observability stack

**Deliverables:**
- Prometheus metrics in all services
- Grafana dashboards (beautiful visualizations)
- Distributed tracing (Jaeger - optional)
- Structured logging (JSON format)
- Health check endpoints
- Alerting rules (basic)

**Files to create:**
```
├── internal/
│   ├── metrics/
│   │   ├── prometheus.go
│   │   └── middleware.go
│   └── observability/
│       ├── tracing.go
│       └── logging.go
├── deployments/
│   └── monitoring/
│       ├── prometheus/
│       │   ├── prometheus.yml
│       │   └── alerts.yml
│       ├── grafana/
│       │   └── dashboards/
│       │       ├── redirect-service.json
│       │       ├── url-service.json
│       │       └── analytics.json
│       └── docker-compose.monitoring.yml
```

**Key Metrics:**
- Request rate, latency (p50, p95, p99)
- Cache hit/miss ratio
- Database connection pool usage
- Queue depth (RabbitMQ)
- Error rates by service

---

### Phase 7: Testing & Documentation (Week 8)
**Goal:** Production-grade quality

**Deliverables:**
- Unit tests (80%+ coverage)
- Integration tests (API endpoints)
- Load testing (k6 scripts)
- Performance benchmarks
- API documentation (Swagger/OpenAPI)
- Comprehensive README
- Architecture diagrams

**Files to create:**
```
├── tests/
│   ├── integration/
│   │   ├── url_test.go
│   │   ├── redirect_test.go
│   │   └── analytics_test.go
│   └── load/
│       ├── k6-redirect.js
│       ├── k6-create-url.js
│       └── results/
├── docs/
│   ├── API.md (OpenAPI spec)
│   ├── ARCHITECTURE.md
│   ├── DEPLOYMENT.md
│   ├── diagrams/
│   │   ├── system-architecture.png
│   │   ├── data-flow.png
│   │   └── database-schema.png
│   └── CONTRIBUTING.md
├── api/
│   └── openapi.yaml
└── README.md (comprehensive)
```

**Load Testing Scenarios:**
- Redirect performance: 10K req/s sustained
- URL creation: 1K req/s sustained
- Analytics ingestion: 5K events/s
- Mixed workload: 80% read, 20% write

---

### Phase 8: Polish & Portfolio Presentation (Week 9-10)
**Goal:** Make it interview-ready

**Deliverables:**
- Security hardening (input validation, SQL injection prevention)
- Rate limiting refinement
- Error handling improvements
- Code cleanup & refactoring
- Performance optimization
- Blog post / technical writeup
- Demo video (optional)
- Deploy to public domain

**Portfolio Package:**
1. **GitHub Repo:**
   - Clean commit history
   - Detailed README with badges
   - Architecture diagrams
   - Setup instructions
   - Contributing guidelines

2. **Live Demo:**
   - Public URL (short.yourdomain.com)
   - Demo API keys
   - Sample analytics dashboard

3. **Documentation:**
   - Medium/Dev.to blog post explaining design
   - Lessons learned section
   - Performance optimization journey
   - Cost breakdown & optimization

4. **Presentation:**
   - 5-minute video walkthrough (Loom)
   - Slides explaining architecture
   - Load testing results with graphs

---

## Technology Stack Summary

**Languages & Frameworks:**
- Go 1.22+ (all services)
- Chi router (HTTP routing)
- gRPC for inter-service communication

**Databases:**
- PostgreSQL 16 + TimescaleDB extension (metadata + time-series)
- ClickHouse 23.x (OLAP - optional, can run locally)
- Redis 7.x (caching, rate limiting)

**Message Queue:**
- RabbitMQ (lighter weight than Kafka, sufficient for learning)
- Alternative: NATS (even lighter, still production-grade)

**Infrastructure:**
- Kubernetes 1.28+
- Helm 3.x
- Terraform
- Docker

**Observability:**
- Prometheus + Grafana
- Jaeger (tracing)
- ELK Stack or Grafana Loki (logging)

**Libraries:**
- `github.com/redis/go-redis/v9` - Redis client
- `github.com/jackc/pgx/v5` - PostgreSQL driver with connection pooling
- `github.com/ClickHouse/clickhouse-go/v2` - ClickHouse driver (optional)
- `github.com/rabbitmq/amqp091-go` - RabbitMQ client
- `github.com/golang-jwt/jwt/v5` - JWT authentication
- `github.com/prometheus/client_golang` - Prometheus metrics
- `go.opentelemetry.io/otel` - Distributed tracing
- `github.com/skip2/go-qrcode` - QR code generation
- `github.com/oschwald/geoip2-golang` - GeoIP lookup (MaxMind free DB)
- `github.com/ua-parser/uap-go` - User-Agent parsing
- `golang.org/x/crypto/bcrypt` - Password hashing
- `google.golang.org/grpc` - gRPC framework
- `google.golang.org/protobuf` - Protocol Buffers

**TUI Libraries:**
- `github.com/charmbracelet/bubbletea` - Terminal UI framework (recommended)
- `github.com/charmbracelet/lipgloss` - Styling for terminal UIs
- `github.com/charmbracelet/bubbles` - TUI components (spinner, table, etc)
- Alternative: `github.com/rivo/tview` - Rich terminal UI widgets

---

## Cost Breakdown (Under $50/month Budget)

### Option 1: Free Tier Only (Local Development + Demo)
**Cost: $0/month**
- Local development: Docker Compose
- Frontend hosting: Vercel/Netlify (free tier)
- Database: Supabase (free tier - 500MB PostgreSQL)
- Redis: Upstash (free tier - 10K commands/day)
- Demo deployment: Railway/Render free tier
- Message queue: RabbitMQ (self-hosted in Docker)
- **Perfect for:** Learning, local demos, portfolio showcase

### Option 2: Minimal Cloud (Recommended for Resume)
**Cost: ~$30-40/month**
- **VPS (Hetzner/DigitalOcean):** $12-20/month
  - 4GB RAM, 2 vCPU
  - Run all services via Docker Compose
  - PostgreSQL + TimescaleDB + Redis + ClickHouse + RabbitMQ
- **Upstash Redis (Serverless):** $0-10/month
  - Only for distributed caching across regions if needed
  - Optional: Use in-VPS Redis for single-region
- **Domain:** $12/year (~$1/month) - Namecheap
- **Cloudflare:** Free (CDN, DDoS protection, SSL)
- **Supabase (backup/demo DB):** Free tier
- **Monitoring:** Grafana Cloud (free tier - 10K metrics)

**Total: $33-41/month** (cheapest production-like setup)

### Option 3: Hybrid Approach
**Cost: ~$45-50/month**
- Option 2 VPS setup + Additional features:
- **Cloudflare Workers:** $5/month (10M requests)
  - Ultra-fast edge redirects
  - Shows understanding of edge computing
- **AWS S3 (backups):** $1-2/month (minimal usage)
- **Better VPS:** $25/month (8GB RAM) for smoother demos

**What you demonstrate with this budget:**
✅ Microservices architecture
✅ Database optimization (indexes, partitioning)
✅ Caching strategies (multi-tier)
✅ Async message processing
✅ Horizontal scaling design (K8s manifests ready)
✅ Monitoring & observability
✅ CI/CD pipelines
✅ System design thinking

**What recruiters see:**
- "Can architect scalable systems"
- "Understands cost optimization"
- "Production-ready code with tests"
- "DevOps and infrastructure knowledge"

---

## Summary & Execution Plan

### What We're Building
A **production-grade URL shortener** that demonstrates enterprise system design principles while staying within a **$30-50/month budget**. This isn't a toy project—it's a portfolio piece that showcases:

- Distributed systems architecture
- Database optimization & scaling strategies
- Caching & performance optimization
- Async message processing
- DevOps & infrastructure skills
- Testing & observability best practices

### Key Decisions Made
1. **PostgreSQL + TimescaleDB** instead of Cassandra (cost savings, still shows time-series understanding)
2. **RabbitMQ** instead of Kafka (lighter weight, sufficient for scale)
3. **Single VPS deployment** with K8s manifests ready (shows scaling understanding)
4. **All microservices architecture** maintained (demonstrates system design knowledge)
5. **Full observability stack** included (Prometheus, Grafana)

### Time Commitment
- **8-10 weeks** for full implementation (working part-time)
- **4-5 weeks** for MVP (core features + basic analytics)
- **2 weeks** for minimal working demo (just redirect + analytics)

### Next Steps (Phase 1 - Week 1)

If you approve this plan, I'll start with:

1. **Initialize project structure**
   - Create Go module with proper layout
   - Set up Docker Compose environment
   - Configure PostgreSQL + Redis

2. **Implement Snowflake ID generator**
   - Distributed unique ID generation
   - Base62 encoding
   - Unit tests

3. **Build redirect service**
   - Ultra-fast redirect handler
   - Multi-tier caching (in-memory + Redis)
   - Prometheus metrics

4. **Create URL service**
   - CRUD operations for URLs
   - PostgreSQL schema with proper indexes
   - Input validation

5. **Basic API Gateway**
   - Request routing
   - Logging middleware
   - Health checks

**By end of Week 1, you'll have:** A working URL shortener that can create and redirect URLs with sub-10ms latency (cached).

---

## Questions Before We Start

1. **Where should I create the project?**
   - Current directory: `/Users/varunhotani/`
   - Or specify a different location?

2. **Project name preference?**
   - `url-shortener` (simple)
   - `go-shortly` (catchy)
   - Something else?

3. **Which deployment option appeals to you?**
   - Option 1: Free tier only (local + free hosting)
   - Option 2: Single VPS for $30-40/month (recommended)
   - Option 3: Hybrid with edge computing ($45-50/month)

4. **Learning pace?**
   - Fast track: I implement quickly, you review and learn
   - Moderate: I explain key decisions as we go
   - Slow: Detailed explanations for each component

Ready to build something impressive for your resume?

---

## Eraser.io Architecture Diagram Prompt

Copy and paste this into https://app.eraser.io to generate the system architecture diagram:

```
title URL Shortener - System Architecture

// Client Layer
tui-client [icon: terminal, color: blue] {
  Bubble Tea TUI
  gRPC Client
}

web-client [icon: browser, color: blue] {
  React/Next.js
  REST API Client
}

// Gateway Layer
api-gateway [icon: api, color: green] {
  REST API (HTTP)
  gRPC Gateway
  Rate Limiting
  JWT Auth
  Request Routing
}

// Microservices Layer
url-service [icon: link, color: purple] {
  CreateURL (gRPC)
  GetURL (gRPC)
  UpdateURL (gRPC)
  Snowflake ID Gen
}

redirect-service [icon: arrow-right, color: orange] {
  HTTP GET /{code}
  Multi-tier Cache
  Ultra-fast Redirects
  Click Event Publisher
}

analytics-service [icon: chart, color: teal] {
  Track Clicks (gRPC)
  GeoIP Enrichment
  UA Parsing
  Batch Processing
}

user-service [icon: user, color: indigo] {
  Register/Login (gRPC)
  API Key Management
  JWT Tokens
}

custom-url-service [icon: edit, color: pink] {
  Custom Aliases (gRPC)
  Domain Verification
  QR Code Generation
}

reporting-service [icon: dashboard, color: cyan] {
  GetStats (gRPC)
  Time-series Queries
  Geo Analytics
  Export Data
}

// Worker Layer
analytics-worker [icon: cog, color: yellow] {
  RabbitMQ Consumer
  Batch Insert
  Data Enrichment
}

cleanup-worker [icon: trash, color: red] {
  Expired Links
  Scheduled Cleanup
}

// Data Layer
postgres-primary [icon: database, color: navy] {
  PostgreSQL Primary
  Writes Only
  WAL Streaming
}

postgres-replica-1 [icon: database, color: navy] {
  Replica 1
  Reads Only
}

postgres-replica-2 [icon: database, color: navy] {
  Replica 2
  Reads Only
}

postgres-replica-3 [icon: database, color: navy] {
  Replica 3
  Reads Only
}

redis-cluster [icon: memory, color: red] {
  Redis Cluster
  Multi-tier Cache
  Rate Limiting
  Distributed Locks
}

timescaledb [icon: time, color: purple] {
  TimescaleDB
  Click Events
  Continuous Aggregates
  Auto-partitioning
}

rabbitmq [icon: queue, color: orange] {
  RabbitMQ
  Click Events Queue
  Async Processing
}

clickhouse [icon: analytics, color: yellow] {
  ClickHouse (Optional)
  OLAP Queries
  Aggregations
}

// Monitoring Layer
prometheus [icon: graph, color: orange] {
  Prometheus
  Metrics Collection
}

grafana [icon: dashboard, color: orange] {
  Grafana
  Dashboards
  Alerting
}

jaeger [icon: trace, color: teal] {
  Jaeger
  Distributed Tracing
}

// Connections

// Client to Gateway
tui-client > api-gateway: gRPC
web-client > api-gateway: HTTPS/REST

// Gateway to Services (gRPC)
api-gateway > url-service: gRPC
api-gateway > user-service: gRPC
api-gateway > custom-url-service: gRPC
api-gateway > reporting-service: gRPC

// Redirect Service (Direct - No Gateway)
web-client > redirect-service: HTTP GET /{code}

// Services to Databases
url-service > postgres-primary: Writes
url-service > postgres-replica-1: Reads (Load Balanced)
url-service > postgres-replica-2: Reads (Load Balanced)
url-service > postgres-replica-3: Reads (Load Balanced)

redirect-service > redis-cluster: Cache Lookup
redirect-service > postgres-replica-1: Cache Miss Reads

user-service > postgres-primary: Writes
user-service > postgres-replica-1: Reads

custom-url-service > postgres-primary: Writes
custom-url-service > redis-cluster: Distributed Locks

// Replication
postgres-primary > postgres-replica-1: WAL Streaming
postgres-primary > postgres-replica-2: WAL Streaming
postgres-primary > postgres-replica-3: WAL Streaming

// Analytics Flow
redirect-service > rabbitmq: Publish Click Event
analytics-service > rabbitmq: Consume (gRPC)
analytics-worker > rabbitmq: Batch Consumer
analytics-worker > timescaledb: Batch Insert
analytics-worker > clickhouse: Optional OLAP

reporting-service > timescaledb: Query Aggregates
reporting-service > clickhouse: Complex Analytics

// Cache Flow
url-service > redis-cluster: Cache Writes
api-gateway > redis-cluster: Rate Limiting

// Monitoring
url-service > prometheus: Metrics
redirect-service > prometheus: Metrics
analytics-service > prometheus: Metrics
user-service > prometheus: Metrics
custom-url-service > prometheus: Metrics
reporting-service > prometheus: Metrics

prometheus > grafana: Metrics Data

url-service > jaeger: Traces
redirect-service > jaeger: Traces
analytics-service > jaeger: Traces
```

**Instructions for Eraser.io:**
1. Go to https://app.eraser.io
2. Create new diagram
3. Select "Diagram as Code"
4. Paste the above code
5. The tool will auto-generate a beautiful architecture diagram
6. Customize colors/layout as needed
7. Export as PNG/SVG for your README

---

## Visual Data Flow Diagrams

### 1. URL Creation Flow
```
User (GUI)
  → API Gateway (JWT Auth)
    → URL Service (gRPC)
      → Snowflake ID Generator
      → PostgreSQL PRIMARY (INSERT)
      → Redis Cache (Warm)
      → Response: {short_code, short_url}
```

### 2. URL Redirect Flow (Critical Path)
```
User clicks short.ly/abc123
  → Redirect Service
    → L1 Cache (In-Memory LRU) [Hit? → Redirect 301]
      ↓ Miss
    → L2 Cache (Redis) [Hit? → Update L1 → Redirect 301]
      ↓ Miss
    → PostgreSQL REPLICA (SELECT) → Update L2+L1 → Redirect 301

  → Async: Publish Click Event to RabbitMQ (non-blocking)
```

### 3. Analytics Pipeline Flow
```
Click Event (RabbitMQ)
  → Analytics Worker (Batch of 50-100 events)
    → GeoIP Enrichment (MaxMind DB)
    → User-Agent Parsing
    → Batch INSERT to TimescaleDB
    → Optional: Batch INSERT to ClickHouse

  → Continuous Aggregates (Auto-refresh every hour)
    → daily_stats materialized view
```

### 4. Custom URL Creation Flow
```
User requests: short.ly/my-brand
  → API Gateway
    → Custom URL Service (gRPC)
      → Redis Distributed Lock ("lock:alias:my-brand")
      → Check PostgreSQL PRIMARY (ExistsInPrimary)
      → IF available:
          → INSERT to PostgreSQL PRIMARY
          → Cache in Redis
          → Release Lock
          → Return Success
      → ELSE:
          → Return "Alias taken, suggestions: my-brand-1, my-brand-2"
```

### 5. Read/Write Split Flow
```
WRITE Operations (URL Creation, User Registration):
  Service → PostgreSQL PRIMARY → WAL Streaming → Replicas

READ Operations (URL Lookup, Analytics):
  Service → Load Balancer → Round-Robin → PostgreSQL REPLICA (1, 2, or 3)

Critical Read-After-Write:
  Service → PostgreSQL PRIMARY (to avoid replication lag)
```

