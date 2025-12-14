# Authentication & JWT Tokens

> **Stateless authentication: Why JWT over sessions**

## Overview

Every modern web application needs authentication. Our URL shortener uses **JWT (JSON Web Tokens)** for a stateless, scalable authentication system.

But why JWT? Why not traditional sessions? What are the trade-offs?

In this document:
1. **Authentication Approaches** - Sessions vs Tokens vs OAuth
2. **JWT Structure** - Header, Payload, Signature explained
3. **Why JWT?** - Stateless benefits and scalability
4. **Security Considerations** - Token expiry, secret management
5. **Implementation** - Code walkthrough

---

## Part 1: Authentication Approaches

### Option 1: Session-Based (Traditional)

**Flow:**
1. User logs in with username/password
2. Server creates session, stores in database/Redis
3. Server returns session ID in cookie
4. Client sends cookie with each request
5. Server looks up session to verify identity

**Storage:**
```
Session Store (Redis):
session:abc123 → { user_id: "42", created_at: "2024-01-01", ... }
```

**Pros:**
- Simple to implement
- Easy revocation (delete session from store)
- Can store complex session data

**Cons:**
- **Stateful**: Requires session storage (database/Redis query on every request)
- **Scaling challenge**: Sessions must be shared across servers (sticky sessions or centralized store)
- **Performance**: Database/Redis lookup adds latency

### Option 2: Token-Based (JWT)

**Flow:**
1. User logs in with username/password
2. Server generates JWT containing user info
3. Server returns JWT to client
4. Client stores JWT (localStorage/cookie)
5. Client sends JWT with each request
6. Server **validates JWT signature** (no database lookup!)

**Token:**
```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiNDIiLCJleHAiOjE3MDUwMDAwMDB9.xyz...
```

**Pros:**
- **Stateless**: No server-side storage needed
- **Scalable**: Any server can validate tokens
- **Fast**: No database lookup (just verify signature)

