package repository

import (
	"database/sql"
	"log/slog"
	"mydal/src/internal/domain"
)

type TrackRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewTrackRepository(db *sql.DB, logger *slog.Logger) *TrackRepository {
	return &TrackRepository{db: db, logger: logger}
}

func (r *TrackRepository) GetTrackByID(id string) (*domain.Track, error) {
	var track domain.Track
	err := r.db.QueryRow("SELECT id, title, artist_id FROM tracks WHERE id = $1", id).Scan(&track.ID, &track.Title, &track.Artist.ID)
	if err != nil {
		r.logger.Error("Failed to get track by ID", "error", err)
		return nil, err
	}
	return &track, nil
}

func (r *TrackRepository) CreateTrack(track *domain.Track) error {
	return r.db.QueryRow(
		"INSERT INTO tracks (title, artist_id) VALUES ($1, $2) RETURNING id",
		track.Title, track.Artist.ID,
	).Scan(&track.ID)
}

func (r *TrackRepository) DeleteTrack(id string) error {
	_, err := r.db.Exec("DELETE FROM tracks WHERE id = $1", id)
	if err != nil {
		r.logger.Error("Failed to delete track", "error", err)
	}
	return err
}
