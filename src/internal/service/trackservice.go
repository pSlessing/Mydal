package service

import (
	"context"
	"log/slog"
	"mydal/src/internal/domain"
)

// TrackRepository is the persistence the track service needs.
type TrackRepository interface {
	GetTrackByID(ctx context.Context, id string) (*domain.Track, error)
	CreateTrack(ctx context.Context, track *domain.Track) error
	UpdateStorageKey(id, storageKey string) error
	DeleteTrack(ctx context.Context, id string) error
}

type TrackService struct {
	trackRepo TrackRepository
	logger    *slog.Logger
}

func NewTrackService(trackRepo TrackRepository, logger *slog.Logger) *TrackService {
	return &TrackService{trackRepo: trackRepo, logger: logger}
}

func (s *TrackService) GetTrackByID(ctx context.Context, id string) (*domain.Track, error) {
	return s.trackRepo.GetTrackByID(ctx, id)
}

func (s *TrackService) CreateTrack(ctx context.Context, track *domain.Track) error {
	return s.trackRepo.CreateTrack(ctx, track)
}

func (s *TrackService) UpdateStorageKey(id, storageKey string) error {
	return s.trackRepo.UpdateStorageKey(id, storageKey)
}

func (s *TrackService) DeleteTrack(ctx context.Context, id string) error {
	return s.trackRepo.DeleteTrack(ctx, id)
}
