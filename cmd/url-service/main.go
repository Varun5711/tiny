package main

import (
	"context"
	"net"

	"github.com/Varun5711/shorternit/internal/cache"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
	"github.com/Varun5711/shorternit/internal/idgen"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/redis"
	"github.com/Varun5711/shorternit/internal/service"
	"github.com/Varun5711/shorternit/internal/storage"
	pb "github.com/Varun5711/shorternit/proto/url"
	"google.golang.org/grpc"
)

func main() {
	log := logger.New("url-service")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config: %v", err)
	}

	idGen, err := idgen.NewGenerator(cfg.Snowflake.DatacenterID, cfg.Snowflake.WorkerID)
	if err != nil {
		log.Fatal("Failed to create ID generator: %v", err)
	}

	ctx := context.Background()

	redisClient, err := redis.NewRedisClient(ctx, redis.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatal("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	urlCache := cache.NewMultiTierCache(
		cfg.Cache.L1Capacity,
		redisClient.GetClient(),
		cfg.Cache.L2TTL,
	)

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

	store := storage.NewPostgresStorage(dbManager)
	urlService := service.NewURLService(store, idGen, urlCache, redisClient.GetClient(), cfg.Services.BaseURL, cfg.Services.DefaultURLTTL)

	grpcServer := grpc.NewServer()
	pb.RegisterURLServiceServer(grpcServer, urlService)

	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal("Failed to listen on :50051: %v", err)
	}

	log.Info("Listening on :50051")

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal("Server error: %v", err)
	}
}
