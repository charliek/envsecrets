package cache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliek/envsecrets/internal/constants"
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

	// Seed mock storage with pack data, refs, FORMAT, and HEAD using the real hash
	mockStorage.SetData("owner/repo/objects.pack", []byte("mock-pack-data"))
	mockStorage.SetData("owner/repo/refs", []byte("refs/heads/master "+hash+"\n"))
	mockStorage.SetData("owner/repo/FORMAT", []byte("1"))
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

func TestDetectRemoteVersion_ValidFormat(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	mockStorage.SetData("owner/repo/FORMAT", []byte("1"))

	ctx := context.Background()
	info, err := c.DetectRemoteVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, info.Version)
	require.True(t, info.Detected)
}

func TestDetectRemoteVersion_NoFormatFile(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	ctx := context.Background()
	info, err := c.DetectRemoteVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, info.Version)
	require.False(t, info.Detected)
}

func TestDetectRemoteVersion_WhitespaceFormat(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	mockStorage.SetData("owner/repo/FORMAT", []byte("1\n"))

	ctx := context.Background()
	info, err := c.DetectRemoteVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, info.Version)
	require.True(t, info.Detected)
}

func TestDetectRemoteVersion_EmptyFormat(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	mockStorage.SetData("owner/repo/FORMAT", []byte(""))

	ctx := context.Background()
	_, err := c.DetectRemoteVersion(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty")
}

func TestDetectRemoteVersion_NonNumericFormat(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	mockStorage.SetData("owner/repo/FORMAT", []byte("abc"))

	ctx := context.Background()
	_, err := c.DetectRemoteVersion(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-numeric")
}

func TestDetectRemoteVersion_NegativeVersion(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	mockStorage.SetData("owner/repo/FORMAT", []byte("-1"))

	ctx := context.Background()
	_, err := c.DetectRemoteVersion(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid version")
}

func TestDetectRemoteVersion_ZeroVersion(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	mockStorage.SetData("owner/repo/FORMAT", []byte("0"))

	ctx := context.Background()
	_, err := c.DetectRemoteVersion(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid version")
}

func TestDetectRemoteVersion_FutureVersion(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	mockStorage.SetData("owner/repo/FORMAT", []byte("99"))

	ctx := context.Background()
	info, err := c.DetectRemoteVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, 99, info.Version)
	require.True(t, info.Detected)
}

func TestCheckVersionCompatibility_NoFormat(t *testing.T) {
	info := &domain.StorageFormatInfo{Version: 0, Detected: false}
	err := CheckVersionCompatibility(info)
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrVersionUnknown))
	require.Contains(t, err.Error(), "envsecrets delete")
}

func TestCheckVersionCompatibility_Current(t *testing.T) {
	info := &domain.StorageFormatInfo{Version: constants.CurrentFormatVersion, Detected: true}
	err := CheckVersionCompatibility(info)
	require.NoError(t, err)
}

func TestCheckVersionCompatibility_TooNew(t *testing.T) {
	info := &domain.StorageFormatInfo{Version: 99, Detected: true}
	err := CheckVersionCompatibility(info)
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrVersionTooNew))
	require.Contains(t, err.Error(), "upgrade envsecrets")
}

func TestWriteFormatMarker(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	ctx := context.Background()
	err := c.WriteFormatMarker(ctx, 1)
	require.NoError(t, err)

	data, ok := mockStorage.GetData("owner/repo/FORMAT")
	require.True(t, ok)
	require.Equal(t, "1", string(data))
}

func TestSyncToStorage_WritesFormatBeforeHead(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	err := c.WriteEncrypted(".env", []byte("encrypted"))
	require.NoError(t, err)

	mockRepo.Add(".env.age")
	_, err = c.Commit("initial")
	require.NoError(t, err)

	ctx := context.Background()
	err = c.SyncToStorage(ctx)
	require.NoError(t, err)

	// Verify FORMAT was uploaded
	exists, err := mockStorage.Exists(ctx, "owner/repo/FORMAT")
	require.NoError(t, err)
	require.True(t, exists)

	data, ok := mockStorage.GetData("owner/repo/FORMAT")
	require.True(t, ok)
	require.Equal(t, "1", string(data))

	// Verify FORMAT was uploaded before HEAD
	formatIdx := -1
	headIdx := -1
	for i, path := range mockStorage.UploadOrder {
		if path == "owner/repo/FORMAT" {
			formatIdx = i
		}
		if path == "owner/repo/HEAD" {
			headIdx = i
		}
	}
	require.NotEqual(t, -1, formatIdx, "FORMAT should be uploaded")
	require.NotEqual(t, -1, headIdx, "HEAD should be uploaded")
	require.Less(t, formatIdx, headIdx, "FORMAT must be uploaded before HEAD")
}

