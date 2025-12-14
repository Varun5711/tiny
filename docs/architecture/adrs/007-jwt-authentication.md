# ADR-007: JWT for Authentication

## Status
Accepted

## Context

Users need to authenticate to:
- Create shortened URLs
- View their URL list
- Access analytics for their URLs

Authentication options considered:
1. **Session-based** - Server-side sessions with cookies
2. **JWT (JSON Web Tokens)** - Stateless tokens
3. **OAuth 2.0** - Delegated authentication
4. **API Keys** - Long-lived keys per user

## Decision

Use JWT (JSON Web Tokens) with HS256 signing for authentication.

**Token structure:**
```
Header: { "alg": "HS256", "typ": "JWT" }
Payload: {
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "exp": 1704153600,
  "iat": 1704067200
}
Signature: HMAC-SHA256(header.payload, secret)
```

**Flow:**
```
1. User registers/logs in
   POST /api/auth/login { email, password }

2. Server validates credentials, returns JWT
   { token: "eyJhbG...", expires_at: 1704153600 }

3. Client includes token in subsequent requests
   Authorization: Bearer eyJhbG...

4. Server validates token signature and expiry
   → Extract user_id from claims
   → Process request
```

## Consequences

### Positive
- **Stateless** - No server-side session storage
- **Scalable** - Any instance can validate tokens
- **Self-contained** - User info embedded in token
- **Cross-service** - Same token works across all services
- **Mobile friendly** - Easy to store and transmit

### Negative
- **Can't revoke** - Token valid until expiry (without blocklist)
- **Token size** - Larger than session ID (~200 bytes vs ~32 bytes)
- **Secret management** - Must secure signing key
- **No refresh by default** - Need separate refresh token flow

### Security configuration
```go
// Token settings
SigningMethod: HS256
SecretKey:     env.JWT_SECRET (32+ bytes, random)
TokenDuration: 24 hours
```

### Token validation
```go
func ValidateToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{},
        func(token *jwt.Token) (interface{}, error) {
            // Verify signing method
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method")
            }
            return []byte(secretKey), nil
        })

    if err != nil {
        return nil, err
    }

    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token")
    }

    return claims, nil
}
```

### Why HS256 over RS256?
```
HS256 (Symmetric):
  + Faster signing and verification
  + Simpler key management (single secret)
  + Sufficient for single-organization use
  - Secret must be shared if external validation needed

RS256 (Asymmetric):
  + Public key can be shared for verification
  + Better for third-party token validation
  - Slower operations
  - More complex key management

Decision: HS256 - all services are internal, performance matters
```

### Token refresh strategy (future)
```
Current: Single token, 24h expiry
  - User must re-login daily
  - Simple implementation

Future option: Access + Refresh tokens
  - Access token: 15 minutes
  - Refresh token: 7 days
  - Refresh endpoint to get new access token
  - Can revoke refresh tokens in database
```

### Password security
```go
// Hashing (on registration/password change)
hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
// Cost = 10, ~100ms to hash

// Verification (on login)
err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
```

## References
- [JWT Introduction](https://jwt.io/introduction)
- [JWT Best Practices](https://auth0.com/blog/a-look-at-the-latest-draft-for-jwt-bcp/)
- [bcrypt](https://en.wikipedia.org/wiki/Bcrypt)
