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

// minPartSize is the floor for the part size computed below. minio-go
// buffers a whole part in memory before sending it, so this is also
// (roughly) the minimum per-upload memory cost once a part size is set.
const minPartSize = 16 << 20 // 16 MiB

// maxParts is S3's own multipart-upload ceiling: a PartSize too small for the
// upload cap would need more parts than a multipart upload can have.
const maxParts = 10000

// MinIOStore is the S3-compatible BlobStore implementation.
type MinIOStore struct {
	client   *minio.Client
	bucket   string
	partSize uint64
	logger   *slog.Logger
}

// NewMinIOStore builds a store whose part size, for uploads of unknown
// length, is derived from maxUploadBytes: large enough that the largest
// allowed upload still fits under maxParts, never smaller than minPartSize.
// Without this, minio-go sizes parts for a 5 TiB object (~537 MiB each) and
// buffers a whole part in memory, so a handful of concurrent chunked uploads
// can exhaust a small host.
func NewMinIOStore(client *minio.Client, bucket string, maxUploadBytes int64, logger *slog.Logger) *MinIOStore {
	partSize := uint64(minPartSize)
	if computed := uint64(maxUploadBytes) / maxParts; maxUploadBytes > 0 && computed > partSize {
		partSize = computed
	}
	return &MinIOStore{client: client, bucket: bucket, partSize: partSize, logger: logger}
}

// notFound reports whether err is MinIO's "no such key", so callers see
// domain.ErrNotFound rather than a vendor error. Only the specific code, not
// every 404, matters here: NoSuchBucket is also a 404, and treating it the
// same as a missing key would report a deleted bucket as "track not found"
// instead of the outage it actually is.
func notFound(err error) bool {
	var resp minio.ErrorResponse
	return errors.As(err, &resp) && resp.Code == "NoSuchKey"
}

func (s *MinIOStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	opts := minio.PutObjectOptions{ContentType: contentType}
	if size < 0 {
		// Unknown length: bound the part size (and so the buffer minio-go
		// holds per part) instead of letting it default to one sized for a
		// 5 TiB object. One thread keeps it to a single buffer per upload.
		opts.PartSize = s.partSize
		opts.NumThreads = 1
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, opts)
	if err != nil {
		s.logger.Error("Failed to put object", "key", key, "error", err)
		return fmt.Errorf("put object %q: %w", key, err)
	}
	return nil
}

func (s *MinIOStore) Get(ctx context.Context, key string) (io.ReadSeekCloser, ObjectInfo, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("get object %q: %w", key, err)
	}
	// GetObject is lazy and reports a missing key only on first read, so stat
	// it now to surface that early - which, done here rather than by a
	// separate Stat call before Get, is what turns two round trips into one.
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		if notFound(err) {
			return nil, ObjectInfo{}, fmt.Errorf("object %s: %w", key, domain.ErrNotFound)
		}
		return nil, ObjectInfo{}, fmt.Errorf("stat object %q: %w", key, err)
	}
	return obj, ObjectInfo{
		Key:          info.Key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
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

// List returns every object key in the bucket. It exists for the orphan
// sweep, which is a maintenance path run out-of-band from request handling,
// so loading the whole listing into memory is acceptable here in a way it
// would not be on a request path.
func (s *MinIOStore) List(ctx context.Context) ([]string, error) {
	var keys []string
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list bucket %q: %w", s.bucket, obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
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
