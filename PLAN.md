# Mydal development plan

Mydal is a Go backend for a self-hosted music library: Postgres holds the
catalogue, MinIO (S3-compatible) holds the audio files, and an HTTP API exposes
both. The foundations are sound — embedded migrations applied at startup,
validated config, graceful shutdown, request id / logging / recovery
middleware, a `BlobStore` seam, a distroless image, compose and CI. What sits
on top of them is uneven: the artist path was rebuilt to a contract the other
three resources never caught up with, and in two places the code and the schema
disagree outright.

This plan is therefore **defect-driven**. Part 1 is the list of everything
found wrong, Part 2 sequences the fixes, Part 3 keeps the forward roadmap that
resumes once the defects are gone.

Each item names the files it touches so it can be picked up independently.

---

## Current state (September 2026)

- Layered layout `handlers -> service -> repository -> domain`, wired by hand
  in `cmd/server/main.go`. `go build ./...` and `go vet ./...` are clean.
- 16 routes under `/api/v1`, plus Swagger UI at `/swagger/`.
- Five tables across four reversible migrations, embedded and applied at
  startup (`-migrate-only` applies and exits).
- Three tiers of code quality, which is the root of most of Part 1:

  | Tier | Resource | Traits |
  |---|---|---|
  | Current | artists | `ctx` threaded, consumer-side interface, DTOs, UUID validation, typed errors mapped to 400/404/409/500 |
  | Half-converted | tracks | `ctx` and interface present; decodes into the domain struct, plain-text errors, one method still without `ctx` |
  | Pre-refactor | albums, playlists | concrete `*repository.X` dependencies, no `ctx`, no DTOs, `http.Error` text, every failure a 500 or a blanket 404 |

- **No tests exist.** `make test` and the CI test step pass vacuously.

---

## Part 1 — Defects

Severity: **critical** = the endpoint cannot work; **high** = silent data loss
or a misleading contract; **medium** = correctness or maintainability debt.

### D1 — Playlists query a column that does not exist (critical)

`internal/repository/playlistrepo.go` selects, inserts and updates a `song_ids`
array column on `playlists`. Migration `000004` creates no such column:
membership lives in the `playlist_tracks` join table, keyed
`(playlist_id, track_id)` with an ordered `position` and a deferrable
uniqueness constraint. Every one of the five playlist endpoints fails at
runtime with `column "song_ids" does not exist`.

`domain.Playlist.SongIDs []string` encodes the same wrong model, and the
`github.com/lib/pq` dependency exists solely for the `pq.Array` calls here —
the actual driver is `pgx/v5/stdlib`.

Touches: `internal/repository/playlistrepo.go`, `internal/domain/playlist.go`,
`go.mod`.

### D2 — Album fields are accepted and then dropped (high)

`AlbumRepository` only ever touches `id, title, artist_id`:

- `domain.Album.ReleaseYear int` has **no corresponding column** — the schema
  has `release_date DATE`. A client can post a release year, see it echoed back
  in the 201 response, and never find it again.
- `cover_key` is in the schema and in the domain struct but is never read or
  written, so the cover-art work in Part 3 has nowhere to land.
- `albums.created_at` exists in the schema and not in the domain struct.
- `GetAlbumByID` does not map `sql.ErrNoRows` to `domain.ErrNotFound`.

Touches: `internal/domain/album.go`, `internal/repository/albumrepo.go`,
possibly a new migration.

### D3 — Blobs are never deleted, and a failed upload orphans one (high)

- `DeleteTrack` removes the row and leaves the object in MinIO forever. The
  track handler holds a `BlobStore` already; it simply does not use it here.
- Deleting an artist cascades to their tracks in Postgres
  (`ON DELETE CASCADE`), so the rows vanish and every object behind them is
  orphaned with no remaining record of its key.
- In `UploadTrackFile`, if `UpdateStorageKey` fails after `Put` succeeded, the
  object is written and nothing points at it.
- Re-uploading a track with a different content type produces a different
  extension and therefore a different key, orphaning the previous object.

Touches: `internal/api/handlers/trackhandler.go`,
`internal/service/trackservice.go`, `internal/repository/artistrepo.go`.

