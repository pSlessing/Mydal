package repository

import (
	"database/sql"
	"log/slog"
	"mydal/src/internal/domain"

	"github.com/lib/pq"
)

type PlaylistRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewPlaylistRepository(db *sql.DB, logger *slog.Logger) *PlaylistRepository {
	return &PlaylistRepository{db: db, logger: logger}
}

func (r *PlaylistRepository) GetPlaylistByID(id string) (*domain.Playlist, error) {
	var p domain.Playlist
	err := r.db.QueryRow(
		"SELECT id, title, description, song_ids, created_at FROM playlists WHERE id = $1", id,
	).Scan(&p.ID, &p.Title, &p.Description, pq.Array(&p.SongIDs), &p.CreatedAt)
	if err != nil {
		r.logger.Error("Failed to get playlist by ID", "error", err)
		return nil, err
	}
	return &p, nil
}

func (r *PlaylistRepository) CreatePlaylist(p *domain.Playlist) error {
	return r.db.QueryRow(
		"INSERT INTO playlists (title, description, song_ids) VALUES ($1, $2, $3) RETURNING id, created_at",
		p.Title, p.Description, pq.Array(p.SongIDs),
	).Scan(&p.ID, &p.CreatedAt)
}

func (r *PlaylistRepository) DeletePlaylist(id string) error {
	_, err := r.db.Exec("DELETE FROM playlists WHERE id = $1", id)
	if err != nil {
		r.logger.Error("Failed to delete playlist", "error", err)
	}
	return err
}

func (r *PlaylistRepository) AddTrack(playlistID, trackID string) error {
	_, err := r.db.Exec(
		"UPDATE playlists SET song_ids = array_append(song_ids, $1) WHERE id = $2",
		trackID, playlistID,
	)
	if err != nil {
		r.logger.Error("Failed to add track to playlist", "error", err)
	}
	return err
}

func (r *PlaylistRepository) RemoveTrack(playlistID, trackID string) error {
	_, err := r.db.Exec(
		"UPDATE playlists SET song_ids = array_remove(song_ids, $1) WHERE id = $2",
		trackID, playlistID,
	)
	if err != nil {
		r.logger.Error("Failed to remove track from playlist", "error", err)
	}
	return err
}
