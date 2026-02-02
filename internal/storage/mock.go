package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"

	"github.com/charliek/envsecrets/internal/domain"
)

// MockStorage implements Storage for testing
type MockStorage struct {
	mu      sync.RWMutex
	objects map[string][]byte

	// For error injection
	UploadError   error
	DownloadError error
	ListError     error
	DeleteError   error
	ExistsError   error
}

// NewMockStorage creates a new mock storage
func NewMockStorage() *MockStorage {
	return &MockStorage{
		objects: make(map[string][]byte),
	}
}

// Upload implements Storage.Upload
func (m *MockStorage) Upload(ctx context.Context, path string, r io.Reader) error {
	if m.UploadError != nil {
		return m.UploadError
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[path] = data
	return nil
}

// Download implements Storage.Download
func (m *MockStorage) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	if m.DownloadError != nil {
		return nil, m.DownloadError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.objects[path]
	if !ok {
		return nil, domain.Errorf(domain.ErrFileNotFound, "object not found: %s", path)
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// List implements Storage.List
func (m *MockStorage) List(ctx context.Context, prefix string) ([]string, error) {
	if m.ListError != nil {
		return nil, m.ListError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var paths []string
	for path := range m.objects {
		if strings.HasPrefix(path, prefix) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// Delete implements Storage.Delete
func (m *MockStorage) Delete(ctx context.Context, path string) error {
	if m.DeleteError != nil {
		return m.DeleteError
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, path)
	return nil
}

// Exists implements Storage.Exists
func (m *MockStorage) Exists(ctx context.Context, path string) (bool, error) {
	if m.ExistsError != nil {
		return false, m.ExistsError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.objects[path]
	return ok, nil
}

// GetData returns the raw data for a path (for testing)
func (m *MockStorage) GetData(path string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.objects[path]
	return data, ok
}

// SetData sets the raw data for a path (for testing)
func (m *MockStorage) SetData(path string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[path] = data
}

// Clear removes all objects (for testing)
func (m *MockStorage) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects = make(map[string][]byte)
}

// Count returns the number of objects (for testing)
func (m *MockStorage) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.objects)
}
