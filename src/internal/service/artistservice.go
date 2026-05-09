package service

import (
	"mydal/src/internal/domain"
	"mydal/src/internal/repository"
)

type ArtistService struct {
	repository *repository.ArtistRepository
}

func NewArtistService(repo *repository.ArtistRepository) *ArtistService {
	return &ArtistService{repository: repo}
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
