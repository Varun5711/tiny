package main

import (
	"context"
	"net/http"

	"github.com/Varun5711/shorternit/internal/cache"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/events"
	"github.com/Varun5711/shorternit/internal/handlers"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/middleware"
	"github.com/Varun5711/shorternit/internal/redis"
)

func main() {
	log := logger.New("redirect-service")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config: %v", err)
	}

	redisClient, err := redis.NewRedisClient(context.Background(), redis.Config{
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

	clickProducer := events.NewClickProducer(redisClient.GetClient(), cfg.Redis.StreamName)

	redirectHandler, err := handlers.NewRedirectHandler(cfg.Services.URLServiceAddr, clickProducer, urlCache)
	if err != nil {
		log.Fatal("Failed to connect to url-service: %v", err)
	}

	rateLimiter := middleware.NewRateLimiter(
		redisClient.GetClient(),
		cfg.RateLimit.Requests,
		cfg.RateLimit.Window,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/", redirectHandler.HandleRedirect)

	handler := middleware.Recovery(log)(mux)
	handler = rateLimiter.Middleware(handler)

	log.Info("Listening on :%s", cfg.Services.RedirectServicePort)

	if err := http.ListenAndServe(":"+cfg.Services.RedirectServicePort, handler); err != nil {
		log.Fatal("Server error: %v", err)
	}
}
