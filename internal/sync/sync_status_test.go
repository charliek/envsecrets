package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliek/envsecrets/internal/cache"
	"github.com/charliek/envsecrets/internal/crypto"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/git"
	"github.com/charliek/envsecrets/internal/project"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/stretchr/testify/require"
)

// testEnv represents a shared "remote" (mock storage) that multiple test
// machines push/pull against.
type testEnv struct {
	storage   *storage.MockStorage
	encrypter *crypto.MockEncrypter
	repoInfo  *domain.RepoInfo
}

func newTestEnv() *testEnv {
	return &testEnv{
		storage:   storage.NewMockStorage(),
		encrypter: crypto.NewMockEncrypter(),
		repoInfo:  &domain.RepoInfo{Owner: "owner", Name: "repo"},
	}
}

// testMachine simulates one developer's machine: its own project tree, its
// own cache dir, but the same remote.
type testMachine struct {
	t          *testing.T
	env        *testEnv
	projectDir string
	cache      *cache.Cache
	discovery  *project.Discovery
	syncer     *Syncer
}

func (env *testEnv) newMachine(t *testing.T, tracked []string) *testMachine {
	t.Helper()

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".git"), 0700))

	envSecretsPath := filepath.Join(projectDir, ".envsecrets")
	require.NoError(t, os.WriteFile(envSecretsPath, []byte(strings.Join(tracked, "\n")+"\n"), 0600))

	disc, err := project.NewDiscovery(projectDir)
	require.NoError(t, err)

	cacheDir := t.TempDir()
	gitRepo, err := git.NewGoGitRepository(cacheDir)
	require.NoError(t, err)
	require.NoError(t, gitRepo.Init())
	c := cache.NewCacheWithRepo(env.repoInfo, env.storage, gitRepo, cacheDir)

	syncer := NewSyncer(disc, env.repoInfo, env.storage, env.encrypter, c)

	return &testMachine{
		t:          t,
		env:        env,
		projectDir: projectDir,
		cache:      c,
		discovery:  disc,
		syncer:     syncer,
	}
}

func (m *testMachine) writeFile(name, content string) {
	m.t.Helper()
	require.NoError(m.t, os.WriteFile(filepath.Join(m.projectDir, name), []byte(content), 0600))
}

func (m *testMachine) deleteFile(name string) {
	m.t.Helper()
	require.NoError(m.t, os.Remove(filepath.Join(m.projectDir, name)))
}

func (m *testMachine) push() *domain.PushResult {
	m.t.Helper()
	res, err := m.syncer.Push(context.Background(), PushOptions{Message: "test"})
	require.NoError(m.t, err)
	return res
}

func (m *testMachine) pull() *domain.PullResult {
	m.t.Helper()
	res, err := m.syncer.Pull(context.Background(), PullOptions{Force: true})
	require.NoError(m.t, err)
	return res
}

func (m *testMachine) status() *domain.SyncStatus {
	m.t.Helper()
	s, err := m.syncer.GetSyncStatus(context.Background())
	require.NoError(m.t, err)
	return s
}

// TestSyncStatus_FirstPushInit: a fresh machine with no remote yet.
func TestSyncStatus_FirstPushInit(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	a.writeFile(".env", "X=1")

	s := a.status()
	require.Equal(t, domain.SyncActionFirstPushInit, s.Action)
	require.Empty(t, s.LocalHead)
	require.Empty(t, s.RemoteHead)
}

// TestSyncStatus_NothingTracked: empty .envsecrets.
func TestSyncStatus_NothingTracked(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{}) // empty tracking list

	s := a.status()
	require.Equal(t, domain.SyncActionNothingTracked, s.Action)
}

// TestSyncStatus_InSync: push, then status reports in-sync with no changes.
func TestSyncStatus_InSync(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	a.writeFile(".env", "X=1")
	a.push()

	s := a.status()
	require.Equal(t, domain.SyncActionInSync, s.Action)
	require.NotEmpty(t, s.LocalHead)
	require.Equal(t, s.LocalHead, s.RemoteHead)
	require.Equal(t, s.LocalHead, s.LastSynced, "push must update LAST_SYNCED to the new commit")
}

// TestSyncStatus_Push: machine has unpushed local edits.
func TestSyncStatus_Push(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	a.writeFile(".env", "X=1")
	a.push()

	// Edit locally without pushing
	a.writeFile(".env", "X=2")

	s := a.status()
	require.Equal(t, domain.SyncActionPush, s.Action, "unpushed local edits should recommend push")
	require.Equal(t, []string{".env"}, s.LocalChanges)
	require.Empty(t, s.RemoteChanges)
	require.Empty(t, s.Conflicts)
}

