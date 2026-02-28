package git

import (
	"bytes"
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

func TestGoGitRepository_Log(t *testing.T) {
	t.Run("includeFiles true returns changed files", func(t *testing.T) {
		repo, repoPath := setupTestRepo(t)

		// Commit 1: add file-a.txt
		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "file-a.txt"), []byte("a"), 0600))
		require.NoError(t, repo.Add("file-a.txt"))
		_, err := repo.Commit("add file-a")
		require.NoError(t, err)

		// Commit 2: add file-b.txt and modify file-a.txt
		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "file-b.txt"), []byte("b"), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "file-a.txt"), []byte("a2"), 0600))
		require.NoError(t, repo.Add("file-a.txt"))
		require.NoError(t, repo.Add("file-b.txt"))
		_, err = repo.Commit("add file-b and modify file-a")
		require.NoError(t, err)

		commits, err := repo.Log(10, true)
		require.NoError(t, err)
		require.Len(t, commits, 2)

		// Most recent commit first
		require.Equal(t, []string{"file-a.txt", "file-b.txt"}, commits[0].Files)
		require.Equal(t, []string{"file-a.txt"}, commits[1].Files)
	})

	t.Run("includeFiles false returns empty files", func(t *testing.T) {
		repo, repoPath := setupTestRepo(t)

		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("x"), 0600))
		require.NoError(t, repo.Add("file.txt"))
		_, err := repo.Commit("add file")
		require.NoError(t, err)

		commits, err := repo.Log(10, false)
		require.NoError(t, err)
		require.Len(t, commits, 1)
		require.Nil(t, commits[0].Files)
	})

	t.Run("root commit with includeFiles true", func(t *testing.T) {
		repo, repoPath := setupTestRepo(t)

		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "root.txt"), []byte("root"), 0600))
		require.NoError(t, repo.Add("root.txt"))
		_, err := repo.Commit("root commit")
		require.NoError(t, err)

		commits, err := repo.Log(1, true)
		require.NoError(t, err)
		require.Len(t, commits, 1)
		require.Equal(t, []string{"root.txt"}, commits[0].Files)
	})

	t.Run("limits number of commits", func(t *testing.T) {
		repo, repoPath := setupTestRepo(t)

		for i := 0; i < 5; i++ {
			require.NoError(t, os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte{byte(i)}, 0600))
			require.NoError(t, repo.Add("file.txt"))
			_, err := repo.Commit("commit")
			require.NoError(t, err)
		}

		commits, err := repo.Log(3, false)
		require.NoError(t, err)
		require.Len(t, commits, 3)
	})
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

func TestGoGitRepository_PackAllUnpackAll(t *testing.T) {
	t.Run("round-trip preserves objects", func(t *testing.T) {
		// Create source repo with commits
		srcRepo, srcPath := setupTestRepo(t)

		require.NoError(t, os.WriteFile(filepath.Join(srcPath, "file1.txt"), []byte("hello"), 0600))
		require.NoError(t, srcRepo.Add("file1.txt"))
		hash1, err := srcRepo.Commit("first commit")
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(srcPath, "file2.txt"), []byte("world"), 0600))
		require.NoError(t, srcRepo.Add("file2.txt"))
		_, err = srcRepo.Commit("second commit")
		require.NoError(t, err)

		// Pack all objects
		var buf bytes.Buffer
		err = srcRepo.PackAll(&buf)
		require.NoError(t, err)
		require.Greater(t, buf.Len(), 0, "packfile should not be empty")

		// Create destination repo and unpack
		dstRepo, dstPath := setupTestRepo(t)
		err = dstRepo.UnpackAll(&buf)
		require.NoError(t, err)

		// Set refs in destination and checkout
		srcRefs, err := srcRepo.GetAllRefs()
		require.NoError(t, err)

		for name, hash := range srcRefs {
			if name == "HEAD" {
				continue
			}
			require.NoError(t, dstRepo.SetRef(name, hash))
		}

		// Checkout the first commit to verify objects exist
		err = dstRepo.Checkout(hash1)
		require.NoError(t, err)

		// Verify file content
		content, err := os.ReadFile(filepath.Join(dstPath, "file1.txt"))
		require.NoError(t, err)
		require.Equal(t, "hello", string(content))
	})

	t.Run("empty repo produces no packfile data", func(t *testing.T) {
		repo, _ := setupTestRepo(t)

		var buf bytes.Buffer
		err := repo.PackAll(&buf)
		require.NoError(t, err)
		require.Equal(t, 0, buf.Len(), "empty repo should produce no packfile data")
	})

	t.Run("not initialized returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoPath := filepath.Join(tmpDir, "test-repo")
		repo, err := NewGoGitRepository(repoPath)
		require.NoError(t, err)

		var buf bytes.Buffer
		require.ErrorIs(t, repo.PackAll(&buf), domain.ErrNotInitialized)
		require.ErrorIs(t, repo.UnpackAll(&buf), domain.ErrNotInitialized)
	})
}

