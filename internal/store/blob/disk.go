package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type DiskStore struct {
	root string
}

func NewDiskStore(root string) (*DiskStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("disk blob root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve disk blob root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create disk blob root: %w", err)
	}
	return &DiskStore{root: abs}, nil
}

func (s *DiskStore) Put(ctx context.Context, key string, body io.Reader, size int64, mime string) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create blob directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create blob temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	written, err := io.Copy(tmp, body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write blob: %w", err)
	}
	if size >= 0 && written != size {
		return fmt.Errorf("write blob: expected %d bytes, wrote %d", size, written)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit blob: %w", err)
	}
	removeTmp = false
	return nil
}

func (s *DiskStore) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	path, err := s.pathForKey(key)
	if err != nil {
		return nil, 0, err
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	return file, info.Size(), nil
}

func (s *DiskStore) Delete(ctx context.Context, key string) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *DiskStore) Exists(ctx context.Context, key string) (bool, error) {
	path, err := s.pathForKey(key)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *DiskStore) Close() error {
	return nil
}

func (s *DiskStore) pathForKey(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", errors.New("blob key is required")
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("invalid blob key %q", key)
	}
	path := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid blob key %q", key)
	}
	return path, nil
}
