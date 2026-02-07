.PHONY: run build clean docker-run lint

# Run the API server
run:
	@echo "Starting Migoku API Server..."
	LOG_LEVEL=debug API_SECRET="top-secret" go run ./...

# Build the server binary
build:
	@echo "Building migoku..."
	go build -o bin/migoku
	@echo "Binary created: migoku"

# Run with Docker Compose
docker-run:
	@echo "Starting Migoku API Server with Docker..."
	docker compose up --build

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f migoku

# Run linting
lint:
	golangci-lint run --config .golangci.yml
