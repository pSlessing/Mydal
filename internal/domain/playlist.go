package domain

import "time"

type Playlist struct {
	ID          string
	Title       string
	Description string
	TrackIDs    []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
