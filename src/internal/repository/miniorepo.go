package repository

import (
	"log/slog"

	"github.com/minio/minio-go/v7"
)

type Miniorepo struct {
	minioClient *minio.Client
	logger      *slog.Logger
}

func NewMiniorepo(minioClient *minio.Client, logger *slog.Logger) *Miniorepo {

	return &Miniorepo{minioClient: minioClient, logger: logger}
}
