package domain

import "time"

type Track struct {
	ID       string
	Title    string
	ArtistID string
	// AlbumID is empty for a track that belongs to no album.
	AlbumID     string
	Duration    time.Duration
	Bitrate     int
	Format      string // "flac", "mp3", etc.
	FileSize    int64
	TrackNumber int
	DiscNumber  int
	StorageKey  string // path in S3 or local fs
	ContentHash string
	CreatedAt   time.Time
}
