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

## Quick Start

```bash
# Without container
make run

# With container
make docker-run
```

Server runs on `http://localhost:8080` with interactive API documentation at `/docs`.

OpenAPI spec is available at `/openapi.yaml`.

## Configuration

Environment variables:
- `PORT` - Server port (default: 8080)
- `CORS_ORIGINS` - Allowed CORS origins (comma-separated, default: "*")
- `API_SECRET` - Secret used to sign keys ( in case of comprimised key, change this and will generate new keys)
- `CACHE_TTL` - Cache duration (default: 10s) this also interval to update database with migaku so the shorter it is the more accurate.
- `LOG_LEVEL` - Log level: DEBUG, INFO, WARN, ERROR (default: INFO)

## Development

```bash
make run
make build
make clean
make docker-run
```

## Endpoints

<!-- endpoints-start -->
<!-- endpoints-end -->

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

Thanks to:
- [https://github.com/SebastianGuadalupe/MigakuStats](https://github.com/SebastianGuadalupe/MigakuStats) I learned how Migaku works by reading the plugin.
- waraki user on Discord for the decoding writing logic and login

And all of contributors to the repo.
<!-- readme: contributors -start -->
<table>
	<tbody>
		<tr>
            <td align="center">
                <a href="https://github.com/khatibomar">
                    <img src="https://avatars.githubusercontent.com/u/35725554?v=4" width="100;" alt="khatibomar"/>
                    <br />
                    <sub><b>عين</b></sub>
                </a>
            </td>
            <td align="center">
                <a href="https://github.com/StayBlue">
                    <img src="https://avatars.githubusercontent.com/u/23127866?v=4" width="100;" alt="StayBlue"/>
                    <br />
                    <sub><b>StayBlue</b></sub>
                </a>
            </td>
		</tr>
	<tbody>
</table>
<!-- readme: contributors -end -->