package service

import (
	"context"
	"fmt"
	"log/slog"
	"mydal/internal/domain"
)

// PlaylistRepository is the persistence the playlist service needs. It is
// declared here, at the consumer, so the service can be tested without
// Postgres.
type PlaylistRepository interface {
	GetPlaylistByID(ctx context.Context, id string) (*domain.Playlist, error)
	CreatePlaylist(ctx context.Context, p *domain.Playlist) error
	DeletePlaylist(ctx context.Context, id string) error
	AddTrack(ctx context.Context, playlistID, trackID string) error
	RemoveTrack(ctx context.Context, playlistID, trackID string) error
}

type PlaylistService struct {
	repo   PlaylistRepository
	logger *slog.Logger
}

func NewPlaylistService(repo PlaylistRepository, logger *slog.Logger) *PlaylistService {
	return &PlaylistService{repo: repo, logger: logger}
}

func (s *PlaylistService) GetPlaylistByID(ctx context.Context, id string) (*domain.Playlist, error) {
	return s.repo.GetPlaylistByID(ctx, id)
}

func (s *PlaylistService) CreatePlaylist(ctx context.Context, p *domain.Playlist) error {
	if err := requireNonEmpty("title", p.Title); err != nil {
		return err
	}
	for i, trackID := range p.TrackIDs {
		if err := requireUUID(fmt.Sprintf("track_ids[%d]", i), trackID); err != nil {
			return err
		}
	}
	return s.repo.CreatePlaylist(ctx, p)
}

func (s *PlaylistService) DeletePlaylist(ctx context.Context, id string) error {
	return s.repo.DeletePlaylist(ctx, id)
}

func (s *PlaylistService) AddTrack(ctx context.Context, playlistID, trackID string) error {
	return s.repo.AddTrack(ctx, playlistID, trackID)
}

func (s *PlaylistService) RemoveTrack(ctx context.Context, playlistID, trackID string) error {
	return s.repo.RemoveTrack(ctx, playlistID, trackID)
}
