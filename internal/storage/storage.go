package storage

import (
	"context"
	"io"
)

// Storage defines the interface for cloud storage operations
type Storage interface {
	// Upload uploads data to the given path
	Upload(ctx context.Context, path string, r io.Reader) error

	// Download downloads data from the given path
	Download(ctx context.Context, path string) (io.ReadCloser, error)

	// List lists all objects with the given prefix
	List(ctx context.Context, prefix string) ([]string, error)

	// Delete deletes the object at the given path
	Delete(ctx context.Context, path string) error

	// Exists checks if an object exists at the given path
	Exists(ctx context.Context, path string) (bool, error)

	// Close releases any resources held by the storage client
	Close() error
}
