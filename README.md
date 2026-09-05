# Mydal

A self-hosted music streaming backend. Postgres holds the catalogue, MinIO
(S3-compatible) holds the audio files, and an HTTP API exposes both.

Mydal is early: artists, tracks, albums and playlists are wired end to end,
but only the artist endpoints have been through the error-contract and DTO
cleanup. See `PLAN.md` for the roadmap.

> **No authentication yet.** Run it on a trusted network only.

## Prerequisites

- Docker with Compose v2 — for the quick start, that is all you need
- Go 1.25+ — only to build or run outside containers

## Quick start

```sh
make up          # builds the app image and starts Postgres, MinIO and the API
curl localhost:8080/api/v1/artists -d '{"name":"Boards of Canada"}'
make logs        # follow the API logs
make down        # stop, keeping the data
```

Migrations are embedded in the binary and applied at startup, and the MinIO
bucket is created if missing, so there is no setup step.

The stack listens on 8080 (API), 5432 (Postgres), 9000 (MinIO) and 9001 (MinIO
console, `minioadmin` / `minioadmin`). If any of those are taken, override the
host port and leave the container ports alone:

```sh
APP_PORT=18080 POSTGRES_PORT=15432 MINIO_PORT=19000 MINIO_CONSOLE_PORT=19001 make up
```

`make down` keeps the named volumes, so the catalogue and the audio files
survive a restart. `make down-clean` deletes them.

## Running outside containers

Start only the dependencies, then run the server against them:

```sh
docker compose up -d db bucket
cp example.env .env
make run
```

`.env` is read if present; in production the variables come from the
environment instead. `make migrate` applies migrations and exits, for
deployments that want that as a separate step.

## Configuration

| Variable | Required | Meaning |
|---|---|---|
| `ADDR` | yes | Listen address, e.g. `:8080` |
| `DATABASE_URL` | yes | Postgres DSN |
| `MINIO_ENDPOINT` | yes | MinIO host:port |
| `BUCKET_NAME` | yes | Bucket for audio and artwork; created at startup |
| `MINIO_ACCESS_KEY` | | MinIO access key |
| `MINIO_SECRET_KEY` | | MinIO secret key |
| `MINIO_USE_SSL` | | `true` to reach MinIO over HTTPS |
| `LOG_LEVEL` | | `debug`, `info`, `warn`, `error` (default `info`) |
| `MODE` | | Anything but `production` forces `sslmode=disable` on the DSN |

Startup fails immediately, naming the variable, if a required one is missing.

## API

Everything is under `/api/v1`. Requests and responses are JSON.

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/artists` | Create an artist |
| `GET` | `/api/v1/artists/{id}` | Fetch an artist |
| `DELETE` | `/api/v1/artists/{id}` | Delete an artist |
| `POST` | `/api/v1/tracks` | Create a track |
| `GET` | `/api/v1/tracks/{id}` | Fetch a track |
| `DELETE` | `/api/v1/tracks/{id}` | Delete a track |
| `PUT` | `/api/v1/tracks/{id}/file` | Upload the audio file for a track |
| `GET` | `/api/v1/tracks/{id}/stream` | Stream a track's audio |
| `POST` | `/api/v1/albums` | Create an album |
| `GET` | `/api/v1/albums/{id}` | Fetch an album |
| `DELETE` | `/api/v1/albums/{id}` | Delete an album |
| `POST` | `/api/v1/playlists` | Create a playlist |
| `GET` | `/api/v1/playlists/{id}` | Fetch a playlist |
| `DELETE` | `/api/v1/playlists/{id}` | Delete a playlist |
| `PUT` | `/api/v1/playlists/{id}/tracks/{trackId}` | Add a track to a playlist |
| `DELETE` | `/api/v1/playlists/{id}/tracks/{trackId}` | Remove a track from a playlist |

```sh
$ curl -s localhost:8080/api/v1/artists -d '{"name":"Aphex Twin","bio":"Cornwall"}'
{"id":"...","name":"Aphex Twin","bio":"Cornwall","created_at":"..."}
```

Errors carry a JSON body and a meaningful status: `400` for malformed input or
a non-UUID id, `404` for a missing resource, `409` for a conflict, `500` for
anything unclassified — whose detail is logged rather than returned. That is
the artist contract; the track, album and playlist handlers predate it and
still answer with plain-text errors and domain-shaped JSON. Every response
carries an `X-Request-Id`, echoing the request's own if it sent one.

## API documentation

Swagger UI is served at `http://localhost:8080/swagger/` while the server is
running, and the generated spec is published to
<https://pslessing.github.io/Mydal/> on every push to `main`. Regenerate it
after changing routes or annotations:

```sh
make swagger
```

## Development

`make help` lists every target. The common ones:

| Target | Does |
|---|---|
| `make run` | Run the server locally |
| `make test` | Tests with the race detector |
| `make vet` / `make lint` | `go vet` / `golangci-lint` |
| `make swagger` | Regenerate the OpenAPI docs |
| `make up` / `make down` | Start / stop the Docker stack |
| `make migrate` | Apply migrations and exit |

CI runs vet, lint, tests and a Docker build against a live Postgres and MinIO.

## Layout

```
cmd/server        entry point: config, wiring, HTTP server
internal/api      router, middleware, handlers, DTOs
internal/service  business logic
internal/repository  Postgres access
internal/storage  BlobStore interface and its MinIO implementation
internal/domain   entities and sentinel errors
internal/config, internal/logging, internal/httpx
migrations        embedded SQL migrations
```

## License

MIT — see `LICENSE`.
