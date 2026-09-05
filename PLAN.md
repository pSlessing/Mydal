# Mydal development plan

Mydal is a Go backend for a self-hosted music library: Postgres holds the catalogue, MinIO (S3-compatible) holds the audio files, and an HTTP API exposes both. This document is the roadmap from the current skeleton to a usable platform. It has two parts:

1. **Path to completion** — ordered milestones that end in a working streaming backend.
2. **Additional features** — what to build once the core works.

The "refactor now" groundwork that used to head this document is done: schema
and domain reconciled, migrations embedded and applied at startup, config
validated, `context` threaded, consumer-side interfaces, typed errors mapped to
status codes, a versioned HTTP layer with real middleware, the storage layer
behind a `BlobStore`, and the developer tooling below.

Each item names the files it touches so it can be picked up independently.

---

## Current state (September 2026)

What exists:

- Layered layout: `handlers -> service -> repository -> domain`, wired by hand in `cmd/server/main.go`, which validates config, tunes the DB pool, creates the MinIO bucket if missing, and shuts down gracefully on `SIGINT`/`SIGTERM`.
- Artists: create / get / delete work end to end under `/api/v1`, with request/response DTOs, UUID validation, and typed errors mapped to 400/404/409/500 by `httpx.WriteError`. Every request gets an id, a log line and a panic guard. Every repository and service method takes a `ctx`, threaded from the request.
- Tracks: repository exists, service and handler are empty stubs, routes are registered. `TrackService` has no methods yet, so its consumer holds it concretely.
- Storage: `internal/storage` exposes a `BlobStore` interface (`Put`, `Get`, `Stat`, `Delete`, `PresignedGetURL`) with a `MinIOStore` implementation, injected into the track handler.
- Five tables across four SQL migrations (artists, albums, tracks, playlists + `playlist_tracks`), reconciled with the domain structs and reversible. They are embedded in the binary and applied at startup by `migrations/migrations.go` (`-migrate-only` applies and exits).
- Routing is the stdlib `http.ServeMux`; Postgres is reached through `pgx/v5/stdlib`; `cmd/`, `internal/` and `migrations/` sit at the module root.
- Tooling: `Makefile`, multi-stage distroless `Dockerfile`, a three-service `compose.yaml` (app, Postgres 16, MinIO) with healthchecks, named volumes and overridable host ports, `golangci-lint` config and a GitHub Actions workflow. Still no tests — Milestone A owns those.

The build and `go vet` are clean.

---

## Part 1 — Path to completion

"Complete" here means: a user can upload audio files, the library is browsable by artist / album / track, playlists work, and any track can be streamed to a normal audio player. Each milestone is independently shippable.

### Milestone A — Solid foundation

Deliverables:

- Unit tests for handlers and services (using the consumer-side interfaces and fakes).
- Integration tests for repositories using `testcontainers-go` (Postgres + MinIO) or the compose stack, applying the embedded migrations per test run.
- `/healthz` (process up) and `/readyz` (DB ping + bucket reachable) endpoints.

Exit criteria: `make test` is green, `docker compose up` gives a working API with the artist endpoints, and the plan below can be executed without touching foundations again.

### Milestone B — Complete the catalogue CRUD

Finish the resources that already have tables:

- **Tracks:** implement `TrackService` and `TrackHandler` (currently empty). `GET /tracks`, `GET /tracks/{id}`, `PATCH /tracks/{id}` (metadata edit), `DELETE /tracks/{id}`. Creation happens through upload (Milestone C), not a bare POST.
- **Albums:** new repository, service, handler. `GET /albums`, `GET /albums/{id}` (with its tracks in order), `PATCH`, `DELETE`. `GET /artists/{id}/albums`.
- **Artists:** add `GET /artists` (list) and `PATCH /artists/{id}`. Include `bio`.
- **Playlists:** repository, service, handler against the `playlist_tracks` join table. `POST /playlists`, `GET /playlists`, `GET /playlists/{id}`, `PATCH /playlists/{id}`, `DELETE`, plus `POST /playlists/{id}/tracks` (append), `DELETE /playlists/{id}/tracks/{trackId}`, `PUT /playlists/{id}/tracks` (reorder: full ordered list of track IDs).
- **Pagination** on every list endpoint from day one: `?limit=&cursor=` or `?limit=&offset=`. Cursor (keyset on `created_at, id`) is the better default for a library that can hold tens of thousands of tracks.
- **Input validation** in the service layer (non-empty titles, valid UUIDs, position ranges).

