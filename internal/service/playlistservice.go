package service

import (
	"log/slog"
	"mydal/internal/domain"
	"mydal/internal/repository"
)

type PlaylistService struct {
	repo   *repository.PlaylistRepository
	logger *slog.Logger
}

func NewPlaylistService(repo *repository.PlaylistRepository, logger *slog.Logger) *PlaylistService {
	return &PlaylistService{repo: repo, logger: logger}
}

func (s *PlaylistService) GetPlaylistByID(id string) (*domain.Playlist, error) {
	return s.repo.GetPlaylistByID(id)
}

func (s *PlaylistService) CreatePlaylist(p *domain.Playlist) error {
	return s.repo.CreatePlaylist(p)
}

func (s *PlaylistService) DeletePlaylist(id string) error {
	return s.repo.DeletePlaylist(id)
}

func (s *PlaylistService) AddTrack(playlistID, trackID string) error {
	return s.repo.AddTrack(playlistID, trackID)
}

func (s *PlaylistService) RemoveTrack(playlistID, trackID string) error {
	return s.repo.RemoveTrack(playlistID, trackID)
}
