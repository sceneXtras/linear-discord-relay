.PHONY: build run run-server test docker-build docker-run clean

# Build the binary
build:
	go build -o linear-daily-digest .

# Run one-shot report
run:
	go run main.go

# Run as HTTP server
run-server:
	MODE=server go run main.go

# Build Docker image
docker-build:
	docker build -t linear-daily-digest .

# Run with Docker Compose (server mode)
docker-run:
	docker-compose up -d

# Stop Docker Compose
docker-stop:
	docker-compose down

# Trigger report via HTTP (requires server running)
trigger:
	curl http://localhost:8080/report

# Health check
health:
	curl http://localhost:8080/health

# Clean build artifacts
clean:
	rm -f linear-daily-digest
