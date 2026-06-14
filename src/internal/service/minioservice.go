package service

import (
	"context"
	"io"
	"log/slog"
	"mydal/src/internal/repository"

	"github.com/minio/minio-go/v7"
)

type Minioservice struct {
	minioRepo *repository.Miniorepo
	logger    *slog.Logger
}

func NewMinioservice(minioRepo *repository.Miniorepo, logger *slog.Logger) *Minioservice {
	return &Minioservice{minioRepo: minioRepo, logger: logger}
}

func (s *Minioservice) UploadTrack(ctx context.Context, bucket, key, contentType string, size int64, reader io.Reader) error {
	returnVal, err := s.minioRepo.PutObject(ctx, bucket, key, reader, size, &minio.PutObjectOptions{ContentType: contentType})
	s.logger.Info("Uploaded track to MinIO", "bucket", bucket, "key", key, "size", size, "uploadInfo", returnVal)
	return err
}

func (s *Minioservice) GetTrackObject(ctx context.Context, bucket, key string) (*minio.Object, error) {
	return s.minioRepo.GetObject(ctx, bucket, key)
}
