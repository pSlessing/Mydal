package domain

import "time"

type Playlist struct {
	ID        string
	Name      string
	Tracks    []Track
	CreatedAt time.Time
	UpdatedAt time.Time
}
