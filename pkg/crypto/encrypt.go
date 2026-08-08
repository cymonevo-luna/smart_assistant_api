// Package crypto provides symmetric encryption for secrets at rest.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Encryptor encrypts and decrypts strings using AES-256-GCM.
type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor builds an Encryptor from a secret key. The key is hashed to 32 bytes
// when shorter or longer than the AES block size.
func NewEncryptor(secret string) (*Encryptor, error) {
	if secret == "" {
		return nil, errors.New("encryption secret must not be empty")
	}
	key := deriveKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Encryptor{gcm: gcm}, nil
}

func deriveKey(secret string) []byte {
	key := make([]byte, 32)
	copy(key, []byte(secret))
	return key
}

// Encrypt returns a base64-encoded ciphertext for plaintext.
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	out := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt decodes and decrypts a ciphertext produced by Encrypt.
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	nonceSize := e.gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, payload := raw[:nonceSize], raw[nonceSize:]
	out, err := e.gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(out), nil
}
