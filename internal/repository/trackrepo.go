package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"mydal/internal/domain"
	"time"
)

// trackColumns is the column list every track query selects, in the order
// scanTrack expects.
const trackColumns = `id, title, artist_id, album_id, duration_ms, bitrate,
	format, file_size, track_number, disc_number, storage_key, created_at`

type TrackRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewTrackRepository(db *sql.DB, logger *slog.Logger) *TrackRepository {
	return &TrackRepository{db: db, logger: logger}
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanTrack maps the storage representation (nullable album_id, milliseconds)
// onto the domain representation (empty string, time.Duration).
func scanTrack(s rowScanner) (*domain.Track, error) {
	var (
		track      domain.Track
		albumID    sql.NullString
		durationMs int64
	)
	err := s.Scan(
		&track.ID, &track.Title, &track.ArtistID, &albumID, &durationMs,
		&track.Bitrate, &track.Format, &track.FileSize, &track.TrackNumber,
		&track.DiscNumber, &track.StorageKey, &track.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	track.AlbumID = albumID.String
	track.Duration = time.Duration(durationMs) * time.Millisecond
	return &track, nil
}

func (r *TrackRepository) GetTrackByID(ctx context.Context, id string) (*domain.Track, error) {
	track, err := scanTrack(r.db.QueryRowContext(ctx,
		"SELECT "+trackColumns+" FROM tracks WHERE id = $1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("track %s: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		r.logger.Error("Failed to get track by ID", "error", err)
		return nil, err
	}
	return track, nil
}

func (r *TrackRepository) CreateTrack(ctx context.Context, track *domain.Track) error {
	albumID := sql.NullString{String: track.AlbumID, Valid: track.AlbumID != ""}
	return r.db.QueryRowContext(ctx,
		`INSERT INTO tracks (title, artist_id, album_id, duration_ms, bitrate,
			format, file_size, track_number, disc_number, storage_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, created_at`,
		track.Title, track.ArtistID, albumID, track.Duration.Milliseconds(),
		track.Bitrate, track.Format, track.FileSize, track.TrackNumber,
		track.DiscNumber, track.StorageKey,
	).Scan(&track.ID, &track.CreatedAt)
}

func (r *TrackRepository) DeleteTrack(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM tracks WHERE id = $1", id)
	if err != nil {
		r.logger.Error("Failed to delete track", "error", err)
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		r.logger.Error("Failed to read rows affected", "error", err)
		return err
	}
	if rows == 0 {
		return fmt.Errorf("track %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (r *TrackRepository) UpdateStorageKey(id, storageKey string) error {
	_, err := r.db.Exec("UPDATE tracks SET storage_key = $1 WHERE id = $2", storageKey, id)
	if err != nil {
		r.logger.Error("Failed to update storage key", "error", err)
	}
	return err
}
