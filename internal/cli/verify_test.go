package cli

import (
	"context"
	"testing"

	"github.com/charliek/envsecrets/internal/crypto"
	"github.com/charliek/envsecrets/internal/domain"
	"github.com/charliek/envsecrets/internal/storage"
	"github.com/stretchr/testify/require"
)

// verifyRepo backs the rotate-passphrase pre-check, so it must actually
// decrypt files stored inside the packfile rather than finding nothing to check.
func TestVerifyRepo_ChecksPackfileContents(t *testing.T) {
	// verifyRepo uses the default cache dir under $HOME
	t.Setenv("HOME", t.TempDir())

	store := storage.NewMockStorage()
	repoInfo := &domain.RepoInfo{Owner: "owner", Name: "repo"}

	enc, err := crypto.NewAgeEncrypter("correct-passphrase")
	require.NoError(t, err)
	encrypted, err := enc.Encrypt([]byte("SECRET=1"))
	require.NoError(t, err)
	pushTestFiles(t, newTestCache(t, repoInfo, store), map[string][]byte{".env": encrypted}, "initial")

	result := verifyRepo(context.Background(), store, repoInfo, enc)
	require.Empty(t, result.Error)
	require.Equal(t, 1, result.FilesVerified)

	wrongEnc, err := crypto.NewAgeEncrypter("wrong-passphrase")
	require.NoError(t, err)
	result = verifyRepo(context.Background(), store, repoInfo, wrongEnc)
	require.Contains(t, result.Error, "decrypt .env failed")
}
