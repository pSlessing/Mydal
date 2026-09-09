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

A second pass on 2026-09-09 re-read every source and test file against the
fixed tree, with `go build`, `go vet`, `go test -short` and
`golangci-lint run` (v2.13.2) all green. It found nine defects the first
audit missed, recorded below as B20 to B28. None is a red build; two are
concurrency holes the single-threaded tests cannot see, one is a data-loss
path in the maintenance command, and the rest are contract and hygiene
gaps. The integration tests were again not run locally.

B20 and B21 have since been fixed and pinned; B22 to B28 remain, alongside
B19.

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

### B20 — `mydal -gc` could delete the object an in-flight upload was about to commit (high, fixed)

`runOrphanSweep` (`main.go:195`) reads every `storage_key` first, then
`gc.Sweep` lists the bucket (`gc.go:41`). An upload whose `Put` lands
between those two calls is in the listing but not in the key set, so the
sweep deletes it; the upload's `SetTrackFile` then commits a row that points
at nothing. Swapping the order narrows the window but does not close it: an
object written before the listing whose row commits after the key read is
still an orphan by the sweep's definition. The README presents `-gc` as a
routine maintenance command and says nothing about stopping the server
first, and `MinIOStore.List` (`minio.go:120`) throws away the
`LastModified` that would let the sweep tell a fresh object from a stale
one.

Two smaller holes in the same code: the sweep compares the bucket against
`tracks.storage_key` only, so the day `albums.cover_key` gets an upload path
(the repository already round-trips the column) every cover object is an
orphan to it; and objects are the only thing it looks at, see B27 for the
multipart parts it cannot see.

Fixed. `BlobStore.List` returns `[]ObjectInfo`, so the sweep sees
`LastModified`. `gc.Sweep` now takes the catalogue read as a callback
(`ReferencedKeys`) and calls it *after* listing the bucket, which puts the
ordering in the sweep rather than in its caller, and skips any unreferenced
object younger than a grace window - `gcGracePeriod`, one hour, against the
15 minutes of `uploadReadTimeout` - reporting those as `Report.Skipped`.
`runOrphanSweep` feeds `albums.cover_key` (new `AlbumRepository.AllCoverKeys`)
into the referenced set alongside `tracks.storage_key`.

Flipping the order opened the mirror case - a key that commits after the
listing names an object the listing could not have seen - so a referenced key
absent from the listing is now `Stat`ed before being reported as missing.

Pinned by four tests in `gc_test.go`: a fresh unreferenced object is skipped
and not deleted; a row that commits between the listing and the key read is
not swept; a key whose object appears after the listing is not reported
missing; a cover key keeps its object. The README now says `-gc` is safe
against a live server, and why.

Still open from this entry: B27, the incomplete multipart uploads no listing
can see.

### B21 — Playlist positions lost density under concurrent mutation (medium, fixed)

B13 closed the single-threaded gap. Under concurrency it is open again:
`DeleteTrack` (`trackrepo.go:98`) reads `(playlist_id, position)` with a
plain SELECT and never takes the `playlists` row lock that `touchPlaylist`
(`playlistrepo.go:144`) uses to serialise `AddTrack` and `RemoveTrack`.
Three interleavings each leave a gap:

- `DeleteTrack` against `RemoveTrack` on the same playlist: the delete's
  position is stale by the time its renumber runs, so the rows past the
  removed one are shifted from the wrong offset.
- `DeleteTrack` against `AddTrack`: the append computes `MAX+1` before the
  renumber commits and lands one past the end.
- Two `DeleteTrack`s of tracks in the same playlist: the second waits on
  the first's row locks, then renumbers from the position it read before
  the wait.

The deferrable unique constraint makes none of this an error, so the
invariant silently decays until the reorder endpoint in Part 3 assumes it.
Related: a catalogue delete changes a playlist's contents without touching
its `updated_at`, so a client using that stamp as a change marker misses it.

