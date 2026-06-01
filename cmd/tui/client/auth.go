// Package client provides gRPC client wrappers used by the TUI to communicate
// with the Tiny backend services. Each wrapper manages its own connection
// lifecycle and applies per-call timeouts so that the TUI remains responsive
// even when a backend is slow or unavailable.
package client

import (
	"context"
	"time"

	userpb "github.com/Varun5711/shorternit/proto/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AuthClient wraps the gRPC UserService for authentication operations
// (register, login, token validation). It holds a single persistent
// connection to the auth microservice.
type AuthClient struct {
	conn    *grpc.ClientConn
	service userpb.UserServiceClient
}

// NewAuthClient dials the auth service at addr with a 5-second connection
// timeout. WithBlock ensures the constructor does not return until the
// connection is ready or the timeout fires, giving the TUI a clear
// startup-time error rather than a deferred failure on the first RPC.
func NewAuthClient(addr string) (*AuthClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	return &AuthClient{
		conn:    conn,
		service: userpb.NewUserServiceClient(conn),
	}, nil
}

// Close shuts down the underlying gRPC connection. It is safe to call
// even if the connection is nil (e.g., if construction failed mid-way).
func (c *AuthClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Register creates a new user account via the auth service. The 10-second
// timeout accommodates potential password-hashing latency on the server side.
func (c *AuthClient) Register(email, password, name string) (*userpb.RegisterResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &userpb.RegisterRequest{
		Email:    email,
		Password: password,
		Name:     name,
	}

	return c.service.Register(ctx, req)
}

// Login authenticates existing credentials and returns a JWT token on success.
func (c *AuthClient) Login(email, password string) (*userpb.LoginResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &userpb.LoginRequest{
		Email:    email,
		Password: password,
	}

	return c.service.Login(ctx, req)
}

// ValidateToken checks whether a JWT is still valid. This is used at TUI
// startup to verify a persisted session token before skipping the login view.
func (c *AuthClient) ValidateToken(token string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &userpb.ValidateTokenRequest{
		Token: token,
	}

	resp, err := c.service.ValidateToken(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Valid, nil
}
