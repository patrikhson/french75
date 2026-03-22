package photo

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// Storage abstracts where photo files are written.
// Swap LocalStorage for an S3 implementation without changing callers.
type Storage interface {
	Save(ctx context.Context, r io.Reader, filename string) (path string, err error)
	URL(path string) string
}

type LocalStorage struct {
	basePath string
	baseURL  string
}

func NewLocalStorage(basePath, baseURL string) *LocalStorage {
	return &LocalStorage{basePath: basePath, baseURL: baseURL}
}

func (s *LocalStorage) Save(_ context.Context, r io.Reader, filename string) (string, error) {
	dst := filepath.Join(s.basePath, filename)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return filename, nil
}

func (s *LocalStorage) URL(path string) string {
	return s.baseURL + "/" + path
}