### D4 — Three different error contracts (high)

`internal/httpx` has exactly the right machinery — `StatusForError` maps the
domain sentinels, `WriteError` logs unclassified errors in full and answers a
generic message so SQL text cannot leak — and only the artist handler uses it.
Elsewhere:

- Track, album and playlist handlers call `http.Error` with plain text, so the
  documented "errors carry a JSON body" contract holds for one resource in four.
- `TrackHandler.DeleteTrack` maps *every* error to 500, discarding the
  `domain.ErrNotFound` the repository correctly returns — deleting a missing
  track answers 500 instead of 404.
- `AlbumHandler.CreateAlbum` and `PlaylistHandler.CreatePlaylist` return
  `err.Error()` straight to the client, which leaks driver and constraint text.
- `GetAlbum` / `GetPlaylist` answer 404 for any error at all, including a
  connection failure.

Touches: all four handlers in `internal/api/handlers/`.

### D5 — Album and playlist layers have no `ctx` and no test seam (medium)

`AlbumService` and `PlaylistService` hold `*repository.AlbumRepository` and
`*repository.PlaylistRepository` concretely, and no method takes a
`context.Context`. Two consequences: a client disconnect cannot cancel the
query, and there is no interface to substitute a fake against, which is
precisely what the unit tests in Milestone 2 need.
`TrackRepository.UpdateStorageKey` is the one track method still missing `ctx`.

Touches: `internal/service/albumservice.go`, `playlistservice.go`,
`trackservice.go`, `internal/repository/trackrepo.go`, `cmd/server/main.go`.

### D6 — Handlers decode straight into domain structs (medium)

`internal/api/handlers/dto.go` states the rule and the artist path follows it;
the track, album and playlist handlers each `json.NewDecoder(r.Body).Decode`
into `domain.Track` / `domain.Album` / `domain.Playlist`. So a client can set
server-owned fields — and `CreateTrack` passes a client-supplied `StorageKey`
into the insert, meaning a caller can point a track row at any object in the
bucket. The response side has the mirror problem: the domain structs carry no
JSON tags, so the wire format is Go field names (`ID`, `ArtistID`,
`CreatedAt`), inconsistent with the artist response's snake_case.

Touches: `internal/api/handlers/dto.go` and the three handlers.

### D7 — No input validation below the artist path (medium)

`pathUUID` exists and only the artist handler calls it. Everywhere else a
non-UUID id reaches Postgres and comes back as an unclassified error mapped to
500 or a misleading 404. There is no validation of non-empty titles, and
creating an album or track under a non-existent `artist_id` raises a foreign
key violation that surfaces as a 500 rather than a 400 or 409.

Touches: the three handlers, and validation in the services.

### D8 — The upload endpoint is fragile (medium)

`PUT /tracks/{id}/file`:

- requires `Content-Length` and rejects chunked transfer encoding outright;
- sets **no maximum body size**, so any client can fill the bucket;
- falls back to an extensionless key for an unrecognised content type, and
  trusts the client's `Content-Type` header rather than sniffing;
- computes no content hash, so there is no dedup and no `ETag` source.

Touches: `internal/api/handlers/trackhandler.go`.

### D9 — Streaming misreports two failures (medium)

`StreamHandler.StreamTrack` maps every `GetTrackByID` error to 404, so a
database outage reads as a missing track. When the row exists but the object
does not, `storage` returns a wrapped `domain.ErrNotFound` that the handler
turns into a 500. Both cases should go through `httpx.WriteError`. The
range/seek path itself is correct — `http.ServeContent` over the seekable MinIO
object handles `Range`, `206`, `If-Range` and `HEAD`.

Touches: `internal/api/handlers/streamhandler.go`.

### D10 — Playlist mutations silently succeed against nothing (medium)

`AddTrack` and `RemoveTrack` issue an `UPDATE` and ignore the row count, so
adding a track to a playlist that does not exist answers 204. `playlists.updated_at`
is in the schema and is never written. Both survive the D1 rewrite unless fixed
with it.

Touches: `internal/repository/playlistrepo.go`.

### D11 — The published OpenAPI spec describes a contract the code does not implement (medium)

