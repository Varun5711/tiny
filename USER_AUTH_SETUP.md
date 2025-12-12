# User Authentication System - Setup Guide

## What Was Created

I've implemented a complete user authentication system to fix the privacy issue where everyone could see all URLs. Now each user will only see their own URLs.

### New Components

1. **User Service** (`cmd/user-service/`)
   - gRPC service running on port 50052
   - Handles user registration, login, profile management
   - JWT token-based authentication (7-day expiration)
   - Secure password hashing with bcrypt

2. **User Proto** (`proto/user/user.proto`)
   - Register: Create new user account
   - Login: Authenticate and get JWT token
   - GetProfile: Get user info from token
   - UpdateProfile: Update user details
   - ValidateToken: Check if token is valid

3. **Authentication Layer** (`internal/auth/`)
   - JWT token generation and validation
   - Password hashing and verification
   - Secure secret key management

4. **User Storage** (`internal/storage/user_storage.go`)
   - Create users
   - Get user by email/ID
   - Update user profile

5. **Database Schema** (`scripts/databases/add_users.sql`)
   - `users` table with email, name, password hash
   - `user_id` column added to `urls` table
   - Foreign key constraint linking URLs to users

## Setup Instructions

### Step 1: Apply Database Migration

Run this command to create the users table and add user_id to urls:

```bash
psql -h localhost -U postgres -d url_shortener -f scripts/databases/add_users.sql
```

If you get a password prompt, the password is likely `postgres` or check your `.env` file.

### Step 2: Set JWT Secret (Important!)

Add this to your `.env` file or export it:

```bash
export JWT_SECRET="your-super-secret-key-change-this-in-production"
```

**IMPORTANT**: Use a strong, random secret in production!

### Step 3: Build the User Service

```bash
go build -o bin/user-service cmd/user-service/main.go
```

### Step 4: Add to Procfile

Add this line to your `Procfile`:

```
user-service: USER_SERVICE_PORT=50052 ./bin/user-service
```

### Step 5: Update Makefile

Update the `build` target in Makefile to include:

```makefile
@go build -o bin/user-service cmd/user-service/main.go
```

### Step 6: Start the User Service

```bash
# If using overmind/mprocs
make dev

# Or run manually
USER_SERVICE_PORT=50052 ./bin/user-service
```

## Next Steps (TODO)

### 1. Update URL Service to Filter by User ID

The URL proto has been updated with `user_id` fields, but the service logic needs to be updated:

- **CreateURL**: Save `user_id` with the URL
- **ListURLs**: Filter URLs by `user_id`
- **GetURL**: Verify user owns the URL (optional, for editing/deleting)

### 2. Add Authentication to TUI

The TUI needs login/signup screens:

- Login screen (email + password)
- Signup screen (name + email + password)
- Store JWT token after login
- Send token with all URL operations
- Extract `user_id` from token and send with requests

### 3. Add Authentication Middleware to API Gateway

The API gateway should:
- Extract JWT token from `Authorization` header
- Validate token using user-service
- Extract `user_id` and pass to url-service

## Testing the User Service

### Register a New User

```bash
grpcurl -plaintext -d '{
  "email": "test@example.com",
  "password": "password123",
  "name": "Test User"
}' localhost:50052 user.UserService/Register
```

### Login

```bash
grpcurl -plaintext -d '{
  "email": "test@example.com",
  "password": "password123"
}' localhost:50052 user.UserService/Login
```

You'll get a JWT token in the response. Save this for authenticated requests.

### Get Profile

```bash
grpcurl -plaintext -d '{
  "token": "your-jwt-token-here"
}' localhost:50052 user.UserService/GetProfile
```

## Security Notes

1. **JWT Secret**: Must be kept secret and never committed to git
2. **Password Requirements**: Minimum 8 characters (can be increased)
3. **Token Expiration**: 7 days (configurable in user-service)
4. **HTTPS**: In production, always use HTTPS/TLS for API calls
5. **Password Storage**: Uses bcrypt with default cost (secure)

## Architecture

```
┌─────────┐     JWT Token      ┌──────────────┐
│   TUI   │ ──────────────────>│ User Service │
└─────────┘                     │  (port 50052)│
     │                          └──────────────┘
     │                                  │
     │ user_id + JWT                    │
     │                                  ▼
     ▼                          ┌──────────────┐
┌─────────────┐                 │   Database   │
│ URL Service │ ───────────────>│    users     │
│ (port 50051)│                 │    urls      │
└─────────────┘                 └──────────────┘
```

## Files Created

- `proto/user/user.proto` - User service definition
- `proto/user/user.pb.go` - Generated protobuf code
- `proto/user/user_grpc.pb.go` - Generated gRPC code
- `internal/auth/jwt.go` - JWT token management
- `internal/auth/password.go` - Password hashing
- `internal/models/user/user.go` - User models
- `internal/storage/user_storage.go` - User database operations
- `internal/service/user_service.go` - User service implementation
- `cmd/user-service/main.go` - User service entry point
- `scripts/databases/add_users.sql` - Database migration

## Dependencies Added

- `github.com/golang-jwt/jwt/v5` - JWT tokens
- `golang.org/x/crypto/bcrypt` - Password hashing
- `github.com/google/uuid` - UUID generation

All dependencies have been added via `go get` and `go mod tidy`.
