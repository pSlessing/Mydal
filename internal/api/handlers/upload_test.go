package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/repository"
	"mydal/internal/service"
	"mydal/internal/storage"
	"mydal/internal/testutil"
)

const testUploadCap = 4096

func flacBody(payload string) []byte { return append([]byte("fLaC"), payload...) }

type uploadFixture struct {
	handler   *TrackHandler
	blobs     storage.BlobStore
	trackRepo *repository.TrackRepository
	artistID  string
	ctx       context.Context
}

func newUploadFixture(t *testing.T) uploadFixture {
	t.Helper()
	db := testutil.DB(t)
	blobs := testutil.Blobs(t)
	quiet := testutil.Quiet()
	ctx := context.Background()

	trackRepo := repository.NewTrackRepository(db, quiet)
	artist := &domain.Artist{Name: "Artist"}
	if err := repository.NewArtistRepository(db, quiet).CreateArtist(ctx, artist); err != nil {
		t.Fatal(err)
	}
	return uploadFixture{
		handler:   NewTrackHandler(service.NewTrackService(trackRepo, blobs, quiet), blobs, testUploadCap, quiet),
		blobs:     blobs,
		trackRepo: trackRepo,
		artistID:  artist.ID,
		ctx:       ctx,
	}
}

func (f uploadFixture) newTrack(t *testing.T) string {
	t.Helper()
	tr := &domain.Track{Title: "T", ArtistID: f.artistID}
	if err := f.trackRepo.CreateTrack(f.ctx, tr); err != nil {
		t.Fatal(err)
	}
	return tr.ID
}

// chunked drops Content-Length the way a streaming client does.
func (f uploadFixture) upload(id, header string, payload []byte, chunked bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/tracks/"+id+"/file", bytes.NewReader(payload))
	if header != "" {
		req.Header.Set("Content-Type", header)
	}
	if chunked {
		req.ContentLength = -1
	}
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	f.handler.UploadTrackFile(rec, req)
	return rec
}

func (f uploadFixture) stored(t *testing.T, id string) *domain.Track {
	t.Helper()
	tr, err := f.trackRepo.GetTrackByID(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// The endpoint required Content-Length and so rejected chunked transfer
// encoding outright.
func TestUploadAcceptsChunkedBodies(t *testing.T) {
	f := newUploadFixture(t)
	id := f.newTrack(t)
	body := flacBody("streamed-audio")

	if rec := f.upload(id, "audio/flac", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("chunked upload = %d, want 204: %s", rec.Code, rec.Body)
	}
	tr := f.stored(t, id)
	if tr.StorageKey != "tracks/"+id+".flac" {
		t.Fatalf("key = %q", tr.StorageKey)
	}
	sum := sha256.Sum256(body)
	if tr.ContentHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("content hash = %q, want %s", tr.ContentHash, hex.EncodeToString(sum[:]))
	}
}

// The endpoint trusted the client's Content-Type, which chose both the object
// key and the type served back later.
func TestUploadIgnoresTheClientContentType(t *testing.T) {
	f := newUploadFixture(t)
	id := f.newTrack(t)

	if rec := f.upload(id, "audio/mpeg", flacBody("really-flac"), false); rec.Code != http.StatusNoContent {
		t.Fatalf("upload = %d: %s", rec.Code, rec.Body)
	}
	if got := f.stored(t, id).StorageKey; got != "tracks/"+id+".flac" {
		t.Fatalf("the header beat the sniffer: %q", got)
	}
	info, err := f.blobs.Stat(f.ctx, "tracks/"+id+".flac")
	if err != nil {
		t.Fatal(err)
	}
	if info.ContentType != "audio/flac" {
		t.Fatalf("stored content type = %q, want audio/flac", info.ContentType)
	}
}

// An unrecognised type used to fall back to an extensionless key rather than
// being refused.
func TestUploadRejectsNonAudio(t *testing.T) {
	f := newUploadFixture(t)
	id := f.newTrack(t)

	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"prose", []byte("this is not audio at all, merely text")},
		{"empty", nil},
	} {
		rec := f.upload(id, "audio/flac", tc.body, false)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400: %s", tc.name, rec.Code, rec.Body)
		}
		if got := f.stored(t, id).StorageKey; got != "" {
			t.Errorf("%s still recorded a key: %q", tc.name, got)
		}
	}
}

// The endpoint set no maximum body size, so any client could fill the bucket.
func TestUploadEnforcesTheBodyCap(t *testing.T) {
	f := newUploadFixture(t)
	id := f.newTrack(t)
	oversized := flacBody(strings.Repeat("A", testUploadCap*2))

	for _, chunked := range []bool{false, true} {
		rec := f.upload(id, "audio/flac", oversized, chunked)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("chunked=%v: %d, want 413: %s", chunked, rec.Code, rec.Body)
		}
	}
	if got := f.stored(t, id).StorageKey; got != "" {
		t.Fatalf("an oversized upload recorded a key: %q", got)
	}
}

// Re-uploading with a different content type produced a different extension
// and so a different key, orphaning the previous object.
func TestReuploadDoesNotOrphanThePreviousObject(t *testing.T) {
	f := newUploadFixture(t)
	id := f.newTrack(t)

	if rec := f.upload(id, "", append([]byte("ID3\x04\x00"), "mp3-audio"...), false); rec.Code != http.StatusNoContent {
		t.Fatalf("first upload = %d: %s", rec.Code, rec.Body)
	}
	if rec := f.upload(id, "", flacBody("flac-audio"), false); rec.Code != http.StatusNoContent {
		t.Fatalf("re-upload = %d: %s", rec.Code, rec.Body)
	}
	if !objectPresent(t, f.blobs, "tracks/"+id+".flac") {
		t.Fatal("the re-upload did not store the new object")
	}
	if objectPresent(t, f.blobs, "tracks/"+id+".mp3") {
		t.Fatal("the replaced object was orphaned")
	}

	// A same-format re-upload overwrites in place and must not delete what it
	// just wrote.
	if rec := f.upload(id, "", flacBody("flac-audio-2"), false); rec.Code != http.StatusNoContent {
		t.Fatalf("same-format re-upload = %d: %s", rec.Code, rec.Body)
	}
	if !objectPresent(t, f.blobs, "tracks/"+id+".flac") {
		t.Fatal("a same-format re-upload deleted the object it had written")
	}
}

// The content hash is unique, so the same audio under a second track is a
// conflict - and the redundant object must not be left behind.
func TestDuplicateAudioIsRejectedAndLeavesNoOrphan(t *testing.T) {
	f := newUploadFixture(t)
	first, second := f.newTrack(t), f.newTrack(t)
	audio := flacBody("identical-audio")

	if rec := f.upload(first, "", audio, false); rec.Code != http.StatusNoContent {
		t.Fatalf("first upload = %d: %s", rec.Code, rec.Body)
	}
	rec := f.upload(second, "", audio, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate audio = %d, want 409: %s", rec.Code, rec.Body)
	}
	if objectPresent(t, f.blobs, "tracks/"+second+".flac") {
		t.Fatal("the rejected duplicate left its object behind")
	}
	if f.stored(t, first).StorageKey == "" {
		t.Fatal("the original upload was disturbed")
	}
}

func objectPresent(t *testing.T, blobs storage.BlobStore, key string) bool {
	t.Helper()
	_, err := blobs.Stat(context.Background(), key)
	return err == nil
}