The annotations claim `domain.Artist` as both the request body and the response
for the artist endpoints; the real types are `createArtistRequest` and
`artistResponse`. Every handler documents `{object} map[string]string` failures
while three of the four actually answer plain text. This spec is published to
GitHub Pages on every push to `main`, so the wrong contract is the public one.

Touches: annotations in all handlers, then `make swagger`.

### D12 — No tests, no health endpoints (medium)

Zero `_test.go` files. `make test` and CI's test step therefore prove nothing,
and every fix above lands unverified. There are no `/healthz` or `/readyz`
endpoints, so compose's `restart: unless-stopped` and any orchestrator have no
signal beyond "the process is alive".

### D13 — No authentication (known)

Every endpoint is unauthenticated. The README says so and the constraint is
accepted for now; it is recorded here because it gates any deployment beyond a
trusted network, and because retrofitting it after the Subsonic layer would be
much worse than before.

---

## Part 2 — Sequenced fixes

### Milestone 1 — A harness, then the two schema lies

Nothing else should be touched until a broken repository can fail a test.

1. **Repository integration harness.** `testcontainers-go` for Postgres and
   MinIO (or the compose stack behind a `-short` guard), applying the embedded
   migrations per run. This is what catches D1 and D2 permanently: both are
   defects that only a query against the real schema can see.
2. **Fix D1.** Rewrite `PlaylistRepository` against `playlist_tracks`: insert
   with `position = COALESCE(MAX(position)+1, 0)`, read membership with a join
   ordered by position, delete and renumber inside a transaction (the
   uniqueness constraint is deferrable precisely so a reorder can). Replace
   `domain.Playlist.SongIDs` with an ordered slice loaded from the join, and
   drop `github.com/lib/pq` from `go.mod`.
3. **Fix D10** in the same rewrite: check `RowsAffected` and return
   `domain.ErrNotFound`; touch `updated_at` on every mutation.
4. **Fix D2.** Decide the album shape — recommended: keep `release_date DATE`
   and give the domain a nullable `ReleaseDate`, rather than migrating the
   column down to a year and losing precision. Select and insert every column,
   add `CreatedAt`, and map `sql.ErrNoRows` to `domain.ErrNotFound`.

Exit criteria: every playlist and album endpoint answers correctly against a
real Postgres, proven by tests that fail if the schema and the queries drift
apart again.

### Milestone 2 — One contract, everywhere

Bring tracks, albums and playlists up to the artist path.

1. **D5:** thread `ctx` through both services and both repositories, declare
   consumer-side interfaces (`AlbumService`, `PlaylistService`,
   `AlbumRepository`, `PlaylistRepository`) next to their consumers, and add
   `ctx` to `UpdateStorageKey`. Rewire `main.go`.
2. **D4:** replace every `http.Error` with `httpx.WriteError`, and make each
   repository return the sentinels — `ErrNotFound` from `RowsAffected == 0` and
   `sql.ErrNoRows`, `ErrConflict` from a unique violation, `ErrInvalidInput`
   from a foreign key violation (`pgconn.PgError` codes `23505` and `23503`).
3. **D6:** request and response DTOs for tracks, albums and playlists in
   `dto.go`, snake_case like `artistResponse`, with server-owned fields
   unsettable from the wire.
4. **D7:** `pathUUID` on every id parameter; non-empty title validation in the
   services returning `domain.ErrInvalidInput`.
5. **Unit tests** for each handler against fakes, now that the interfaces from
   step 1 exist.

Exit criteria: the README's error paragraph — 400 / 404 / 409 / 500 with a JSON
body — is true of every endpoint, and the caveat about tracks, albums and
playlists can be deleted.

### Milestone 3 — Make the storage layer honest

1. **D3:** delete the object when a track is deleted; delete the object on
   upload failure; delete the previous object when a track's file is replaced.
   For artist deletion, collect the affected storage keys inside the
   transaction *before* the cascade, then remove the objects — and accept that
   a crash between the two leaves an orphan, which is why the sweep below
   matters more than the cleanup path.
2. **Orphan sweep** as a CLI subcommand (`-gc`, alongside `-migrate-only`):
   objects with no row, rows with no object. This is the durable answer;
   cleanup-on-error is only the fast path.
