package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mydal/internal/domain"
	"time"
)

// trackColumns is the column list every track query selects, in the order
// scanTrack expects.
const trackColumns = `id, title, artist_id, album_id, duration_ms, bitrate,
	format, file_size, track_number, disc_number, storage_key, content_hash,
	created_at`

type TrackRepository struct {
	db *sql.DB
}

func NewTrackRepository(db *sql.DB) *TrackRepository {
	return &TrackRepository{db: db}
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
//
// A track can belong to several playlists at once, and playlist_tracks.
// track_id is ON DELETE CASCADE, so the DELETE below silently drops a
// membership row out of each - leaving a gap in what RemoveTrack documents as
// a dense 0..n-1 position sequence. The gap left by each membership is only
// knowable before the cascade removes it, so those (playlist_id, position)
// pairs are read first, inside the same transaction, and each playlist is
// renumbered after the delete commits its effect.
func (r *TrackRepository) DeleteTrack(ctx context.Context, id string) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		"SELECT playlist_id, position FROM playlist_tracks WHERE track_id = $1", id)
	if err != nil {
		return "", err
	}
	type membership struct {
		playlistID string
		position   int
	}
	var memberships []membership
	for rows.Next() {
		var m membership
		if err := rows.Scan(&m.playlistID, &m.position); err != nil {
			rows.Close()
			return "", err
		}
		memberships = append(memberships, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}

	var storageKey string
	err = tx.QueryRowContext(ctx,
		"DELETE FROM tracks WHERE id = $1 RETURNING storage_key", id,
	).Scan(&storageKey)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("track %s: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return "", err
	}

	for _, m := range memberships {
		if _, err := tx.ExecContext(ctx,
			"UPDATE playlist_tracks SET position = position - 1 WHERE playlist_id = $1 AND position > $2",
			m.playlistID, m.position,
		); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return storageKey, nil
}

// AllStorageKeys returns the storage_key of every track that has a file, for
// the orphan sweep to compare against the bucket's own listing.
func (r *TrackRepository) AllStorageKeys(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT storage_key FROM tracks WHERE storage_key <> ''")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// SetTrackFile records where an uploaded file landed, what it hashes to, and
// the format and size sniffed and counted from the bytes actually uploaded -
// which is what makes them trustworthy, unlike the same-named fields on
// createTrackRequest, which are only ever the client's claim. They are
// written in one statement with the key and hash because all four describe
// the same upload. A hash that another track already holds is a conflict,
// which is what makes the unique index a dedup check rather than just an
// integrity one.
func (r *TrackRepository) SetTrackFile(ctx context.Context, id, storageKey, contentHash, format string, fileSize int64) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE tracks SET storage_key = $1, content_hash = $2, format = $3, file_size = $4 WHERE id = $5",
		storageKey, contentHash, format, fileSize, id)
	if err != nil {
		return classify(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("track %s: %w", id, domain.ErrNotFound)
	}
	return nil
}
