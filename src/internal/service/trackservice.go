package service

import (
	"log/slog"
	"mydal/src/internal/domain"
	"mydal/src/internal/repository"
)

type TrackService struct {
	trackRepo *repository.TrackRepository
	logger    *slog.Logger
}

func NewTrackService(trackRepo *repository.TrackRepository, logger *slog.Logger) *TrackService {
	return &TrackService{trackRepo: trackRepo, logger: logger}
}

func (s *TrackService) GetTrackByID(id string) (*domain.Track, error) {
	return s.trackRepo.GetTrackByID(id)
}

func (s *TrackService) CreateTrack(track *domain.Track) error {
	return s.trackRepo.CreateTrack(track)
}

func (s *TrackService) UpdateStorageKey(id, storageKey string) error {
	return s.trackRepo.UpdateStorageKey(id, storageKey)
}

func (s *TrackService) DeleteTrack(id string) error {
	return s.trackRepo.DeleteTrack(id)
}
