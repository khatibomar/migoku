[![go](https://github.com/khatibomar/migoku/actions/workflows/go.yml/badge.svg)](https://github.com/khatibomar/migoku/actions/workflows/go.yml)

# Migoku

A Go HTTP server that provides REST API access to Migaku's local IndexedDB data with browser automation and caching.

**Note:** Migoku is an unofficial, community-made utility that enables API access to your Migaku data stored locally in your browser.

Main important information this API can return are bunch of statistics, and information about your decks.
Like words you are learning and their metadata, etc...

> ⚠️ still in beta phase, I am doing some breaking changes from time to time

## Why?

At the moment of writing this project, Migaku doesn't officially support a way to access our own statistics through an API.
So that's why I built this project as a way to have an API that we can build cool things with it, like

<img width="559" height="869" alt="Image" src="https://github.com/user-attachments/assets/01ab7c28-0eb3-453f-84e5-a1e149640f4e" />

## Prerequisites

- Go
- Chrome/Chromium browser

## Quick Start

```bash
# Without container
make run

# With container
make docker-run
```

Server runs on `http://localhost:8080` with interactive API documentation at the root endpoint.

> First login per user is going to be slow, cause the login page download a huge wasm file. Subsequent login requests will be faster.

## Configuration

Environment variables:
- `PORT` - Server port (default: 8080)
- `HEADLESS` - Run browser headless (default: true, set to "false" for visible)
- `CORS_ORIGINS` - Allowed CORS origins (comma-separated, default: "*")
- `API_SECRET` - Secret used to sign keys ( in case of comprimised key, change this and will generate new keys)
- `CACHE_TTL` - Cache duration (default: 5m)
- `LOG_LEVEL` - Log level: DEBUG, INFO, WARN, ERROR (default: INFO)
- `TARGET_LANG` - Optional language selection when Migaku prompts (use language code like `ja` or name like `Japanese`)

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
- tests

## Credits

Thanks to [https://github.com/SebastianGuadalupe/MigakuStats](https://github.com/SebastianGuadalupe/MigakuStats) I learned how Migaku works by reading the plugin.
