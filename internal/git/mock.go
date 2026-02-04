package git

import (
	"sync"
	"time"

	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
)

// Compile-time assertion that MockRepository implements Repository
var _ Repository = (*MockRepository)(nil)

// MockRepository implements Repository for testing
type MockRepository struct {
	mu          sync.RWMutex
	initialized bool
	files       map[string][]byte
	commits     []domain.Commit
	staged      map[string]bool
	head        string

	// Error injection
	InitError             error
	AddError              error
	CommitError           error
	LogError              error
	CheckoutError         error
	CheckoutBranchError   error
	GetDefaultBranchError error
	ReadError             error
	WriteError            error
	RemoveError           error

	// Configurable default branch (defaults to "main")
	DefaultBranch string
}

// NewMockRepository creates a new mock repository
func NewMockRepository() *MockRepository {
	return &MockRepository{
		files:  make(map[string][]byte),
		staged: make(map[string]bool),
	}
}

// Init implements Repository.Init
func (m *MockRepository) Init() error {
	if m.InitError != nil {
		return m.InitError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initialized = true
	return nil
}

// Add implements Repository.Add
func (m *MockRepository) Add(paths ...string) error {
	if m.AddError != nil {
		return m.AddError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized {
		return domain.ErrNotInitialized
	}
	for _, path := range paths {
		m.staged[path] = true
	}
	return nil
}

// Commit implements Repository.Commit
func (m *MockRepository) Commit(message string) (string, error) {
	if m.CommitError != nil {
		return "", m.CommitError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized {
		return "", domain.ErrNotInitialized
	}

	hash := generateMockHash()
	commit := domain.Commit{
		Hash:      hash,
		ShortHash: hash[:constants.ShortHashLength],
		Message:   message,
		Author:    "test",
		Date:      time.Now(),
	}
	m.commits = append([]domain.Commit{commit}, m.commits...)
	m.head = hash
	m.staged = make(map[string]bool)
	return hash, nil
}

// Log implements Repository.Log
func (m *MockRepository) Log(n int) ([]domain.Commit, error) {
	if m.LogError != nil {
		return nil, m.LogError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return nil, domain.ErrNotInitialized
	}

	if n > len(m.commits) {
		n = len(m.commits)
	}
	return m.commits[:n], nil
}

// Checkout implements Repository.Checkout
func (m *MockRepository) Checkout(ref string) error {
	if m.CheckoutError != nil {
		return m.CheckoutError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized {
		return domain.ErrNotInitialized
	}

	// Find the commit
	for _, c := range m.commits {
		if c.Hash == ref || c.ShortHash == ref {
			m.head = c.Hash
			return nil
		}
	}
	return domain.Errorf(domain.ErrRefNotFound, "ref not found: %s", ref)
}

// CheckoutBranch implements Repository.CheckoutBranch
func (m *MockRepository) CheckoutBranch(branch string) error {
	if m.CheckoutBranchError != nil {
		return m.CheckoutBranchError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized {
		return domain.ErrNotInitialized
	}

	// In the mock, we just accept any branch name
	// Real implementation would validate the branch exists
	return nil
}

// GetDefaultBranch implements Repository.GetDefaultBranch
func (m *MockRepository) GetDefaultBranch() (string, error) {
	if m.GetDefaultBranchError != nil {
		return "", m.GetDefaultBranchError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return "", domain.ErrNotInitialized
	}

	if m.DefaultBranch != "" {
		return m.DefaultBranch, nil
	}
	return "main", nil
}

// ListFiles implements Repository.ListFiles
func (m *MockRepository) ListFiles() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return nil, domain.ErrNotInitialized
	}

	var files []string
	for path := range m.files {
		files = append(files, path)
	}
	return files, nil
}

// ReadFile implements Repository.ReadFile
func (m *MockRepository) ReadFile(path, ref string) ([]byte, error) {
	if m.ReadError != nil {
		return nil, m.ReadError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return nil, domain.ErrNotInitialized
	}

	data, ok := m.files[path]
	if !ok {
		return nil, domain.Errorf(domain.ErrFileNotFound, "file not found: %s", path)
	}
	return data, nil
}

// WriteFile implements Repository.WriteFile
func (m *MockRepository) WriteFile(path string, content []byte) error {
	if m.WriteError != nil {
		return m.WriteError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = content
	return nil
}

// RemoveFile implements Repository.RemoveFile
func (m *MockRepository) RemoveFile(path string) error {
	if m.RemoveError != nil {
		return m.RemoveError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized {
		return domain.ErrNotInitialized
	}

	delete(m.files, path)
	m.staged[path] = true
	return nil
}

// Head implements Repository.Head
func (m *MockRepository) Head() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return "", domain.ErrNotInitialized
	}
	return m.head, nil
}

// HasChanges implements Repository.HasChanges
func (m *MockRepository) HasChanges() (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return false, domain.ErrNotInitialized
	}
	return len(m.staged) > 0, nil
}

// SetFile sets a file in the mock repository (for testing)
func (m *MockRepository) SetFile(path string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = content
}

// GetFile gets a file from the mock repository (for testing)
func (m *MockRepository) GetFile(path string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.files[path]
	return data, ok
}

func generateMockHash() string {
	return "abcdef1234567890abcdef1234567890abcdef12"[:40-len("mock")] + "mock"
}
