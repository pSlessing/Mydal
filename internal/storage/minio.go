package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mydal/internal/domain"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
)

// MinIOStore is the S3-compatible BlobStore implementation.
type MinIOStore struct {
	client *minio.Client
	bucket string
	logger *slog.Logger
}

func NewMinIOStore(client *minio.Client, bucket string, logger *slog.Logger) *MinIOStore {
	return &MinIOStore{client: client, bucket: bucket, logger: logger}
}

// notFound reports whether err is MinIO's "no such key", so callers see
// domain.ErrNotFound rather than a vendor error.
func notFound(err error) bool {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		return resp.Code == "NoSuchKey" || resp.StatusCode == 404
	}
	return false
}

func (s *MinIOStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		s.logger.Error("Failed to put object", "key", key, "error", err)
		return fmt.Errorf("put object %q: %w", key, err)
	}
	return nil
}

func (s *MinIOStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	// GetObject is lazy and reports a missing key only on first read, so make
	// the error surface here instead of halfway through a response.
	if _, err := obj.Stat(); err != nil {
		obj.Close()
		if notFound(err) {
			return nil, fmt.Errorf("object %s: %w", key, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("stat object %q: %w", key, err)
	}
	return obj, nil
}

func (s *MinIOStore) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if notFound(err) {
			return ObjectInfo{}, fmt.Errorf("object %s: %w", key, domain.ErrNotFound)
		}
		return ObjectInfo{}, fmt.Errorf("stat object %q: %w", key, err)
	}
	return ObjectInfo{
		Key:          info.Key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

func (s *MinIOStore) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		s.logger.Error("Failed to delete object", "key", key, "error", err)
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}

// Ping checks the bucket exists, which proves both that MinIO answers and
// that the credentials work.
func (s *MinIOStore) Ping(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("reach bucket %q: %w", s.bucket, err)
	}
	if !ok {
		return fmt.Errorf("bucket %q does not exist", s.bucket)
	}
	return nil
}

func (s *MinIOStore) PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign object %q: %w", key, err)
	}
	return u.String(), nil
}
