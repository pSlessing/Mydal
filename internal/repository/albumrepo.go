package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mydal/internal/domain"
)

type AlbumRepository struct {
	db *sql.DB
}

func NewAlbumRepository(db *sql.DB) *AlbumRepository {
	return &AlbumRepository{db: db}
}

func (r *AlbumRepository) GetAlbumByID(ctx context.Context, id string) (*domain.Album, error) {
	var album domain.Album
	var releaseDate sql.NullTime
	err := r.db.QueryRowContext(ctx,
		"SELECT id, title, artist_id, release_date, cover_key, created_at FROM albums WHERE id = $1",
		id,
	).Scan(&album.ID, &album.Title, &album.ArtistID, &releaseDate, &album.CoverKey, &album.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("album %s: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if releaseDate.Valid {
		album.ReleaseDate = &releaseDate.Time
	}
	return &album, nil
}

func (r *AlbumRepository) CreateAlbum(ctx context.Context, album *domain.Album) error {
	return classify(r.db.QueryRowContext(ctx,
		"INSERT INTO albums (title, artist_id, release_date, cover_key) VALUES ($1, $2, $3, $4) RETURNING id, created_at",
		album.Title, album.ArtistID, album.ReleaseDate, album.CoverKey,
	).Scan(&album.ID, &album.CreatedAt))
}

func (r *AlbumRepository) DeleteAlbum(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM albums WHERE id = $1", id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("album %s: %w", id, domain.ErrNotFound)
	}
	return nil
}
