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
