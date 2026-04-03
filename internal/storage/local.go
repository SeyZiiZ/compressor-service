package storage

import (
	"io"
	"os"
	"path/filepath"
)

type LocalStorage struct {
	basePath string
}

func NewLocalStorage(basePath string) (*LocalStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, err
	}
	return &LocalStorage{basePath: basePath}, nil
}

func (l *LocalStorage) Save(jobID string, filename string, reader io.Reader) (string, error) {
	dir := filepath.Join(l.basePath, jobID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return "", err
	}
	return path, nil
}

func (l *LocalStorage) Get(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (l *LocalStorage) Delete(path string) error {
	dir := filepath.Dir(path)
	// Only delete if within our base path
	if filepath.HasPrefix(dir, l.basePath) {
		return os.RemoveAll(dir)
	}
	return os.Remove(path)
}

func (l *LocalStorage) GetFullPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(l.basePath, path)
}
