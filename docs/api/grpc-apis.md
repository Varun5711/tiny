# gRPC APIs

This document describes all gRPC service definitions and their methods used for internal service-to-service communication in the Tiny URL Shortener system.

## Overview

The system uses **Protocol Buffers (proto3)** for defining service contracts and **gRPC** for efficient inter-service communication. The API Gateway translates HTTP/REST requests to gRPC calls to backend services.

**Proto files location:** `/api/proto/`

##Service Communication Flow

```
Client (HTTP/REST)
    ↓
API Gateway (Port 8080)
    ↓ gRPC
    ├── URL Service (Port 50051)
    └── User Service (Port 50052)
```

---

## URL Service

**Port:** 50051
**Proto file:** `api/proto/url/url.proto`
**Package:** `url`
**Go package:** `github.com/Varun5711/shorternit/proto/url`

### Service Definition

```protobuf
service URLService {
  rpc CreateURL(CreateURLRequest) returns (CreateURLResponse);
  rpc GetURL(GetURLRequest) returns (GetURLResponse);
  rpc ListURLs(ListURLsRequest) returns (ListURLsResponse);
  rpc DeleteURL(DeleteURLRequest) returns (DeleteURLResponse);
  rpc IncrementClicks(IncrementClicksRequest) returns (IncrementClicksResponse);
  rpc CreateCustomURL(CreateCustomURLRequest) returns (CreateCustomURLResponse);
}
```

### Methods

#### CreateURL

Creates a shortened URL with an auto-generated short code.

**Request:** `CreateURLRequest`
```protobuf
message CreateURLRequest {
  string long_url = 1;      // Required: The original long URL
  string user_id = 2;       // Required: User identifier
  int64 expires_at = 4;     // Optional: Unix timestamp for expiration
}
```

**Response:** `CreateURLResponse`
```protobuf
message CreateURLResponse {
  string short_code = 1;    // Generated short code (Base62)
  string short_url = 2;     // Complete short URL
  string long_url = 3;      // Original long URL
  int64 created_at = 4;     // Unix timestamp
  int64 expires_at = 5;     // Unix timestamp (0 if never expires)
  string qr_code = 6;       // Base64-encoded QR code image
}
```

**Example:**
```json
// Request
{
  "long_url": "https://example.com/very/long/path/to/resource",
  "user_id": "1234567890",
  "expires_at": 1735689600
}

// Response
{
  "short_code": "abc123",
  "short_url": "http://localhost:8081/abc123",
  "long_url": "https://example.com/very/long/path/to/resource",
  "created_at": 1704153600,
  "expires_at": 1735689600,
  "qr_code": "iVBORw0KGgoAAAANSUhEUgAA..."
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing or invalid long_url
- `Internal` (code 13): Database or internal service error

---

#### GetURL

Retrieves a URL by its short code.

**Request:** `GetURLRequest`
```protobuf
message GetURLRequest {
  string short_code = 1;    // Required: The short code to lookup
}
```

**Response:** `GetURLResponse`
```protobuf
message GetURLResponse {
  URL url = 1;              // URL details (see URL message below)
  bool found = 2;           // Whether the URL was found
}

message URL {
  string short_code = 1;
  string long_url = 2;
  int64 clicks = 3;
  int64 created_at = 4;
  int64 updated_at = 5;
  bool is_active = 6;
  int64 expires_at = 7;
  string short_url = 8;
}
```

**Example:**
```json
// Request
{
  "short_code": "abc123"
}