Fixed. `DeleteTrack` opens with one statement that both takes the lock and
bumps the stamp: `UPDATE playlists SET updated_at = now() WHERE id IN (SELECT
id FROM playlists WHERE id IN (<the track's playlists>) ORDER BY id FOR
UPDATE)`. The inner ordered `FOR UPDATE` rules out a deadlock with a
concurrent delete whose track sits in the same playlists in another order, and
the lock is the same `playlists` row lock `touchPlaylist` takes, so all three
interleavings above now serialise. The `updated_at` bump closes the related
gap in the same statement.

Pinned by two integration tests in `playlistrepo_test.go`: a 20-round race
harness running `DeleteTrack`, `AddTrack` and a second `DeleteTrack` against
overlapping playlists, asserting positions are exactly `0..n-1` after every
round; and one asserting `updated_at` moves when a member is deleted from the
catalogue. Both fail against the previous `DeleteTrack` (positions `[0 2]`, an
unchanged stamp) and pass under `-race`.

### B22 — A keyword/value `DATABASE_URL` is corrupted, and its password logged (medium)

pgx accepts both `postgres://...` and `host=... user=... password=...`, and
the README asks only for "a Postgres DSN". `config.Load` (`config.go:140`)
runs the keyword form through `url.Parse`, which does not fail on it, then
re-encodes it as `host=localhost%20user=...?sslmode=disable` (checked with
the standard library). Outside production the server therefore cannot
connect to a database given in that form, with an error that blames the
DSN rather than the rewrite. `redactURL` (`config.go:50`) has the same
blind spot: with no userinfo to redact, the password is logged in clear by
`logger.Debug("Configuration loaded", ...)`.

Adjacent, and worth fixing in the same pass: `MODE` is never validated.
`prod`, `Production` or a stray space all mean "not production" and force
`sslmode=disable` onto a DSN that named no mode - the opposite of what the
operator meant, and silent.

Fix: parse with `pgconn.ParseConfig` (already a dependency), which
understands both forms; apply the sslmode default only when the parsed
config has none, and render the redacted form from the parsed struct
instead of guessing at the string. Reject any `MODE` other than
`development` or `production`. Test: a keyword DSN round-trips unchanged
apart from the added `sslmode`, and its password never appears in
`LogValue`.

### B23 — A `urn:uuid:` id passes validation and reaches Postgres as a 500 (low)

`pathUUID` (`artisthandler.go:33`) and `requireUUID` (`validate.go:23`)
accept whatever `uuid.Parse` accepts, which includes the
`urn:uuid:xxxxxxxx-...` form. Postgres accepts braces and hyphen-less hex
but not the URN prefix, so `GET /tracks/urn:uuid:<id>` is SQLSTATE 22P02,
which `classify` does not know, and so a 500 with an error log - the exact
outcome those two validators exist to prevent.

Fix: both return `parsed.String()` and callers use the canonical form, which
also normalises the brace and hyphen-less spellings before they reach a
query or an error message. Test: the URN form in `contract_test.go` and
`validate_test.go` is either a 404 or a 400, never a 500.

### B24 — Concurrent re-uploads to one track orphan an object silently (low)

Two `PUT /tracks/{id}/file` requests for the same track both read the same
`previousKey` (`trackhandler.go:122`). Each writes a fresh object; the first
`SetTrackFile` wins and deletes the previous key; the second wins too (same
row, so the hash index does not object), deletes the already-gone previous
key, and leaves the first request's object with no row and no log line.
Only B20's sweep would ever find it, and B20 says that sweep is not yet safe
to run.

Fix: `SetTrackFile` returns the key it displaced (`UPDATE ... RETURNING
(SELECT storage_key FROM tracks WHERE id = $5)`, evaluated before the
assignment) and the handler deletes that instead of what it read earlier.
Test: two uploads to one track in parallel leave exactly one object in the
bucket.

### B25 — Client faults on the write paths answer with the wrong status (low)

Three cases, each a client mistake that comes back as something else:

- `decodeJSON` (`decode.go:26`) folds `MaxBytesError` into "malformed JSON
  body", a 400, while the upload endpoint answers 413 for the same fault.
