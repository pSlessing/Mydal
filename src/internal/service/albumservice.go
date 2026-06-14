package service

import (
	"log/slog"
	"mydal/src/internal/domain"
	"mydal/src/internal/repository"
)

type AlbumService struct {
	repo   *repository.AlbumRepository
	logger *slog.Logger
}

func NewAlbumService(repo *repository.AlbumRepository, logger *slog.Logger) *AlbumService {
	return &AlbumService{repo: repo, logger: logger}
}

func (s *AlbumService) GetAlbumByID(id string) (*domain.Album, error) {
	return s.repo.GetAlbumByID(id)
}

func (s *AlbumService) CreateAlbum(album *domain.Album) error {
	return s.repo.CreateAlbum(album)
}

func (s *AlbumService) DeleteAlbum(id string) error {
	return s.repo.DeleteAlbum(id)
}
