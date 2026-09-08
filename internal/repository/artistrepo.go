package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"mydal/internal/domain"
)

type ArtistRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewArtistRepository(db *sql.DB, logger *slog.Logger) *ArtistRepository {
	return &ArtistRepository{db: db, logger: logger}
}

func (r *ArtistRepository) GetArtistByID(ctx context.Context, id string) (*domain.Artist, error) {
	var artist domain.Artist
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, bio, created_at FROM artists WHERE id = $1",
		id,
	).Scan(&artist.ID, &artist.Name, &artist.Bio, &artist.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("artist %s: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		r.logger.Error("Failed to get artist by ID", "error", err)
		return nil, err
	}
	return &artist, nil
}

func (r *ArtistRepository) CreateArtist(ctx context.Context, artist *domain.Artist) error {
	return classify(r.db.QueryRowContext(ctx,
		"INSERT INTO artists (name, bio) VALUES ($1, $2) RETURNING id, created_at",
		artist.Name, artist.Bio,
	).Scan(&artist.ID, &artist.CreatedAt))
}

// DeleteArtist removes the artist and returns the storage keys of every track
// that went with them. tracks.artist_id is ON DELETE CASCADE, so those rows
// vanish with the artist and nothing afterwards records which objects they
// pointed at - the keys have to be collected inside the transaction, before
// the cascade fires.
func (r *ArtistRepository) DeleteArtist(ctx context.Context, id string) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		r.logger.Error("Failed to begin transaction", "error", err)
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		"SELECT storage_key FROM tracks WHERE artist_id = $1 AND storage_key <> ''", id)
	if err != nil {
		r.logger.Error("Failed to collect artist storage keys", "error", err)
		return nil, err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result, err := tx.ExecContext(ctx, "DELETE FROM artists WHERE id = $1", id)
	if err != nil {
		r.logger.Error("Failed to delete artist", "error", err)
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		r.logger.Error("Failed to read rows affected", "error", err)
		return nil, err
	}
	if affected == 0 {
		return nil, fmt.Errorf("artist %s: %w", id, domain.ErrNotFound)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return keys, nil
}
