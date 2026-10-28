package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/git"
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

func TestPush_ConfirmedDeletionsAllowExactlyThoseFiles(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()

	a.deleteFile(".env.local")
	res, err := a.syncer.Push(context.Background(), PushOptions{
		Message:            "confirmed",
		ConfirmedDeletions: []string{".env.local"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.FilesDeleted)
}

// A file another machine publishes after this push's deletions were
// confirmed must not be deleted unseen: the confirmed set no longer covers
// everything the push would delete.
func TestPush_RefusesDeletionAddedAfterConfirmation(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	b := env.newMachine(t, []string{".env", ".env.local", ".env.new"})
	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()
	b.pull()

	// A confirms deleting .env.local; meanwhile B publishes .env.new,
	// which A doesn't track (so A's push would orphan-delete it)
	a.deleteFile(".env.local")
	b.writeFile(".env.new", "N=1")
	b.push()

	_, err := a.syncer.Push(context.Background(), PushOptions{
		Message:            "stale confirmation",
		ConfirmedDeletions: []string{".env.local"},
	})
	var pending *domain.PendingDeletionsError
	require.True(t, errors.As(err, &pending))
	require.ElementsMatch(t, []string{".env.local", ".env.new"}, pending.Files)

	// Remote still has B's file
	b.writeFile(".env", "X=1")
	require.Equal(t, domain.SyncActionInSync, b.status().Action)
}

// revert --push to a version that predates a tracked file deletes it; the
// rollback approves exactly the files absent at that ref. Approval must not
// depend on this run's local deletions, or retrying after a failed upload
// (file already gone locally) would be refused.
func TestRevertPush_PublishesRollbackDeletionsOnRetry(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	a.writeFile(".env", "X=1")
	first := a.push().CommitHash
	a.writeFile(".env.local", "L=1")
	a.push()

	revertPush := func() (*domain.PushResult, error) {
		_, err := a.syncer.Pull(ctx, PullOptions{Ref: first, Force: true})
		require.NoError(t, err)
		absent, err := a.syncer.FilesAbsentAtRef(first)
		require.NoError(t, err)
		require.Equal(t, []string{".env.local"}, absent)
		return a.syncer.Push(ctx, PushOptions{Message: "revert", Force: true, ConfirmedDeletions: absent})
	}

	// First attempt removes .env.local locally, then the upload fails
	env.storage.UploadError = errors.New("upload failed")
	_, err := revertPush()
	require.Error(t, err)
	env.storage.UploadError = nil
	require.NoFileExists(t, filepath.Join(a.projectDir, ".env.local"))

	pushRes, err := revertPush()
	require.NoError(t, err)
	require.Equal(t, 1, pushRes.FilesDeleted)

	c := env.newMachine(t, []string{".env", ".env.local"})
	c.pull()
	require.NoFileExists(t, filepath.Join(c.projectDir, ".env.local"))
}

// writeLegacyBaseline simulates a pre-upgrade machine: the only baseline is
// the shared marker in the cache, recording hash.
func writeLegacyBaseline(t *testing.T, m *testMachine, hash string) {
	t.Helper()
	gitRepo, err := git.NewGoGitRepository(m.cache.Path())
	require.NoError(t, err)
	legacy := cache.NewCacheWithRepo(m.env.repoInfo, m.env.storage, gitRepo, m.cache.Path())
	require.NoError(t, legacy.WriteLastSynced(hash))
	require.NoError(t, os.RemoveAll(m.baselineDir()))
}

// After upgrading, a stale checkout must not adopt the legacy marker another
// checkout wrote: its old content would look like local edits and sync would
// push them over newer remote content.
func TestLegacyBaseline_StaleCheckoutDoesNotAdopt(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	a.writeFile(".env", "X=1")
	a.push()

	// B is a second checkout sharing A's cache, holding the old content
	b := a.newWorktree(t, "stale", ".env\n")
	b.writeFile(".env", "X=1")

	a.writeFile(".env", "X=2")
	h2 := a.push().CommitHash
	writeLegacyBaseline(t, a, h2)
	require.NoError(t, os.RemoveAll(b.baselineDir()))

	// B runs first: its tree is at the old commit, so no adoption
	require.Equal(t, domain.SyncActionFirstPull, b.status().Action)
	_, err := b.syncer.Push(context.Background(), PushOptions{Message: "stale"})
	require.ErrorIs(t, err, domain.ErrDivergedHistory, "stale checkout must not push over remote")

	// A's tree matches h2, so A adopts it
	s := a.status()
	require.Equal(t, domain.SyncActionInSync, s.Action)
	require.Equal(t, h2, s.LastSynced)
}
