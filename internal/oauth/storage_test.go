//go:build mcp_go_client_oauth

package oauth

import (
	"os"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestStorage_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStorage(dir)
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
	store, err := NewStorage(dir)
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
	store, err := NewStorage(dir)
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

func TestStorage_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStorage(dir)
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
