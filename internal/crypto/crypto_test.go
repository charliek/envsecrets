package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgeEncrypter_RoundTrip(t *testing.T) {
	passphrase := "test-passphrase-123"
	plaintext := []byte("hello, world! this is a secret message")

	enc, err := NewAgeEncrypter(passphrase)
	require.NoError(t, err)

	// Encrypt
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)
	require.NotEqual(t, plaintext, ciphertext)
	require.NotEmpty(t, ciphertext)

	// Decrypt
	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestAgeEncrypter_WrongPassphrase(t *testing.T) {
	plaintext := []byte("secret data")

	enc1, err := NewAgeEncrypter("passphrase1")
	require.NoError(t, err)

	enc2, err := NewAgeEncrypter("passphrase2")
	require.NoError(t, err)

	// Encrypt with enc1
	ciphertext, err := enc1.Encrypt(plaintext)
	require.NoError(t, err)

	// Try to decrypt with enc2
	_, err = enc2.Decrypt(ciphertext)
	require.Error(t, err)
}

func TestAgeEncrypter_EmptyPlaintext(t *testing.T) {
	enc, err := NewAgeEncrypter("passphrase")
	require.NoError(t, err)

	// Encrypt empty data
	ciphertext, err := enc.Encrypt([]byte{})
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)

	// Decrypt
	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Empty(t, decrypted)
}

func TestAgeEncrypter_LargeData(t *testing.T) {
	enc, err := NewAgeEncrypter("passphrase")
	require.NoError(t, err)

	// Create 1MB of data
	plaintext := make([]byte, 1024*1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestMockEncrypter_RoundTrip(t *testing.T) {
	enc := NewMockEncrypter()
	plaintext := []byte("test data")

	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestMockEncrypter_Errors(t *testing.T) {
	enc := &MockEncrypter{
		EncryptError: ErrEncryptTest,
		DecryptError: ErrDecryptTest,
	}

	_, err := enc.Encrypt([]byte("test"))
	require.ErrorIs(t, err, ErrEncryptTest)

	_, err = enc.Decrypt([]byte("test"))
	require.ErrorIs(t, err, ErrDecryptTest)
}

var ErrEncryptTest = &testError{msg: "encrypt error"}
var ErrDecryptTest = &testError{msg: "decrypt error"}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
