# Migaku Stats API

A Go HTTP server that provides REST API access to Migaku's local IndexedDB data with browser automation and caching.

## Prerequisites

- Go
- Chrome/Chromium browser

## Quick Start

```bash
# Install dependencies
make install

# Set credentials (optional, for auto-login)
export EMAIL="your@email.com"
export PASSWORD="yourpassword"
export HEADLESS=true

# Start container
make docker-run
```

Server runs on `http://localhost:8080` with interactive API documentation at the root endpoint.

## Configuration

Environment variables:
- `EMAIL` / `PASSWORD` - Migaku credentials for auto-login
- `PORT` - Server port (default: 8080)
- `HEADLESS` - Run browser headless (default: true, set to "false" for visible)
- `CORS_ORIGINS` - Allowed CORS origins (comma-separated, default: "*")
- `API_SECRET` - API key for authentication (optional, enables auth if set)
- `CACHE_TTL` - Cache duration (default: 5m)
- `LOG_LEVEL` - Log level: DEBUG, INFO, WARN, ERROR (default: INFO)

## Development

```bash
make run
make build
make clean
make docker-run
```

## Cache

- In-memory caching with 5-minute default TTL
- Automatic refresh on expiry
- Configurable via API or environment

## Upcoming

- rate limiting
- more endpoints
- refactoring