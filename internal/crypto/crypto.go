package crypto

import (
	"bytes"

	"filippo.io/age"
	"github.com/charliek/envsecrets/internal/constants"
	"github.com/charliek/envsecrets/internal/domain"
	limitedio "github.com/charliek/envsecrets/internal/io"
)

// Encrypter provides encryption and decryption operations
type Encrypter interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// AgeEncrypter implements Encrypter using age encryption
type AgeEncrypter struct {
	identity  *age.ScryptIdentity
	recipient *age.ScryptRecipient
}

// NewAgeEncrypter creates a new age-based encrypter with the given passphrase
func NewAgeEncrypter(passphrase string) (*AgeEncrypter, error) {
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, domain.Errorf(domain.ErrEncryptFailed, "failed to create identity: %v", err)
	}

	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return nil, domain.Errorf(domain.ErrEncryptFailed, "failed to create recipient: %v", err)
	}

	// Use the defined work factor for encryption
	// Existing files encrypted with lower work factors will still decrypt correctly
	recipient.SetWorkFactor(constants.ScryptWorkFactor)

	return &AgeEncrypter{
		identity:  identity,
		recipient: recipient,
	}, nil
}

// Encrypt encrypts plaintext using age with scrypt
func (e *AgeEncrypter) Encrypt(plaintext []byte) ([]byte, error) {
	var buf bytes.Buffer

	w, err := age.Encrypt(&buf, e.recipient)
	if err != nil {
		return nil, domain.Errorf(domain.ErrEncryptFailed, "failed to create encrypt writer: %v", err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return nil, domain.Errorf(domain.ErrEncryptFailed, "failed to write encrypted data: %v", err)
	}

	if err := w.Close(); err != nil {
		return nil, domain.Errorf(domain.ErrEncryptFailed, "failed to close encrypt writer: %v", err)
	}

	return buf.Bytes(), nil
}

// Decrypt decrypts ciphertext using age with scrypt
func (e *AgeEncrypter) Decrypt(ciphertext []byte) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(ciphertext), e.identity)
	if err != nil {
		return nil, domain.Errorf(domain.ErrDecryptFailed, "failed to decrypt (verify passphrase is correct): %v", err)
	}

	// Use size-limited read to prevent memory exhaustion
	plaintext, err := limitedio.LimitedReadAll(r, constants.MaxEnvFileSize, "decrypted content")
	if err != nil {
		if domain.GetExitCode(err) != constants.ExitUnknownError {
			return nil, err // Return file size error as-is
		}
		return nil, domain.Errorf(domain.ErrDecryptFailed, "failed to read decrypted data: %v", err)
	}

	return plaintext, nil
}

// Verify checks if the passphrase can decrypt the given ciphertext
func (e *AgeEncrypter) Verify(ciphertext []byte) error {
	_, err := e.Decrypt(ciphertext)
	return err
}