### Milestone C — Upload pipeline

This is the first feature that makes the system useful.

1. **`POST /tracks/upload`** accepts `multipart/form-data` with the audio file. Stream the multipart part straight to the blob store (`PutObject` with `io.Reader`) rather than buffering in memory; set a max request size.
2. **Deduplicate by content hash.** Compute SHA-256 while streaming (use `io.TeeReader`). Store it in a new `tracks.content_hash` column with a unique index. A re-upload of the same file returns the existing track (200) instead of creating a duplicate.
3. **Extract tags** with `github.com/dhowden/tag` (title, artist, album, album artist, year, track/disc number, genre, embedded cover art) and duration/bitrate/format with a probe. Options: `ffprobe` via `os/exec` (most formats, needs the binary), or pure-Go decoders per format (`go-flac`, `go-mp3`, etc.). Start with `ffprobe` behind an interface so the pure-Go route stays open.
4. **Resolve or create Artist and Album** from the tags inside one transaction: find-or-create artist by normalised name, find-or-create album by `(artist_id, title)`, then insert the track. Add a unique index on `artists(lower(name))` and `albums(artist_id, lower(title))` to make this safe under concurrent uploads.
5. **Store cover art** from the tags in the blob store under `covers/{album_id}` and set `albums.cover_key`.
6. **Storage key scheme:** `tracks/{track_id}.{ext}` (opaque, stable), not the original filename. Keep the original filename in a column for display.
7. **Clean up on failure:** if the DB insert fails after the object was written, delete the object. Long-term, a periodic orphan sweep (objects with no row, rows with no object) is more robust than relying on cleanup-on-error.

### Milestone D — Streaming

1. **`GET /tracks/{id}/stream`** proxies the object from the blob store to the client. Requirements for browsers, mobile apps and `mpv`/`vlc` to seek and scrub:
   - Honour `Range` requests. Fetch the object with `GetObject` and `opts.SetRange(start, end)`, respond `206 Partial Content` with `Content-Range` and `Accept-Ranges: bytes`.
   - Send correct `Content-Type` (`audio/flac`, `audio/mpeg`, `audio/ogg`, `audio/mp4`...) and `Content-Length`.
   - Support `HEAD`.
   - Set `ETag` from the content hash and handle `If-None-Match` / `If-Range`.
   - Use the request `ctx` so a client disconnect cancels the object read (this is why 1.4 matters).
2. **Alternative: presigned URLs.** `GET /tracks/{id}/stream` returns a 302 to a short-lived presigned MinIO URL. Cheaper for the Go process, and MinIO handles ranges natively, but it exposes the MinIO endpoint to clients and complicates auth later. Recommend: implement the proxy first, keep presigned as an option behind a config flag for deployments that want it.
3. **`GET /tracks/{id}/download`** with `Content-Disposition: attachment; filename="..."` using the stored original filename.
4. **`GET /albums/{id}/cover`** and `GET /artists/{id}/image` serving art the same way, with cache headers (`Cache-Control: public, max-age=...`).

### Milestone E — Search and browse

- `GET /search?q=` across artists, albums, tracks. Start with Postgres `ILIKE` on a `pg_trgm` GIN index; move to `tsvector` full-text if ranking matters.
- Sort and filter parameters on list endpoints (`?sort=title|created_at|year`, `?artist_id=`, `?album_id=`, `?genre=`).
- Genres: add a `genre` column on tracks (from tags) and `GET /genres`.
- Library statistics endpoint (`GET /stats`: counts, total duration, total size) — cheap and useful for a UI.

### Milestone F — Ship it

- OpenAPI spec (hand-written or generated with `swaggo`/`oapi-codegen`) served at `/api/v1/openapi.json`. Makes clients and a future frontend much easier.
- Structured request logging with request IDs, and basic metrics (`/metrics`, Prometheus format) for request count/duration and upload throughput.
- Tagged release with a Docker image and a `compose.yaml` that works from a clean checkout.