// Response
{
  "url": {
    "short_code": "abc123",
    "long_url": "https://example.com/path",
    "clicks": 42,
    "created_at": 1704153600,
    "updated_at": 1704153600,
    "is_active": true,
    "expires_at": 1735689600,
    "short_url": "http://localhost:8081/abc123"
  },
  "found": true
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing short_code
- `NotFound` (code 5): URL not found (also returned via found=false)
- `Internal` (code 13): Database error

---

#### ListURLs

Lists all URLs created by a specific user with pagination.

**Request:** `ListURLsRequest`
```protobuf
message ListURLsRequest {
  int32 limit = 1;          // Maximum number of URLs to return
  int32 offset = 2;         // Number of URLs to skip
  string user_id = 3;       // Required: User identifier
}
```

**Response:** `ListURLsResponse`
```protobuf
message ListURLsResponse {
  repeated URL urls = 1;    // Array of URL objects
  int32 total = 2;          // Total count of user's URLs
  bool has_more = 3;        // Whether more results exist
}
```

**Example:**
```json
// Request
{
  "limit": 10,
  "offset": 0,
  "user_id": "1234567890"
}

// Response
{
  "urls": [
    {
      "short_code": "abc123",
      "long_url": "https://example.com/path1",
      "clicks": 42,
      "created_at": 1704153600,
      "updated_at": 1704153600,
      "is_active": true,
      "expires_at": 0,
      "short_url": "http://localhost:8081/abc123"
    },
    {
      "short_code": "def456",
      "long_url": "https://example.com/path2",
      "clicks": 15,
      "created_at": 1704240000,
      "updated_at": 1704240000,
      "is_active": true,
      "expires_at": 1735689600,
      "short_url": "http://localhost:8081/def456"
    }
  ],
  "total": 15,
  "has_more": true
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing user_id or invalid limit/offset
- `Internal` (code 13): Database error

---

#### DeleteURL

Deletes a shortened URL by its short code.

**Request:** `DeleteURLRequest`
```protobuf
message DeleteURLRequest {
  string short_code = 1;    // Required: The short code to delete
}
```

**Response:** `DeleteURLResponse`
```protobuf
message DeleteURLResponse {
  bool success = 1;         // Whether deletion was successful
}
```

**Example:**
```json
// Request
{
  "short_code": "abc123"
}

// Response
{
  "success": true
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing short_code
- `NotFound` (code 5): URL not found
- `Internal` (code 13): Database error

**Note:** This method is defined in the proto but not currently exposed via the HTTP API.

---

#### IncrementClicks

Increments the click count for a URL (used by Analytics Worker).

**Request:** `IncrementClicksRequest`
```protobuf
message IncrementClicksRequest {
  string short_code = 1;    // Required: The short code to increment
}
```

**Response:** `IncrementClicksResponse`
```protobuf
message IncrementClicksResponse {
  int64 clicks = 1;         // Updated click count
}
```

**Example:**
```json
// Request
{
  "short_code": "abc123"
}

// Response
{
  "clicks": 43
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing short_code
- `NotFound` (code 5): URL not found
- `Internal` (code 13): Database error

**Note:** This method is used internally by the analytics-worker service, not exposed via HTTP API.

---

#### CreateCustomURL

Creates a shortened URL with a user-specified alias/short code.

**Request:** `CreateCustomURLRequest`
```protobuf
message CreateCustomURLRequest {
  string alias = 1;         // Required: Custom alias (3-50 chars, alphanumeric + _ -)
  string long_url = 2;      // Required: The original long URL
  int64 expires_at = 3;     // Optional: Unix timestamp for expiration
  string user_id = 4;       // Required: User identifier
}
```

**Response:** `CreateCustomURLResponse`
```protobuf
message CreateCustomURLResponse {
  string short_code = 1;    // The custom alias (same as request)
  string short_url = 2;     // Complete short URL
  string long_url = 3;      // Original long URL
  int64 created_at = 4;     // Unix timestamp
  int64 expires_at = 5;     // Unix timestamp (0 if never expires)
  string qr_code = 6;       // Base64-encoded QR code image
}
```

**Example:**
```json
// Request
{
  "alias": "my-custom-link",
  "long_url": "https://example.com/path",
  "expires_at": 1735689600,
  "user_id": "1234567890"
}

// Response
{
  "short_code": "my-custom-link",
  "short_url": "http://localhost:8081/my-custom-link",
  "long_url": "https://example.com/path",
  "created_at": 1704153600,
  "expires_at": 1735689600,
  "qr_code": "iVBORw0KGgoAAAANSUhEUgAA..."
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing fields or invalid alias format
- `AlreadyExists` (code 6): Alias already taken
- `Internal` (code 13): Database error

---

## User Service

**Port:** 50052
**Proto file:** `api/proto/user/user.proto`
**Package:** `user`
**Go package:** `github.com/Varun5711/shorternit/proto/user`

### Service Definition

```protobuf
service UserService {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc GetProfile(GetProfileRequest) returns (GetProfileResponse);
  rpc UpdateProfile(UpdateProfileRequest) returns (UpdateProfileResponse);
  rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
}
```

### Methods

#### Register

Registers a new user account and generates a JWT token.

**Request:** `RegisterRequest`
```protobuf
message RegisterRequest {
  string email = 1;         // Required: User email
  string password = 2;      // Required: Password (min 6 chars)
  string name = 3;          // Required: Full name
}
```

**Response:** `RegisterResponse`
```protobuf
message RegisterResponse {
  string user_id = 1;       // Generated user ID
  string email = 2;         // User email
  string name = 3;          // Full name
  string token = 4;         // JWT token (7-day expiry)
  int64 created_at = 5;     // Unix timestamp
}
```

**Example:**
```json
// Request
{
  "email": "user@example.com",
  "password": "securePassword123",
  "name": "John Doe"
}

// Response
{
  "user_id": "1234567890",
  "email": "user@example.com",
  "name": "John Doe",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "created_at": 1704153600
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing required fields or invalid email
- `AlreadyExists` (code 6): Email already registered
- `Internal` (code 13): Database or token generation error

---

#### Login

Authenticates a user and generates a JWT token.

**Request:** `LoginRequest`
```protobuf
message LoginRequest {
  string email = 1;         // Required: User email
  string password = 2;      // Required: Password
}
```

**Response:** `LoginResponse`
```protobuf
message LoginResponse {
  string user_id = 1;       // User ID
  string email = 2;         // User email
  string name = 3;          // Full name
  string token = 4;         // JWT token (7-day expiry)
  int64 expires_at = 5;     // Token expiration Unix timestamp
}
```

**Example:**
```json
// Request
{
  "email": "user@example.com",
  "password": "securePassword123"
}

// Response
{
  "user_id": "1234567890",
  "email": "user@example.com",
  "name": "John Doe",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": 1704758400
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing email or password
- `Unauthenticated` (code 16): Invalid credentials
- `Internal` (code 13): Database or token generation error

---

#### GetProfile

Retrieves user profile information using JWT token.

**Request:** `GetProfileRequest`
```protobuf
message GetProfileRequest {
  string token = 1;         // Required: JWT token
}
```

**Response:** `GetProfileResponse`
```protobuf
message GetProfileResponse {
  User user = 1;            // User details
}

message User {
  string id = 1;
  string email = 2;
  string name = 3;
  int64 created_at = 4;
  int64 updated_at = 5;
}
```

**Example:**
```json
// Request
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}

// Response
{
  "user": {
    "id": "1234567890",
    "email": "user@example.com",
    "name": "John Doe",
    "created_at": 1704153600,
    "updated_at": 1704153600
  }
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing token
- `Unauthenticated` (code 16): Invalid or expired token
- `Internal` (code 13): Database error

---

#### UpdateProfile

Updates user profile information.

**Request:** `UpdateProfileRequest`
```protobuf
message UpdateProfileRequest {
  string token = 1;         // Required: JWT token
  string name = 2;          // Optional: New name
  string email = 3;         // Optional: New email
}
```

**Response:** `UpdateProfileResponse`
```protobuf
message UpdateProfileResponse {
  User user = 1;            // Updated user details
}
```

**Example:**
```json
// Request
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "name": "Jane Doe"
}

// Response
{
  "user": {
    "id": "1234567890",
    "email": "user@example.com",
    "name": "Jane Doe",
    "created_at": 1704153600,
    "updated_at": 1704240000
  }
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing token
- `Unauthenticated` (code 16): Invalid or expired token
- `AlreadyExists` (code 6): Email already taken by another user
- `Internal` (code 13): Database error

**Note:** This method is defined but not currently exposed via the HTTP API.

---

#### ValidateToken

Validates a JWT token and returns user information (used for authentication middleware).

**Request:** `ValidateTokenRequest`
```protobuf
message ValidateTokenRequest {
  string token = 1;         // Required: JWT token to validate
}
```

**Response:** `ValidateTokenResponse`
```protobuf
message ValidateTokenResponse {
  bool valid = 1;           // Whether token is valid
  string user_id = 2;       // User ID (if valid)
  int64 expires_at = 3;     // Token expiration timestamp
}
```

**Example:**
```json
// Request
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}

// Response
{
  "valid": true,
  "user_id": "1234567890",
  "expires_at": 1704758400
}
```

**Errors:**
- `InvalidArgument` (code 3): Missing token
- `Internal` (code 13): Token parsing error

**Note:** This method is used internally by the API Gateway for authenticating requests.

---

## Analytics Service

**Port:** Not directly exposed (used via Redis Streams and ClickHouse queries)
**Proto file:** `api/proto/analytics/analytics.proto`
**Package:** `analytics`
**Go package:** `github.com/Varun5711/shorternit/proto/analytics`

### Service Definition

```protobuf
service AnalyticsService {
  rpc TrackClick(TrackClickRequest) returns (TrackClickResponse);
  rpc GetStats(GetStatsRequest) returns (GetStatsResponse);
  rpc GetTimeSeries(GetTimeSeriesRequest) returns (GetTimeSeriesResponse);
  rpc GetGeoStats(GetGeoStatsRequest) returns (GetGeoStatsResponse);
}
```

### Methods

#### TrackClick

Records a click event (called by Redirect Service).

**Request:** `TrackClickRequest`
```protobuf
message TrackClickRequest {
  string short_code = 1;    // Short code that was clicked
  string ip_address = 2;    // Client IP address
  string user_agent = 3;    // User agent string
  string referer = 4;       // HTTP referer
  int64 timestamp = 5;      // Unix timestamp
}
```

**Response:** `TrackClickResponse`
```protobuf
message TrackClickResponse {
  bool success = 1;         // Whether tracking succeeded
  string click_id = 2;      // Generated click event ID
}
```

**Example:**
```json
// Request
{
  "short_code": "abc123",
  "ip_address": "192.168.1.1",
  "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ...",
  "referer": "https://google.com",
  "timestamp": 1704153600
}

// Response
{
  "success": true,
  "click_id": "evt_1234567890"
}
```

**Note:** In the current implementation, click tracking is done via Redis Streams, not direct gRPC calls.

---

#### GetStats

Retrieves statistics for a short code.

**Request:** `GetStatsRequest`
```protobuf
message GetStatsRequest {
  string short_code = 1;    // Required: Short code to get stats for
  int64 start_time = 2;     // Optional: Start timestamp (Unix)
  int64 end_time = 3;       // Optional: End timestamp (Unix)
}
```

**Response:** `GetStatsResponse`
```protobuf
message GetStatsResponse {
  string short_code = 1;
  int64 total_clicks = 2;
  int64 unique_visitors = 3;
  repeated CountryStat countries = 4;
  repeated RefererStat referrers = 5;
  DeviceStats device_stats = 6;
}

message CountryStat {
  string country_code = 1;
  string country_name = 2;
  int64 click_count = 3;
  float percentage = 4;
}

message RefererStat {
  string referer = 1;
  int64 click_count = 2;
  float percentage = 3;
}

message DeviceStats {
  int64 mobile = 1;
  int64 desktop = 2;
  int64 tablet = 3;
  int64 other = 4;
}
```

---

#### GetTimeSeries

Retrieves click data over time with configurable granularity.

**Request:** `GetTimeSeriesRequest`
```protobuf
message GetTimeSeriesRequest {
  string short_code = 1;    // Required: Short code
  int64 start_time = 2;     // Start timestamp
  int64 end_time = 3;       // End timestamp
  string granularity = 4;   // "hour", "day", or "week"
}
```

**Response:** `GetTimeSeriesResponse`
```protobuf
message GetTimeSeriesResponse {
  repeated TimeSeriesPoint points = 1;
}

message TimeSeriesPoint {
  int64 timestamp = 1;      // Unix timestamp
  int64 clicks = 2;         // Total clicks in this period
  int64 unique_visitors = 3; // Unique IPs in this period
}
```

---

#### GetGeoStats

Retrieves geographic distribution of clicks.

**Request:** `GetGeoStatsRequest`
```protobuf
message GetGeoStatsRequest {
  string short_code = 1;    // Required: Short code
  int32 limit = 2;          // Maximum countries/cities to return
}
```

**Response:** `GetGeoStatsResponse`
```protobuf
message GetGeoStatsResponse {
  repeated CountryStat countries = 1;
  repeated CityStat cities = 2;
}

message CityStat {
  string city = 1;
  string country_code = 2;
  int64 click_count = 3;
  float percentage = 4;
}
```

---

## Error Handling

All gRPC services use standard gRPC status codes:

| Code | Name | Description |
|------|------|-------------|
| 0 | OK | Success |
| 3 | INVALID_ARGUMENT | Invalid request parameters |
| 5 | NOT_FOUND | Resource not found |
| 6 | ALREADY_EXISTS | Resource already exists (duplicate) |
| 13 | INTERNAL | Internal server error |
| 16 | UNAUTHENTICATED | Authentication failed |

Error responses include:
- Status code
- Error message
- Optional error details

**Example error:**
```json
{
  "code": 3,
  "message": "long_url is required",
  "details": []
}
```

---

## Authentication

### JWT Token Structure

Tokens include:
- **User ID**: Unique user identifier
- **Email**: User email address
- **Expiration**: 7 days from issuance
- **Signature**: HMAC-SHA256 with secret key

### Token Usage

1. **HTTP API:** Include in `Authorization` header as `Bearer <token>`
2. **gRPC:** Token passed in request messages (e.g., GetProfileRequest.token)

---

## Performance Considerations

### Caching

- URL lookups use multi-tier cache (L1 in-memory + L2 Redis)
- Cache key pattern: `url:{short_code}`
- Cache TTL: Configurable (default: 15 minutes)

### Timeouts

- Default gRPC timeout: 5 seconds
- Heavy query timeout: 10 seconds
- Use context with timeout for all gRPC calls

### Connection Pooling

- gRPC connections are persistent (HTTP/2)
- Connection pool size: Configurable per service
- Keep-alive: Enabled with 30-second ping interval

---

## Code Generation

Regenerate Go code from proto files:

```bash
make proto
```

This runs:
```bash
protoc --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  api/proto/**/*.proto
```

Generated files:
- `{service}.pb.go` - Message definitions
- `{service}_grpc.pb.go` - Service stubs and server interfaces

---

## See Also

- [gRPC Code Examples](./grpc-examples.md) - Complete client and server examples
- [OpenAPI Specification](../api/openapi/api-gateway.yaml) - REST API documentation
- [System Architecture](../architecture/system-design.md) - Service communication patterns
