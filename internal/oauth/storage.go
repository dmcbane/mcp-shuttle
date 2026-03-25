//go:build mcp_go_client_oauth

package oauth

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Storage persists OAuth tokens and client credentials to ~/.mcp-auth/.
type Storage struct {
	dir string
}

// NewStorage creates a Storage rooted at dir, creating it if needed.
// If dir is empty, defaults to ~/.mcp-auth/.
func NewStorage(dir string) (*Storage, error) {
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
	return &Storage{dir: dir}, nil
}

// keyHash returns a hex-encoded SHA-256 hash of the server URL, used as a file key.
func keyHash(serverURL string) string {
	h := sha256.Sum256([]byte(serverURL))
	return fmt.Sprintf("%x", h)
}

// LoadToken reads a stored token for the given server URL.
// Returns nil, nil if no token is stored.
func (s *Storage) LoadToken(serverURL string) (*oauth2.Token, error) {
	path := filepath.Join(s.dir, keyHash(serverURL)+"_tokens.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading token file: %w", err)
	}
	var st StoredToken
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing token file: %w", err)
	}
	return &oauth2.Token{
		AccessToken:  st.AccessToken,
		TokenType:    st.TokenType,
		RefreshToken: st.RefreshToken,
		Expiry:       st.Expiry,
	}, nil
}

// SaveToken persists a token for the given server URL.
func (s *Storage) SaveToken(serverURL string, token *oauth2.Token) error {
	st := StoredToken{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling token: %w", err)
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
