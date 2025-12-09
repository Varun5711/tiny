package main

import (
	"log"
	"net/http"

	"github.com/Varun5711/shorternit/internal/handlers"
)

func main() {
	log.Println("Starting API Gateway...")

	urlServiceAddr := "localhost:50051"
	baseURL := "http://localhost:8080"

	httpHandler, err := handlers.NewHTTPHandler(urlServiceAddr, baseURL)
	if err != nil {
		log.Fatalf("Failed to connect to url-service: %v", err)
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

	log.Println("API Gateway listening on :8080")
	log.Println("POST /api/urls - Create short URL")
	log.Println("GET  /api/urls - List all URLs")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
