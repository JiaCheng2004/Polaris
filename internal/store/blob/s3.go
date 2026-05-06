package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Options struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	UseSSL          bool
}

type S3Store struct {
	client *minio.Client
	bucket string
}

func NewS3Store(opts S3Options) (*S3Store, error) {
	endpoint, secure, err := normalizeS3Endpoint(opts.Endpoint, opts.UseSSL)
	if err != nil {
		return nil, err
	}
	bucket := strings.TrimSpace(opts.Bucket)
	if bucket == "" {
		return nil, errors.New("s3 blob bucket is required")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(opts.AccessKeyID, opts.SecretAccessKey, opts.SessionToken),
		Secure: secure,
		Region: strings.TrimSpace(opts.Region),
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 blob client: %w", err)
	}
	return &S3Store{client: client, bucket: bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, body io.Reader, size int64, mime string) error {
	cleanKey, err := cleanS3Key(key)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, s.bucket, cleanKey, body, size, minio.PutObjectOptions{ContentType: mime})
	if err != nil {
		return fmt.Errorf("put s3 blob: %w", err)
	}
	return nil
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	cleanKey, err := cleanS3Key(key)
	if err != nil {
		return nil, 0, err
	}
	info, err := s.client.StatObject(ctx, s.bucket, cleanKey, minio.StatObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("stat s3 blob: %w", err)
	}
	object, err := s.client.GetObject(ctx, s.bucket, cleanKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("get s3 blob: %w", err)
	}
	return object, info.Size, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	cleanKey, err := cleanS3Key(key)
	if err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, cleanKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete s3 blob: %w", err)
	}
	return nil
}

func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	cleanKey, err := cleanS3Key(key)
	if err != nil {
		return false, err
	}
	_, err = s.client.StatObject(ctx, s.bucket, cleanKey, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "NoSuchKey", "NoSuchBucket", "NotFound":
		return false, nil
	default:
		return false, fmt.Errorf("stat s3 blob: %w", err)
	}
}

func (s *S3Store) Close() error {
	return nil
}

func normalizeS3Endpoint(raw string, defaultSecure bool) (string, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false, errors.New("s3 blob endpoint is required")
	}
	secure := defaultSecure
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", false, fmt.Errorf("parse s3 blob endpoint: %w", err)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http":
			secure = false
		case "https":
			secure = true
		default:
			return "", false, fmt.Errorf("unsupported s3 blob endpoint scheme %q", parsed.Scheme)
		}
		trimmed = parsed.Host
		if parsed.Path != "" && parsed.Path != "/" {
			return "", false, errors.New("s3 blob endpoint must not include a path")
		}
	}
	return trimmed, secure, nil
}

func cleanS3Key(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", errors.New("blob key is required")
	}
	if strings.Contains(trimmed, "\\") || strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("invalid blob key %q", key)
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid blob key %q", key)
	}
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid blob key %q", key)
		}
	}
	return clean, nil
}
