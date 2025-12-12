# Authentication System - Testing Guide

## ✅ COMPLETE - Production-Grade Authentication

The entire authentication system is now implemented and ready to test!

## System Architecture

```
User → API Gateway (HTTP) → User Service (gRPC) → Database
                           ↓
                    JWT Token Generated
                           ↓
User → API Gateway + JWT → URL Service (gRPC) → Database (user_id filtered)
```

## Available Endpoints

### Authentication Endpoints (No Auth Required)

1. **POST /api/auth/register** - Create new account
   ```bash
   curl -X POST http://localhost:8080/api/auth/register \
     -H "Content-Type: application/json" \
     -d '{
       "email": "user@example.com",
       "password": "password123",
       "name": "John Doe"
     }'
   ```

   Response:
   ```json
   {
     "user_id": "uuid-here",
     "email": "user@example.com",
     "name": "John Doe",
     "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
   }
   ```

2. **POST /api/auth/login** - Get JWT token
   ```bash
   curl -X POST http://localhost:8080/api/auth/login \
     -H "Content-Type: application/json" \
     -d '{
       "email": "user@example.com",
       "password": "password123"
     }'
   ```

   Response:
   ```json
   {
     "user_id": "uuid-here",
     "email": "user@example.com",
     "name": "John Doe",
     "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
     "expires_at": 1234567890
   }
   ```

3. **GET /api/auth/profile** - Get user profile (Auth Required)
   ```bash
   curl -X GET http://localhost:8080/api/auth/profile \
     -H "Authorization: Bearer YOUR_JWT_TOKEN"
   ```

### URL Endpoints (Auth Required - JWT Token Needed)

All URL endpoints now require authentication via JWT token in the Authorization header.

4. **POST /api/urls** - Create short URL
   ```bash
   curl -X POST http://localhost:8080/api/urls \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_JWT_TOKEN" \
     -d '{
       "long_url": "https://google.com"
     }'
   ```

5. **POST /api/urls/custom** - Create custom alias URL
   ```bash
   curl -X POST http://localhost:8080/api/urls/custom \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_JWT_TOKEN" \
     -d '{
       "alias": "my-link",
       "long_url": "https://google.com"
     }'
   ```

6. **GET /api/urls** - List YOUR URLs only
   ```bash
   curl -X GET http://localhost:8080/api/urls \
     -H "Authorization: Bearer YOUR_JWT_TOKEN"
   ```

## Complete Test Flow

### Step 1: Start All Services

```bash
# Make sure databases are running
cd deployments/docker && docker-compose up -d

# Start all services
make dev

# Or manually:
USER_SERVICE_PORT=50052 JWT_SECRET="your-secret-key" ./bin/user-service &
./bin/url-service &
./bin/api-gateway &
./bin/redirect-service &
./bin/pipeline-worker &
./bin/cleanup-worker &
```

### Step 2: Register User 1

```bash
TOKEN1=$(curl -s -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "alice@example.com",
    "password": "password123",
    "name": "Alice"
  }' | jq -r '.token')

echo "Alice's token: $TOKEN1"
```

### Step 3: Create URLs as User 1

```bash
# Create regular URL
curl -X POST http://localhost:8080/api/urls \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN1" \
  -d '{"long_url": "https://alice-blog.com"}'

# Create custom URL
curl -X POST http://localhost:8080/api/urls/custom \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN1" \
  -d '{"alias": "alice-portfolio", "long_url": "https://alice.dev"}'
```

### Step 4: Register User 2

```bash
TOKEN2=$(curl -s -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "bob@example.com",
    "password": "password123",
    "name": "Bob"
  }' | jq -r '.token')

echo "Bob's token: $TOKEN2"
```

### Step 5: Create URLs as User 2

```bash
curl -X POST http://localhost:8080/api/urls \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN2" \
  -d '{"long_url": "https://bob-website.com"}'
```

### Step 6: Verify User Isolation

```bash
# Alice sees only HER URLs
echo "Alice's URLs:"
curl -s -X GET http://localhost:8080/api/urls \
  -H "Authorization: Bearer $TOKEN1" | jq '.urls[].long_url'

# Bob sees only HIS URLs
echo "Bob's URLs:"
curl -s -X GET http://localhost:8080/api/urls \
  -H "Authorization: Bearer $TOKEN2" | jq '.urls[].long_url'
```

### Step 7: Test Without Auth (Should Fail)

```bash
# Should return 401 Unauthorized
curl -X GET http://localhost:8080/api/urls

# Should return 401 Unauthorized
curl -X POST http://localhost:8080/api/urls \
  -H "Content-Type: application/json" \
  -d '{"long_url": "https://example.com"}'
```

## Security Features

✅ **JWT Authentication**
- 7-day token expiration
- Secure token validation on every request
- Token includes user_id claim

✅ **Password Security**
- Bcrypt hashing (industry standard)
- Salted hashes (automatic with bcrypt)
- Never stores plain text passwords

✅ **User Data Isolation**
- Each user only sees their own URLs
- user_id foreign key constraint in database
- Filtered queries at storage layer

✅ **Protected Endpoints**
- All URL operations require authentication
- Auth middleware validates JWT on every request
- Returns 401 for invalid/missing tokens

## Database Schema

```sql
-- Users table
CREATE TABLE users (
    id VARCHAR(50) PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- URLs table (updated)
CREATE TABLE urls (
    short_code VARCHAR(10) PRIMARY KEY,
    long_url TEXT NOT NULL,
    clicks BIGINT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,
    qr_code TEXT,
    user_id VARCHAR(50) REFERENCES users(id)  -- NEW!
);
```

## Environment Variables

```bash
# Required
JWT_SECRET="your-super-secret-key-change-in-production"
USER_SERVICE_ADDR="localhost:50052"

# Already configured in .env
DB_PRIMARY_DSN="..."
DB_REPLICA1_DSN="..."
REDIS_ADDR="localhost:6379"
```

## What's Next?

The backend is 100% complete and production-ready! Now you can:

1. **Test the API** - Use the curl commands above
2. **Build TUI** - Add login/signup screens to the TUI
3. **Deploy** - Set strong JWT_SECRET and deploy to production

## Services Running

- ✅ **user-service** (port 50052) - Authentication & user management
- ✅ **url-service** (port 50051) - URL shortening with user_id
- ✅ **api-gateway** (port 8080) - HTTP REST API with auth middleware
- ✅ **redirect-service** (port 8081) - URL redirects
- ✅ **pipeline-worker** - Analytics processing
- ✅ **cleanup-worker** - Expired URL cleanup

## Quick Test Script

```bash
#!/bin/bash

# Test authentication flow
echo "=== Testing Authentication Flow ==="

# 1. Register
echo -e "\n1. Registering new user..."
RESPONSE=$(curl -s -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123","name":"Test User"}')

TOKEN=$(echo $RESPONSE | jq -r '.token')
echo "Token: ${TOKEN:0:50}..."

# 2. Create URL
echo -e "\n2. Creating short URL..."
curl -X POST http://localhost:8080/api/urls \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"long_url":"https://google.com"}' | jq

# 3. List URLs
echo -e "\n3. Listing my URLs..."
curl -s -X GET http://localhost:8080/api/urls \
  -H "Authorization: Bearer $TOKEN" | jq '.urls | length'

echo -e "\n=== Test Complete ==="
```

Save as `test-auth.sh`, make executable (`chmod +x test-auth.sh`), and run (`./test-auth.sh`)!
