package repository

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/testutil"
)

// DeleteArtist used to collect its tracks' storage keys with a plain SELECT,
// so an upload's SetTrackFile that committed after that SELECT but before the
// cascading DELETE had its new key collected nowhere: the row vanished with
// the artist, and the object it now pointed at was orphaned for good.
//
// DeleteArtist's SELECT now locks the rows FOR UPDATE first. That forces one
// of two outcomes for a concurrent SetTrackFile, and this test checks that
// exactly one of them happens: either the row is still there when SetTrackFile
// runs, in which case DeleteArtist's transaction has not started (or has
// rolled back) and will see the row still there once it does start - so its
// key is collected - or the row is already gone, in which case SetTrackFile
// affects zero rows and reports ErrNotFound, so nothing is orphaned in the
// first place.
func TestDeleteArtistDoesNotOrphanAConcurrentUpload(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()

	artistRepo := NewArtistRepository(db)
	trackRepo := NewTrackRepository(db)

	for range 20 {
		artist := &domain.Artist{Name: "Racer"}
		if err := artistRepo.CreateArtist(ctx, artist); err != nil {
			t.Fatal(err)
		}
		track := &domain.Track{Title: "T", ArtistID: artist.ID}
		if err := trackRepo.CreateTrack(ctx, track); err != nil {
			t.Fatal(err)
		}
		const raceKey = "tracks/raced.flac"

		var wg sync.WaitGroup
		var keys []string
		var deleteErr, setErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			keys, deleteErr = artistRepo.DeleteArtist(ctx, artist.ID)
		}()
		go func() {
			defer wg.Done()
			setErr = trackRepo.SetTrackFile(ctx, track.ID, raceKey, "deadbeef", "flac", 1234)
		}()
		wg.Wait()

		if deleteErr != nil {
			t.Fatalf("DeleteArtist: %v", deleteErr)
		}

		uploadWon := setErr == nil
		keyCollected := slices.Contains(keys, raceKey)
		if uploadWon != keyCollected {
			t.Fatalf("upload succeeded=%v but key collected=%v (inconsistent): keys=%v, setErr=%v",
				uploadWon, keyCollected, keys, setErr)
		}
		if !uploadWon && !errors.Is(setErr, domain.ErrNotFound) {
			t.Fatalf("SetTrackFile lost the race with an unexpected error: %v", setErr)
		}
	}
}
