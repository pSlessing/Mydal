package repository

import (
	"context"
	"errors"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/testutil"
)

// A constraint violation is a client mistake, and used to surface as a 500
// with the driver's text handed straight to the caller.
func TestConstraintViolationsBecomeSentinels(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	quiet := testutil.Quiet()
	missing := "00000000-0000-0000-0000-000000000000"

	albums := NewAlbumRepository(db, quiet)
	tracks := NewTrackRepository(db, quiet)
	playlists := NewPlaylistRepository(db, quiet)
	artistID := seedArtist(t, NewArtistRepository(db, quiet))
	trackID := seedTrack(t, tracks, artistID, "a")

	// Foreign key violations: 400.
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"album under missing artist", albums.CreateAlbum(ctx, &domain.Album{Title: "X", ArtistID: missing})},
		{"track under missing artist", tracks.CreateTrack(ctx,
			&domain.Track{Title: "X", ArtistID: missing, StorageKey: "k"})},
		{"playlist with missing track", playlists.CreatePlaylist(ctx,
			&domain.Playlist{Title: "P", TrackIDs: []string{missing}})},
	} {
		if !errors.Is(tc.err, domain.ErrInvalidInput) {
			t.Errorf("%s: %v, want ErrInvalidInput", tc.name, tc.err)
		}
	}

	p := &domain.Playlist{Title: "P"}
	if err := playlists.CreatePlaylist(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := playlists.AddTrack(ctx, p.ID, missing); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("add missing track: %v, want ErrInvalidInput", err)
	}

	// Unique violations: 409.
	err := playlists.CreatePlaylist(ctx, &domain.Playlist{Title: "Dup", TrackIDs: []string{trackID, trackID}})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("duplicate membership: %v, want ErrConflict", err)
	}
}

// An outage must stay unclassified, or a 500 would be reported as a client
// error.
func TestOutageStaysUnclassified(t *testing.T) {
	db := testutil.DB(t)
	repo := NewAlbumRepository(db, testutil.Quiet())
	artistID := seedArtist(t, NewArtistRepository(db, testutil.Quiet()))
	db.Close()

	err := repo.CreateAlbum(context.Background(), &domain.Album{Title: "X", ArtistID: artistID})
	if err == nil {
		t.Fatal("expected an error from a closed pool")
	}
	for _, sentinel := range []error{domain.ErrInvalidInput, domain.ErrConflict, domain.ErrNotFound} {
		if errors.Is(err, sentinel) {
			t.Fatalf("closed pool classified as %v: %v", sentinel, err)
		}
	}
}

// Every album, playlist and storage-key call took no context, so a client
// disconnect could not cancel the query behind it.
func TestCancellationReachesPostgres(t *testing.T) {
	db := testutil.DB(t)
	quiet := testutil.Quiet()
	albums := NewAlbumRepository(db, quiet)
	playlists := NewPlaylistRepository(db, quiet)
	tracks := NewTrackRepository(db, quiet)

	live := context.Background()
	artistID := seedArtist(t, NewArtistRepository(db, quiet))
	trackID := seedTrack(t, tracks, artistID, "a")
	album := &domain.Album{Title: "A", ArtistID: artistID}
	if err := albums.CreateAlbum(live, album); err != nil {
		t.Fatal(err)
	}
	p := &domain.Playlist{Title: "P"}
	if err := playlists.CreatePlaylist(live, p); err != nil {
		t.Fatal(err)
	}

	dead, cancel := context.WithCancel(context.Background())
	cancel()

	for name, call := range map[string]func() error{
		"GetAlbumByID":    func() error { return getErr(albums.GetAlbumByID(dead, album.ID)) },
		"CreateAlbum":     func() error { return albums.CreateAlbum(dead, &domain.Album{Title: "X", ArtistID: artistID}) },
		"DeleteAlbum":     func() error { return albums.DeleteAlbum(dead, album.ID) },
		"GetPlaylistByID": func() error { return getErr(playlists.GetPlaylistByID(dead, p.ID)) },
		"CreatePlaylist":  func() error { return playlists.CreatePlaylist(dead, &domain.Playlist{Title: "X"}) },
		"DeletePlaylist":  func() error { return playlists.DeletePlaylist(dead, p.ID) },
		"AddTrack":        func() error { return playlists.AddTrack(dead, p.ID, trackID) },
		"RemoveTrack":     func() error { return playlists.RemoveTrack(dead, p.ID, trackID) },
		"SetTrackFile":    func() error { return tracks.SetTrackFile(dead, trackID, "k", "h") },
	} {
		if err := call(); !errors.Is(err, context.Canceled) {
			t.Errorf("%s with a cancelled context = %v, want context.Canceled", name, err)
		}
	}

	// Nothing was written despite every call being made.
	var albumCount, playlistCount int
	_ = db.QueryRow("SELECT count(*) FROM albums").Scan(&albumCount)
	_ = db.QueryRow("SELECT count(*) FROM playlists").Scan(&playlistCount)
	if albumCount != 1 || playlistCount != 1 {
		t.Fatalf("cancelled writes landed: albums=%d playlists=%d", albumCount, playlistCount)
	}
}
