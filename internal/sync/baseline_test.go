package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/stretchr/testify/require"
)

// newWorktree creates a linked git worktree of m's project (a .git file
// pointing into m's .git/worktrees/) that shares m's cache, like a second
// checkout of the same repo on the same machine.
func (m *testMachine) newWorktree(t *testing.T, name string, tracked string) *testMachine {
	t.Helper()
	wtGitDir := filepath.Join(m.projectDir, ".git", "worktrees", name)
	require.NoError(t, os.MkdirAll(wtGitDir, 0700))

	wtDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wtDir, ".git"), []byte("gitdir: "+wtGitDir+"\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(wtDir, ".envsecrets"), []byte(tracked), 0600))

	disc, err := project.NewDiscovery(wtDir)
	require.NoError(t, err)
	return m.env.newCheckout(t, wtDir, disc, m.cache.Path())
}

// A new worktree sharing the main checkout's cache must not inherit its
// baseline: with no secret files yet it needs a first pull, not a push that
// would delete everything from remote.
func TestSyncStatus_NewWorktreeSharingCacheNeedsFirstPull(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()

	b := a.newWorktree(t, "feature", ".env\n.env.local\n")
	require.Equal(t, domain.SyncActionFirstPull, b.status().Action)

	res := b.pull()
	require.Equal(t, 2, res.FilesCreated)
	got, err := os.ReadFile(filepath.Join(b.projectDir, ".env.local"))
	require.NoError(t, err)
	require.Equal(t, "L=1", string(got))

	// Each checkout now tracks its own baseline
	require.Equal(t, domain.SyncActionInSync, a.status().Action)
	require.Equal(t, domain.SyncActionInSync, b.status().Action)

	a.writeFile(".env", "X=2")
	a.push()
	require.Equal(t, domain.SyncActionPull, b.status().Action)
	require.Equal(t, domain.SyncActionInSync, a.status().Action)
}

// When no tracked file exists locally, status recommends restoring them and
// pull brings them back instead of keeping them "deleted".
func TestSyncStatus_AllFilesMissingRecommendsRestore(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()

	a.deleteFile(".env")
	a.deleteFile(".env.local")

	s := a.status()
	require.Equal(t, domain.SyncActionRestore, s.Action)
	require.Equal(t, []string{".env", ".env.local"}, s.Missing)

	res, err := a.syncer.Pull(context.Background(), PullOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, res.FilesCreated)
	require.Empty(t, res.FilesWithConflicts)
	require.Equal(t, domain.SyncActionInSync, a.status().Action)
}

// Deleting only some tracked files is still an ordinary local change.
func TestSyncStatus_PartialDeleteIsPush(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()

	a.deleteFile(".env.local")
	s := a.status()
	require.Equal(t, domain.SyncActionPush, s.Action)
	require.Equal(t, []string{".env.local"}, s.LocalChanges)
}

func TestPush_RefusesUnconfirmedDeletion(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()
	headBefore, err := a.cache.GetRemoteHead(context.Background())
	require.NoError(t, err)

	a.deleteFile(".env.local")
	a.writeFile(".env", "X=2")
	_, err = a.syncer.Push(context.Background(), PushOptions{Message: "unconfirmed"})

	var pending *domain.PendingDeletionsError
	require.True(t, errors.As(err, &pending))
	require.ErrorIs(t, err, domain.ErrUnconfirmedDeletion)
	require.Equal(t, []string{".env.local"}, pending.Files)

	// Nothing was published, not even the unrelated edit
	headAfter, err := a.cache.GetRemoteHead(context.Background())
	require.NoError(t, err)
	require.Equal(t, headBefore, headAfter)

	res, err := a.syncer.Push(context.Background(), PushOptions{Message: "confirmed", AllowDeletes: true})
	require.NoError(t, err)
	require.Equal(t, 1, res.FilesDeleted)
	require.Equal(t, 1, res.FilesUpdated)
}

// Dropping a file from tracking also deletes it remotely, so it needs the
// same opt-in (rm passes it).
func TestPush_RefusesUnconfirmedUntrackedDeletion(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()

	require.NoError(t, os.WriteFile(filepath.Join(a.projectDir, ".envsecrets"), []byte(".env\n"), 0600))
	_, err := a.syncer.Push(context.Background(), PushOptions{Message: "untrack"})
	var pending *domain.PendingDeletionsError
	require.True(t, errors.As(err, &pending))
	require.Equal(t, []string{".env.local"}, pending.Files)

	res, err := a.syncer.Push(context.Background(), PushOptions{Message: "untrack", AllowDeletes: true})
	require.NoError(t, err)
	require.Equal(t, 1, res.FilesDeleted)
}

func TestPush_DryRunReportsDeletionsWithoutRefusing(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()

	a.deleteFile(".env.local")
	res, err := a.syncer.Push(context.Background(), PushOptions{DryRun: true})
	require.NoError(t, err)
	require.Equal(t, 1, res.FilesDeleted)
}
