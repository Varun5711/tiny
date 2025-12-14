# ADR-004: gRPC for Internal Service Communication

## Status
Accepted

## Context

The system is split into multiple services:
- API Gateway (HTTP) → URL Service
- API Gateway (HTTP) → User Service
- Redirect Service (HTTP) → URL Service

We need an efficient protocol for internal service-to-service communication.

Options considered:
1. **REST/HTTP JSON** - Standard HTTP APIs
2. **gRPC** - Google's RPC framework with Protocol Buffers
3. **GraphQL** - Query language for APIs
4. **Message Queue** - Async communication via broker

## Decision

Use gRPC with Protocol Buffers for synchronous internal communication.

**Service definitions:**
```protobuf
// proto/url/url.proto
service URLService {
  rpc CreateURL(CreateURLRequest) returns (CreateURLResponse);
  rpc GetURL(GetURLRequest) returns (GetURLResponse);
  rpc ListURLs(ListURLsRequest) returns (ListURLsResponse);
  rpc DeleteURL(DeleteURLRequest) returns (DeleteURLResponse);
  rpc IncrementClicks(IncrementClicksRequest) returns (IncrementClicksResponse);
  rpc CreateCustomURL(CreateCustomURLRequest) returns (CreateCustomURLResponse);
}

// proto/user/user.proto
service UserService {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
  rpc GetProfile(GetProfileRequest) returns (GetProfileResponse);
}
```

**Communication pattern:**
```
Client → HTTP → API Gateway → gRPC → URL Service
                            → gRPC → User Service

Client → HTTP → Redirect Service → gRPC → URL Service
```

## Consequences

### Positive
- **Performance** - Binary protocol, 2-10x faster than JSON
- **Type safety** - Compile-time validation via protobuf
- **Code generation** - Client/server stubs auto-generated
- **Streaming** - Supports bidirectional streaming (future use)
- **Language agnostic** - Can add services in other languages
- **Built-in deadlines** - Request timeout handling

### Negative
- **Debugging** - Binary format harder to inspect than JSON
- **Browser support** - Can't call gRPC directly from browser
- **Learning curve** - Team needs to learn protobuf syntax
- **Tooling** - Need protoc compiler in build pipeline

### Performance comparison
```
Operation: GetURL (cache miss)

REST/JSON:
  Serialize:   ~50μs
  Network:     ~1ms
  Deserialize: ~50μs
  Total:       ~1.1ms

gRPC/Protobuf:
  Serialize:   ~5μs
  Network:     ~1ms
  Deserialize: ~5μs
  Total:       ~1.01ms

Savings: ~100μs per request (10% improvement)
At 10K req/sec: 1 second of CPU time saved per second
```

### Why not gRPC for external API?
- Browsers can't speak gRPC natively
- REST is more familiar to API consumers
- Better tooling (curl, Postman, Swagger)
- gRPC-Web exists but adds complexity

## References
- [gRPC](https://grpc.io/)
- [Protocol Buffers](https://developers.google.com/protocol-buffers)
- [gRPC vs REST Performance](https://blog.dreamfactory.com/grpc-vs-rest-how-does-grpc-compare-with-traditional-rest-apis/)
