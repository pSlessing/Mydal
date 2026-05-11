package service

import (
	"log/slog"
	"mydal/src/internal/repository"
)

type TrackService struct {
	trackRepo *repository.TrackRepository
	logger    *slog.Logger
}

func NewTrackService(trackRepo *repository.TrackRepository, logger *slog.Logger) *TrackService {
	return &TrackService{trackRepo: trackRepo, logger: logger}
}
