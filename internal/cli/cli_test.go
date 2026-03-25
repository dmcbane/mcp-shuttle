package cli

import (
	"testing"
)

func TestParse_BasicURL(t *testing.T) {
	cfg, err := Parse([]string{"https://mcp.example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ServerURL != "https://mcp.example.com" {
		t.Errorf("got ServerURL=%q, want %q", cfg.ServerURL, "https://mcp.example.com")
	}
	if cfg.Transport != TransportHTTPFirst {
		t.Errorf("got Transport=%q, want %q", cfg.Transport, TransportHTTPFirst)
	}
	if cfg.CallbackPort != 3334 {
		t.Errorf("got CallbackPort=%d, want 3334", cfg.CallbackPort)
	}
}

func TestParse_NoArgs(t *testing.T) {
	_, err := Parse([]string{})
	if err == nil {
		t.Fatal("expected error for missing server URL")
	}
}

func TestParse_Headers(t *testing.T) {
	cfg, err := Parse([]string{
		"https://mcp.example.com",
		"--header", "Authorization: Bearer token123",
		"--header", "X-Custom: value",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("got Authorization=%q, want %q", cfg.Headers["Authorization"], "Bearer token123")
	}
	if cfg.Headers["X-Custom"] != "value" {
		t.Errorf("got X-Custom=%q, want %q", cfg.Headers["X-Custom"], "value")
	}
}

func TestParse_InvalidHeader(t *testing.T) {
	_, err := Parse([]string{"https://mcp.example.com", "--header", "no-colon"})
	if err == nil {
		t.Fatal("expected error for invalid header format")
	}
}

func TestParse_TransportModes(t *testing.T) {
	modes := []TransportMode{TransportHTTPFirst, TransportSSEFirst, TransportHTTPOnly, TransportSSEOnly}
	for _, mode := range modes {
		cfg, err := Parse([]string{"https://mcp.example.com", "--transport", string(mode)})
		if err != nil {
			t.Fatalf("unexpected error for mode %q: %v", mode, err)
		}
		if cfg.Transport != mode {
			t.Errorf("got Transport=%q, want %q", cfg.Transport, mode)
		}
	}
}

func TestParse_InvalidTransport(t *testing.T) {
	_, err := Parse([]string{"https://mcp.example.com", "--transport", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid transport mode")
	}
}

func TestParse_RefuseHTTP(t *testing.T) {
	_, err := Parse([]string{"http://mcp.example.com"})
	if err == nil {
		t.Fatal("expected error for HTTP without --allow-http")
	}
}

func TestParse_AllowHTTP(t *testing.T) {
	cfg, err := Parse([]string{"http://mcp.example.com", "--allow-http"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.AllowHTTP {
		t.Error("expected AllowHTTP=true")
	}
}

func TestParse_IgnoreTools(t *testing.T) {
	cfg, err := Parse([]string{
		"https://mcp.example.com",
		"--ignore-tool", "delete*",
		"--ignore-tool", "*admin*",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.IgnoreTools) != 2 {
		t.Fatalf("got %d ignore patterns, want 2", len(cfg.IgnoreTools))
	}
	if cfg.IgnoreTools[0] != "delete*" || cfg.IgnoreTools[1] != "*admin*" {
		t.Errorf("got IgnoreTools=%v, want [delete* *admin*]", cfg.IgnoreTools)
	}
}

func TestParse_Resource(t *testing.T) {
	cfg, err := Parse([]string{
		"https://mcp.example.com",
		"--resource", "tenant-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Resource != "tenant-123" {
		t.Errorf("got Resource=%q, want %q", cfg.Resource, "tenant-123")
	}
}

func TestParse_OAuthStaticCredentials(t *testing.T) {
	cfg, err := Parse([]string{
		"https://mcp.example.com",
		"--oauth-client-id", "my-client",
		"--oauth-client-secret", "my-secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OAuthClientID != "my-client" {
		t.Errorf("OAuthClientID = %q, want %q", cfg.OAuthClientID, "my-client")
	}
	if cfg.OAuthClientSecret != "my-secret" {
		t.Errorf("OAuthClientSecret = %q, want %q", cfg.OAuthClientSecret, "my-secret")
	}
}

func TestParse_OAuthStaticCredentials_MissingSecret(t *testing.T) {
	_, err := Parse([]string{
		"https://mcp.example.com",
		"--oauth-client-id", "my-client",
	})
	if err == nil {
		t.Fatal("expected error when --oauth-client-id is set without --oauth-client-secret")
	}
}

func TestParse_HeaderEnvVarExpansion(t *testing.T) {
	t.Setenv("TEST_API_KEY", "secret-key-123")
	cfg, err := Parse([]string{
		"https://mcp.example.com",
		"--header", "Authorization: Bearer ${TEST_API_KEY}",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Headers["Authorization"] != "Bearer secret-key-123" {
		t.Errorf("got Authorization=%q, want %q", cfg.Headers["Authorization"], "Bearer secret-key-123")
	}
}

func TestParse_HeaderEnvVarMissing(t *testing.T) {
	t.Setenv("TEST_MISSING_VAR", "")
	_, err := Parse([]string{
		"https://mcp.example.com",
		"--header", "Authorization: Bearer ${DEFINITELY_UNSET_VAR_12345}",
	})
	if err == nil {
		t.Fatal("expected error for unset env var in header")
	}
}

func TestParse_MaxMessageSize(t *testing.T) {
	cfg, err := Parse([]string{
		"https://mcp.example.com",
		"--max-message-size", "10485760",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxMessageSize != 10485760 {
		t.Errorf("got MaxMessageSize=%d, want 10485760", cfg.MaxMessageSize)
	}
}

func TestParse_MaxMessageSizeDefault(t *testing.T) {
	cfg, err := Parse([]string{"https://mcp.example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default should be 0 (unlimited).
	if cfg.MaxMessageSize != 0 {
		t.Errorf("got MaxMessageSize=%d, want 0 (default)", cfg.MaxMessageSize)
	}
}

func TestParse_InvalidIgnoreToolPattern(t *testing.T) {
	_, err := Parse([]string{
		"https://mcp.example.com",
		"--ignore-tool", "[unclosed",
	})
	if err == nil {
		t.Fatal("expected error for invalid glob pattern in --ignore-tool")
	}
}

func TestParse_AllowTool(t *testing.T) {
	cfg, err := Parse([]string{
		"https://mcp.example.com",
		"--allow-tool", "read_*",
		"--allow-tool", "list_*",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.AllowTools) != 2 {
		t.Fatalf("got %d allow patterns, want 2", len(cfg.AllowTools))
	}
	if cfg.AllowTools[0] != "read_*" || cfg.AllowTools[1] != "list_*" {
		t.Errorf("got AllowTools=%v, want [read_* list_*]", cfg.AllowTools)
	}
}

func TestParse_InvalidAllowToolPattern(t *testing.T) {
	_, err := Parse([]string{
		"https://mcp.example.com",
		"--allow-tool", "[bad",
	})
	if err == nil {
		t.Fatal("expected error for invalid glob pattern in --allow-tool")
	}
}

func TestParse_AllowAndIgnoreToolMutuallyExclusive(t *testing.T) {
	_, err := Parse([]string{
		"https://mcp.example.com",
		"--allow-tool", "read_*",
		"--ignore-tool", "delete_*",
	})
	if err == nil {
		t.Fatal("expected error when both --allow-tool and --ignore-tool are used")
	}
}

func TestParse_DebugAndSilent(t *testing.T) {
	cfg, err := Parse([]string{"https://mcp.example.com", "--debug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Debug {
		t.Error("expected Debug=true")
	}

	cfg, err = Parse([]string{"https://mcp.example.com", "--silent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Silent {
		t.Error("expected Silent=true")
	}
}
