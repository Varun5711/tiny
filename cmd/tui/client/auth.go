package client

import (
	"context"
	"time"

	userpb "github.com/Varun5711/shorternit/proto/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AuthClient struct {
	conn    *grpc.ClientConn
	service userpb.UserServiceClient
}

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

func (c *AuthClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

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

func (c *AuthClient) Login(email, password string) (*userpb.LoginResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &userpb.LoginRequest{
		Email:    email,
		Password: password,
	}

	return c.service.Login(ctx, req)
}

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