// TestSyncStatus_Pull: another machine pushed; this machine has no local edits.
func TestSyncStatus_Pull(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	// A pushes initial state, B pulls it (so both share a baseline at v1)
	a.writeFile(".env", "X=1")
	a.push()
	b.pull()

	// A pushes a change. B has not pulled yet.
	a.writeFile(".env", "X=2")
	a.push()

	s := b.status()
	require.Equal(t, domain.SyncActionPull, s.Action,
		"remote moved while this machine had no local edits — expect pull recommendation")
	require.Equal(t, []string{".env"}, s.RemoteChanges)
	require.Empty(t, s.LocalChanges)
	require.Empty(t, s.Conflicts)
}

// TestSyncStatus_PullThenPush: both sides changed but on disjoint files.
func TestSyncStatus_PullThenPush(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	b := env.newMachine(t, []string{".env", ".env.local"})

	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()
	b.pull()

	// A changes .env; B (independently) changes .env.local
	a.writeFile(".env", "X=2")
	a.push()

	b.writeFile(".env.local", "L=2") // B's local-only edit, not pushed yet

	s := b.status()
	require.Equal(t, domain.SyncActionPullThenPush, s.Action,
		"disjoint changes on both sides should be a fast-forward + push")
	require.Equal(t, []string{".env"}, s.RemoteChanges)
	require.Equal(t, []string{".env.local"}, s.LocalChanges)
	require.Empty(t, s.Conflicts)
}

// TestSyncStatus_Reconcile: same file changed on both sides to different content.
func TestSyncStatus_Reconcile(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()
	b.pull()

	// Both machines edit the SAME file with DIFFERENT content
	a.writeFile(".env", "X=2-from-a")
	a.push()

	b.writeFile(".env", "X=2-from-b") // B's local edit, never pushed

	s := b.status()
	require.Equal(t, domain.SyncActionReconcile, s.Action,
		"same file edited on both sides to different content must require reconciliation")
	require.Equal(t, []string{".env"}, s.LocalChanges)
	require.Equal(t, []string{".env"}, s.RemoteChanges)
	require.Equal(t, []string{".env"}, s.Conflicts)
}

// TestSyncStatus_BothEditedToSameContent: race-equivalent — both machines
// happened to make the identical edit, so there's no real conflict.
func TestSyncStatus_BothEditedToSameContent(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()
	b.pull()

	// Both make the exact same edit
	a.writeFile(".env", "X=2")
	a.push()
	b.writeFile(".env", "X=2") // identical content

	s := b.status()
	require.NotEqual(t, domain.SyncActionReconcile, s.Action,
		"identical edits on both sides are not a conflict")
	require.Empty(t, s.Conflicts)
}

// TestPush_DivergedOverlap_RefusesWithoutForce: the multi-machine safety net.
func TestPush_DivergedOverlap_RefusesWithoutForce(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()
	b.pull() // baseline shared at v1

	// A pushes an update; B independently edits the same file but never pulls.
	a.writeFile(".env", "X=2-from-a")
	a.push()
	b.writeFile(".env", "X=2-from-b")

	_, err := b.syncer.Push(context.Background(), PushOptions{Message: "would clobber"})
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrDivergedHistory),
		"push must refuse when remote moved since this machine's last sync AND files overlap")
}

// TestPush_DivergedOverlap_AllowedWithForce: --force is the documented escape hatch.
func TestPush_DivergedOverlap_AllowedWithForce(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()
	b.pull()

	a.writeFile(".env", "X=2-from-a")
	a.push()
	b.writeFile(".env", "X=2-from-b")

	_, err := b.syncer.Push(context.Background(), PushOptions{Message: "force", Force: true})
	require.NoError(t, err, "--force must bypass the divergence check")
}

// TestPush_DivergedNoOverlap_Proceeds: remote moved on file X, local changed
// only file Y → push proceeds (the existing fast-forward in syncBeforePush
// already aligned, so this is effectively a fast-forward + new commit).
func TestPush_DivergedNoOverlap_Proceeds(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	b := env.newMachine(t, []string{".env", ".env.local"})

	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()
	b.pull()

	// A pushes a change to .env. B independently changes .env.local.
	a.writeFile(".env", "X=2")
	a.push()
	b.writeFile(".env.local", "L=2")

	_, err := b.syncer.Push(context.Background(), PushOptions{Message: "ff"})
	require.NoError(t, err, "non-overlapping divergence must not block push")
}

// TestPull_UpdatesLastSynced: full-HEAD pull writes the marker.
func TestPull_UpdatesLastSynced(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()

	before, _, _ := b.cache.ReadLastSynced()
	require.Empty(t, before)

	b.pull()

	after, _, _ := b.cache.ReadLastSynced()
	require.NotEmpty(t, after, "full-HEAD pull must record this machine's new sync baseline")

	// And it should equal A's commit (= remote HEAD at the time of pull)
	remoteHead, _ := a.cache.GetRemoteHead(context.Background())
	require.Equal(t, remoteHead, after)
}

