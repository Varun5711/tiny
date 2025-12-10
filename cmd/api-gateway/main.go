package main

import (
	"context"
	"net/http"

	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/handlers"
	"github.com/Varun5711/shorternit/internal/logger"
	"github.com/Varun5711/shorternit/internal/middleware"
	"github.com/Varun5711/shorternit/internal/redis"
)

func main() {
	log := logger.New("api-gateway")

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

	httpHandler, err := handlers.NewHTTPHandler(cfg.Services.URLServiceAddr, cfg.Services.BaseURL)
	if err != nil {
		log.Fatal("Failed to connect to url-service: %v", err)
	}

	rateLimiter := middleware.NewRateLimiter(
		redisClient.GetClient(),
		cfg.RateLimit.Requests,
		cfg.RateLimit.Window,
	)

	mux := http.NewServeMux()

	mux.HandleFunc("/api/urls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			httpHandler.CreateURL(w, r)
		} else if r.Method == http.MethodGet {
			httpHandler.ListURLs(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	handler := middleware.CORS(mux)
	handler = middleware.RequestID(handler)
	handler = middleware.Recovery(log)(handler)
	handler = rateLimiter.Middleware(handler)

	log.Info("Listening on :%s", cfg.Services.APIGatewayPort)

	if err := http.ListenAndServe(":"+cfg.Services.APIGatewayPort, handler); err != nil {
		log.Fatal("Server error: %v", err)
	}
}
