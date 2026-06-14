package domain

import "time"

type Playlist struct {
	ID          string
	Title       string
	Description string
	SongIDs     []string
	CreatedAt   time.Time
}
