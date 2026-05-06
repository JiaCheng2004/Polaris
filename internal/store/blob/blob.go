package blob

import (
	"context"
	"io"
)

type BlobStore interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, mime string) error
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Close() error
}
