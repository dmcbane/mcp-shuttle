//go:build mcp_go_client_oauth

package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// StoredToken is the on-disk representation of an OAuth token.
type StoredToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// encryptedPrefix is prepended to encrypted files to distinguish them from plaintext.
const encryptedPrefix = "mcp-shuttle-enc:1:"

// Storage persists OAuth tokens and client credentials to ~/.mcp-auth/.
type Storage struct {
	dir    string
	encKey []byte // AES-256 key for token encryption; nil disables encryption
}

// NewStorage creates a Storage rooted at dir, creating it if needed.
// If dir is empty, defaults to ~/.mcp-auth/.
// Tokens are encrypted at rest using a key derived from (in priority order):
//  1. MCP_SHUTTLE_ENCRYPTION_KEY environment variable
//  2. Machine-specific attributes (UID, hostname, home directory)
//
// If logger is nil, slog.Default() is used.
func NewStorage(dir string, logger *slog.Logger) (*Storage, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		dir = filepath.Join(home, ".mcp-auth")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create storage directory: %w", err)
	}

	secret, usedFallback := encryptionSecret()
	if usedFallback {
		logger.Warn("token encryption using weak fallback key — " +
			"set MCP_SHUTTLE_ENCRYPTION_KEY for stronger protection; " +
			"file permissions (0600) are the primary security control")
	}
	encKey := deriveKey(secret, dir)

	return &Storage{dir: dir, encKey: encKey}, nil
}

// keyHash returns a hex-encoded SHA-256 hash of the server URL, used as a file key.
func keyHash(serverURL string) string {
	h := sha256.Sum256([]byte(serverURL))
	return fmt.Sprintf("%x", h)
}

// LoadToken reads a stored token for the given server URL.
// Returns nil, nil if no token is stored.
// Transparently handles both encrypted and legacy plaintext token files.
func (s *Storage) LoadToken(serverURL string) (*oauth2.Token, error) {
	path := filepath.Join(s.dir, keyHash(serverURL)+"_tokens.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading token file: %w", err)
	}

	jsonData, err := s.decryptData(data)
	if err != nil {
		return nil, fmt.Errorf("decrypting token file: %w", err)
	}

	var st StoredToken
	if err := json.Unmarshal(jsonData, &st); err != nil {
		return nil, fmt.Errorf("parsing token file: %w", err)
	}
	return &oauth2.Token{
		AccessToken:  st.AccessToken,
		TokenType:    st.TokenType,
		RefreshToken: st.RefreshToken,
		Expiry:       st.Expiry,
	}, nil
}

// SaveToken persists an encrypted token for the given server URL.
func (s *Storage) SaveToken(serverURL string, token *oauth2.Token) error {
	st := StoredToken{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}
	jsonData, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling token: %w", err)
	}

	data, err := s.encryptData(jsonData)
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}

	path := filepath.Join(s.dir, keyHash(serverURL)+"_tokens.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing token file: %w", err)
	}
	return nil
}

// DeleteToken removes the stored token for the given server URL.
func (s *Storage) DeleteToken(serverURL string) error {
	path := filepath.Join(s.dir, keyHash(serverURL)+"_tokens.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing token file: %w", err)
	}
	return nil
}

// encryptData encrypts data and prepends the encrypted prefix.
func (s *Storage) encryptData(plaintext []byte) ([]byte, error) {
	if s.encKey == nil {
		return plaintext, nil
	}
	ciphertext, err := encrypt(s.encKey, plaintext)
	if err != nil {
		return nil, err
	}
	encoded := encryptedPrefix + base64.StdEncoding.EncodeToString(ciphertext)
	return []byte(encoded), nil
}

// decryptData handles both encrypted (prefixed) and legacy plaintext data.
func (s *Storage) decryptData(data []byte) ([]byte, error) {
	str := string(data)
	if !strings.HasPrefix(str, encryptedPrefix) {
		// Legacy plaintext token — return as-is for backward compatibility.
		return data, nil
	}

	if s.encKey == nil {
		return nil, fmt.Errorf("encrypted token found but encryption key not available")
	}

	encoded := strings.TrimPrefix(str, encryptedPrefix)
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding encrypted token: %w", err)
	}

	return decrypt(s.encKey, ciphertext)
}
