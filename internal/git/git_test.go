package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a new initialized test repository and returns it with its path.
func setupTestRepo(t *testing.T) (*GoGitRepository, string) {
	t.Helper()
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")
	repo, err := NewGoGitRepository(repoPath)
	require.NoError(t, err, "NewGoGitRepository")
	require.NoError(t, repo.Init(), "Init")
	return repo, repoPath
}

// createInitialCommit creates a test file and commits it, returning the commit hash.
func createInitialCommit(t *testing.T, repo *GoGitRepository, repoPath string) string {
	t.Helper()
	testFile := filepath.Join(repoPath, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0600), "WriteFile")
	require.NoError(t, repo.Add("test.txt"), "Add")
	hash, err := repo.Commit("Initial commit")
	require.NoError(t, err, "Commit")
	return hash
}

func TestGoGitRepository_Init(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")

	repo, err := NewGoGitRepository(repoPath)
	require.NoError(t, err, "NewGoGitRepository")
	require.NoError(t, repo.Init(), "Init")

	// Verify .git directory exists
	gitDir := filepath.Join(repoPath, ".git")
	_, err = os.Stat(gitDir)
	require.False(t, os.IsNotExist(err), ".git directory should be created")

	// Init again should be idempotent
	require.NoError(t, repo.Init(), "Second Init should not fail")
}

func TestGoGitRepository_CheckoutBranch(t *testing.T) {
	t.Run("fails for non-existent branch", func(t *testing.T) {
		repo, repoPath := setupTestRepo(t)
		createInitialCommit(t, repo, repoPath)

		err := repo.CheckoutBranch("nonexistent")
		require.Error(t, err, "CheckoutBranch should fail for non-existent branch")
		require.ErrorIs(t, err, domain.ErrGitError)
	})

	t.Run("succeeds for existing branch and preserves working tree", func(t *testing.T) {
		repo, repoPath := setupTestRepo(t)

		// Create initial commit on master/main
		testFile := filepath.Join(repoPath, "test.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("version 1"), 0600))
		require.NoError(t, repo.Add("test.txt"))
		_, err := repo.Commit("Version 1")
		require.NoError(t, err)

		// Get the default branch name (go-git creates "master" by default)
		branch, err := repo.GetDefaultBranch()
		require.NoError(t, err)

		// Create second commit
		require.NoError(t, os.WriteFile(testFile, []byte("version 2"), 0600))
		require.NoError(t, repo.Add("test.txt"))
		hash2, err := repo.Commit("Version 2")
		require.NoError(t, err)

		// Checkout specific commit (detached HEAD)
		require.NoError(t, repo.Checkout(hash2))

		// Checkout default branch - should succeed and preserve working tree
		err = repo.CheckoutBranch(branch)
		require.NoError(t, err, "CheckoutBranch should succeed for existing branch")

		// Verify working tree files are preserved (Keep: true behavior)
		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		require.Equal(t, "version 2", string(content), "Working tree should be preserved")
	})
}

func TestGoGitRepository_GetDefaultBranch(t *testing.T) {
	t.Run("finds default branch (main or master)", func(t *testing.T) {
		repo, repoPath := setupTestRepo(t)
		createInitialCommit(t, repo, repoPath)

		// go-git defaults to "master" for new repos
		branch, err := repo.GetDefaultBranch()
		require.NoError(t, err, "GetDefaultBranch")

		// go-git creates "master" by default
		require.True(t, branch == "master" || branch == "main",
			"Expected 'main' or 'master', got %q", branch)
	})

	t.Run("returns error when no default branch", func(t *testing.T) {
		repo, _ := setupTestRepo(t)

		// No commits, so no branch exists
		_, err := repo.GetDefaultBranch()
		require.Error(t, err, "GetDefaultBranch should fail when no branch exists")
		require.ErrorIs(t, err, domain.ErrRefNotFound)
	})

	t.Run("returns error when not initialized", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "test-repo")

		repo, err := NewGoGitRepository(repoPath)
		require.NoError(t, err)

		// Don't call Init()
		_, err = repo.GetDefaultBranch()
		require.ErrorIs(t, err, domain.ErrNotInitialized)
	})
}

func TestGoGitRepository_Checkout(t *testing.T) {
	repo, repoPath := setupTestRepo(t)
	testFile := filepath.Join(repoPath, "test.txt")

	// Create first commit
	require.NoError(t, os.WriteFile(testFile, []byte("version 1"), 0600))
	require.NoError(t, repo.Add("test.txt"))
	hash1, err := repo.Commit("Version 1")
	require.NoError(t, err)

	// Create second commit
	require.NoError(t, os.WriteFile(testFile, []byte("version 2"), 0600))
	require.NoError(t, repo.Add("test.txt"))
	_, err = repo.Commit("Version 2")
	require.NoError(t, err)

	// Checkout first commit
	require.NoError(t, repo.Checkout(hash1))

	// Verify file content
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	require.Equal(t, "version 1", string(content))

	// Checkout non-existent ref should fail
	err = repo.Checkout("nonexistent")
	require.Error(t, err, "Checkout should fail for non-existent ref")
	require.ErrorIs(t, err, domain.ErrRefNotFound)
}

func TestMockRepository_GetDefaultBranch(t *testing.T) {
	t.Run("returns main by default", func(t *testing.T) {
		mock := NewMockRepository()
		mock.Init()

		branch, err := mock.GetDefaultBranch()
		require.NoError(t, err)
		require.Equal(t, "main", branch)
	})

	t.Run("returns configured branch", func(t *testing.T) {
		mock := NewMockRepository()
		mock.Init()
		mock.DefaultBranch = "master"

		branch, err := mock.GetDefaultBranch()
		require.NoError(t, err)
		require.Equal(t, "master", branch)
	})

	t.Run("returns error when configured", func(t *testing.T) {
		mock := NewMockRepository()
		mock.Init()
		mock.GetDefaultBranchError = domain.Errorf(domain.ErrRefNotFound, "no branch")

		_, err := mock.GetDefaultBranch()
		require.Error(t, err, "GetDefaultBranch should return configured error")
	})

	t.Run("returns error when not initialized", func(t *testing.T) {
		mock := NewMockRepository()
		// Don't init

		_, err := mock.GetDefaultBranch()
		require.ErrorIs(t, err, domain.ErrNotInitialized)
	})
}
