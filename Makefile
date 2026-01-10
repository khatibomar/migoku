.PHONY: run build clean docker-run lint

# Run the API server
run:
	@echo "Starting Migaku Stats API Server..."
	go run .

# Build the server binary
build:
	@echo "Building migakustat..."
	go build -o bin/migakustat
	@echo "Binary created: migakustat"

# Run with Docker Compose
docker-run:
	@echo "Starting Migaku Stats API Server with Docker..."
	docker-compose up --build

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f migaku-server

# Run linting
lint:
	golangci-lint run