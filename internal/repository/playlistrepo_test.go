package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/testutil"
)

// seedArtist and seedTrack give the foreign keys something real to point at.
func seedArtist(t *testing.T, r *ArtistRepository) string {
	t.Helper()
	a := &domain.Artist{Name: "Artist"}
	if err := r.CreateArtist(context.Background(), a); err != nil {
		t.Fatalf("seed artist: %v", err)
	}
	return a.ID
}

func seedTrack(t *testing.T, r *TrackRepository, artistID, title string) string {
	t.Helper()
	tr := &domain.Track{Title: title, ArtistID: artistID, StorageKey: "tracks/" + title}
	if err := r.CreateTrack(context.Background(), tr); err != nil {
		t.Fatalf("seed track: %v", err)
	}
	return tr.ID
}

// The playlist repository used to select, insert and update a song_ids array
// column that migration 000004 never created: every endpoint failed at runtime
// with `column "song_ids" does not exist`. Membership lives in playlist_tracks.
func TestPlaylistMembershipIsOrdered(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	quiet := testutil.Quiet()
	repo := NewPlaylistRepository(db, quiet)
	tracks := NewTrackRepository(db, quiet)
	artistID := seedArtist(t, NewArtistRepository(db, quiet))

	a := seedTrack(t, tracks, artistID, "a")
	b := seedTrack(t, tracks, artistID, "b")
	c := seedTrack(t, tracks, artistID, "c")

	p := &domain.Playlist{Title: "Mix", Description: "d", TrackIDs: []string{a, b}}
	if err := repo.CreatePlaylist(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == "" || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Fatalf("create did not populate server-owned fields: %+v", p)
	}

	got, err := repo.GetPlaylistByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.Join(got.TrackIDs, ",") != strings.Join([]string{a, b}, ",") {
		t.Fatalf("membership after create = %v", got.TrackIDs)
	}

	// Appending is idempotent, so a repeated PUT is not an error.
	if err := repo.AddTrack(ctx, p.ID, c); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := repo.AddTrack(ctx, p.ID, c); err != nil {
		t.Fatalf("repeated add: %v", err)
	}
	got, _ = repo.GetPlaylistByID(ctx, p.ID)
	if strings.Join(got.TrackIDs, ",") != strings.Join([]string{a, b, c}, ",") {
		t.Fatalf("membership after add = %v", got.TrackIDs)
	}

	// Removing from the middle closes the gap, so positions stay dense and the
	// next append cannot collide with a stale one.
	if err := repo.RemoveTrack(ctx, p.ID, b); err != nil {
		t.Fatalf("remove: %v", err)
	}
	var positions []int
	rows, err := db.Query("SELECT position FROM playlist_tracks WHERE playlist_id=$1 ORDER BY position", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, n)
	}
	rows.Close()
	if len(positions) != 2 || positions[0] != 0 || positions[1] != 1 {
		t.Fatalf("positions after remove = %v, want [0 1]", positions)
	}
	if err := repo.AddTrack(ctx, p.ID, b); err != nil {
		t.Fatalf("add after renumber: %v", err)
	}
	got, _ = repo.GetPlaylistByID(ctx, p.ID)
	if strings.Join(got.TrackIDs, ",") != strings.Join([]string{a, c, b}, ",") {
		t.Fatalf("membership after re-add = %v", got.TrackIDs)
	}
}

// AddTrack and RemoveTrack issued an UPDATE and ignored the row count, so
// mutating a playlist that did not exist answered 204. updated_at was never
// written at all.
func TestPlaylistMutationsReportMissingPlaylists(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	quiet := testutil.Quiet()
	repo := NewPlaylistRepository(db, quiet)
	tracks := NewTrackRepository(db, quiet)
	artistID := seedArtist(t, NewArtistRepository(db, quiet))
	trackID := seedTrack(t, tracks, artistID, "a")
	missing := "00000000-0000-0000-0000-000000000000"

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"get", getErr(repo.GetPlaylistByID(ctx, missing))},
		{"add", repo.AddTrack(ctx, missing, trackID)},
		{"remove", repo.RemoveTrack(ctx, missing, trackID)},
		{"delete", repo.DeletePlaylist(ctx, missing)},
	} {
		if !errors.Is(tc.err, domain.ErrNotFound) {
			t.Errorf("%s on a missing playlist = %v, want ErrNotFound", tc.name, tc.err)
		}
	}

	p := &domain.Playlist{Title: "P"}
	if err := repo.CreatePlaylist(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Removing a track that is not a member is also a miss, not a silent 204.
	if err := repo.RemoveTrack(ctx, p.ID, trackID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("removing a non-member = %v, want ErrNotFound", err)
	}

	if err := repo.AddTrack(ctx, p.ID, trackID); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetPlaylistByID(ctx, p.ID)
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Fatalf("updated_at %s did not advance past created_at %s", got.UpdatedAt, got.CreatedAt)
	}
}

// Deleting a playlist takes its membership with it.
func TestDeletePlaylistCascadesMembership(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	quiet := testutil.Quiet()
	repo := NewPlaylistRepository(db, quiet)
	tracks := NewTrackRepository(db, quiet)
	artistID := seedArtist(t, NewArtistRepository(db, quiet))

	p := &domain.Playlist{Title: "P", TrackIDs: []string{seedTrack(t, tracks, artistID, "a")}}
	if err := repo.CreatePlaylist(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeletePlaylist(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM playlist_tracks WHERE playlist_id=$1", p.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("%d membership rows survived", n)
	}
}

func getErr[T any](_ T, err error) error { return err }
