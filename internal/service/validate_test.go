package service

import (
	"context"
	"errors"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/testutil"
)

// tripwire fails the test if a request that should have been rejected reaches
// persistence. Before validation existed, a non-UUID in a request body reached
// Postgres, where "invalid input syntax for type uuid" is unclassified and so
// became a 500 rather than the 400 it is.
type tripwireAlbumRepo struct{ t *testing.T }

func (f *tripwireAlbumRepo) GetAlbumByID(ctx context.Context, id string) (*domain.Album, error) {
	return nil, nil
}
func (f *tripwireAlbumRepo) CreateAlbum(ctx context.Context, a *domain.Album) error {
	f.t.Error("CreateAlbum reached the repository with invalid input")
	return nil
}
func (f *tripwireAlbumRepo) DeleteAlbum(ctx context.Context, id string) error { return nil }

type tripwirePlaylistRepo struct{ t *testing.T }

func (f *tripwirePlaylistRepo) GetPlaylistByID(ctx context.Context, id string) (*domain.Playlist, error) {
	return nil, nil
}
func (f *tripwirePlaylistRepo) CreatePlaylist(ctx context.Context, p *domain.Playlist) error {
	f.t.Error("CreatePlaylist reached the repository with invalid input")
	return nil
}
func (f *tripwirePlaylistRepo) DeletePlaylist(ctx context.Context, id string) error { return nil }
func (f *tripwirePlaylistRepo) AddTrack(ctx context.Context, p, tr string) error    { return nil }
func (f *tripwirePlaylistRepo) RemoveTrack(ctx context.Context, p, tr string) error { return nil }

type tripwireTrackRepo struct{ t *testing.T }

func (f *tripwireTrackRepo) GetTrackByID(ctx context.Context, id string) (*domain.Track, error) {
	return nil, nil
}
func (f *tripwireTrackRepo) CreateTrack(ctx context.Context, tr *domain.Track) error {
	f.t.Error("CreateTrack reached the repository with invalid input")
	return nil
}
func (f *tripwireTrackRepo) DeleteTrack(ctx context.Context, id string) (string, error) {
	return "", nil
}
func (f *tripwireTrackRepo) SetTrackFile(ctx context.Context, id, k, h string) error { return nil }

func TestCreateValidatesInput(t *testing.T) {
	quiet := testutil.Quiet()
	ctx := context.Background()
	ok := "11111111-1111-1111-1111-111111111111"

	tracks := NewTrackService(&tripwireTrackRepo{t}, nil, quiet)
	albums := NewAlbumService(&tripwireAlbumRepo{t}, quiet)
	playlists := NewPlaylistService(&tripwirePlaylistRepo{t}, quiet)

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"track empty title", tracks.CreateTrack(ctx, &domain.Track{Title: "  ", ArtistID: ok})},
		{"track missing artist_id", tracks.CreateTrack(ctx, &domain.Track{Title: "T"})},
		{"track non-uuid artist_id", tracks.CreateTrack(ctx, &domain.Track{Title: "T", ArtistID: "nope"})},
		{"track non-uuid album_id", tracks.CreateTrack(ctx, &domain.Track{Title: "T", ArtistID: ok, AlbumID: "nope"})},
		{"album empty title", albums.CreateAlbum(ctx, &domain.Album{Title: "", ArtistID: ok})},
		{"album non-uuid artist_id", albums.CreateAlbum(ctx, &domain.Album{Title: "A", ArtistID: "nope"})},
		{"playlist empty title", playlists.CreatePlaylist(ctx, &domain.Playlist{Title: "\t"})},
		{"playlist non-uuid track", playlists.CreatePlaylist(ctx,
			&domain.Playlist{Title: "P", TrackIDs: []string{ok, "nope"}})},
	} {
		if !errors.Is(tc.err, domain.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", tc.name, tc.err)
		}
	}
}

// An optional album_id must stay optional.
func TestEmptyAlbumIDIsAllowed(t *testing.T) {
	repo := &recordingTrackRepo{}
	svc := NewTrackService(repo, nil, testutil.Quiet())
	err := svc.CreateTrack(context.Background(),
		&domain.Track{Title: "T", ArtistID: "11111111-1111-1111-1111-111111111111"})
	if err != nil {
		t.Fatalf("track with no album rejected: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("valid track never reached the repository")
	}
}

type recordingTrackRepo struct{ createCalled bool }

func (f *recordingTrackRepo) GetTrackByID(ctx context.Context, id string) (*domain.Track, error) {
	return nil, nil
}
func (f *recordingTrackRepo) CreateTrack(ctx context.Context, tr *domain.Track) error {
	f.createCalled = true
	return nil
}
func (f *recordingTrackRepo) DeleteTrack(ctx context.Context, id string) (string, error) {
	return "", nil
}
func (f *recordingTrackRepo) SetTrackFile(ctx context.Context, id, k, h string) error { return nil }
