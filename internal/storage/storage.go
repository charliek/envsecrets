package storage

import (
	"context"
	"io"
	"time"
)

// Storage defines the interface for cloud storage operations
type Storage interface {
	// Upload uploads data to the given path
	Upload(ctx context.Context, path string, r io.Reader) error

	// Download downloads data from the given path
	Download(ctx context.Context, path string) (io.ReadCloser, error)

	// List lists all objects with the given prefix
	List(ctx context.Context, prefix string) ([]string, error)

	// ListWithMetadata lists objects with extended metadata (name, size, updated)
	ListWithMetadata(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// Delete deletes the object at the given path
	Delete(ctx context.Context, path string) error

	// Exists checks if an object exists at the given path
	Exists(ctx context.Context, path string) (bool, error)

	// Close releases any resources held by the storage client
	Close() error
}

// ObjectInfo represents a storage object with extended metadata
type ObjectInfo struct {
	// Name is the full path to the object in storage
	Name string
	// Size is the size of the object in bytes
	Size int64
	// Updated is the last modification time
	Updated time.Time
}
