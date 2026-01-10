[![go](https://github.com/khatibomar/migakustat/actions/workflows/go.yml/badge.svg)](https://github.com/khatibomar/migakustat/actions/workflows/go.yml)

# Migaku Stats API

A Go HTTP server that provides REST API access to Migaku's local IndexedDB data with browser automation and caching.

## Why?

At the moment of writing this project, migaku doesn't support a way to access our own statistics through an API.
So that's why I built this project as a way to have an API that we can build cool things with it, like

<img width="672" height="985" alt="Image" src="https://github.com/user-attachments/assets/ce649215-5947-4c6f-9e25-8c2c76f53a84" />

## Prerequisites

- Go
- Chrome/Chromium browser

## Quick Start

```bash
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
- paginating
- more endpoints
- refactoring
- bug fixes as they occur