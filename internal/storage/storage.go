package storage

import (
	"io"
	"time"
)

// Storage abstracts where compressed outputs are persisted.
// Save returns an opaque locator (a filesystem path for local, an object key for S3)
// that is stored in jobs.output_path and later passed back to Get/Delete/PresignGetURL.
type Storage interface {
	Save(jobID string, filename string, reader io.Reader) (string, error)
	Get(path string) (io.ReadCloser, error)
	Delete(path string) error
	GetFullPath(path string) string
	// PresignGetURL returns a time-limited public download URL for the locator.
	// Backends that cannot presign (local disk) return an error so callers fall back
	// to streaming the file directly.
	PresignGetURL(path string, expiry time.Duration) (string, error)
}
