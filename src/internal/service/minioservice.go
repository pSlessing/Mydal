package service

import (
	"log/slog"
	"mydal/src/internal/repository"
)

type Minioservice struct {
	minioRepo *repository.Miniorepo
	logger    *slog.Logger
}

func NewMinioservice(minioRepo *repository.Miniorepo, logger *slog.Logger) *Minioservice {
	return &Minioservice{minioRepo: minioRepo, logger: logger}
}
