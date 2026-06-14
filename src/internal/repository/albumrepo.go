package repository

import (
	"database/sql"
	"log/slog"
	"mydal/src/internal/domain"
)

type AlbumRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewAlbumRepository(db *sql.DB, logger *slog.Logger) *AlbumRepository {
	return &AlbumRepository{db: db, logger: logger}
}

func (r *AlbumRepository) GetAlbumByID(id string) (*domain.Album, error) {
	var album domain.Album
	err := r.db.QueryRow("SELECT id, title, artist_id FROM albums WHERE id = $1", id).Scan(&album.ID, &album.Title, &album.ArtistID)
	if err != nil {
		r.logger.Error("Failed to get album by ID", "error", err)
		return nil, err
	}
	return &album, nil
}

func (r *AlbumRepository) CreateAlbum(album *domain.Album) error {
	return r.db.QueryRow(
		"INSERT INTO albums (title, artist_id) VALUES ($1, $2) RETURNING id",
		album.Title, album.ArtistID,
	).Scan(&album.ID)
}

func (r *AlbumRepository) DeleteAlbum(id string) error {
	_, err := r.db.Exec("DELETE FROM albums WHERE id = $1", id)
	if err != nil {
		r.logger.Error("Failed to delete album", "error", err)
	}
	return err
}
