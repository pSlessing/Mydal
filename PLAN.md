# Mydal development plan

Mydal is a Go backend for a self-hosted music library: Postgres holds the
catalogue, MinIO (S3-compatible) holds the audio, and a versioned HTTP API
under `/api/v1` exposes both. This plan replaces the earlier one committed as
`PLAN.md`: every defect it listed (D1 to D12) has been fixed and is now pinned
by a test, and only D13 (no authentication) remains. What follows is a fresh
audit of the code as it stands on 2026-09-08.

The audit was done by reading every source file, running `go build`, `go vet`,
`go test -short` (all green), `golangci-lint run` (11 findings, see B5), and
regenerating the OpenAPI spec into a scratch directory (no drift from the
committed one). The integration tests were **not** run here: they need a
Postgres and a MinIO, and neither was up on this machine. CI runs them.

---

## Part 0 — How it works

```
cmd/server/main.go        flags, .env, config, DB pool, migrations, MinIO client,
                          bucket creation, hand-wired DI, HTTP server, shutdown
internal/api/router.go    stdlib ServeMux (Go 1.22 method patterns), Swagger UI
internal/api/middleware.go  RequestID -> Logging -> Recover, outside the mux
internal/api/handlers     one handler per resource, DTOs, audio sniffer, probes
internal/service          validation and orchestration (DB row + blob object)
internal/repository       database/sql over pgx, SQLSTATE -> domain sentinel
internal/domain           entities and ErrNotFound / ErrInvalidInput / ErrConflict
internal/storage          BlobStore interface and the MinIO implementation
internal/httpx            error -> status mapping, JSON writers
internal/testutil         per-test Postgres schema and per-test MinIO bucket
migrations                five embedded SQL migrations, applied at startup
```

Request flow for a write: handler validates the path id (`pathUUID`) and
decodes a request DTO, the service checks required fields and body-borne
UUIDs, the repository runs the SQL and turns constraint violations into
sentinels (`classify`), and `httpx.WriteError` maps the sentinel to 400 / 404 /
409 and anything else to a logged 500 with a generic body.

The upload path (`PUT /tracks/{id}/file`) reads the first 512 bytes, sniffs
the container, streams the body through a SHA-256 hasher into MinIO under
`tracks/{id}.{ext}`, then records key and hash in one UPDATE. A unique partial
index on `content_hash` makes a second copy of the same audio a 409. The stream
path (`GET /tracks/{id}/stream`) stats the object, sets `ETag` from the hash,
and hands the seekable MinIO object to `http.ServeContent` for ranges and
conditional requests.

Deletes are "row first, then object": the catalogue is authoritative, and an
object with no row is sweepable whereas a row with no object is a broken
track. Artist deletion collects the cascaded tracks' keys inside the
transaction before the `ON DELETE CASCADE` fires.

---

## Part 1 — Defects and weaknesses

Severity: **high** = data loss, resource exhaustion, or a red build;
**medium** = a wrong answer, a broken invariant, or a misleading contract;
**low** = debt.


### B2 — A chunked upload buffers ~537 MiB of RAM (high)

`internal/storage/minio.go:38` passes `size = -1` for a body without
`Content-Length` and sets no `PartSize`. minio-go's `OptimalPartInfo(-1, 0)`
then sizes parts for a 5 TiB object, about 537 MiB each, and buffers a whole
part in memory before sending it. `MAX_UPLOAD_BYTES` bounds the bytes written,
not the memory used, so a handful of concurrent chunked uploads exhausts a
small host. The test `TestUploadAcceptsChunkedBodies` exercises this path with
a tiny body and cannot see it.

Fix: set `PutObjectOptions.PartSize` when `size < 0`, sized from the cap
(`max(16 MiB, MaxUploadBytes/10000)` keeps every allowed upload inside the
10000-part limit). Consider also setting `NumThreads: 1` to keep one buffer per
request.

Touches: `minio.go` (and `BlobStore.Put` if the cap is threaded through).

### B3 — `POST /artists` accepts an empty name; numeric fields are unbounded (medium)

`internal/service/artistservice.go:33` does no validation, while the album,
track and playlist services all reject a blank title. `{}` creates an artist
with `name: ""`. There is also no uniqueness on artists, so the same name can
be created any number of times, which the future tag-driven upload pipeline
will do constantly.

The track create request accepts negative `duration_ms`, `bitrate`,
`file_size`, `track_number` and `disc_number`, and a `duration_ms` above
2^31-1 is a Postgres "integer out of range" (SQLSTATE 22003) that `classify`
does not know, so it is a 500.

