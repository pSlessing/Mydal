# Mydal

A music library management and streaming backend.

## Prerequisites

- Go 1.25+
- PostgreSQL
- MinIO (or S3-compatible storage)

## Quick start

```bash
cp example.env .env   # then edit to match your environment

# Start Postgres and MinIO dependencies
docker compose up -d

# Build and run the server
make run
```

## API documentation

Swagger/OpenAPI docs are available at `http://localhost:8080/swagger/` when the server is running.

Regenerate the spec after route or handler changes:

```bash
make swagger
```

The generated spec (`swagger.json` / `swagger.yaml`) is also uploaded as a build artifact and deployed to GitHub Pages on every push to `main`.

## Available commands

| Command | Description |
|---|---|
| `make` / `make all` | Regenerate swagger docs and build binary |
| `make swagger` | Regenerate swagger docs only |
| `make build` | Build the server binary |
| `make run` | Build and run the server |
| `make clean` | Remove binary and generated docs |

## Project structure

```
src/
├── cmd/server/       # Entry point
├── internal/
│   ├── api/          # HTTP router and handlers
│   ├── domain/       # Domain models
│   ├── pkg/          # Shared utilities (config, logging)
│   ├── repository/   # Data access layer
│   └── service/      # Business logic
└── migrations/       # Database migrations
```
