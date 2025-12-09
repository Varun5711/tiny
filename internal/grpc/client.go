package grpc

import (
	pb "github.com/Varun5711/shorternit/proto/url"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewURLServiceClient(address string) (pb.URLServiceClient, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return pb.NewURLServiceClient(conn), nil
}