func TestGoGitRepository_PackAllUnpackAll_FullRoundTrip(t *testing.T) {
	// Create repo with multiple commits
	srcRepo, srcPath := setupTestRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(srcPath, "a.txt"), []byte("aaa"), 0600))
	require.NoError(t, srcRepo.Add("a.txt"))
	_, err := srcRepo.Commit("add a")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(srcPath, "b.txt"), []byte("bbb"), 0600))
	require.NoError(t, srcRepo.Add("b.txt"))
	_, err = srcRepo.Commit("add b")
	require.NoError(t, err)

	// Get source log
	srcCommits, err := srcRepo.Log(10, true)
	require.NoError(t, err)

	// Pack, unpack, verify log matches
	var buf bytes.Buffer
	require.NoError(t, srcRepo.PackAll(&buf))

	dstRepo, _ := setupTestRepo(t)
	require.NoError(t, dstRepo.UnpackAll(&buf))

	// Copy refs
	srcRefs, err := srcRepo.GetAllRefs()
	require.NoError(t, err)
	for name, hash := range srcRefs {
		if name == "HEAD" {
			continue
		}
		require.NoError(t, dstRepo.SetRef(name, hash))
	}

	// Checkout HEAD
	require.NoError(t, dstRepo.Checkout(srcRefs["HEAD"]))

	// Verify log matches
	dstCommits, err := dstRepo.Log(10, true)
	require.NoError(t, err)
	require.Equal(t, len(srcCommits), len(dstCommits))

	for i := range srcCommits {
		require.Equal(t, srcCommits[i].Hash, dstCommits[i].Hash)
		require.Equal(t, srcCommits[i].Message, dstCommits[i].Message)
		require.Equal(t, srcCommits[i].Files, dstCommits[i].Files)
	}
}

func TestGoGitRepository_GetAllRefs(t *testing.T) {
	repo, repoPath := setupTestRepo(t)
	hash := createInitialCommit(t, repo, repoPath)

	refs, err := repo.GetAllRefs()
	require.NoError(t, err)

	// Should contain HEAD and the default branch
	require.Equal(t, hash, refs["HEAD"])

	// Should have at least one branch ref
	hasBranchRef := false
	for name := range refs {
		if name != "HEAD" {
			hasBranchRef = true
			break
		}
	}
	require.True(t, hasBranchRef, "should have at least one branch ref")
}

func TestGoGitRepository_SetRef(t *testing.T) {
	repo, repoPath := setupTestRepo(t)
	hash := createInitialCommit(t, repo, repoPath)

	// Set a custom ref
	err := repo.SetRef("refs/heads/test-branch", hash)
	require.NoError(t, err)

	// Verify it appears in GetAllRefs
	refs, err := repo.GetAllRefs()
	require.NoError(t, err)
	require.Equal(t, hash, refs["refs/heads/test-branch"])
}

func TestGoGitRepository_DeleteRef(t *testing.T) {
	repo, repoPath := setupTestRepo(t)
	hash := createInitialCommit(t, repo, repoPath)

	// Set and then delete a ref
	err := repo.SetRef("refs/heads/temp", hash)
	require.NoError(t, err)

	err = repo.DeleteRef("refs/heads/temp")
	require.NoError(t, err)

	// Verify it's gone
	refs, err := repo.GetAllRefs()
	require.NoError(t, err)
	_, exists := refs["refs/heads/temp"]
	require.False(t, exists, "deleted ref should not appear")
}
