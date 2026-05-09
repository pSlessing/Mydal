package service

import (
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

func (s *ArtistService) GetArtistByID(id string) (*domain.Artist, error) {
	return s.repository.GetArtistByID(id)
}

func (s *ArtistService) CreateArtist(artist *domain.Artist) error {
	return s.repository.CreateArtist(artist)
}

func (s *ArtistService) DeleteArtist(id string) error {
	return s.repository.DeleteArtist(id)
}
