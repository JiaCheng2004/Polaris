package files

import (
	"fmt"
	"strings"
	"sync"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/store/blob"
)

// blobStoreCache holds one blob.BlobStore per distinct storage configuration, so
// the (potentially expensive, for S3/minio) client is constructed once per
// config fingerprint -- effectively once per snapshot -- rather than once per
// file read (B6). A superseded store (after a config change) is left to GC
// rather than closed, which is cheap and avoids racing Close() against an
// in-flight read holding the same store.
type blobStoreCache struct {
	mu    sync.Mutex
	key   string
	store blob.BlobStore
	err   error
	built bool
}

func (c *blobStoreCache) get(cfg config.FilesConfig) (blob.BlobStore, error) {
	key := blobConfigFingerprint(cfg)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.built && c.key == key {
		return c.store, c.err
	}
	store, err := BlobStoreFromConfig(cfg)
	c.key, c.store, c.err, c.built = key, store, err, true
	return store, err
}

// blobConfigFingerprint identifies a storage configuration. It includes every
// field that affects the constructed client, so any change (target, credentials,
// TLS) rebuilds; identical config reuses the cached store.
func blobConfigFingerprint(cfg config.FilesConfig) string {
	s := cfg.Storage
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%v",
		strings.ToLower(strings.TrimSpace(s.BlobStore)),
		s.DiskPath,
		s.S3.Endpoint, s.S3.Bucket, s.S3.Region,
		s.S3.AccessKeyID, s.S3.SecretAccessKey, s.S3.SessionToken,
		s.S3.UseSSL,
	)
}

var globalBlobCache = &blobStoreCache{}
