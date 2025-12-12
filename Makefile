.PHONY: help dev build test clean db-up db-down db-reset install

help:
	@echo "Available commands:"
	@echo "  make install       - Install dependencies (overmind, go deps)"
	@echo "  make dev          - Start all services with overmind"
	@echo "  make build        - Build all services"
	@echo "  make test         - Run tests"
	@echo "  make db-up        - Start PostgreSQL + Redis"
	@echo "  make db-down      - Stop PostgreSQL + Redis"
	@echo "  make db-reset     - Reset database (WARNING: destroys data)"
	@echo "  make clean        - Clean build artifacts"

install:
	@echo "Installing Go dependencies..."
	@go mod download
	@go mod tidy
	@echo " Dependencies installed"
	@echo ""
	@echo "To install overmind (process manager):"
	@echo "  macOS: brew install overmind"
	@echo "  Linux: https://github.com/DarthSim/overmind#installation"

dev:
	@overmind start

build:
	@echo "Building all services..."
	@go build -o bin/url-service cmd/url-service/main.go
	@go build -o bin/api-gateway cmd/api-gateway/main.go
	@go build -o bin/redirect-service cmd/redirect-service/main.go
	@go build -o bin/analytics-worker cmd/analytics-worker/main.go
	@go build -o bin/pipeline-worker cmd/pipeline-worker/main.go
	@go build -o bin/cleanup-worker cmd/cleanup-worker/main.go
	@go build -o bin/user-service cmd/user-service/main.go
	@go build -o bin/tui cmd/tui/main.go
	@echo "All services built in bin/"

test:
	@go test -v ./...

db-up:
	@cd deployments/docker && docker-compose up -d
	@echo "PostgreSQL + Redis started"

db-down:
	@cd deployments/docker && docker-compose down
	@echo "PostgreSQL + Redis stopped"

db-reset:
	@echo "This will DELETE all data!"
	@cd deployments/docker && docker-compose down -v
	@cd deployments/docker && docker-compose up -d
	@sleep 5
	@echo "Database reset complete"

clean:
	@rm -rf bin/
	@go clean
	@echo "Cleaned build artifacts"
