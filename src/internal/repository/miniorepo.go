package repository

import (
	"context"
	"io"
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

func (r *Miniorepo) GetObject(ctx context.Context, bucket, key string) (*minio.Object, error) {
	return r.minioClient.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}

func (r *Miniorepo) PutObject(ctx context.Context, bucket, key string, reader io.Reader, size int64, options *minio.PutObjectOptions) (minio.UploadInfo, error) {
	return r.minioClient.PutObject(ctx, bucket, key, reader, size, *options)
}

func (r *Miniorepo) DeleteObject(ctx context.Context, bucket, key string) error {
	return r.minioClient.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}
