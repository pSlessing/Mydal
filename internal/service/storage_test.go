package service

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/repository"
	"mydal/internal/storage"
	"mydal/internal/testutil"
)

func objectExists(t *testing.T, blobs storage.BlobStore, key string) bool {
	t.Helper()
	_, err := blobs.Stat(context.Background(), key)
	if err == nil {
		return true
	}
	if errors.Is(err, domain.ErrNotFound) {
		return false
	}
	t.Fatalf("stat %q: %v", key, err)
	return false
}

// Deleting a track removed the row and left the object in MinIO forever.
// Deleting an artist cascaded to their tracks, so the rows vanished and every
// object behind them was orphaned with no remaining record of its key.
func TestDeletingTheLibraryEmptiesTheBucket(t *testing.T) {
	db := testutil.DB(t)
	blobs, client, bucket := testutil.BlobsWithClient(t)
	ctx := context.Background()
	quiet := testutil.Quiet()

	artistRepo := repository.NewArtistRepository(db, quiet)
	trackRepo := repository.NewTrackRepository(db, quiet)
	artists := NewArtistService(artistRepo, blobs, quiet)
	tracks := NewTrackService(trackRepo, blobs, quiet)

	newArtist := func(name string) string {
		a := &domain.Artist{Name: name}
		if err := artistRepo.CreateArtist(ctx, a); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	newTrack := func(artistID, key string) string {
		tr := &domain.Track{Title: key, ArtistID: artistID, StorageKey: key}
		if err := trackRepo.CreateTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
		body := []byte("audio for " + key)
		if err := blobs.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "audio/flac"); err != nil {
			t.Fatal(err)
		}
		return tr.ID
	}

	// Deleting a track deletes its object.
	solo := newArtist("Solo")
	trackID := newTrack(solo, "tracks/one.flac")
	if err := tracks.DeleteTrack(ctx, trackID); err != nil {
		t.Fatalf("delete track: %v", err)
	}
	if objectExists(t, blobs, "tracks/one.flac") {
		t.Error("track deleted but its object survived")
	}
	if err := tracks.DeleteTrack(ctx, trackID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleting a missing track = %v, want ErrNotFound", err)
	}

	// Deleting an artist deletes every cascaded track's object.
	band := newArtist("Band")
	newTrack(band, "tracks/two.flac")
	newTrack(band, "tracks/three.flac")
	if err := artists.DeleteArtist(ctx, band); err != nil {
		t.Fatalf("delete artist: %v", err)
	}
	for _, key := range []string{"tracks/two.flac", "tracks/three.flac"} {
		if objectExists(t, blobs, key) {
			t.Errorf("the artist cascade orphaned %s", key)
		}
	}
	var rows int
	db.QueryRow("SELECT count(*) FROM tracks WHERE artist_id = $1", band).Scan(&rows)
	if rows != 0 {
		t.Errorf("%d track rows survived the cascade", rows)
	}

	// A track with no uploaded file deletes cleanly.
	bare := &domain.Track{Title: "Bare", ArtistID: solo, StorageKey: ""}
	if err := trackRepo.CreateTrack(ctx, bare); err != nil {
		t.Fatal(err)
	}
	if err := tracks.DeleteTrack(ctx, bare.ID); err != nil {
		t.Fatalf("deleting a track with no object: %v", err)
	}

	if err := artists.DeleteArtist(ctx, solo); err != nil {
		t.Fatal(err)
	}
	if keys := testutil.Keys(t, client, bucket); len(keys) != 0 {
		t.Fatalf("bucket not empty after deleting the library: %v", keys)
	}
}

func TestDeleteArtistReportsMissing(t *testing.T) {
	db := testutil.DB(t)
	blobs := testutil.Blobs(t)
	svc := NewArtistService(repository.NewArtistRepository(db, testutil.Quiet()), blobs, testutil.Quiet())
	err := svc.DeleteArtist(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete missing artist = %v, want ErrNotFound", err)
	}
}