- An upload whose body ends before its declared `Content-Length` fails
  inside minio-go with an unexpected EOF, which `trackhandler.go:187`
  reports as a 500 and logs as an error.
- `CreatePlaylist` with the same track twice in `track_ids` trips the
  membership primary key, so the client sees a 409 whose message names
  `playlist_tracks_pkey`, for what is a malformed request.

Fix: check for `MaxBytesError` in `decodeJSON` and answer 413; treat an
`io.ErrUnexpectedEOF` from `Put` as a 400; reject duplicate `track_ids` in
`PlaylistService.CreatePlaylist` before the INSERT. Pin each.

### B26 — A client-supplied `X-Request-Id` is trusted verbatim (low)

`RequestID` (`middleware.go:52`) echoes whatever the header holds into the
response and into every log line for the request: any bytes, any length up
to the server's 1 MiB header cap. Since the log is JSON the damage is bulk
and noise rather than injection, but a correlation id should not be an
attacker-sized payload.

Fix: accept it only if it is short (128 bytes is plenty) and drawn from a
safe set (`[A-Za-z0-9._-]`); otherwise generate one as if it were absent.
Test: an oversized or hostile header gets a generated id back.

### B27 — Incomplete multipart uploads are never reclaimed (low)

minio-go aborts a multipart upload when `Put` fails, so a client disconnect
is clean. A process that dies mid-upload is not: the parts stay in the
bucket, invisible to `ListObjects` and therefore to `-gc`, and count against
storage forever. That is not a remote case here. `serve` (`main.go:284`)
gives shutdown 30 seconds while an upload may legitimately run for the 15
minutes of `uploadReadTimeout`; `http.Server.Shutdown` does not cancel
handler contexts, so a SIGTERM during a large upload always ends in a
timeout, an exit with error, and an abandoned multipart upload. The
orchestrator's SIGKILL is the same story without the log line.

Fix: have `-gc` also list incomplete multipart uploads older than the B20
grace window and abort them (`ListIncompleteUploads` plus
`AbortMultipartUpload`, or a bucket lifecycle rule set once in
`ensureBucket`). Consider a `BaseContext` on the server that is cancelled
at shutdown so an in-flight upload fails fast and minio-go's own abort runs.

### B28 — `cover_key` is on the wire while `storage_key` is deliberately not (low)

`dto.go:111` refuses `cover_key` on input "for the same reason as
StorageKey: it names an object in the bucket", and B15 replaced
`storage_key` in the track response with `has_file` for that reason. The
album response (`dto.go:132`) still exposes `cover_key`, and the
snake-case test pins it. One convention, please, before the cover upload
in Part 3 makes it a real value.

Fix: `has_cover` bool in `albumResponse`; update `dto_test.go`.

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

### Milestone 5 — Second-pass defects

1. ~~B20: list before keys, `LastModified` through `List`, a grace window,
   `cover_key` in the referenced set.~~ Done; `-gc` is safe against a live
   server and the README says so.
2. B22: parse the DSN with pgconn, validate `MODE`.
3. ~~B21: lock the playlist rows in `DeleteTrack`, bump `updated_at`, race
   test.~~ Done.
4. B23, B24, B25: canonical ids, displaced key from `SetTrackFile`, honest
   client-fault statuses.
5. B26, B27, B28: bounded request ids, multipart cleanup and a cancellable
   server context, `has_cover`.

Exit: `-gc` run against a server mid-upload deletes nothing it should not;
a keyword DSN works in development and never logs its password; the
playlist race test passes with `-race`; no input that passes validation can
produce a 500.

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
| 5 | ~~B20~~, B22, ~~B21~~ | Found on the second pass: a data-loss path in `-gc` (fixed), a broken DSN form, a concurrency hole (fixed) |
| 6 | B23, B24, B25, B26, B27, B28 | Second-pass contract and hygiene gaps |
| 7 | Part 3 | Safe to build once the foundation stops lying |
