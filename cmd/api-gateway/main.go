package main

import (
	"net/http"

	"github.com/Varun5711/shorternit/internal/config"
	"github.com/Varun5711/shorternit/internal/handlers"
	"github.com/Varun5711/shorternit/internal/logger"
)

func main() {
	log := logger.New("api-gateway")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config: %v", err)
	}

	httpHandler, err := handlers.NewHTTPHandler(cfg.Services.URLServiceAddr, cfg.Services.BaseURL)
	if err != nil {
		log.Fatal("Failed to connect to url-service: %v", err)
	}

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

	log.Info("Listening on :%s", cfg.Services.APIGatewayPort)

	if err := http.ListenAndServe(":"+cfg.Services.APIGatewayPort, mux); err != nil {
		log.Fatal("Server error: %v", err)
	}
}
