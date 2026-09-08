// Package storage holds the blob store behind the audio files and cover art.
// It is named for what it does rather than which vendor does it, so a
// local-filesystem implementation can sit alongside the MinIO one - which
// matters for a self-hosted music platform pointed at a directory of files.
package storage

import (
	"context"
	"io"
	"time"
)

// ObjectInfo is the metadata a caller needs without fetching the body.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// BlobStore stores and serves opaque objects by key.
type BlobStore interface {
	// Put writes an object. A negative size means the length is unknown and
	// the reader is streamed.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get opens an object for reading. The caller closes it.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Stat returns an object's metadata.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	// Ping reports whether the store is reachable and the bucket usable. It
	// backs the readiness probe, so it must be cheap.
	Ping(ctx context.Context) error
	// PresignedGetURL mints a temporary direct download URL.
	PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}
