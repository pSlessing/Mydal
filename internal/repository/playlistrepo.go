package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mydal/internal/domain"
)

type PlaylistRepository struct {
	db *sql.DB
}

func NewPlaylistRepository(db *sql.DB) *PlaylistRepository {
	return &PlaylistRepository{db: db}
}

func (r *PlaylistRepository) GetPlaylistByID(ctx context.Context, id string) (*domain.Playlist, error) {
	var p domain.Playlist
	err := r.db.QueryRowContext(ctx,
		"SELECT id, title, description, created_at, updated_at FROM playlists WHERE id = $1", id,
	).Scan(&p.ID, &p.Title, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("playlist %s: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}

	p.TrackIDs, err = r.trackIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PlaylistRepository) CreatePlaylist(ctx context.Context, p *domain.Playlist) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := tx.QueryRowContext(ctx,
		"INSERT INTO playlists (title, description) VALUES ($1, $2) RETURNING id, created_at, updated_at",
		p.Title, p.Description,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return err
	}

	for i, trackID := range p.TrackIDs {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO playlist_tracks (playlist_id, track_id, position) VALUES ($1, $2, $3)",
			p.ID, trackID, i,
		); err != nil {
			return classify(err)
		}
	}

	return tx.Commit()
}

func (r *PlaylistRepository) DeletePlaylist(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM playlists WHERE id = $1", id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("playlist %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

// AddTrack appends a track to the end of the playlist. It is idempotent: a
// track already in the playlist keeps the position it has.
func (r *PlaylistRepository) AddTrack(ctx context.Context, playlistID, trackID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := touchPlaylist(ctx, tx, playlistID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO playlist_tracks (playlist_id, track_id, position)
		 SELECT $1, $2, COALESCE(MAX(position) + 1, 0) FROM playlist_tracks WHERE playlist_id = $1
		 ON CONFLICT (playlist_id, track_id) DO NOTHING`,
		playlistID, trackID,
	); err != nil {
		return classify(err)
	}

	return tx.Commit()
}

// RemoveTrack drops a track from the playlist and closes the gap it leaves, so
// positions stay a dense 0..n-1 sequence. The uniqueness constraint on
// (playlist_id, position) is deferrable, so the renumbering UPDATE is checked
// once at the end of the statement rather than row by row.
func (r *PlaylistRepository) RemoveTrack(ctx context.Context, playlistID, trackID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := touchPlaylist(ctx, tx, playlistID); err != nil {
		return err
	}

	var position int
	err = tx.QueryRowContext(ctx,
		"DELETE FROM playlist_tracks WHERE playlist_id = $1 AND track_id = $2 RETURNING position",
		playlistID, trackID,
	).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("track %s in playlist %s: %w", trackID, playlistID, domain.ErrNotFound)
	}
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE playlist_tracks SET position = position - 1 WHERE playlist_id = $1 AND position > $2",
		playlistID, position,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// touchPlaylist stamps updated_at and doubles as the existence check every
// mutation needs: without it an UPDATE against a missing playlist reports
// success.
func touchPlaylist(ctx context.Context, tx *sql.Tx, playlistID string) error {
	result, err := tx.ExecContext(ctx, "UPDATE playlists SET updated_at = now() WHERE id = $1", playlistID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("playlist %s: %w", playlistID, domain.ErrNotFound)
	}
	return nil
}

func (r *PlaylistRepository) trackIDs(ctx context.Context, playlistID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT track_id FROM playlist_tracks WHERE playlist_id = $1 ORDER BY position",
		playlistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
