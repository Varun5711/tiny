package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/Varun5711/shorternit/internal/database"
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

	dbConfig := database.Config{
		PrimaryDSN: "postgres://urlshortener:devpassword@localhost:5432/urlshortener?sslmode=disable",
		ReplicaDSNs: []string{
			"postgres://urlshortener:devpassword@localhost:5433/urlshortener?sslmode=disable",
			"postgres://urlshortener:devpassword@localhost:5434/urlshortener?sslmode=disable",
			"postgres://urlshortener:devpassword@localhost:5435/urlshortener?sslmode=disable",
		},
		MaxConns:        25,
		MinConns:        5,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
	}

	ctx := context.Background()
	dbManager, err := database.NewDBManager(ctx, dbConfig)
	if err != nil {
		log.Fatalf("Failed to create DB manager: %v", err)
	}
	defer dbManager.Close()

	log.Println("Connected to PostgreSQL (1 primary + 3 replicas)")

	store := storage.NewPostgresStorage(dbManager)
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
