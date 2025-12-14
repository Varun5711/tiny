# gRPC Code Examples

This document provides practical code examples for using the gRPC services in the Tiny URL Shortener system.

## Table of Contents

1. [Client Setup](#client-setup)
2. [URL Service Examples](#url-service-examples)
3. [User Service Examples](#user-service-examples)
4. [Analytics Service Examples](#analytics-service-examples)
5. [Error Handling](#error-handling)
6. [Best Practices](#best-practices)

---

## Client Setup

### Creating a gRPC Connection

```go
package main

import (
    "context"
    "log"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    urlpb "github.com/Varun5711/shorternit/proto/url"
    userpb "github.com/Varun5711/shorternit/proto/user"
)

func main() {
    urlConn, err := grpc.Dial(
        "localhost:50051",
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock(),
        grpc.WithTimeout(5*time.Second),
    )
    if err != nil {
        log.Fatalf("Failed to connect to URL service: %v", err)
    }
    defer urlConn.Close()

    userConn, err := grpc.Dial(
        "localhost:50052",
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock(),
        grpc.WithTimeout(5*time.Second),
    )
    if err != nil {
        log.Fatalf("Failed to connect to User service: %v", err)
    }
    defer userConn.Close()

    urlClient := urlpb.NewURLServiceClient(urlConn)
    userClient := userpb.NewUserServiceClient(userConn)
}
```

### Connection Pooling (Production)

```go
package grpcclient

import (
    "context"
    "sync"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/keepalive"
)

type ClientPool struct {
    conn *grpc.ClientConn
    mu   sync.RWMutex
}

func NewClientPool(target string) (*ClientPool, error) {
    conn, err := grpc.Dial(
        target,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time:                30 * time.Second,
            Timeout:             10 * time.Second,
            PermitWithoutStream: true,
        }),
        grpc.WithDefaultCallOptions(
            grpc.MaxCallRecvMsgSize(10 * 1024 * 1024),
            grpc.MaxCallSendMsgSize(10 * 1024 * 1024),
        ),
    )
    if err != nil {
        return nil, err
    }

    return &ClientPool{conn: conn}, nil
}

func (p *ClientPool) GetConn() *grpc.ClientConn {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.conn
}

func (p *ClientPool) Close() error {
    p.mu.Lock()
    defer p.mu.Unlock()
    return p.conn.Close()
}
```

---

## URL Service Examples

### Example 1: Create a Short URL

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    urlpb "github.com/Varun5711/shorternit/proto/url"
)

func CreateShortURL(client urlpb.URLServiceClient, longURL, userID string) (*urlpb.CreateURLResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := &urlpb.CreateURLRequest{
        LongUrl:   longURL,
        UserId:    userID,
        ExpiresAt: 0,
    }

    resp, err := client.CreateURL(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to create URL: %w", err)
    }

    return resp, nil
}

func main() {
    resp, err := CreateShortURL(urlClient, "https://example.com/very/long/path", "user123")
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("Short URL created: %s\n", resp.ShortUrl)
    fmt.Printf("Short code: %s\n", resp.ShortCode)
    fmt.Printf("Created at: %s\n", time.Unix(resp.CreatedAt, 0))
}
```

### Example 2: Create Custom Alias URL

```go
func CreateCustomURL(client urlpb.URLServiceClient, alias, longURL, userID string, expiresAt int64) (*urlpb.CreateCustomURLResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := &urlpb.CreateCustomURLRequest{
        Alias:     alias,
        LongUrl:   longURL,
        UserId:    userID,
        ExpiresAt: expiresAt,
    }

    resp, err := client.CreateCustomURL(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to create custom URL: %w", err)
    }

    return resp, nil
}

func main() {
    expiresAt := time.Now().AddDate(0, 6, 0).Unix()

    resp, err := CreateCustomURL(
        urlClient,
        "my-awesome-link",
        "https://example.com/awesome-page",
        "user123",
        expiresAt,
    )
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("Custom URL created: %s\n", resp.ShortUrl)
    fmt.Printf("Expires at: %s\n", time.Unix(resp.ExpiresAt, 0))
}
```

### Example 3: Get URL Details

```go
func GetURL(client urlpb.URLServiceClient, shortCode string) (*urlpb.URL, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := &urlpb.GetURLRequest{
        ShortCode: shortCode,
    }

    resp, err := client.GetURL(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to get URL: %w", err)
    }

    if !resp.Found {
        return nil, fmt.Errorf("URL not found")
    }

    return resp.Url, nil
}

func main() {
    url, err := GetURL(urlClient, "abc123")
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("Short code: %s\n", url.ShortCode)
    fmt.Printf("Long URL: %s\n", url.LongUrl)
    fmt.Printf("Clicks: %d\n", url.Clicks)
    fmt.Printf("Is active: %v\n", url.IsActive)
}
```

### Example 4: List User URLs with Pagination

```go
func ListUserURLs(client urlpb.URLServiceClient, userID string, page, pageSize int32) (*urlpb.ListURLsResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    offset := (page - 1) * pageSize

    req := &urlpb.ListURLsRequest{
        UserId: userID,
        Limit:  pageSize,
        Offset: offset,
    }

    resp, err := client.ListURLs(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to list URLs: %w", err)
    }

    return resp, nil
}

func main() {
    page := int32(1)
    pageSize := int32(10)

    resp, err := ListUserURLs(urlClient, "user123", page, pageSize)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("Total URLs: %d\n", resp.Total)
    fmt.Printf("Has more: %v\n", resp.HasMore)
    fmt.Println("\nURLs:")
    for i, url := range resp.Urls {
        fmt.Printf("%d. %s -> %s (clicks: %d)\n",
            i+1, url.ShortUrl, url.LongUrl, url.Clicks)
    }
}
```

### Example 5: Delete URL

```go
func DeleteURL(client urlpb.URLServiceClient, shortCode string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := &urlpb.DeleteURLRequest{
        ShortCode: shortCode,
    }

    resp, err := client.DeleteURL(ctx, req)
    if err != nil {
        return fmt.Errorf("failed to delete URL: %w", err)
    }

    if !resp.Success {
        return fmt.Errorf("deletion was not successful")
    }

    return nil
}

func main() {
    err := DeleteURL(urlClient, "abc123")
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Println("URL deleted successfully")
}
```

---

## User Service Examples

### Example 6: User Registration

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    userpb "github.com/Varun5711/shorternit/proto/user"
)

func RegisterUser(client userpb.UserServiceClient, email, password, name string) (*userpb.RegisterResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := &userpb.RegisterRequest{
        Email:    email,
        Password: password,
        Name:     name,
    }

    resp, err := client.Register(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to register: %w", err)
    }

    return resp, nil
}

func main() {
    resp, err := RegisterUser(
        userClient,
        "user@example.com",
        "securePassword123",
        "John Doe",
    )
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("User registered successfully!\n")
    fmt.Printf("User ID: %s\n", resp.UserId)
    fmt.Printf("Email: %s\n", resp.Email)
    fmt.Printf("Name: %s\n", resp.Name)
    fmt.Printf("Token: %s\n", resp.Token)
    fmt.Printf("Created at: %s\n", time.Unix(resp.CreatedAt, 0))
}
```

### Example 7: User Login

```go
func LoginUser(client userpb.UserServiceClient, email, password string) (*userpb.LoginResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := &userpb.LoginRequest{
        Email:    email,
        Password: password,
    }

    resp, err := client.Login(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to login: %w", err)
    }

    return resp, nil
}

func main() {
    resp, err := LoginUser(userClient, "user@example.com", "securePassword123")
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("Login successful!\n")
    fmt.Printf("User ID: %s\n", resp.UserId)
    fmt.Printf("Token: %s\n", resp.Token)
    fmt.Printf("Token expires at: %s\n", time.Unix(resp.ExpiresAt, 0))
}
```

### Example 8: Get User Profile

```go
func GetUserProfile(client userpb.UserServiceClient, token string) (*userpb.User, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := &userpb.GetProfileRequest{
        Token: token,
    }

    resp, err := client.GetProfile(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to get profile: %w", err)
    }

    return resp.User, nil
}

func main() {
    token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

    user, err := GetUserProfile(userClient, token)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("User Profile:\n")
    fmt.Printf("ID: %s\n", user.Id)
    fmt.Printf("Email: %s\n", user.Email)
    fmt.Printf("Name: %s\n", user.Name)
    fmt.Printf("Created: %s\n", time.Unix(user.CreatedAt, 0))
}
```

### Example 9: Validate Token

```go
func ValidateToken(client userpb.UserServiceClient, token string) (bool, string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    req := &userpb.ValidateTokenRequest{
        Token: token,
    }

    resp, err := client.ValidateToken(ctx, req)
    if err != nil {
        return false, "", fmt.Errorf("failed to validate token: %w", err)
    }

    return resp.Valid, resp.UserId, nil
}

func main() {
    token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

    valid, userID, err := ValidateToken(userClient, token)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    if valid {
        fmt.Printf("Token is valid for user: %s\n", userID)
    } else {
        fmt.Println("Token is invalid")
    }
}
```

---

## Analytics Service Examples

### Example 10: Get URL Statistics

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    analyticspb "github.com/Varun5711/shorternit/proto/analytics"
)

func GetURLStats(client analyticspb.AnalyticsServiceClient, shortCode string) (*analyticspb.GetStatsResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    req := &analyticspb.GetStatsRequest{
        ShortCode: shortCode,
        StartTime: 0,
        EndTime:   time.Now().Unix(),
    }

    resp, err := client.GetStats(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to get stats: %w", err)
    }

    return resp, nil
}

func main() {
    stats, err := GetURLStats(analyticsClient, "abc123")
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Printf("Statistics for %s:\n", stats.ShortCode)
    fmt.Printf("Total clicks: %d\n", stats.TotalClicks)
    fmt.Printf("Unique visitors: %d\n", stats.UniqueVisitors)

    fmt.Println("\nTop Countries:")
    for i, country := range stats.Countries {
        fmt.Printf("%d. %s: %d clicks (%.1f%%)\n",
            i+1, country.CountryName, country.ClickCount, country.Percentage)
    }

    fmt.Println("\nDevice Stats:")
    ds := stats.DeviceStats
    fmt.Printf("Desktop: %d\n", ds.Desktop)
    fmt.Printf("Mobile: %d\n", ds.Mobile)
    fmt.Printf("Tablet: %d\n", ds.Tablet)
}
```

### Example 11: Get Time Series Data

```go
func GetTimeSeries(client analyticspb.AnalyticsServiceClient, shortCode string, days int) (*analyticspb.GetTimeSeriesResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    endTime := time.Now()
    startTime := endTime.AddDate(0, 0, -days)

    req := &analyticspb.GetTimeSeriesRequest{
        ShortCode:   shortCode,
        StartTime:   startTime.Unix(),
        EndTime:     endTime.Unix(),
        Granularity: "day",
    }

    resp, err := client.GetTimeSeries(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to get time series: %w", err)
    }

    return resp, nil
}

func main() {
    resp, err := GetTimeSeries(analyticsClient, "abc123", 7)
    if err != nil {
        log.Fatalf("Error: %v", err)
    }

    fmt.Println("Click timeline (last 7 days):")
    for _, point := range resp.Points {
        date := time.Unix(point.Timestamp, 0).Format("2006-01-02")
        fmt.Printf("%s: %d clicks (%d unique)\n",
            date, point.Clicks, point.UniqueVisitors)
    }
}
```

---

## Error Handling

### Handling gRPC Errors

```go
package main

import (
    "context"
    "fmt"
    "log"

    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"

    urlpb "github.com/Varun5711/shorternit/proto/url"
)

func HandleGRPCError(err error) {
    st, ok := status.FromError(err)
    if !ok {
        log.Printf("Unknown error: %v", err)
        return
    }

    switch st.Code() {
    case codes.InvalidArgument:
        log.Printf("Invalid argument: %s", st.Message())
    case codes.NotFound:
        log.Printf("Resource not found: %s", st.Message())
    case codes.AlreadyExists:
        log.Printf("Resource already exists: %s", st.Message())
    case codes.Unauthenticated:
        log.Printf("Authentication failed: %s", st.Message())
    case codes.DeadlineExceeded:
        log.Printf("Request timeout: %s", st.Message())
    case codes.Internal:
        log.Printf("Internal server error: %s", st.Message())
    default:
        log.Printf("gRPC error [%s]: %s", st.Code(), st.Message())
    }
}

func SafeCreateURL(client urlpb.URLServiceClient, longURL, userID string) (*urlpb.CreateURLResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := &urlpb.CreateURLRequest{
        LongUrl: longURL,
        UserId:  userID,
    }

    resp, err := client.CreateURL(ctx, req)
    if err != nil {
        HandleGRPCError(err)
        return nil, err
    }

    return resp, nil
}
```

### Retry Logic with Exponential Backoff

```go
package grpcclient

import (
    "context"
    "fmt"
    "time"

    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

func RetryWithBackoff(
    ctx context.Context,
    maxRetries int,
    fn func(ctx context.Context) error,
) error {
    var err error
    backoff := 100 * time.Millisecond

    for attempt := 0; attempt <= maxRetries; attempt++ {
        err = fn(ctx)
        if err == nil {
            return nil
        }

        st, ok := status.FromError(err)
        if !ok {
            return err
        }

        switch st.Code() {
        case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists:
            return err
        case codes.DeadlineExceeded, codes.Unavailable, codes.Internal:
            if attempt == maxRetries {
                return fmt.Errorf("max retries exceeded: %w", err)
            }

            select {
            case <-time.After(backoff):
                backoff *= 2
                if backoff > 10*time.Second {
                    backoff = 10 * time.Second
                }
            case <-ctx.Done():
                return ctx.Err()
            }
        default:
            return err
        }
    }

    return err
}

func CreateURLWithRetry(client urlpb.URLServiceClient, longURL, userID string) (*urlpb.CreateURLResponse, error) {
    var resp *urlpb.CreateURLResponse

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    err := RetryWithBackoff(ctx, 3, func(ctx context.Context) error {
        var err error
        resp, err = client.CreateURL(ctx, &urlpb.CreateURLRequest{
            LongUrl: longURL,
            UserId:  userID,
        })
        return err
    })

    if err != nil {
        return nil, err
    }

    return resp, nil
}
```

---

## Best Practices

### 1. Always Use Context with Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

resp, err := client.CreateURL(ctx, req)
```

### 2. Use Connection Pooling

```go
var urlClientPool *ClientPool

func init() {
    pool, err := NewClientPool("localhost:50051")
    if err != nil {
        log.Fatal(err)
    }
    urlClientPool = pool
}

func CreateURL(longURL, userID string) error {
    client := urlpb.NewURLServiceClient(urlClientPool.GetConn())

}
```

### 3. Handle Errors Properly

```go
resp, err := client.CreateURL(ctx, req)
if err != nil {
    st, ok := status.FromError(err)
    if ok {
        switch st.Code() {
        case codes.InvalidArgument:
            return fmt.Errorf("validation error: %s", st.Message())
        case codes.AlreadyExists:
            return fmt.Errorf("duplicate: %s", st.Message())
        default:
            return fmt.Errorf("grpc error: %w", err)
        }
    }
    return err
}
```

### 4. Use Interceptors for Cross-Cutting Concerns

```go
func LoggingInterceptor() grpc.UnaryClientInterceptor {
    return func(
        ctx context.Context,
        method string,
        req, reply interface{},
        cc *grpc.ClientConn,
        invoker grpc.UnaryInvoker,
        opts ...grpc.CallOption,
    ) error {
        start := time.Now()
        err := invoker(ctx, method, req, reply, cc, opts...)
        duration := time.Since(start)

        log.Printf("gRPC call: %s, duration: %v, error: %v",
            method, duration, err)

        return err
    }
}

conn, err := grpc.Dial(
    "localhost:50051",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithUnaryInterceptor(LoggingInterceptor()),
)
```

### 5. Graceful Shutdown

```go
func main() {
    conn, err := grpc.Dial("localhost:50051", opts...)
    if err != nil {
        log.Fatal(err)
    }

    defer func() {
        if err := conn.Close(); err != nil {
            log.Printf("Error closing connection: %v", err)
        }
    }()

    client := urlpb.NewURLServiceClient(conn)
}
```

---

## Complete Example: URL Shortener Client

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    urlpb "github.com/Varun5711/shorternit/proto/url"
    userpb "github.com/Varun5711/shorternit/proto/user"
)

type Client struct {
    urlClient  urlpb.URLServiceClient
    userClient userpb.UserServiceClient
    token      string
    userID     string
}

func NewClient(urlAddr, userAddr string) (*Client, error) {
    urlConn, err := grpc.Dial(urlAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return nil, err
    }

    userConn, err := grpc.Dial(userAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return nil, err
    }

    return &Client{
        urlClient:  urlpb.NewURLServiceClient(urlConn),
        userClient: userpb.NewUserServiceClient(userConn),
    }, nil
}

func (c *Client) Register(email, password, name string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    resp, err := c.userClient.Register(ctx, &userpb.RegisterRequest{
        Email:    email,
        Password: password,
        Name:     name,
    })
    if err != nil {
        return err
    }

    c.token = resp.Token
    c.userID = resp.UserId
    fmt.Printf("Registered as %s (ID: %s)\n", resp.Email, resp.UserId)
    return nil
}

func (c *Client) Login(email, password string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    resp, err := c.userClient.Login(ctx, &userpb.LoginRequest{
        Email:    email,
        Password: password,
    })
    if err != nil {
        return err
    }

    c.token = resp.Token
    c.userID = resp.UserId
    fmt.Printf("Logged in as %s\n", resp.Email)
    return nil
}

func (c *Client) ShortenURL(longURL string) (string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    resp, err := c.urlClient.CreateURL(ctx, &urlpb.CreateURLRequest{
        LongUrl: longURL,
        UserId:  c.userID,
    })
    if err != nil {
        return "", err
    }

    return resp.ShortUrl, nil
}

func (c *Client) ListMyURLs() error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    resp, err := c.urlClient.ListURLs(ctx, &urlpb.ListURLsRequest{
        UserId: c.userID,
        Limit:  100,
        Offset: 0,
    })
    if err != nil {
        return err
    }

    fmt.Printf("\nYour URLs (%d total):\n", resp.Total)
    for i, url := range resp.Urls {
        fmt.Printf("%d. %s -> %s (clicks: %d)\n",
            i+1, url.ShortUrl, url.LongUrl, url.Clicks)
    }

    return nil
}

func main() {
    email := flag.String("email", "", "Email address")
    password := flag.String("password", "", "Password")
    name := flag.String("name", "", "Full name (for registration)")
    longURL := flag.String("url", "", "URL to shorten")
    flag.Parse()

    client, err := NewClient("localhost:50051", "localhost:50052")
    if err != nil {
        log.Fatal(err)
    }

    if *name != "" {
        if err := client.Register(*email, *password, *name); err != nil {
            log.Fatal(err)
        }
    } else {
        if err := client.Login(*email, *password); err != nil {
            log.Fatal(err)
        }
    }

    if *longURL != "" {
        shortURL, err := client.ShortenURL(*longURL)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Printf("\nShort URL: %s\n", shortURL)
    }

    if err := client.ListMyURLs(); err != nil {
        log.Fatal(err)
    }
}
```

**Usage:**
```bash
go run client.go -email="user@example.com" -password="test123" -name="John Doe"
go run client.go -email="user@example.com" -password="test123" -url="https://example.com/very/long/path"
```

---

## See Also

- [gRPC API Reference](./grpc-apis.md) - Complete API documentation
- [OpenAPI Specification](../api/openapi/api-gateway.yaml) - REST API docs
- [System Architecture](../architecture/system-design.md) - Service communication patterns
