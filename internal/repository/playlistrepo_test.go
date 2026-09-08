package repository

import (
	"context"
	"database/sql"
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
	repo := NewPlaylistRepository(db)
	tracks := NewTrackRepository(db)
	artistID := seedArtist(t, NewArtistRepository(db))

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
	repo := NewPlaylistRepository(db)
	tracks := NewTrackRepository(db)
	artistID := seedArtist(t, NewArtistRepository(db))
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
	repo := NewPlaylistRepository(db)
	tracks := NewTrackRepository(db)
	artistID := seedArtist(t, NewArtistRepository(db))

	p := &domain.Playlist{Title: "P", TrackIDs: []string{seedTrack(t, tracks, artistID, "a")}}
	if err := repo.CreatePlaylist(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeletePlaylist(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow("SELECT count(*) FROM playlist_tracks WHERE playlist_id=$1", p.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("%d membership rows survived", n)
	}
}

// positionsOf reads a playlist's positions in order, for asserting they stay
// a dense 0..n-1 sequence.
func positionsOf(t *testing.T, db *sql.DB, playlistID string) []int {
	t.Helper()
	rows, err := db.Query(
		"SELECT position FROM playlist_tracks WHERE playlist_id=$1 ORDER BY position", playlistID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var positions []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, n)
	}
	return positions
}

// Deleting a track cascades it out of playlist_tracks without renumbering,
// unlike RemoveTrack - leaving a gap RemoveTrack's own comment claims cannot
// happen. DeleteTrack now closes that gap itself, including when the track
// sits in more than one playlist at a position other members do not share.
func TestDeleteTrackRenumbersEveryPlaylistItLeaves(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	playlists := NewPlaylistRepository(db)
	tracks := NewTrackRepository(db)
	artistID := seedArtist(t, NewArtistRepository(db))

	a := seedTrack(t, tracks, artistID, "a")
	b := seedTrack(t, tracks, artistID, "b")
	c := seedTrack(t, tracks, artistID, "c")

	// b sits in the middle of one playlist and at the end of another, so the
	// same delete must renumber each independently.
	p1 := &domain.Playlist{Title: "P1", TrackIDs: []string{a, b, c}}
	if err := playlists.CreatePlaylist(ctx, p1); err != nil {
		t.Fatal(err)
	}
	p2 := &domain.Playlist{Title: "P2", TrackIDs: []string{a, c, b}}
	if err := playlists.CreatePlaylist(ctx, p2); err != nil {
		t.Fatal(err)
	}

	if _, err := tracks.DeleteTrack(ctx, b); err != nil {
		t.Fatalf("delete track: %v", err)
	}

	if got := positionsOf(t, db, p1.ID); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("p1 positions after delete = %v, want [0 1]", got)
	}
	if got := positionsOf(t, db, p2.ID); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("p2 positions after delete = %v, want [0 1]", got)
	}

	got1, err := playlists.GetPlaylistByID(ctx, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got1.TrackIDs, ",") != strings.Join([]string{a, c}, ",") {
		t.Fatalf("p1 membership after delete = %v", got1.TrackIDs)
	}
	got2, err := playlists.GetPlaylistByID(ctx, p2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got2.TrackIDs, ",") != strings.Join([]string{a, c}, ",") {
		t.Fatalf("p2 membership after delete = %v", got2.TrackIDs)
	}

	// The dense sequence must still hold for a subsequent append.
	d := seedTrack(t, tracks, artistID, "d")
	if err := playlists.AddTrack(ctx, p1.ID, d); err != nil {
		t.Fatal(err)
	}
	if got := positionsOf(t, db, p1.ID); len(got) != 3 || got[2] != 2 {
		t.Fatalf("p1 positions after re-add = %v, want [.. 2]", got)
	}
}

func getErr[T any](_ T, err error) error { return err }