At the end of Milestone F, Mydal is a functional, self-hostable music streaming backend.

---

## Part 2 — Additional features

Roughly ordered by value-to-effort for a *local* music platform. Items within a group are independent.

### Library management

- **Filesystem scanner.** Point Mydal at a directory (`MUSIC_DIR`) and have it walk, hash, tag-extract and import every file, reusing the Milestone C pipeline with a "local filesystem" `BlobStore` that references files in place instead of copying them into MinIO. For many self-hosters this is the *primary* ingestion path, so it may deserve promotion into Part 1. Watch the directory with `fsnotify` for incremental updates.
- **Metadata editing that writes back to files.** Optional: when a track's tags are edited via the API, rewrite the ID3/Vorbis tags in the stored file.
- **Bulk operations:** re-scan, re-tag, re-hash, orphan cleanup, as admin endpoints or a CLI subcommand (`mydal scan`, `mydal gc`).
- **External metadata lookup** via MusicBrainz (release/recording IDs, canonical artist names) and Cover Art Archive for missing artwork. Rate-limited, opt-in.
- **Multiple artists per track / album artist vs track artist.** Real libraries need it (compilations, features). Model with a `track_artists` join table and an `album_artist_id`. Worth designing the schema for early even if the UI comes later.
- **Lyrics** from embedded tags or a `.lrc` sidecar, served at `GET /tracks/{id}/lyrics` (synced and unsynced).

### Playback

- **Transcoding on the fly** (FLAC → Opus/MP3 at a requested bitrate) with `ffmpeg` for bandwidth-constrained clients. Cache transcoded output in the blob store keyed by `(track_id, codec, bitrate)`.
- **ReplayGain / loudness normalisation** values extracted at import (`rsgain` or ffmpeg's `loudnorm`) and exposed in track metadata.
- **Gapless playback support:** expose encoder delay/padding from tags so clients can do it.
- **HLS output** if a browser-based player needs adaptive streaming; likely unnecessary for a LAN.

### Users and personalisation

- **Authentication.** Single-admin API key first, then multi-user with sessions or JWT. Everything before this point is "trusted network only", and that should be stated in the README.
- **Per-user playlists, favourites (`liked` tracks/albums/artists), and ratings.**
- **Play history / scrobbling:** `POST /tracks/{id}/played`, `GET /me/history`, "recently played", "most played". Optional Last.fm / ListenBrainz scrobble forwarding.
- **Smart playlists** defined by a rule set (genre = X and year > Y and not played in 30 days) evaluated as a query.
- **Playback queue sync** across devices (server-held queue state).

### Interoperability

- **Subsonic / OpenSubsonic API compatibility layer.** This is the single highest-leverage feature for a self-hosted server: dozens of mature clients on every platform (Symfonium, DSub, Sonixd, play:Sub, Substreamer, Feishin...) work instantly. It maps almost 1:1 onto the resources above and can live in its own package (`internal/subsonic`) that adapts to the existing services.
- **DLNA/UPnP** exposure for smart speakers and TVs.
- **Export/import** of the library (playlists as M3U, full catalogue as JSON) for backup and migration.

### Frontend

- A minimal web UI (single-page, served from the Go binary via `embed`) for browsing, playing and uploading. Keep the API the source of truth so third-party clients remain first-class.

### Operations

- Backups: `pg_dump` + bucket sync scripts, or documented volume snapshot procedure.
- Rate limiting and upload quotas once auth exists.
- Multi-tenancy / library separation is *not* a goal; keep it out of the schema.

---

## Suggested sequencing summary

| Order | Work                                    | Why first |
|-------|-----------------------------------------|-----------|
| 1     | Milestone A (tests, health)              | Prevents regression during B–D |
| 2     | Milestone B (CRUD)                       | Straightforward, unblocks C |
| 3     | Milestone C (upload) + scanner from Part 2 if local files are the main source | First real value |
| 4     | Milestone D (streaming)                  | The product works |
| 5     | Milestones E, F                          | Usable and shippable |
| 6     | Auth, then OpenSubsonic                  | Safe to expose; instant client ecosystem |
| 7     | Everything else in Part 2 by preference  |           |
