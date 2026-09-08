package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

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

type tripwireArtistRepo struct{ t *testing.T }

func (f *tripwireArtistRepo) GetArtistByID(ctx context.Context, id string) (*domain.Artist, error) {
	return nil, nil
}
func (f *tripwireArtistRepo) CreateArtist(ctx context.Context, a *domain.Artist) error {
	f.t.Error("CreateArtist reached the repository with invalid input")
	return nil
}
func (f *tripwireArtistRepo) DeleteArtist(ctx context.Context, id string) ([]string, error) {
	return nil, nil
}

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
func (f *tripwireTrackRepo) SetTrackFile(ctx context.Context, id, k, h, format string, size int64) error {
	return nil
}

func TestCreateValidatesInput(t *testing.T) {
	quiet := testutil.Quiet()
	ctx := context.Background()
	ok := "11111111-1111-1111-1111-111111111111"

	tracks := NewTrackService(&tripwireTrackRepo{t}, nil, quiet)
	albums := NewAlbumService(&tripwireAlbumRepo{t}, quiet)
	playlists := NewPlaylistService(&tripwirePlaylistRepo{t}, quiet)
	artists := NewArtistService(&tripwireArtistRepo{t}, nil, quiet)

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"track empty title", tracks.CreateTrack(ctx, &domain.Track{Title: "  ", ArtistID: ok})},
		{"track missing artist_id", tracks.CreateTrack(ctx, &domain.Track{Title: "T"})},
		{"track non-uuid artist_id", tracks.CreateTrack(ctx, &domain.Track{Title: "T", ArtistID: "nope"})},
		{"track non-uuid album_id", tracks.CreateTrack(ctx, &domain.Track{Title: "T", ArtistID: ok, AlbumID: "nope"})},
		{"track negative duration", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, Duration: -time.Millisecond})},
		{"track duration overflows int32", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, Duration: (math.MaxInt32 + 1) * time.Millisecond})},
		{"track negative bitrate", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, Bitrate: -1})},
		{"track bitrate overflows int32", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, Bitrate: math.MaxInt32 + 1})},
		{"track negative file_size", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, FileSize: -1})},
		{"track negative track_number", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, TrackNumber: -1})},
		{"track track_number overflows int32", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, TrackNumber: math.MaxInt32 + 1})},
		{"track negative disc_number", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, DiscNumber: -1})},
		{"track disc_number overflows int32", tracks.CreateTrack(ctx,
			&domain.Track{Title: "T", ArtistID: ok, DiscNumber: math.MaxInt32 + 1})},
		{"album empty title", albums.CreateAlbum(ctx, &domain.Album{Title: "", ArtistID: ok})},
		{"album non-uuid artist_id", albums.CreateAlbum(ctx, &domain.Album{Title: "A", ArtistID: "nope"})},
		{"playlist empty title", playlists.CreatePlaylist(ctx, &domain.Playlist{Title: "\t"})},
		{"playlist non-uuid track", playlists.CreatePlaylist(ctx,
			&domain.Playlist{Title: "P", TrackIDs: []string{ok, "nope"}})},
		{"artist empty name", artists.CreateArtist(ctx, &domain.Artist{Name: "   "})},
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
func (f *recordingTrackRepo) SetTrackFile(ctx context.Context, id, k, h, format string, size int64) error {
	return nil
}
