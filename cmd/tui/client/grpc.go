package client

import (
	"context"
	"time"

	pb "github.com/Varun5711/shorternit/proto/url"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn    *grpc.ClientConn
	service pb.URLServiceClient
	token   string
	userID  string
}

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

func (c *Client) SetAuth(token, userID string) {
	c.token = token
	c.userID = userID
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

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

func (c *Client) GetURL(shortCode string) (*pb.GetURLResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.GetURLRequest{
		ShortCode: shortCode,
	}

	return c.service.GetURL(ctx, req)
}
