package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Varun5711/shorternit/internal/auth"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/service"
	"github.com/Varun5711/shorternit/internal/storage"
	pb "github.com/Varun5711/shorternit/proto/user"
	"google.golang.org/grpc"
)

func main() {
	log := logger.New("user-service")
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config: %v", err)
	}

	dbConfig := database.Config{
		PrimaryDSN:      cfg.Database.PrimaryDSN,
		ReplicaDSNs:     cfg.Database.ReplicaDSNs,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
	}

	dbManager, err := database.NewDBManager(ctx, dbConfig)
	if err != nil {
		log.Fatal("Failed to connect to database: %v", err)
	}
	defer dbManager.Close()

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "your-secret-key-change-in-production"
		log.Warn("JWT_SECRET not set, using default (insecure for production)")
	}

	jwtManager := auth.NewJWTManager(jwtSecret, 7*24*time.Hour)
	userStorage := storage.NewUserStorage(dbManager)
	userService := service.NewUserService(userStorage, jwtManager)

	port := os.Getenv("USER_SERVICE_PORT")
	if port == "" {
		port = "50052"
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterUserServiceServer(grpcServer, userService)

	log.Info("User service listening on port %s", port)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal("Failed to serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down user service...")
	grpcServer.GracefulStop()
	log.Info("User service stopped")
}
