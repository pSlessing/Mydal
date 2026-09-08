package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/repository"
	"mydal/internal/service"
	"mydal/internal/storage"
	"mydal/internal/testutil"
)

// countingBlobs wraps a real BlobStore and counts calls to Get and Stat, so a
// test can check how many round trips to the store a request actually made.
type countingBlobs struct {
	storage.BlobStore
	gets, stats int
}

func (c *countingBlobs) Get(ctx context.Context, key string) (io.ReadSeekCloser, storage.ObjectInfo, error) {
	c.gets++
	return c.BlobStore.Get(ctx, key)
}

func (c *countingBlobs) Stat(ctx context.Context, key string) (storage.ObjectInfo, error) {
	c.stats++
	return c.BlobStore.Stat(ctx, key)
}

// StreamTrack mapped every GetTrackByID error to 404, so a database outage
// read as a missing track; and when the row existed but the object did not,
// the wrapped ErrNotFound became a 500.
func TestStreamClassifiesItsFailures(t *testing.T) {
	db := testutil.DB(t)
	blobs := testutil.Blobs(t)
	ctx := context.Background()
	quiet := testutil.Quiet()

	trackRepo := repository.NewTrackRepository(db)
	h := NewStreamHandler(service.NewTrackService(trackRepo, blobs, quiet), blobs, quiet)
	artist := &domain.Artist{Name: "Artist"}
	if err := repository.NewArtistRepository(db).CreateArtist(ctx, artist); err != nil {
		t.Fatal(err)
	}

	get := func(id string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/tracks/"+id+"/stream", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h.StreamTrack(rec, req)
		return rec
	}

	// A row whose object is gone, and a row with no upload at all.
	orphan := &domain.Track{Title: "Orphan", ArtistID: artist.ID, StorageKey: "tracks/vanished.flac"}
	if err := trackRepo.CreateTrack(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	bare := &domain.Track{Title: "Bare", ArtistID: artist.ID}
	if err := trackRepo.CreateTrack(ctx, bare); err != nil {
		t.Fatal(err)
	}

	assertJSONError(t, "missing track", get("00000000-0000-0000-0000-000000000000", nil), http.StatusNotFound)
	assertJSONError(t, "object gone", get(orphan.ID, nil), http.StatusNotFound)
	assertJSONError(t, "no file uploaded", get(bare.ID, nil), http.StatusNotFound)
	assertJSONError(t, "malformed id", get("not-a-uuid", nil), http.StatusBadRequest)

	// A fully uploaded track, for the caching and range paths.
	audio := append([]byte("fLaC"), bytes.Repeat([]byte("abcdefgh"), 64)...)
	sum := sha256.Sum256(audio)
	hash := hex.EncodeToString(sum[:])
	tr := &domain.Track{Title: "T", ArtistID: artist.ID, StorageKey: "tracks/ok.flac"}
	if err := trackRepo.CreateTrack(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if err := blobs.Put(ctx, "tracks/ok.flac", bytes.NewReader(audio), int64(len(audio)), "audio/flac"); err != nil {
		t.Fatal(err)
	}
	if err := trackRepo.SetTrackFile(ctx, tr.ID, "tracks/ok.flac", hash, "flac", int64(len(audio))); err != nil {
		t.Fatal(err)
	}

	rec := get(tr.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream = %d: %s", rec.Code, rec.Body)
	}
	if !bytes.Equal(rec.Body.Bytes(), audio) {
		t.Fatalf("streamed %d bytes, want %d", rec.Body.Len(), len(audio))
	}
	if rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Error("Accept-Ranges not advertised")
	}
	if got := rec.Header().Get("ETag"); got != strconv.Quote(hash) {
		t.Fatalf("ETag = %q, want %q", got, strconv.Quote(hash))
	}

	// The ETag is a real validator, not decoration.
	if rec = get(tr.ID, map[string]string{"If-None-Match": strconv.Quote(hash)}); rec.Code != http.StatusNotModified {
		t.Errorf("If-None-Match = %d, want 304", rec.Code)
	} else if rec.Body.Len() != 0 {
		t.Errorf("304 carried %d bytes", rec.Body.Len())
	}
	if rec = get(tr.ID, map[string]string{"If-None-Match": `"stale"`}); rec.Code != http.StatusOK {
		t.Errorf("stale If-None-Match = %d, want 200", rec.Code)
	}

	// Ranges, which http.ServeContent handles over the seekable object.
	rec = get(tr.ID, map[string]string{"Range": "bytes=4-11"})
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range = %d, want 206", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), audio[4:12]) {
		t.Errorf("range body = %q", rec.Body.Bytes())
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 4-11/"+strconv.Itoa(len(audio)) {
		t.Errorf("Content-Range = %q", cr)
	}
	if rec = get(tr.ID, map[string]string{"Range": "bytes=0-7", "If-Range": strconv.Quote(hash)}); rec.Code != http.StatusPartialContent {
		t.Errorf("fresh If-Range = %d, want 206", rec.Code)
	}
	if rec = get(tr.ID, map[string]string{"Range": "bytes=0-7", "If-Range": `"stale"`}); rec.Code != http.StatusOK {
		t.Errorf("stale If-Range = %d, want a full 200", rec.Code)
	}
}

// The handler used to Stat the object for its metadata, then Get it for the
// body - two round trips to the store before the first byte went out. Get now
// returns the metadata itself, so streaming should reach the store exactly
// once.
func TestStreamMakesOneRoundTripToTheStore(t *testing.T) {
	db := testutil.DB(t)
	blobs := &countingBlobs{BlobStore: testutil.Blobs(t)}
	ctx := context.Background()
	quiet := testutil.Quiet()

	trackRepo := repository.NewTrackRepository(db)
	artist := &domain.Artist{Name: "Artist"}
	if err := repository.NewArtistRepository(db).CreateArtist(ctx, artist); err != nil {
		t.Fatal(err)
	}
	audio := append([]byte("fLaC"), []byte("some-audio-bytes")...)
	tr := &domain.Track{Title: "T", ArtistID: artist.ID}
	if err := trackRepo.CreateTrack(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if err := blobs.Put(ctx, "tracks/ok.flac", bytes.NewReader(audio), int64(len(audio)), "audio/flac"); err != nil {
		t.Fatal(err)
	}
	if err := trackRepo.SetTrackFile(ctx, tr.ID, "tracks/ok.flac", "somehash", "flac", int64(len(audio))); err != nil {
		t.Fatal(err)
	}

	h := NewStreamHandler(service.NewTrackService(trackRepo, blobs, quiet), blobs, quiet)
	req := httptest.NewRequest(http.MethodGet, "/tracks/"+tr.ID+"/stream", nil)
	req.SetPathValue("id", tr.ID)
	rec := httptest.NewRecorder()
	h.StreamTrack(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("stream = %d: %s", rec.Code, rec.Body)
	}
	if blobs.gets != 1 {
		t.Errorf("Get called %d times, want 1", blobs.gets)
	}
	if blobs.stats != 0 {
		t.Errorf("Stat called %d times, want 0 - streaming should get everything it needs from Get", blobs.stats)
	}
}
