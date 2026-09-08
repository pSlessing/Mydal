package service

import (
	"context"
	"log/slog"
	"mydal/internal/domain"
	"mydal/internal/storage"
)

// TrackRepository is the persistence the track service needs.
type TrackRepository interface {
	GetTrackByID(ctx context.Context, id string) (*domain.Track, error)
	CreateTrack(ctx context.Context, track *domain.Track) error
	SetTrackFile(ctx context.Context, id, storageKey, contentHash string) error
	// DeleteTrack returns the storage key the deleted row held.
	DeleteTrack(ctx context.Context, id string) (string, error)
}

type TrackService struct {
	trackRepo TrackRepository
	blobs     storage.BlobStore
	logger    *slog.Logger
}

func NewTrackService(trackRepo TrackRepository, blobs storage.BlobStore, logger *slog.Logger) *TrackService {
	return &TrackService{trackRepo: trackRepo, blobs: blobs, logger: logger}
}

func (s *TrackService) GetTrackByID(ctx context.Context, id string) (*domain.Track, error) {
	return s.trackRepo.GetTrackByID(ctx, id)
}

func (s *TrackService) CreateTrack(ctx context.Context, track *domain.Track) error {
	if err := requireNonEmpty("title", track.Title); err != nil {
		return err
	}
	if err := requireUUID("artist_id", track.ArtistID); err != nil {
		return err
	}
	// album_id is optional; validate it only when one was given.
	if track.AlbumID != "" {
		if err := requireUUID("album_id", track.AlbumID); err != nil {
			return err
		}
	}
	return s.trackRepo.CreateTrack(ctx, track)
}

func (s *TrackService) SetTrackFile(ctx context.Context, id, storageKey, contentHash string) error {
	return s.trackRepo.SetTrackFile(ctx, id, storageKey, contentHash)
}

// DeleteTrack removes the row and then the object behind it. The row goes
// first because the catalogue is authoritative: a surviving object with no row
// is sweepable, whereas a surviving row with no object is a broken track. A
// failed object delete is therefore logged, not returned - the delete did
// succeed as far as the client is concerned.
func (s *TrackService) DeleteTrack(ctx context.Context, id string) error {
	storageKey, err := s.trackRepo.DeleteTrack(ctx, id)
	if err != nil {
		return err
	}
	if storageKey == "" {
		return nil
	}
	// The row is already gone, so finish the cleanup even if the client has
	// disconnected and cancelled the request context.
	if err := s.blobs.Delete(context.WithoutCancel(ctx), storageKey); err != nil {
		s.logger.Error("Orphaned object: track row deleted but object remains",
			"track_id", id, "storage_key", storageKey, "error", err)
	}
	return nil
}