Fix: `requireNonEmpty("name")` in `CreateArtist`; non-negative and range
checks in `TrackService.CreateTrack`; a unique index on `artists(lower(name))`
once the find-or-create semantics are decided (see Part 3).

Touches: `artistservice.go`, `trackservice.go`, `validate.go`, a migration.

### B4 — Unknown paths and wrong methods answer plain text (medium)

Verified against `NewRouter`:

| Request | Status | Body |
|---|---|---|
| `GET /nope` | 404 | `text/plain` "404 page not found" |
| `PATCH /api/v1/artists/{id}` | 405 | `text/plain` "Method Not Allowed", `Allow: DELETE, GET, HEAD` |
| `GET /api/v1/artists` | 405 | `text/plain` "Method Not Allowed", `Allow: POST` |

The README promises one JSON error contract with the 416 on `/stream` as the
only exception. A wrong method on a known endpoint is an endpoint error and
should keep the contract.

Fix: register a catch-all `"/"` handler that answers JSON 404, and intercept
the mux's own 405 (a small `ResponseWriter` wrapper that rewrites a 405 whose
body the mux is about to write, preserving `Allow`). Add a router test; there
are none today.

Touches: `router.go`, new `router_test.go`.


### B6 — The OpenAPI spec documents the probes at the wrong path (medium)

`@BasePath /api/v1` applies to every `@Router`, so `healthhandler.go:38` and
`:50` render as `/api/v1/healthz` and `/api/v1/readyz` in the published spec.
Those paths 404. The spec is deployed to GitHub Pages on every push to `main`.

Fix: drop `@BasePath` and write the full path in each `@Router`, or exclude
the probes from the spec and document them only in the README. Regenerate with
`make swagger`.

### B7 — Artist deletion can orphan an in-flight upload, and there is no sweep (medium)

`artistrepo.go:58` collects the tracks' storage keys with a plain SELECT under
read committed. An upload whose `SetTrackFile` commits after that SELECT and
before the DELETE has its row cascaded away and its object never collected.
`artistservice.go:39` says such orphans are for "the orphan sweep" to reclaim.
No sweep exists: there is no `-gc` flag, no listing of the bucket, nothing
that compares rows to objects.

Fix: `SELECT ... FOR UPDATE` on the tracks rows in `DeleteArtist`, so a
concurrent `SetTrackFile` waits and then sees zero rows (the handler already
deletes the object in that case). Then implement the sweep as
`mydal -gc [-dry-run]`: list the bucket, list `storage_key` values, delete
objects with no row, and report rows whose object is missing. Every
"Orphaned object" log line in the codebase is a promise that this exists.

Touches: `artistrepo.go`, `main.go`, a new `internal/gc` package, `BlobStore`
gains a `List`.

### B8 — Catalogue metadata is never reconciled with the uploaded file (medium)

`format`, `file_size`, `bitrate` and `duration_ms` are whatever the client
claimed at `POST /tracks`. The upload endpoint sniffs the real format and
streams every byte through a hasher, yet `trackrepo.go:108` records only key
and hash. `GET /tracks/{id}` can say `"format":"mp3"` for a FLAC, and
`file_size` for an uploaded track is 0 unless the client guessed.

Fix: extend `SetTrackFile` to write `format` and `file_size` (count bytes on
the way through with an `io.Writer` next to the hasher). Duration and bitrate
need a probe and belong to the upload pipeline in Part 3. Drop those four
fields from `createTrackRequest` once the upload sets them, or keep them as
hints and overwrite on upload.

### B9 — Streaming makes two HEAD requests before the first byte (medium)

`streamhandler.go:59` calls `Stat`, then `:65` calls `Get`, and `minio.go:55`
inside `Get` calls `obj.Stat()` again to surface a missing key early. Then
`http.ServeContent` seeks to the end and back, each of which minio-go turns
into a request. That is at least two HEADs and two GETs per play, and more
per range request.

Fix: have `Get` return the `ObjectInfo` it already fetched
(`Get(ctx, key) (io.ReadSeekCloser, ObjectInfo, error)`) and drop the separate
`Stat` in the handler. Longer term, translate the `Range` header into
`GetObjectOptions.SetRange` for a single request per play.

### B10 — Errors are logged twice, without a request id (medium)

Every repository logs failures at `Error` level and returns them; the handler
then logs the same error again through `WriteError`. Neither line carries the
`request_id` that the middleware minted, so the two cannot be correlated with
the access log line. Client mistakes (a foreign key violation in
`CreatePlaylist`) are also logged at `Error`.

