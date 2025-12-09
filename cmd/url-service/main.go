package main

import (
	"log"
	"net"

	"github.com/Varun5711/shorternit/internal/idgen"
	"github.com/Varun5711/shorternit/internal/service"
	"github.com/Varun5711/shorternit/internal/storage"
	pb "github.com/Varun5711/shorternit/proto/url"
	"google.golang.org/grpc"
)

func main() {
	log.Println("Starting URL Service...")

	idGen, err := idgen.NewGenerator(1, 1)
	if err != nil {
		log.Fatalf("Failed to create ID generator: %v", err)
	}

	store := storage.NewMemoryStorage()
	urlService := service.NewURLService(store, idGen)

	grpcServer := grpc.NewServer()
	pb.RegisterURLServiceServer(grpcServer, urlService)

	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen on port 50051: %v", err)
	}

	log.Println("URL Service listening on :50051")

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
