package service

import (
	"context"
	"log/slog"
	"mydal/internal/domain"
)

// AlbumRepository is the persistence the album service needs. It is declared
// here, at the consumer, so the service can be tested without Postgres.
type AlbumRepository interface {
	GetAlbumByID(ctx context.Context, id string) (*domain.Album, error)
	CreateAlbum(ctx context.Context, album *domain.Album) error
	DeleteAlbum(ctx context.Context, id string) error
}

type AlbumService struct {
	repo   AlbumRepository
	logger *slog.Logger
}

func NewAlbumService(repo AlbumRepository, logger *slog.Logger) *AlbumService {
	return &AlbumService{repo: repo, logger: logger}
}

func (s *AlbumService) GetAlbumByID(ctx context.Context, id string) (*domain.Album, error) {
	return s.repo.GetAlbumByID(ctx, id)
}

func (s *AlbumService) CreateAlbum(ctx context.Context, album *domain.Album) error {
	if err := requireNonEmpty("title", album.Title); err != nil {
		return err
	}
	if err := requireUUID("artist_id", album.ArtistID); err != nil {
		return err
	}
	return s.repo.CreateAlbum(ctx, album)
}

func (s *AlbumService) DeleteAlbum(ctx context.Context, id string) error {
	return s.repo.DeleteAlbum(ctx, id)
}
