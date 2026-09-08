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
	format, file_size, track_number, disc_number, storage_key, content_hash,
	created_at`

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
		track       domain.Track
		albumID     sql.NullString
		contentHash sql.NullString
		durationMs  int64
	)
	err := s.Scan(
		&track.ID, &track.Title, &track.ArtistID, &albumID, &durationMs,
		&track.Bitrate, &track.Format, &track.FileSize, &track.TrackNumber,
		&track.DiscNumber, &track.StorageKey, &contentHash, &track.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	track.AlbumID = albumID.String
	track.ContentHash = contentHash.String
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
	return classify(r.db.QueryRowContext(ctx,
		`INSERT INTO tracks (title, artist_id, album_id, duration_ms, bitrate,
			format, file_size, track_number, disc_number, storage_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, created_at`,
		track.Title, track.ArtistID, albumID, track.Duration.Milliseconds(),
		track.Bitrate, track.Format, track.FileSize, track.TrackNumber,
		track.DiscNumber, track.StorageKey,
	).Scan(&track.ID, &track.CreatedAt))
}

// DeleteTrack removes the row and returns the storage key it held, so the
// caller can delete the object the catalogue no longer points at. The key is
// returned rather than looked up separately because only the DELETE knows
// which row it actually removed.
func (r *TrackRepository) DeleteTrack(ctx context.Context, id string) (string, error) {
	var storageKey string
	err := r.db.QueryRowContext(ctx,
		"DELETE FROM tracks WHERE id = $1 RETURNING storage_key", id,
	).Scan(&storageKey)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("track %s: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		r.logger.Error("Failed to delete track", "error", err)
		return "", err
	}
	return storageKey, nil
}

// SetTrackFile records where an uploaded file landed and what it hashes to.
// The two move together - a key without its hash would leave the dedup index
// blind to an object that is already stored - so they are written in one
// statement. A hash that another track already holds is a conflict, which is
// what makes the unique index a dedup check rather than just an integrity one.
func (r *TrackRepository) SetTrackFile(ctx context.Context, id, storageKey, contentHash string) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE tracks SET storage_key = $1, content_hash = $2 WHERE id = $3",
		storageKey, contentHash, id)
	if err != nil {
		r.logger.Error("Failed to record track file", "error", err)
		return classify(err)
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
