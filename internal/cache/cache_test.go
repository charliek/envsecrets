package cache

import (
	"context"
	"strings"
	"testing"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/git"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestCache_WriteReadEncrypted(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "test", Name: "repo"}
	cache := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, "/tmp/cache")

	// Write encrypted content
	content := []byte("encrypted data")
	err := cache.WriteEncrypted(".env", content)
	require.NoError(t, err)

	// Read it back
	readContent, err := cache.ReadEncrypted(".env")
	require.NoError(t, err)
	require.Equal(t, content, readContent)
}

func TestCache_Commit(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "test", Name: "repo"}
	cache := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, "/tmp/cache")

	// Write a file
	err := cache.WriteEncrypted(".env", []byte("data"))
	require.NoError(t, err)

	// Stage and commit
	mockRepo.Add(".env.age")
	hash, err := cache.Commit("test commit")
	require.NoError(t, err)
	require.NotEmpty(t, hash)
}

func TestCache_SyncToStoragePackfile(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	// Write and commit a file
	err := c.WriteEncrypted(".env", []byte("encrypted"))
	require.NoError(t, err)

	mockRepo.Add(".env.age")
	_, err = c.Commit("initial")
	require.NoError(t, err)

	ctx := context.Background()
	err = c.SyncToStorage(ctx)
	require.NoError(t, err)

	// Verify HEAD was uploaded
	exists, err := mockStorage.Exists(ctx, "owner/repo/HEAD")
	require.NoError(t, err)
	require.True(t, exists)

	// Verify refs was uploaded
	exists, err = mockStorage.Exists(ctx, "owner/repo/refs")
	require.NoError(t, err)
	require.True(t, exists)

	// HEAD should contain a valid hash
	headData, ok := mockStorage.GetData("owner/repo/HEAD")
	require.True(t, ok)
	require.Len(t, strings.TrimSpace(string(headData)), 40)
}

func TestCache_SyncFromStoragePackfile(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	// Create a commit so there's a valid hash to checkout
	mockRepo.SetFile(".env.age", []byte("encrypted"))
	mockRepo.Add(".env.age")
	hash, err := mockRepo.Commit("initial")
	require.NoError(t, err)

	// Seed mock storage with pack data, refs, and HEAD using the real hash
	mockStorage.SetData("owner/repo/objects.pack", []byte("mock-pack-data"))
	mockStorage.SetData("owner/repo/refs", []byte("refs/heads/master "+hash+"\n"))
	mockStorage.SetData("owner/repo/HEAD", []byte(hash))

	ctx := context.Background()
	err = c.SyncFromStorage(ctx)
	require.NoError(t, err)

	// Verify SetRef was called for the branch ref
	refs, err := mockRepo.GetAllRefs()
	require.NoError(t, err)
	require.Equal(t, hash, refs["refs/heads/master"])
}

func TestCache_SyncFromStorage_EmptyRepo(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	// No data in storage - should succeed (empty repo)
	ctx := context.Background()
	err := c.SyncFromStorage(ctx)
	require.NoError(t, err)
}

func TestCache_DeleteRemote(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	mockRepo.Init()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	// Set up packfile-format files
	ctx := context.Background()
	mockStorage.SetData("owner/repo/objects.pack", []byte("pack"))
	mockStorage.SetData("owner/repo/refs", []byte("refs"))
	mockStorage.SetData("owner/repo/HEAD", []byte("head"))

	err := c.DeleteRemote(ctx)
	require.NoError(t, err)

	// All files should be deleted
	require.Equal(t, 0, mockStorage.Count())
}

func TestCache_RepoInfoPath(t *testing.T) {
	repoInfo := &domain.RepoInfo{Owner: "acme", Name: "myapp"}
	require.Equal(t, "acme/myapp", repoInfo.CachePath())
}

func TestCache_SecureJoinPath(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "test", Name: "repo"}
	cache := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, "/tmp/cache")

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "normal file",
			path:    ".env.age",
			wantErr: false,
		},
		{
			name:    "nested file",
			path:    "config/prod.env.age",
			wantErr: false,
		},
		{
			name:        "path traversal attempt",
			path:        "../../../etc/passwd",
			wantErr:     true,
			errContains: "path traversal not allowed",
		},
		{
			name:        "path traversal with normal prefix",
			path:        "config/../../../etc/passwd",
			wantErr:     true,
			errContains: "path traversal not allowed",
		},
		{
			name:    "double dots in filename (not traversal)",
			path:    "file..name.age",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := cache.secureJoinPath(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, result)
				// Verify result is under base directory
				require.Contains(t, result, "/tmp/cache")
			}
		})
	}
}

func TestCache_Validate_NonExistent(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockStorage := storage.NewMockStorage()

	// Use a non-existent directory
	repoInfo := &domain.RepoInfo{Owner: "test", Name: "repo"}
	cache := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, "/nonexistent/path/to/cache")

	// Validate should report non-existent
	health := cache.Validate()
	require.False(t, health.Exists, "cache directory should not exist")
	require.False(t, health.GitValid, "git should not be valid")
	require.Nil(t, health.Error, "should have no error for non-existent")
}

func TestCache_Validate_CorruptedNoGit(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockStorage := storage.NewMockStorage()

	// Create a directory without .git
	dir := t.TempDir()
	repoInfo := &domain.RepoInfo{Owner: "test", Name: "repo"}
	cache := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, dir)

	// Validate should report corrupted (no .git directory)
	health := cache.Validate()
	require.True(t, health.Exists, "cache directory should exist")
	require.False(t, health.GitValid, "git should not be valid")
	require.NotNil(t, health.Error, "should have error for missing .git")
	require.Contains(t, health.Error.Error(), ".git directory")
}
