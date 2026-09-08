package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mydal/internal/domain"
)

type ArtistRepository struct {
	db *sql.DB
}

func NewArtistRepository(db *sql.DB) *ArtistRepository {
	return &ArtistRepository{db: db}
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
//
// The SELECT locks every track row for the artist, not just the ones that
// currently have a file, and only then reads storage_key. That closes the
// race with a concurrent upload's SetTrackFile: if its UPDATE reaches a row
// first, this SELECT blocks until it commits and so reads the new key; if
// this SELECT locks the row first, the UPDATE blocks until this transaction
// commits and then affects zero rows (the row is gone), which SetTrackFile
// already reports as ErrNotFound and the handler already cleans up as an
// unreferenced object. Either way, no track can finish an upload whose key
// this method fails to collect.
func (r *ArtistRepository) DeleteArtist(ctx context.Context, id string) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		"SELECT storage_key FROM tracks WHERE artist_id = $1 FOR UPDATE", id)
	if err != nil {
		return nil, err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		if key != "" {
			keys = append(keys, key)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result, err := tx.ExecContext(ctx, "DELETE FROM artists WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
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
