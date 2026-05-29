//go:build gm

package gm

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"github.com/emmansun/gmsm/sm4"
)

// SM4KeySize is the required key size for SM4 (16 bytes / 128 bits).
const SM4KeySize = 16

var (
	ErrInvalidKeySize    = errors.New("gm: SM4 key must be 16 bytes")
	ErrCiphertextTooShort = errors.New("gm: ciphertext too short")
)

// SM4Encrypt encrypts plaintext using SM4-GCM with the given key.
// The key must be exactly 16 bytes. Returns nonce || ciphertext.
func SM4Encrypt(key, plaintext []byte) ([]byte, error) {
	if len(key) != SM4KeySize {
		return nil, ErrInvalidKeySize
	}

	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// SM4Decrypt decrypts ciphertext (nonce || encrypted) using SM4-GCM with the given key.
// The key must be exactly 16 bytes.
func SM4Decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(key) != SM4KeySize {
		return nil, ErrInvalidKeySize
	}

	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrCiphertextTooShort
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return aead.Open(nil, nonce, ct, nil)
}