**Cons:**
- **Hard to revoke**: Token valid until expiration (can't "delete" it)
- **Size**: Tokens are larger than session IDs (~200 bytes vs 20 bytes)
- **Security**: If secret key leaks, all tokens compromised

### Option 3: OAuth 2.0 (Third-Party)

**Flow:**
1. User clicks "Login with Google"
2. Redirect to Google
3. User authorizes app
4. Google returns access token
5. App uses token to fetch user info from Google

**Pros:**
- No password storage
- Leverages trusted providers (Google, GitHub, Facebook)
- Single sign-on (SSO)

**Cons:**
- Dependency on third-party
- Complex flow
- Limited offline access

### Our Choice: JWT

**Why JWT for a URL shortener?**

1. **Stateless scaling**: Can add more API Gateway instances without session coordination
2. **Simple**: No session store infrastructure (one less system)
3. **Fast**: No database lookup on every request
4. **Good enough revocation**: 7-day expiry limits damage if token stolen

**Trade-off accepted**: Can't instantly revoke tokens (acceptable for our use case).

---

## Part 2: JWT Structure

A JWT has three parts separated by dots (`.`):

```
HEADER.PAYLOAD.SIGNATURE
```

### Example JWT

```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiNDIiLCJleHAiOjE3MDUwMDAwMDB9.3f8kD9xH2lP...
```

### Part 1: Header

```json
{
  "alg": "HS256",
  "typ": "JWT"
}
```

Base64URL encoded → `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9`

**Fields:**
- `alg`: Algorithm (HS256 = HMAC-SHA256)
- `typ`: Type (always "JWT")

### Part 2: Payload (Claims)

```json
{
  "user_id": "42",
  "email": "user@example.com",
  "exp": 1735689600,
  "iat": 1735084800
}
```

Base64URL encoded → `eyJ1c2VyX2lkIjoiNDIi...`

**Standard claims:**
- `exp`: Expiration time (Unix timestamp)
- `iat`: Issued at (Unix timestamp)
- `sub`: Subject (user ID)
- `iss`: Issuer (who created token)

**Custom claims:**
- `user_id`: Our application-specific claim
- `email`: User's email

**Important**: Payload is **NOT encrypted**, only Base64 encoded. Anyone can decode and read it!

```bash
echo "eyJ1c2VyX2lkIjoiNDIi..." | base64 -d
# {"user_id":"42","email":"user@example.com",...}
```

**Never put secrets in JWT payload!**

### Part 3: Signature

```
HMACSHA256(
  base64UrlEncode(header) + "." + base64UrlEncode(payload),
  secret_key
)
```

**Purpose**: Prove token wasn't tampered with.

**How it works:**
1. Take header + payload
2. Sign with secret key using HMAC-SHA256
3. Append signature to token

**Verification:**
1. Split token into header, payload, signature
2. Recreate signature using same algorithm and secret
3. Compare signatures:
   - **Match**: Token valid, trust the payload
   - **Mismatch**: Token tampered or forged, reject

**Key insight**: Even though payload is readable, attackers can't modify it without breaking the signature.

---

## Part 3: JWT in Our System

### Token Generation (User Login)

From `internal/service/user_service.go` (simplified):

```go
func (s *UserService) Login(ctx context.Context, email, password string) (string, error) {
    // 1. Find user
    user, err := s.storage.GetByEmail(ctx, email)
    if err != nil {
        return "", fmt.Errorf("invalid credentials")
    }

    // 2. Verify password (bcrypt)
    err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
    if err != nil {
        return "", fmt.Errorf("invalid credentials")
    }

    // 3. Generate JWT
    claims := jwt.MapClaims{
        "user_id": user.ID,
        "email":   user.Email,
        "exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),  // 7 days
        "iat":     time.Now().Unix(),
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    tokenString, err := token.SignedString([]byte(s.jwtSecret))

    return tokenString, nil
}
```

**Steps:**
1. Query database for user by email
2. Verify password with bcrypt (hashed comparison)
3. Create JWT claims (user_id, email, expiry)
4. Sign token with secret key
5. Return token to client

**Client stores token** (localStorage or cookie).

### Token Validation (API Gateway)

From `internal/middleware/auth.go` (simplified):

```go
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Extract token from header
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.JSON(401, gin.H{"error": "missing authorization header"})
            c.Abort()
            return
        }

        tokenString := strings.TrimPrefix(authHeader, "Bearer ")

        // 2. Parse and validate token
        token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
            // Verify algorithm
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method")
            }
            return []byte(jwtSecret), nil
        })

        if err != nil || !token.Valid {
            c.JSON(401, gin.H{"error": "invalid token"})
            c.Abort()
            return
        }

        // 3. Extract claims
        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok {
            c.JSON(401, gin.H{"error": "invalid claims"})
            c.Abort()
            return
        }

        // 4. Check expiration
        exp := int64(claims["exp"].(float64))
        if time.Now().Unix() > exp {
            c.JSON(401, gin.H{"error": "token expired"})
            c.Abort()
            return
        }

        // 5. Store user info in context
        c.Set("user_id", claims["user_id"])
        c.Set("email", claims["email"])

        c.Next()  // Proceed to handler
    }
}
```

**Steps:**
1. Extract token from `Authorization: Bearer <token>` header
2. Parse JWT and validate signature
3. Extract claims (user_id, email)
4. Check expiration
5. Store user info in request context for handlers

**No database query!** Everything needed is in the token.

---

## Part 4: Security Considerations

### 1. Secret Key Management

**Our secret:**
```go
jwtSecret := os.Getenv("JWT_SECRET")  // From environment variable
```

**Security rules:**
- **Never commit secret to Git**: Use environment variables
- **Rotate regularly**: Change secret every 3-6 months
- **Long and random**: Min 32 characters, cryptographically random
  ```bash
  openssl rand -base64 32
  ```

**What if secret leaks?**
- Attacker can forge valid tokens
- All tokens must be invalidated (change secret)
- Users must re-login

### 2. Token Expiration (7 Days)

**Why 7 days?**

**Too short (1 hour):**
- Constant re-logins (bad UX)
- More load on login endpoint

**Too long (30 days):**
- Stolen tokens valid longer
- Can't revoke (user deleted? Token still valid!)

**7 days balances:**
- Reasonable UX (weekly re-login acceptable)
- Limited damage window (stolen token expires in 7 days max)

**Refresh tokens** (not implemented):
- Short-lived access tokens (1 hour)
- Long-lived refresh tokens (30 days)
- Trade-off: More complexity, better security

### 3. HTTPS Only

**Critical**: Always use HTTPS in production.

**Why?**
- Tokens sent in headers (readable over HTTP)
- MITM attacks can steal tokens

**In development**:
- HTTP acceptable (localhost)
- Use HTTPS proxies (ngrok) for testing

### 4. Password Hashing (bcrypt)

From `internal/service/user_service.go`:

```go
func (s *UserService) Register(ctx context.Context, email, password, name string) error {
    // Hash password with bcrypt
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 10)
    if err != nil {
        return err
    }

    user := &User{
        Email:        email,
        PasswordHash: string(hashedPassword),
        Name:         name,
    }

    return s.storage.Create(ctx, user)
}
```

**bcrypt cost: 10**
- Each increment doubles computation time
- Cost 10: ~100ms to hash (acceptable for login)
- Cost 12: ~400ms (more secure, slower)

**Why bcrypt?**
- **Slow**: Makes brute-force attacks impractical
- **Salted**: Each password gets random salt (same password → different hash)
- **Adaptive**: Can increase cost as computers get faster

**Alternatives:**
- **Argon2**: Newer, memory-hard (even more secure)
- **PBKDF2**: Older, NIST-approved (less secure than bcrypt)
- **SHA256**: Fast (bad for passwords! Use slow algorithms)

---

## Part 5: Token Revocation (The Hard Problem)

### The Problem

User logs in → receives token (valid 7 days).

Next day, user is deleted/banned. But token is still valid!

**Why?**
JWT is stateless. Server doesn't store tokens, so it can't "delete" them.

### Solutions

**1. Blocklist (compromises statelessness)**
```go
// Store revoked tokens in Redis
revokedTokens := map[string]bool{
    "token123": true,
    "token456": true,
}

// Check on every request
if revokedTokens[tokenString] {
    return errors.New("token revoked")
}
```

**Pros**: Can revoke instantly
**Cons**: Now stateful (Redis query on every request)

**2. Short expiry + Refresh tokens**
- Access token: 1 hour expiry
- Refresh token: 30 days expiry, stored in database
- Revoke refresh token (database delete)
- Access token expires naturally in 1 hour

**Pros**: Balance stateless (access token) with revocation (refresh token)
**Cons**: More complex flow

**3. Accept delayed revocation** (our approach)
- 7-day expiry
- Deleted users can't login again (new tokens)
- Existing tokens valid until expiry
- Acceptable for our use case (URL shortener, low stakes)

**Trade-off**: Simplicity vs perfect revocation.

---

## Summary

**JWT Advantages:**
- Stateless (no session store)
- Fast (no database lookup)
- Scalable (any server can validate)

**JWT Disadvantages:**
- Hard to revoke
- Larger than session IDs
- Secret key must be protected

**Our Implementation:**
- HS256 (HMAC-SHA256) signing
- 7-day expiration
- bcrypt password hashing (cost 10)
- Secret from environment variable

**Security Best Practices:**
- HTTPS only (production)
- Never commit secrets
- Long, random secret keys
- Appropriate expiry time

**Key Insight:**
JWT trades instant revocation for stateless simplicity. For most applications (including ours), this is a good trade-off. For high-security apps (banking), use refresh tokens or stick with sessions.

---

**Up next**: [Rate Limiting with Sliding Window →](./08-rate-limiting.md)

Learn how we use Redis sorted sets to implement a sliding window rate limiter that protects our APIs from abuse.

---

**Word Count**: ~2,200 words
**Code References**: `internal/service/user_service.go`, `internal/middleware/auth.go`
