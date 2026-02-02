package crypto

import (
	"encoding/base64"

	"github.com/charliek/envsecrets/internal/domain"
)

// MockEncrypter is a mock implementation for testing
type MockEncrypter struct {
	EncryptFunc func(plaintext []byte) ([]byte, error)
	DecryptFunc func(ciphertext []byte) ([]byte, error)

	// For simple use cases
	EncryptError error
	DecryptError error
}

// NewMockEncrypter creates a new mock encrypter that does reversible base64 encoding
func NewMockEncrypter() *MockEncrypter {
	return &MockEncrypter{
		EncryptFunc: func(plaintext []byte) ([]byte, error) {
			encoded := base64.StdEncoding.EncodeToString(plaintext)
			return []byte("MOCK:" + encoded), nil
		},
		DecryptFunc: func(ciphertext []byte) ([]byte, error) {
			if len(ciphertext) < 5 || string(ciphertext[:5]) != "MOCK:" {
				return nil, domain.ErrDecryptFailed
			}
			decoded, err := base64.StdEncoding.DecodeString(string(ciphertext[5:]))
			if err != nil {
				return nil, domain.Errorf(domain.ErrDecryptFailed, "invalid mock ciphertext")
			}
			return decoded, nil
		},
	}
}

// Encrypt implements Encrypter
func (m *MockEncrypter) Encrypt(plaintext []byte) ([]byte, error) {
	if m.EncryptError != nil {
		return nil, m.EncryptError
	}
	if m.EncryptFunc != nil {
		return m.EncryptFunc(plaintext)
	}
	return plaintext, nil
}

// Decrypt implements Encrypter
func (m *MockEncrypter) Decrypt(ciphertext []byte) ([]byte, error) {
	if m.DecryptError != nil {
		return nil, m.DecryptError
	}
	if m.DecryptFunc != nil {
		return m.DecryptFunc(ciphertext)
	}
	return ciphertext, nil
}
