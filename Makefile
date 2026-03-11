.PHONY: run build clean docker-run lint examples

# Run the API server
run: build
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

# Serve static HTML examples from ./examples
# Open in browser:
#   http://localhost:4173/stats-dashboard/
#   http://localhost:4173/ai-study-coach/
examples:
	@echo "Serving static examples on http://localhost:4173"
	@echo "Open /stats-dashboard/ or /ai-study-coach/"
	cd examples && python3 -m http.server 4173