3. **D8:** cap the request body with `http.MaxBytesReader`, accept a streaming
   upload without `Content-Length` by passing `-1` to `Put` (the `BlobStore`
   interface already documents that), sniff the type from the first 512 bytes
   rather than trusting the header, and hash with `io.TeeReader` into a new
   `tracks.content_hash` column with a unique index.
4. **D9:** route both stream failures through `httpx.WriteError`, and set
   `ETag` from the content hash once step 3 provides one.

Exit criteria: deleting everything in the library leaves an empty bucket, and
the sweep reports zero orphans.

### Milestone 4 — Observability and a true spec

1. **D12:** `/healthz` (process up) and `/readyz` (DB ping plus bucket
   reachable), wired into compose's healthcheck for the app service.
2. **D11:** correct every annotation to the DTOs that Milestone 2 introduced,
   regenerate with `make swagger`, and confirm the GitHub Pages spec matches
   the implementation.
3. Raise CI beyond vacuous: fail the build under a coverage floor, and run the
   repository integration tests from Milestone 1 against the existing service
   containers.

Exit criteria: `make test` proves something, and the published spec can be
handed to a client generator without surprises.

---

## Part 3 — Once the defects are gone

The original roadmap, compressed. None of it should start before Milestone 2.

- **Complete the CRUD.** List endpoints for every resource with keyset
  pagination on `(created_at, id)` from day one, `PATCH` for metadata edits,
  `GET /artists/{id}/albums`, `GET /albums/{id}` with its tracks in order, and
  playlist reorder as `PUT /playlists/{id}/tracks` taking the full ordered list.
- **Upload pipeline.** `POST /tracks/upload` with `multipart/form-data`
  streamed straight to the blob store; tag extraction with
  `github.com/dhowden/tag` plus a duration/bitrate probe behind an interface
  (`ffprobe` first, pure-Go decoders kept open); find-or-create artist and
  album inside one transaction, guarded by unique indexes on `artists(lower(name))`
  and `albums(artist_id, lower(title))`; cover art to `covers/{album_id}`.
  Milestone 3's hashing already provides the dedup key.
- **Filesystem scanner.** Point Mydal at `MUSIC_DIR`, walk it and reuse the
  upload pipeline through a local-filesystem `BlobStore` that references files
  in place. For most self-hosters this is the *primary* ingestion path and may
  deserve to come before the HTTP upload.
- **Serving.** `GET /tracks/{id}/download` with `Content-Disposition`,
  `GET /albums/{id}/cover` and `GET /artists/{id}/image` with cache headers,
  and presigned-URL redirects behind a config flag for deployments that prefer
  MinIO to serve the bytes.
- **Search and browse.** `GET /search?q=` over a `pg_trgm` GIN index, sort and
  filter parameters on the list endpoints, a `genre` column from tags, and
  `GET /stats`.
- **Authentication (D13).** A single admin API key first, then multi-user with
  sessions. Everything before this is trusted-network-only.
- **OpenSubsonic compatibility layer** in its own `internal/subsonic` package,
  adapting the existing services. The highest-leverage feature in the whole
  document: dozens of mature clients work the moment it lands. It must come
  after auth, since the protocol carries credentials on every request.
- Then, by preference: transcoding and ReplayGain, play history and
  favourites, smart playlists, an embedded web UI, MusicBrainz lookup, lyrics,
  multiple artists per track, M3U/JSON export, metrics at `/metrics`.

Multi-tenancy is explicitly not a goal; keep it out of the schema.

---

## Sequencing summary

| Order | Work | Why here |
|---|---|---|
| 1 | Milestone 1 — harness, D1, D2, D10 | Two endpoints are broken against their own schema, and nothing is provable without the harness |
| 2 | Milestone 2 — D4–D7 | One contract before any new surface is built on the old one |
| 3 | Milestone 3 — D3, D8, D9 | Stops the library leaking storage before it holds anything worth keeping |
| 4 | Milestone 4 — D11, D12 | Makes the CI signal and the public spec mean something |
| 5 | Part 3 features | Safe to build once the foundation stops lying |
