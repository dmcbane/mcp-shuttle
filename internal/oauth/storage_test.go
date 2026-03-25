//go:build mcp_go_client_oauth

package oauth

import (
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestStorage_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	serverURL := "https://mcp.example.com/sse"
	expiry := time.Now().Add(time.Hour).Truncate(time.Second)

	token := &oauth2.Token{
		AccessToken:  "access-123",
		TokenType:    "Bearer",
		RefreshToken: "refresh-456",
		Expiry:       expiry,
	}

	if err := store.SaveToken(serverURL, token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	loaded, err := store.LoadToken(serverURL)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadToken returned nil")
	}
	if loaded.AccessToken != "access-123" {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, "access-123")
	}
	if loaded.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, "refresh-456")
	}
	if !loaded.Expiry.Equal(expiry) {
		t.Errorf("Expiry = %v, want %v", loaded.Expiry, expiry)
	}
}

func TestStorage_LoadMissing(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	token, err := store.LoadToken("https://nonexistent.example.com")
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if token != nil {
		t.Errorf("expected nil token for missing key, got %+v", token)
	}
}

func TestStorage_Delete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	serverURL := "https://mcp.example.com"
	token := &oauth2.Token{AccessToken: "to-delete", TokenType: "Bearer"}

	if err := store.SaveToken(serverURL, token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	if err := store.DeleteToken(serverURL); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	loaded, err := store.LoadToken(serverURL)
	if err != nil {
		t.Fatalf("LoadToken after delete: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil after delete, got %+v", loaded)
	}
}

func TestStorage_EncryptedOnDisk(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	serverURL := "https://mcp.example.com"
	token := &oauth2.Token{AccessToken: "super-secret", TokenType: "Bearer"}

	if err := store.SaveToken(serverURL, token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	// Read the raw file and verify it's encrypted (not plaintext JSON).
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		content := string(data)
		if strings.Contains(content, "super-secret") {
			t.Error("token file contains plaintext access token — should be encrypted")
		}
		if !strings.HasPrefix(content, encryptedPrefix) {
			t.Errorf("token file missing encrypted prefix, got: %s", content[:min(50, len(content))])
		}
	}
}

func TestStorage_BackwardCompatibility_PlaintextToken(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// Simulate a legacy plaintext token file (pre-encryption).
	serverURL := "https://legacy.example.com"
	plaintextJSON := `{"access_token":"legacy-token","token_type":"Bearer","refresh_token":"legacy-refresh"}`
	path := dir + "/" + keyHash(serverURL) + "_tokens.json"
	if err := os.WriteFile(path, []byte(plaintextJSON), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// LoadToken should transparently handle the plaintext file.
	loaded, err := store.LoadToken(serverURL)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadToken returned nil for legacy plaintext token")
	}
	if loaded.AccessToken != "legacy-token" {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, "legacy-token")
	}
	if loaded.RefreshToken != "legacy-refresh" {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, "legacy-refresh")
	}
}

func TestStorage_EnvVarEncryptionKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MCP_SHUTTLE_ENCRYPTION_KEY", "my-custom-secret-key")

	store, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	serverURL := "https://mcp.example.com"
	token := &oauth2.Token{AccessToken: "env-key-token", TokenType: "Bearer"}

	if err := store.SaveToken(serverURL, token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	// A storage instance without the env var should fail to decrypt.
	t.Setenv("MCP_SHUTTLE_ENCRYPTION_KEY", "different-key")
	store2, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	_, err = store2.LoadToken(serverURL)
	if err == nil {
		t.Fatal("expected decryption failure with different env key")
	}

	// Restore correct key — should decrypt fine.
	t.Setenv("MCP_SHUTTLE_ENCRYPTION_KEY", "my-custom-secret-key")
	store3, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	loaded, err := store3.LoadToken(serverURL)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded.AccessToken != "env-key-token" {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, "env-key-token")
	}
}

func TestStorage_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStorage(dir, nil)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	serverURL := "https://mcp.example.com"
	token := &oauth2.Token{AccessToken: "secret", TokenType: "Bearer"}

	if err := store.SaveToken(serverURL, token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	// Check the token file has 0600 permissions.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatalf("Info: %v", err)
		}
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("file %s has permissions %o, want 0600", e.Name(), perm)
		}
	}
}
