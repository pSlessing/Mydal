package service

import (
	"context"
	"log/slog"
	"mydal/src/internal/domain"
)

// ArtistRepository is the persistence the artist service needs. It is declared
// here, at the consumer, so the service can be tested without Postgres.
type ArtistRepository interface {
	GetArtistByID(ctx context.Context, id string) (*domain.Artist, error)
	CreateArtist(ctx context.Context, artist *domain.Artist) error
	DeleteArtist(ctx context.Context, id string) error
}

type ArtistService struct {
	repository ArtistRepository
	logger     *slog.Logger
}

func NewArtistService(repo ArtistRepository, logger *slog.Logger) *ArtistService {
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
