package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"

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
	client    *minio.Client
	bucket    string
	trackRepo *repository.TrackRepository
	artistID  string
	ctx       context.Context
}

func newUploadFixture(t *testing.T) uploadFixture {
	t.Helper()
	db := testutil.DB(t)
	blobs, client, bucket := testutil.BlobsWithClient(t)
	quiet := testutil.Quiet()
	ctx := context.Background()

	trackRepo := repository.NewTrackRepository(db)
	artist := &domain.Artist{Name: "Artist"}
	if err := repository.NewArtistRepository(db).CreateArtist(ctx, artist); err != nil {
		t.Fatal(err)
	}
	return uploadFixture{
		handler:   NewTrackHandler(service.NewTrackService(trackRepo, blobs, quiet), blobs, testUploadCap, quiet),
		blobs:     blobs,
		client:    client,
		bucket:    bucket,
		trackRepo: trackRepo,
		artistID:  artist.ID,
		ctx:       ctx,
	}
}

// keys lists every object currently in the fixture's bucket, for assertions
// about orphans that can't be checked by guessing a key.
func (f uploadFixture) keys(t *testing.T) []string {
	t.Helper()
	return testutil.Keys(t, f.client, f.bucket)
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
	if !strings.HasPrefix(tr.StorageKey, "tracks/"+id+"/") || !strings.HasSuffix(tr.StorageKey, ".flac") {
		t.Fatalf("key = %q, want tracks/%s/<random>.flac", tr.StorageKey, id)
	}
	sum := sha256.Sum256(body)
	if tr.ContentHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("content hash = %q, want %s", tr.ContentHash, hex.EncodeToString(sum[:]))
	}
}

// format and file_size used to stay whatever the client claimed at creation,
// even after the real bytes were uploaded and sniffed - so the catalogue
// could say "mp3" for a file that was actually a FLAC. The upload now
// overwrites both with what it actually sniffed and counted.
func TestUploadReconcilesFormatAndFileSize(t *testing.T) {
	f := newUploadFixture(t)
	tr := &domain.Track{
		Title: "T", ArtistID: f.artistID,
		Format: "mp3", FileSize: 999999,
	}
	if err := f.trackRepo.CreateTrack(f.ctx, tr); err != nil {
		t.Fatal(err)
	}
	body := flacBody("this-is-actually-a-flac")

	if rec := f.upload(tr.ID, "", body, false); rec.Code != http.StatusNoContent {
		t.Fatalf("upload = %d: %s", rec.Code, rec.Body)
	}
	got := f.stored(t, tr.ID)
	if got.Format != "flac" {
		t.Errorf("format = %q, want flac (the sniffed format, not the client's claim)", got.Format)
	}
	if got.FileSize != int64(len(body)) {
		t.Errorf("file_size = %d, want %d (the uploaded byte count, not the client's claim)", got.FileSize, len(body))
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
	key := f.stored(t, id).StorageKey
	if !strings.HasSuffix(key, ".flac") {
		t.Fatalf("the header beat the sniffer: %q", key)
	}
	info, err := f.blobs.Stat(f.ctx, key)
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
	mp3Key := f.stored(t, id).StorageKey

	if rec := f.upload(id, "", flacBody("flac-audio"), false); rec.Code != http.StatusNoContent {
		t.Fatalf("re-upload = %d: %s", rec.Code, rec.Body)
	}
	flacKey := f.stored(t, id).StorageKey
	if !objectPresent(t, f.blobs, flacKey) {
		t.Fatal("the re-upload did not store the new object")
	}
	if objectPresent(t, f.blobs, mp3Key) {
		t.Fatal("the replaced object was orphaned")
	}

	// A same-format re-upload used to overwrite the object in place (same id,
	// same extension -> same key) and then delete it if the write failed. It
	// must now land under a fresh key instead.
	if rec := f.upload(id, "", flacBody("flac-audio-2"), false); rec.Code != http.StatusNoContent {
		t.Fatalf("same-format re-upload = %d: %s", rec.Code, rec.Body)
	}
	flacKey2 := f.stored(t, id).StorageKey
	if flacKey2 == flacKey {
		t.Fatal("a same-format re-upload reused the previous object's key")
	}
	if !objectPresent(t, f.blobs, flacKey2) {
		t.Fatal("a same-format re-upload did not store the new object")
	}
	if objectPresent(t, f.blobs, flacKey) {
		t.Fatal("a same-format re-upload orphaned the previous object")
	}
}

// This is the exact sequence B1 described: track A holds a FLAC, track B
// holds a different FLAC, and B's bytes are uploaded onto A. Because A and B
// share a format, the old code wrote both to the same key, so MinIO already
// held B's bytes under A's name by the time the content-hash dedup index
// rejected the write - and the handler's cleanup then deleted that key as
// "nothing points at it", taking A's live audio with it.
func TestSameFormatReuploadSurvivesADedupConflict(t *testing.T) {
	f := newUploadFixture(t)
	a, b := f.newTrack(t), f.newTrack(t)
	audioA := flacBody("audio-a")
	audioB := flacBody("audio-b")

	if rec := f.upload(a, "", audioA, false); rec.Code != http.StatusNoContent {
		t.Fatalf("upload to a = %d: %s", rec.Code, rec.Body)
	}
	if rec := f.upload(b, "", audioB, false); rec.Code != http.StatusNoContent {
		t.Fatalf("upload to b = %d: %s", rec.Code, rec.Body)
	}
	keyA := f.stored(t, a).StorageKey

	rec := f.upload(a, "", audioB, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("dedup conflict = %d, want 409: %s", rec.Code, rec.Body)
	}
	if got := f.stored(t, a).StorageKey; got != keyA {
		t.Fatalf("track a's recorded key changed to %q after the failed re-upload", got)
	}
	if !objectPresent(t, f.blobs, keyA) {
		t.Fatal("the failed re-upload deleted track a's live audio")
	}

	body, _, err := f.blobs.Get(f.ctx, keyA)
	if err != nil {
		t.Fatalf("track a's object is gone: %v", err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, audioA) {
		t.Fatal("track a's audio was overwritten by the failed re-upload")
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
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body)
	}
	if body["code"] != "duplicate_audio" {
		t.Errorf("code = %q, want duplicate_audio: %s", body["code"], rec.Body)
	}
	// The rejected copy's key is random and never recorded anywhere on
	// failure, so its absence is checked by listing the whole bucket rather
	// than guessing the key.
	if keys := f.keys(t); len(keys) != 1 {
		t.Fatalf("bucket contents = %v, want exactly the first upload's object", keys)
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
