package repository

import (
	"database/sql"
	"mydal/src/internal/domain"
)

type ArtistRepository struct {
	db *sql.DB
}

func NewArtistRepository(db *sql.DB) *ArtistRepository {
	return &ArtistRepository{db: db}
}

func (r *ArtistRepository) GetArtistByID(id string) (*domain.Artist, error) {
	var artist domain.Artist
	err := r.db.QueryRow("SELECT id, name FROM artists WHERE id = $1", id).Scan(&artist.ID, &artist.Name)
	if err != nil {
		return nil, err
	}
	return &artist, nil
}

func (r *ArtistRepository) CreateArtist(artist *domain.Artist) error {
	_, err := r.db.Exec("INSERT INTO artists (id, name) VALUES ($1, $2)", artist.ID, artist.Name)
	return err
}

func (r *ArtistRepository) DeleteArtist(id string) error {
	_, err := r.db.Exec("DELETE FROM artists WHERE id = $1", id)
	return err
}
