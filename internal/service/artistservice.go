package service

import (
	"context"
	"log/slog"
	"mydal/internal/domain"
	"mydal/internal/storage"
)

// ArtistRepository is the persistence the artist service needs. It is declared
// here, at the consumer, so the service can be tested without Postgres.
type ArtistRepository interface {
	GetArtistByID(ctx context.Context, id string) (*domain.Artist, error)
	CreateArtist(ctx context.Context, artist *domain.Artist) error
	// DeleteArtist returns the storage keys of the tracks the cascade removed.
	DeleteArtist(ctx context.Context, id string) ([]string, error)
}

type ArtistService struct {
	repository ArtistRepository
	blobs      storage.BlobStore
	logger     *slog.Logger
}

func NewArtistService(repo ArtistRepository, blobs storage.BlobStore, logger *slog.Logger) *ArtistService {
	return &ArtistService{repository: repo, blobs: blobs, logger: logger}
}

func (s *ArtistService) GetArtistByID(ctx context.Context, id string) (*domain.Artist, error) {
	return s.repository.GetArtistByID(ctx, id)
}

func (s *ArtistService) CreateArtist(ctx context.Context, artist *domain.Artist) error {
	if err := requireNonEmpty("name", artist.Name); err != nil {
		return err
	}
	return s.repository.CreateArtist(ctx, artist)
}

// DeleteArtist removes the artist and the objects behind the tracks that
// cascaded away with them. A crash between the commit and the deletes leaves
// orphans, which the orphan sweep is there to reclaim.
func (s *ArtistService) DeleteArtist(ctx context.Context, id string) error {
	storageKeys, err := s.repository.DeleteArtist(ctx, id)
	if err != nil {
		return err
	}
	cleanup := context.WithoutCancel(ctx)
	for _, key := range storageKeys {
		if err := s.blobs.Delete(cleanup, key); err != nil {
			s.logger.Error("Orphaned object: artist deleted but track object remains",
				"artist_id", id, "storage_key", key, "error", err)
		}
	}
	return nil
}