Fix: log once, at the edge, with the request id (pull it from the context in
`WriteError`, or carry a request-scoped `*slog.Logger` in the context). Delete
the repository-level `logger.Error` calls; the wrapped error already names the
operation.

### B11 — JSON bodies are unbounded and lax (medium)

The four `json.NewDecoder(r.Body).Decode` sites (`albumhandler.go:68`,
`artisthandler.go:80`, `trackhandler.go:74`, `playlisthandler.go:71`) read
without `http.MaxBytesReader`, accept unknown fields silently, and accept
trailing bytes after the first JSON value. A one-line `decodeJSON` helper with
a 1 MiB cap, `DisallowUnknownFields`, and a trailing-token check fixes all
four. Separately, `serve` sets no `ReadTimeout` (correct, because of
streaming), so a slow upload holds a connection forever; set a read deadline
in the upload handler via `http.ResponseController` (the recorder's `Unwrap`
already makes that work).

### B12 — `internal/api` depends on `cmd/server/docs` (low)

`router.go:8` blank-imports the generated docs from under the binary. A
library package depending on the entry point's generated code inverts the
dependency direction and drags `swag` into every consumer of the router.
Move the generated package to `internal/api/docs` (adjust `make swagger` and
the docs workflow) or register the Swagger handler in `main.go`.

### B13 — Playlist positions lose density when a track is deleted (low)

`RemoveTrack` renumbers to keep positions a dense `0..n-1`, and the comment
documents that as an invariant. Deleting a track cascades through
`playlist_tracks` without renumbering, leaving gaps. Nothing breaks today
(`AddTrack` uses `MAX+1`, reads order by position), but the reorder endpoint
planned in Part 3 will assume density. Also, `playlist_tracks(track_id)` has
no index, so that cascade scans the table.

Fix: either drop the density claim or renumber in `DeleteTrack`; add
`CREATE INDEX playlist_tracks_track_id_idx ON playlist_tracks (track_id)`.

### B14 — Configuration edges (low)

- `config.go:69`: `MINIO_USE_SSL` is true only for the literal `true`; use
  `strconv.ParseBool` and reject garbage.
- `config.go:110`: any `MODE` other than `production` overwrites an explicit
  `sslmode` in `DATABASE_URL`. Only set it when absent.
- An unrecognised `LOG_LEVEL` silently becomes `info`; log a warning.
- `config.go:36`: the MinIO access key is logged in clear at debug.
- `-healthcheck` runs `config.Load()` and so fails on a missing
  `DATABASE_URL` for a reason unrelated to health. Only `ADDR` is needed.

### B15 — Wire-format inconsistencies (low)

- `trackResponse` exposes `storage_key`, which is bucket layout, not API. A
  `has_file` boolean says what a client needs.
- `album_id` is omitted when empty but `release_date` is `null` when absent:
  two conventions for absence in one API. Pick one.
- 400 and 409 messages carry Postgres constraint names
  (`conflict: tracks_content_hash_key already exists`). A stable `code`
  field (`not_found`, `invalid_input`, `conflict`, `duplicate_audio`) beside
  the message would let clients branch without parsing prose.

### B16 — Storage and HTTP helpers with no caller (low)

`httpx.RespondWithPacket` and `BlobStore.PresignedGetURL` are unused.
`RespondNoContent` is used by the artist handler while the other six 204 sites
call `w.WriteHeader` directly. `RespondWithJSON` falls back to a `text/plain`
`http.Error` when marshalling fails, the one place a non-JSON 500 can come
from. `storage.notFound` treats every 404 as `ErrNotFound`, so a deleted
bucket reads as "track not found" instead of an outage.

### B17 — CI and tooling drift (low)

- `ci.yml:67` pins golangci-lint to `latest`; `swagger-docs.yml:24` installs
  `swag@latest` while `go.mod` pins v1.16.6. Both should match a version.
- The docs workflow regenerates the spec but never checks the committed copy
  matches (`git diff --exit-code cmd/server/docs`). They match today.
- No `gofmt -l` or `go mod tidy` diff check in CI (both clean today).
- CI builds the image and never runs it; a `docker run ... -healthcheck`
  smoke test would catch a broken distroless image.
- `go.mod` says `go 1.25.7` (a patch version) while the Dockerfile uses
  `golang:1.25-alpine`; the image build relies on toolchain auto-download
  whenever the tag lags. Prefer `go 1.25` plus a `toolchain` line.
