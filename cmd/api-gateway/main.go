package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/Varun5711/shorternit/internal/analytics"
	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/database"
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

	httpHandler, err := handlers.NewHTTPHandler(cfg.Services.URLServiceAddr, cfg.Services.BaseURL)
	if err != nil {
		log.Fatal("Failed to connect to url-service: %v", err)
	}

	analyticsService := analytics.NewService(dbManager)
	analyticsHandler := handlers.NewAnalyticsHandler(analyticsService)

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

	mux.HandleFunc("/api/analytics/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/stats"):
			analyticsHandler.GetStats(w, r)
		case strings.HasSuffix(path, "/timeline"):
			analyticsHandler.GetTimeline(w, r)
		case strings.HasSuffix(path, "/geo"):
			analyticsHandler.GetGeoStats(w, r)
		case strings.HasSuffix(path, "/devices"):
			analyticsHandler.GetDeviceStats(w, r)
		case strings.HasSuffix(path, "/referrers"):
			analyticsHandler.GetReferrers(w, r)
		default:
			http.NotFound(w, r)
		}
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
