package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/charliek/envsecrets/internal/crypto"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/stretchr/testify/require"
)

// pushEncrypted pushes a single .env encrypted with enc to the given repo
func pushEncrypted(t *testing.T, store storage.Storage, repoInfo *domain.RepoInfo, enc crypto.Encrypter) {
	t.Helper()
	encrypted, err := enc.Encrypt([]byte("SECRET=" + repoInfo.Name))
	require.NoError(t, err)
	pushTestFiles(t, newTestCache(t, repoInfo, store), map[string][]byte{".env": encrypted}, "initial")
}

func TestPlanRotation_RejectsUnknownPassphrase(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := storage.NewMockStorage()

	oldEnc, err := crypto.NewAgeEncrypter("old-passphrase")
	require.NoError(t, err)
	newEnc, err := crypto.NewAgeEncrypter("new-passphrase")
	require.NoError(t, err)
	otherEnc, err := crypto.NewAgeEncrypter("other-passphrase")
	require.NoError(t, err)

	pushEncrypted(t, store, &domain.RepoInfo{Owner: "owner", Name: "a"}, oldEnc)
	pushEncrypted(t, store, &domain.RepoInfo{Owner: "owner", Name: "b"}, otherEnc)

	_, err = planRotation(context.Background(), store, []string{"owner/a", "owner/b"}, oldEnc, newEnc)
	require.ErrorContains(t, err, "owner/b")
}

// An interrupted rotation leaves repos on both passphrases; re-running with
// the same pair must skip the finished repos and rotate the rest.
func TestRotation_ResumesAfterPartialFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := storage.NewMockStorage()
	repos := []string{"owner/a", "owner/b"}
	repoA := &domain.RepoInfo{Owner: "owner", Name: "a"}
	repoB := &domain.RepoInfo{Owner: "owner", Name: "b"}

	oldEnc, err := crypto.NewAgeEncrypter("old-passphrase")
	require.NoError(t, err)
	newEnc, err := crypto.NewAgeEncrypter("new-passphrase")
	require.NoError(t, err)

	pushEncrypted(t, store, repoA, oldEnc)
	pushEncrypted(t, store, repoB, oldEnc)

	plan, err := planRotation(ctx, store, repos, oldEnc, newEnc)
	require.NoError(t, err)
	require.Equal(t, repos, plan.ToRotate)

	// First run: A rotates, B fails to upload
	require.NoError(t, rotateRepo(ctx, store, repoA, oldEnc, newEnc))
	store.UploadError = errors.New("upload failed")
	require.Error(t, rotateRepo(ctx, store, repoB, oldEnc, newEnc))
	store.UploadError = nil

	// Retry: A is already done, B still needs rotating
	plan, err = planRotation(ctx, store, repos, oldEnc, newEnc)
	require.NoError(t, err)
	require.Equal(t, []string{"owner/b"}, plan.ToRotate)
	require.Equal(t, []string{"owner/a"}, plan.AlreadyRotated)

	require.NoError(t, rotateRepo(ctx, store, repoB, oldEnc, newEnc))

	plan, err = planRotation(ctx, store, repos, oldEnc, newEnc)
	require.NoError(t, err)
	require.Empty(t, plan.ToRotate)
	require.Equal(t, repos, plan.AlreadyRotated)
}