- `compose.yaml:23` names the MinIO container `mydal`, which is what a reader
  expects the app container to be called.

### B18 — Test gaps (low)

No tests for the middleware (request id echo, log line, panic recovery and
the `ErrAbortHandler` passthrough), the router (B4), `config.Load`,
`MinIOStore` directly (`notFound`, `Ping`), or `migrations.Down`.
`NewHealthHandler` takes `*sql.DB` rather than `Pinger`, so its test
constructs the struct by hand. The upload tests use a 4 KiB cap and cannot
observe B2.

### B19 — No authentication (known, unchanged)

Every endpoint, and the Swagger UI, is open. The README says so. Recorded
because it gates any deployment beyond a trusted network and because the
OpenSubsonic layer in Part 3 carries credentials on every request.

---

## Part 2 — Sequenced fixes

### Milestone 1 — Stop losing data and memory

1. B1: non-colliding object keys, delete the previous key only after the row
   commits, test the same-format duplicate case.
2. B2: `PartSize` for unknown-length uploads.
3. B7: `FOR UPDATE` in `DeleteArtist`, then the `-gc` sweep.
4. B5: clear lint, pin the linter.

Exit: a re-upload can never detach a track from its audio; a chunked upload
costs tens of MiB, not hundreds; `mydal -gc` reports zero orphans after the
storage tests run.

### Milestone 2 — Make the contract true everywhere

1. B3: artist name validation, numeric bounds.
2. B4: JSON 404 and 405 from the router, with a router test.
3. B6: correct probe paths in the spec.
4. B11: bounded, strict JSON decoding; a read deadline on uploads.
5. B15: `code` field on errors, `has_file`, one absence convention.

Exit: the README's error paragraph holds for every request the server can
receive, not only for routed ones, and the published spec has no path that
404s.

### Milestone 3 — Honest metadata and cheaper streaming

1. B8: record sniffed `format` and counted `file_size` on upload.
2. B9: one stat per play, then range passthrough.
3. B10: single, correlated error logging.
4. B13: index and density decision.

### Milestone 4 — Hygiene

B12, B14, B16, B17, B18. None blocks a user; all reduce the cost of the
features below.

---

## Part 3 — Roadmap once the defects are gone

Unchanged in substance from the previous plan, ordered by leverage.

- **Complete the CRUD.** List endpoints with keyset pagination on
  `(created_at, id)`, `PATCH` for metadata, `GET /artists/{id}/albums`,
  `GET /albums/{id}/tracks` in order, playlist reorder as
  `PUT /playlists/{id}/tracks` taking the full ordered list (needs B13).
- **Upload pipeline.** Tag extraction (`github.com/dhowden/tag`), a
  duration and bitrate probe behind an interface, find-or-create artist and
  album in one transaction guarded by unique indexes on `artists(lower(name))`
  and `albums(artist_id, lower(title))`, cover art to `covers/{album_id}`
  with an upload and a `GET /albums/{id}/cover`. B8 is the first step of this.
- **Filesystem scanner.** Point Mydal at `MUSIC_DIR` and ingest in place
  through a local-filesystem `BlobStore`. For most self-hosters this is the
  primary ingestion path.
- **Serving.** `GET /tracks/{id}/download` with `Content-Disposition`; a
  presigned-URL redirect behind a config flag, which is what
  `PresignedGetURL` was added for.
- **Search and browse.** `pg_trgm` GIN index and `GET /search?q=`, sort and
  filter on the list endpoints, a `genre` column from tags, `GET /stats`.
- **Authentication (B19).** A single admin API key first, then users and
  sessions. Also gate `/swagger/` behind it or a config flag.
- **OpenSubsonic layer** in `internal/subsonic`, adapting the existing
  services. The highest-leverage feature in the document; it must follow auth.
- Then: transcoding and ReplayGain, play history and favourites, smart
  playlists, an embedded web UI (needs CORS), MusicBrainz lookup, lyrics,
  multiple artists per track, M3U export, `/metrics`.

Multi-tenancy is explicitly not a goal; keep it out of the schema.

---

## Summary

| Order | Items | Why here |
|---|---|---|
| 1 | B1, B2, B7, B5 | Data loss, memory exhaustion, a red build |
| 2 | B3, B4, B6, B11, B15 | The documented contract is not yet the whole truth |
| 3 | B8, B9, B10, B13 | Correct metadata and cheaper, debuggable serving |
| 4 | B12, B14, B16, B17, B18 | Debt that makes every later feature cheaper |
| 5 | Part 3 | Safe to build once the foundation stops lying |
