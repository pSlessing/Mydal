package domain

import "time"

type Artist struct {
	ID        string
	Name      string
	Bio       string
	CreatedAt time.Time
}