func TestDeleteRemote_IncludesFormat(t *testing.T) {
	mockStorage := storage.NewMockStorage()
	mockRepo := git.NewMockRepository()
	mockRepo.Init()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	ctx := context.Background()
	mockStorage.SetData("owner/repo/objects.pack", []byte("pack"))
	mockStorage.SetData("owner/repo/refs", []byte("refs"))
	mockStorage.SetData("owner/repo/FORMAT", []byte("1"))
	mockStorage.SetData("owner/repo/HEAD", []byte("head"))

	err := c.DeleteRemote(ctx)
	require.NoError(t, err)

	require.Equal(t, 0, mockStorage.Count())
}

func TestSyncFromStorage_EmptyRepo_NoVersionCheck(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	// No data in storage - should succeed without version check
	ctx := context.Background()
	err := c.SyncFromStorage(ctx)
	require.NoError(t, err)
}

func TestSyncFromStorage_MissingFormat_WithData(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	// Has HEAD but no FORMAT — should fail with ErrVersionUnknown
	hash, err := mockRepo.Commit("initial")
	require.NoError(t, err)
	mockStorage.SetData("owner/repo/HEAD", []byte(hash))

	ctx := context.Background()
	err = c.SyncFromStorage(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrVersionUnknown))
}

func TestSyncFromStorage_FutureVersion(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()

	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	hash, err := mockRepo.Commit("initial")
	require.NoError(t, err)
	mockStorage.SetData("owner/repo/HEAD", []byte(hash))
	mockStorage.SetData("owner/repo/FORMAT", []byte("99"))

	ctx := context.Background()
	err = c.SyncFromStorage(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrVersionTooNew))
}

func TestCache_LastSynced_RoundTrip(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	hash := strings.Repeat("a", 40)
	require.NoError(t, c.WriteLastSynced(hash))

	got, mtime, err := c.ReadLastSynced()
	require.NoError(t, err)
	require.Equal(t, hash, got)
	require.False(t, mtime.IsZero(), "mtime must be populated for present marker")
}

func TestCache_LastSynced_MissingIsEmpty(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	// No marker written yet — must NOT error
	got, mtime, err := c.ReadLastSynced()
	require.NoError(t, err)
	require.Empty(t, got)
	require.True(t, mtime.IsZero())
}

func TestCache_LastSynced_CorruptIsEmpty(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, dir)

	// Write garbage directly bypassing the helper to simulate corruption.
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, LastSyncedFileName), []byte("not-a-hash"), 0600))

	got, _, err := c.ReadLastSynced()
	require.NoError(t, err, "corrupt marker must not propagate as error")
	require.Empty(t, got)
}

func TestCache_LastSynced_RejectsInvalidHash(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	require.Error(t, c.WriteLastSynced("nope"))
	require.Error(t, c.WriteLastSynced(""))
	require.Error(t, c.WriteLastSynced(strings.Repeat("z", 40))) // non-hex
}

func TestCache_Reset_ClearsLastSynced(t *testing.T) {
	mockRepo := git.NewMockRepository()
	mockRepo.Init()
	mockStorage := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}
	c := NewCacheWithRepo(repoInfo, mockStorage, mockRepo, t.TempDir())

	// Set a marker, then Reset (no remote → goes to fresh-init path)
	require.NoError(t, c.WriteLastSynced(strings.Repeat("a", 40)))

	ctx := context.Background()
	require.NoError(t, c.Reset(ctx))

	got, _, err := c.ReadLastSynced()
	require.NoError(t, err)
	require.Empty(t, got, "Reset must clear the LAST_SYNCED marker — it implies the cache is no longer trusted")
}