// TestPull_RefDoesNotUpdateLastSynced: a historical-ref checkout should NOT
// change this machine's "last synced to remote HEAD" baseline.
func TestPull_RefDoesNotUpdateLastSynced(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()
	firstHash, _ := a.cache.GetRemoteHead(context.Background())

	a.writeFile(".env", "X=2")
	a.push()

	b.pull() // gives B a clean baseline at the latest hash

	baselineBefore, _, _ := b.cache.ReadLastSynced()
	require.NotEmpty(t, baselineBefore)

	// Now check out a historical ref
	_, err := b.syncer.Pull(context.Background(), PullOptions{Ref: firstHash, Force: true})
	require.NoError(t, err)

	baselineAfter, _, _ := b.cache.ReadLastSynced()
	require.Equal(t, baselineBefore, baselineAfter,
		"pull --ref must not overwrite LAST_SYNCED with an arbitrary historical commit")
}

// TestSyncStatus_FirstPull_NoBaseline_TreeMatches: a fresh / post-Reset
// machine whose working tree happens to match remote still has no
// LAST_SYNCED marker. status MUST recommend FirstPull (not InSync), because
// push will refuse without a baseline. This is the case CodeRabbit flagged.
func TestSyncStatus_FirstPull_NoBaseline_TreeMatches(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()

	// B starts fresh (never pulled / pushed). Simulate a working tree that
	// happens to match remote anyway — e.g. someone manually copied the
	// secret in, or the marker got cleared while the file content didn't.
	b.writeFile(".env", "X=1")

	s := b.status()
	require.Equal(t, domain.SyncActionFirstPull, s.Action,
		"missing LAST_SYNCED must recommend FirstPull even when heads/content already match")
	require.Empty(t, s.LastSynced)
}

// TestSyncStatus_FirstPull_NoBaseline_TreeDiffers: same case but with a
// working-tree mismatch — also FirstPull.
func TestSyncStatus_FirstPull_NoBaseline_TreeDiffers(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()

	b.writeFile(".env", "X=different")

	s := b.status()
	require.Equal(t, domain.SyncActionFirstPull, s.Action)
}

// TestPush_WarnsOnLastSyncedWriteFailure: when WriteLastSynced fails after a
// successful remote push, the result must surface a warning so callers know
// the next push could spuriously hit ErrDivergedHistory. We simulate the
// failure by making the cache's .git directory read-only mid-push.
//
// On macOS, root mode bits on a directory don't always block writes for the
// owner, so we skip the test if we can't actually trigger the failure.
func TestPush_WarnsOnLastSyncedWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod-based denial doesn't apply")
	}

	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	a.writeFile(".env", "X=1")

	// First push to ensure the cache .git dir exists.
	_, err := a.syncer.Push(context.Background(), PushOptions{Message: "init"})
	require.NoError(t, err)

	// Make the .git dir read-only so the LAST_SYNCED tmp+rename can't write.
	gitDir := filepath.Join(a.cache.Path(), ".git")
	require.NoError(t, os.Chmod(gitDir, 0500))
	t.Cleanup(func() { _ = os.Chmod(gitDir, 0700) })

	// Confirm we actually broke writes; if the OS still lets us write, skip.
	probe := filepath.Join(gitDir, ".write-probe")
	if err := os.WriteFile(probe, []byte("x"), 0600); err == nil {
		_ = os.Remove(probe)
		t.Skip("filesystem ignores chmod 500 on directories — cannot simulate write failure")
	}

	a.writeFile(".env", "X=2")
	res, err := a.syncer.Push(context.Background(), PushOptions{Message: "with-failure"})
	require.NoError(t, err, "push must still succeed remotely even if marker write fails")
	require.NotEmpty(t, res.Warning, "marker write failure must produce a Warning")
	require.Contains(t, res.Warning, "envsecrets pull",
		"warning should tell the user how to repair")
}

// TestSyncStatus_CorruptBaselineRecommendsFirstPull: when LAST_SYNCED points
// at a commit the cache no longer has, GetSyncStatus must NOT blow up — it
// should treat the baseline as missing and recommend FirstPull, exactly the
// same way it handles an empty marker. Without this the user gets a hard
// error from `status` they can't act on.
func TestSyncStatus_CorruptBaselineRecommendsFirstPull(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	a.writeFile(".env", "X=1")
	a.push()

	// Point LAST_SYNCED at a hash that does not exist in this cache.
	bogus := "deadbeef" + strings.Repeat("0", 32)
	require.NoError(t, a.cache.WriteLastSynced(bogus))

	s := a.status()
	require.Equal(t, domain.SyncActionFirstPull, s.Action,
		"corrupt baseline (LAST_SYNCED → unknown commit) must surface as FirstPull, not a hard error")
}

