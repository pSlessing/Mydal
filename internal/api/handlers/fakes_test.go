package handlers

import (
	"context"
	"time"

	"mydal/internal/domain"
)

// The fakes below exist because the services and repositories are reached
// through consumer-side interfaces. Before that seam existed, every handler
// test needed a real Postgres.

type fakeTrackRepo struct {
	track     domain.Track
	gotCtx    context.Context
	getErr    error
	createErr error
	setErr    error
	created   domain.Track
	setKey    string
	setHash   string
	reached   []string
}

func (f *fakeTrackRepo) GetTrackByID(ctx context.Context, id string) (*domain.Track, error) {
	f.gotCtx, f.reached = ctx, append(f.reached, "GetTrackByID")
	if f.getErr != nil {
		return nil, f.getErr
	}
	t := f.track
	return &t, nil
}

func (f *fakeTrackRepo) CreateTrack(ctx context.Context, tr *domain.Track) error {
	f.gotCtx, f.reached = ctx, append(f.reached, "CreateTrack")
	if f.createErr != nil {
		return f.createErr
	}
	tr.ID = "server-generated"
	tr.CreatedAt = time.Unix(0, 0).UTC()
	f.created = *tr
	return nil
}

func (f *fakeTrackRepo) DeleteTrack(ctx context.Context, id string) (string, error) {
	f.gotCtx, f.reached = ctx, append(f.reached, "DeleteTrack")
	return f.track.StorageKey, f.getErr
}

func (f *fakeTrackRepo) SetTrackFile(ctx context.Context, id, storageKey, contentHash string) error {
	f.reached = append(f.reached, "SetTrackFile")
	if f.setErr != nil {
		return f.setErr
	}
	f.setKey, f.setHash = storageKey, contentHash
	f.track.StorageKey = storageKey
	return nil
}

type fakeAlbumService struct {
	album   domain.Album
	err     error
	gotCtx  context.Context
	created domain.Album
	reached []string
}

func (f *fakeAlbumService) GetAlbumByID(ctx context.Context, id string) (*domain.Album, error) {
	f.gotCtx, f.reached = ctx, append(f.reached, "GetAlbumByID")
	if f.err != nil {
		return nil, f.err
	}
	a := f.album
	return &a, nil
}

func (f *fakeAlbumService) CreateAlbum(ctx context.Context, a *domain.Album) error {
	f.gotCtx, f.reached = ctx, append(f.reached, "CreateAlbum")
	if f.err != nil {
		return f.err
	}
	a.ID = "server-generated"
	f.created = *a
	return nil
}

func (f *fakeAlbumService) DeleteAlbum(ctx context.Context, id string) error {
	f.gotCtx, f.reached = ctx, append(f.reached, "DeleteAlbum")
	return f.err
}

type fakePlaylistService struct {
	playlist domain.Playlist
	err      error
	gotCtx   context.Context
	created  domain.Playlist
	reached  []string
}

func (f *fakePlaylistService) GetPlaylistByID(ctx context.Context, id string) (*domain.Playlist, error) {
	f.gotCtx, f.reached = ctx, append(f.reached, "GetPlaylistByID")
	if f.err != nil {
		return nil, f.err
	}
	p := f.playlist
	return &p, nil
}

func (f *fakePlaylistService) CreatePlaylist(ctx context.Context, p *domain.Playlist) error {
	f.gotCtx, f.reached = ctx, append(f.reached, "CreatePlaylist")
	if f.err != nil {
		return f.err
	}
	p.ID = "server-generated"
	f.created = *p
	return nil
}

func (f *fakePlaylistService) DeletePlaylist(ctx context.Context, id string) error {
	f.gotCtx, f.reached = ctx, append(f.reached, "DeletePlaylist")
	return f.err
}

func (f *fakePlaylistService) AddTrack(ctx context.Context, playlistID, trackID string) error {
	f.gotCtx, f.reached = ctx, append(f.reached, "AddTrack")
	return f.err
}

func (f *fakePlaylistService) RemoveTrack(ctx context.Context, playlistID, trackID string) error {
	f.gotCtx, f.reached = ctx, append(f.reached, "RemoveTrack")
	return f.err
}

type fakeArtistService struct {
	artist domain.Artist
	err    error
	gotCtx context.Context
}

func (f *fakeArtistService) GetArtistByID(ctx context.Context, id string) (*domain.Artist, error) {
	f.gotCtx = ctx
	if f.err != nil {
		return nil, f.err
	}
	a := f.artist
	return &a, nil
}

func (f *fakeArtistService) CreateArtist(ctx context.Context, a *domain.Artist) error {
	f.gotCtx = ctx
	if f.err != nil {
		return f.err
	}
	a.ID = "server-generated"
	return nil
}

func (f *fakeArtistService) DeleteArtist(ctx context.Context, id string) error {
	f.gotCtx = ctx
	return f.err
}
