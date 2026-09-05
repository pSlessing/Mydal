package repository

import (
	"database/sql"
	"log/slog"
	"mydal/src/internal/domain"
)

type ArtistRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewArtistRepository(db *sql.DB, logger *slog.Logger) *ArtistRepository {
	return &ArtistRepository{db: db, logger: logger}
}

func (r *ArtistRepository) GetArtistByID(id string) (*domain.Artist, error) {
	var artist domain.Artist
	err := r.db.QueryRow(
		"SELECT id, name, bio, created_at FROM artists WHERE id = $1",
		id,
	).Scan(&artist.ID, &artist.Name, &artist.Bio, &artist.CreatedAt)
	if err != nil {
		r.logger.Error("Failed to get artist by ID", "error", err)
		return nil, err
	}
	return &artist, nil
}

func (r *ArtistRepository) CreateArtist(artist *domain.Artist) error {
	return r.db.QueryRow(
		"INSERT INTO artists (name, bio) VALUES ($1, $2) RETURNING id, created_at",
		artist.Name, artist.Bio,
	).Scan(&artist.ID, &artist.CreatedAt)
}

func (r *ArtistRepository) DeleteArtist(id string) error {
	_, err := r.db.Exec("DELETE FROM artists WHERE id = $1", id)
	if err != nil {
		r.logger.Error("Failed to delete artist", "error", err)
	}
	return err
}