// TestPush_CorruptBaselineRefuses: same scenario, push side. The push must
// refuse with an actionable message pointing at pull.
func TestPush_CorruptBaselineRefuses(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	a.writeFile(".env", "X=1")
	a.push()

	// Edit locally and corrupt the marker.
	a.writeFile(".env", "X=2")
	bogus := "deadbeef" + strings.Repeat("0", 32)
	require.NoError(t, a.cache.WriteLastSynced(bogus))

	_, err := a.syncer.Push(context.Background(), PushOptions{Message: "should refuse"})
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrDivergedHistory),
		"corrupt baseline must surface as ErrDivergedHistory so the user knows to pull first")
}

// TestPull_StaleTreeIsNotConflict: when this machine's working tree matches
// the LAST_SYNCED baseline (i.e. no local edits) but remote has moved, pull
// should overwrite without prompting. Without LAST_SYNCED awareness the user
// would be forced to use --force on every catch-up pull.
func TestPull_StaleTreeIsNotConflict(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()
	b.pull() // baseline = first commit, working tree = X=1

	// A pushes a new value. B has NOT edited locally; tree still X=1.
	a.writeFile(".env", "X=2")
	a.push()

	// B pulls without Force, no resolver — old behavior would fail with
	// ErrConflict because the tree disagrees with remote. New behavior
	// notices the tree matches the baseline and accepts the overwrite.
	res, err := b.syncer.Pull(context.Background(), PullOptions{})
	require.NoError(t, err, "stale-but-unchanged tree must not be flagged as a conflict")
	require.Empty(t, res.FilesWithConflicts)
	require.Equal(t, 1, res.FilesUpdated)
}

// TestPull_PreservesLocalOnlyChanges: pull must NOT overwrite a file when
// only the user changed it (remote didn't touch it). This is the disjoint
// leg of pull_then_push — pull should skip the file so push can publish it.
func TestPull_PreservesLocalOnlyChanges(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env", ".env.local"})
	b := env.newMachine(t, []string{".env", ".env.local"})

	a.writeFile(".env", "X=1")
	a.writeFile(".env.local", "L=1")
	a.push()
	b.pull()

	// A changes only .env. B changes only .env.local.
	a.writeFile(".env", "X=2")
	a.push()
	b.writeFile(".env.local", "L=b-edit")

	res, err := b.syncer.Pull(context.Background(), PullOptions{})
	require.NoError(t, err, "disjoint changes must not produce a pull conflict")
	require.Empty(t, res.FilesWithConflicts)

	// Working tree should now show: .env updated to remote, .env.local preserved.
	envOnDisk, err := os.ReadFile(filepath.Join(b.projectDir, ".env"))
	require.NoError(t, err)
	require.Equal(t, "X=2", string(envOnDisk), "remote-only change should land in working tree")

	localOnDisk, err := os.ReadFile(filepath.Join(b.projectDir, ".env.local"))
	require.NoError(t, err)
	require.Equal(t, "L=b-edit", string(localOnDisk), "local-only change must be preserved through pull")
}

// TestPull_LocallyEditedIsStillConflict: same scenario but B genuinely
// edited the file. That MUST still be flagged as a conflict — the new
// baseline-aware code only relaxes the no-edits case.
func TestPull_LocallyEditedIsStillConflict(t *testing.T) {
	env := newTestEnv()
	a := env.newMachine(t, []string{".env"})
	b := env.newMachine(t, []string{".env"})

	a.writeFile(".env", "X=1")
	a.push()
	b.pull()

	a.writeFile(".env", "X=2")
	a.push()

	// B made a real local edit before pulling.
	b.writeFile(".env", "X=b-local")

	_, err := b.syncer.Pull(context.Background(), PullOptions{})
	require.Error(t, err, "real local edit overlapping with remote change must surface as conflict")
	require.ErrorIs(t, err, domain.ErrConflict)
}

// TestSameContent verifies the small helper that both classification and
// overlap detection lean on.
func TestSameContent(t *testing.T) {
	tests := []struct {
		name    string
		a       []byte
		aExists bool
		b       []byte
		bExists bool
		want    bool
	}{
		{"both absent", nil, false, nil, false, true},
		{"a absent only", nil, false, []byte("x"), true, false},
		{"b absent only", []byte("x"), true, nil, false, false},
		{"equal bytes", []byte("hello"), true, []byte("hello"), true, true},
		{"different bytes", []byte("hello"), true, []byte("world"), true, false},
		{"both empty present", []byte{}, true, []byte{}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, sameContent(tc.a, tc.aExists, tc.b, tc.bExists))
		})
	}
}
