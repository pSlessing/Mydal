package domain

import "time"

type Track struct {
	ID         string
	Title      string
	Artist     Artist
	Album      Album
	Duration   time.Duration
	Bitrate    int
	Format     string // "flac", "mp3", etc.
	StorageKey string // path in S3 or local fs
	CreatedAt  time.Time
}
