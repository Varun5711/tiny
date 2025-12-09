package main

import (
	"context"
	"net/http"

	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/events"
	"github.com/Varun5711/shorternit/internal/handlers"
	"github.com/Varun5711/shorternit/internal/logger"
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

	clickProducer := events.NewClickProducer(redisClient.GetClient(), cfg.Redis.StreamName)

	redirectHandler, err := handlers.NewRedirectHandler(cfg.Services.URLServiceAddr, clickProducer)
	if err != nil {
		log.Fatal("Failed to connect to url-service: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", redirectHandler.HandleRedirect)

	log.Info("Listening on :%s", cfg.Services.RedirectServicePort)

	if err := http.ListenAndServe(":"+cfg.Services.RedirectServicePort, mux); err != nil {
		log.Fatal("Server error: %v", err)
	}
}
