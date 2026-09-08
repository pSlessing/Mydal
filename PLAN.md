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
