package storage

import "io"

type Storage interface {
	Save(jobID string, filename string, reader io.Reader) (string, error)
	Get(path string) (io.ReadCloser, error)
	Delete(path string) error
	GetFullPath(path string) string
}
