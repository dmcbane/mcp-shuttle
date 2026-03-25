//go:build mcp_go_client_oauth

package oauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/user"

	"golang.org/x/crypto/hkdf"
)

// deriveKey produces a 32-byte AES-256 key from a secret and salt using HKDF-SHA256.
func deriveKey(secret, salt string) []byte {
	hkdfReader := hkdf.New(sha256.New, []byte(secret), []byte(salt), []byte("mcp-shuttle-token-encryption"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		panic("hkdf: " + err.Error()) // should never happen with valid inputs
	}
	return key
}

// encrypt encrypts plaintext using AES-256-GCM. The nonce is prepended to the ciphertext.
func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt decrypts ciphertext produced by encrypt.
func decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	return plaintext, nil
}

// machineSecret returns a stable, machine-specific string used as the base secret
// for token encryption key derivation. It combines the user's UID/username and
// hostname to create a value that is:
//   - Stable across process restarts
//   - Different across user accounts on the same machine
//   - Different across machines for the same user
//
// This is NOT a cryptographic secret — it provides defense-in-depth against
// casual file access (e.g., another user reading ~/.mcp-auth/ on a shared system).
// The primary protection is the 0600 file permissions.
func machineSecret() string {
	parts := ""

	if u, err := user.Current(); err == nil {
		parts += u.Uid + ":" + u.Username
	}

	if hostname, err := os.Hostname(); err == nil {
		parts += ":" + hostname
	}

	if home, err := os.UserHomeDir(); err == nil {
		parts += ":" + home
	}

	if parts == "" {
		parts = "mcp-shuttle-fallback-secret"
	}

	return parts
}
