package client

import (
	"context"
	"time"

	pb "github.com/Varun5711/shorternit/proto/url"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the gRPC URLService for URL CRUD operations. It carries
// auth state (token + userID) so that every RPC is automatically scoped
// to the logged-in user without requiring the caller to pass credentials
// on each call.
type Client struct {
	conn    *grpc.ClientConn
	service pb.URLServiceClient
	token   string
	userID  string
}

// NewClient dials the URL service at addr with a 5-second blocking timeout.
// See NewAuthClient for rationale on why the connection is blocking.
func NewClient(addr string) (*Client, error) {
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

	return &Client{
		conn:    conn,
		service: pb.NewURLServiceClient(conn),
	}, nil
}

// SetAuth stores the JWT token and user ID obtained after login or signup.
// Subsequent RPC calls embed the userID in request payloads so the URL
// service can enforce per-user ownership.
func (c *Client) SetAuth(token, userID string) {
	c.token = token
	c.userID = userID
}

// Close shuts down the gRPC connection. Safe to call on a nil conn.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// CreateURL shortens a long URL using a server-generated short code.
// The expiresAt timestamp is a Unix epoch; the TUI defaults to 3 days.
func (c *Client) CreateURL(longURL string, expiresAt int64) (*pb.CreateURLResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.CreateURLRequest{
		LongUrl:   longURL,
		UserId:    c.userID,
		ExpiresAt: expiresAt,
	}

	return c.service.CreateURL(ctx, req)
}

// CreateCustomURL shortens a long URL using a user-chosen alias instead
// of a generated code. The alias is validated server-side as well.
func (c *Client) CreateCustomURL(alias, longURL string, expiresAt int64) (*pb.CreateCustomURLResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.CreateCustomURLRequest{
		Alias:     alias,
		LongUrl:   longURL,
		UserId:    c.userID,
		ExpiresAt: expiresAt,
	}

	return c.service.CreateCustomURL(ctx, req)
}

// ListURLs fetches a paginated list of the authenticated user's short URLs.
// The TUI currently fetches up to 100 URLs in one call and handles
// pagination client-side for simplicity.
func (c *Client) ListURLs(limit, offset int32) (*pb.ListURLsResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.ListURLsRequest{
		Limit:  limit,
		Offset: offset,
		UserId: c.userID,
	}

	return c.service.ListURLs(ctx, req)
}

// GetURL retrieves the details of a single short URL by its code.
func (c *Client) GetURL(shortCode string) (*pb.GetURLResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.GetURLRequest{
		ShortCode: shortCode,
	}

	return c.service.GetURL(ctx, req)
}
