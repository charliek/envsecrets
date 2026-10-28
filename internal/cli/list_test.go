package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/git"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/charliek/envsecrets/internal/ui"
	"github.com/stretchr/testify/require"
)

// newTestCache creates a cache backed by a real git repository in a temp dir
func newTestCache(t *testing.T, repoInfo *domain.RepoInfo, store storage.Storage) *cache.Cache {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.NewGoGitRepository(dir)
	require.NoError(t, err)
	return cache.NewCacheWithRepo(repoInfo, store, repo, dir)
}

// pushTestFiles commits the given encrypted contents and syncs them to storage
func pushTestFiles(t *testing.T, c *cache.Cache, files map[string][]byte, message string) {
	t.Helper()
	require.NoError(t, c.Init())
	for name, content := range files {
		require.NoError(t, c.WriteEncrypted(name, content))
	}
	require.NoError(t, c.StageAll())
	_, err := c.Commit(message)
	require.NoError(t, err)
	require.NoError(t, c.SyncToStorage(context.Background()))
}

func TestListCacheFiles_ListsFilesFromPackfile(t *testing.T) {
	store := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}

	// Push from one machine's cache...
	pushTestFiles(t, newTestCache(t, repoInfo, store), map[string][]byte{
		".env":              []byte("encrypted-env"),
		"config/secrets.md": []byte("encrypted-secrets"),
	}, "initial")

	// ...and list from a fresh cache, as another machine would
	var stdout bytes.Buffer
	out := ui.NewOutputWithWriters(&stdout, &bytes.Buffer{}, false, false)
	err := listCacheFiles(context.Background(), newTestCache(t, repoInfo, store), out, "owner/repo")
	require.NoError(t, err)

	got := stdout.String()
	require.Contains(t, got, "Files in owner/repo:")
	require.Contains(t, got, ".env")
	require.Contains(t, got, "config/secrets.md")
	require.Contains(t, got, "2 file(s)")
	require.NotContains(t, got, "objects.pack")
}

func TestListCacheFiles_JSON(t *testing.T) {
	store := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}

	source := newTestCache(t, repoInfo, store)
	pushTestFiles(t, source, map[string][]byte{".env": []byte("v1"), ".env.local": []byte("local")}, "first")
	pushTestFiles(t, source, map[string][]byte{".env": []byte("version-two")}, "second")

	var stdout bytes.Buffer
	out := ui.NewOutputWithWriters(&stdout, &bytes.Buffer{}, false, true)
	err := listCacheFiles(context.Background(), newTestCache(t, repoInfo, store), out, "owner/repo")
	require.NoError(t, err)

	var files []struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		Updated string `json:"updated"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &files))
	require.Len(t, files, 2)
	require.Equal(t, ".env", files[0].Name)
	require.Equal(t, int64(len("version-two")), files[0].Size)
	require.NotEmpty(t, files[0].Updated)
	require.Equal(t, ".env.local", files[1].Name)
	require.NotEmpty(t, files[1].Updated)
}

func TestListCacheFiles_MissingRepo(t *testing.T) {
	store := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "missing"}

	var stdout bytes.Buffer
	out := ui.NewOutputWithWriters(&stdout, &bytes.Buffer{}, false, false)
	err := listCacheFiles(context.Background(), newTestCache(t, repoInfo, store), out, "owner/missing")
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "No files found in owner/missing")
}

func TestListCacheFiles_MissingRepoJSON(t *testing.T) {
	store := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "missing"}

	var stdout bytes.Buffer
	out := ui.NewOutputWithWriters(&stdout, &bytes.Buffer{}, false, true)
	err := listCacheFiles(context.Background(), newTestCache(t, repoInfo, store), out, "owner/missing")
	require.NoError(t, err)
	require.JSONEq(t, "[]", stdout.String())
}
