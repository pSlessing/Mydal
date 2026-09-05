package service

import (
	"context"
	"log/slog"
	"mydal/src/internal/domain"
	"mydal/src/internal/repository"
)

type ArtistService struct {
	repository *repository.ArtistRepository
	logger     *slog.Logger
}

func NewArtistService(repo *repository.ArtistRepository, logger *slog.Logger) *ArtistService {
	return &ArtistService{repository: repo, logger: logger}
}

func (s *ArtistService) GetArtistByID(ctx context.Context, id string) (*domain.Artist, error) {
	return s.repository.GetArtistByID(ctx, id)
}

func (s *ArtistService) CreateArtist(ctx context.Context, artist *domain.Artist) error {
	return s.repository.CreateArtist(ctx, artist)
}

func (s *ArtistService) DeleteArtist(ctx context.Context, id string) error {
	return s.repository.DeleteArtist(ctx, id)
}
