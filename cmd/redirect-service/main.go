package main

import (
	"log"
	"net/http"

	"github.com/Varun5711/shorternit/internal/handlers"
)

func main() {
	log.Println("Starting Redirect Service...")

	urlServiceAddr := "localhost:50051"

	redirectHandler, err := handlers.NewRedirectHandler(urlServiceAddr)
	if err != nil {
		log.Fatalf("Failed to connect to url-service: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", redirectHandler.HandleRedirect)

	log.Println("Redirect Service listening on :8081")
	log.Println("GET /{shortCode} - Redirect to long URL")

	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
