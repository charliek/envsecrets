package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestDiscovery_GitDir(t *testing.T) {
	t.Run("clone uses .git directory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0700))
		d, err := NewDiscovery(root)
		require.NoError(t, err)
		got, err := d.GitDir()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(d.ProjectRoot(), ".git"), got)
	})

	t.Run("worktree follows absolute gitdir", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(t.TempDir(), "main", ".git", "worktrees", "wt")
		require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: "+target+"\n"), 0600))
		d, err := NewDiscovery(root)
		require.NoError(t, err)
		got, err := d.GitDir()
		require.NoError(t, err)
		require.Equal(t, target, got)
	})

	t.Run("relative gitdir resolves from the checkout", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../main/.git/worktrees/wt"), 0600))
		d, err := NewDiscovery(root)
		require.NoError(t, err)
		got, err := d.GitDir()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(filepath.Dir(d.ProjectRoot()), "main", ".git", "worktrees", "wt"), got)
	})

	t.Run("malformed .git file errors", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("nonsense"), 0600))
		d, err := NewDiscovery(root)
		require.NoError(t, err)
		_, err = d.GitDir()
		require.ErrorIs(t, err, domain.ErrGitError)
	})
}

func TestDiscovery_SyncBaselinePath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0700))
	d, err := NewDiscovery(root)
	require.NoError(t, err)

	got, err := d.SyncBaselinePath(&domain.RepoInfo{Owner: "acme", Name: "app"})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(d.ProjectRoot(), ".git", "envsecrets", "acme", "app", "last-synced"), got)

	_, err = d.SyncBaselinePath(&domain.RepoInfo{Owner: "..", Name: ".."})
	require.Error(t, err, "repo names must not escape the git dir")
}
