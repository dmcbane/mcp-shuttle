//go:build mcp_go_client_oauth

package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestCallbackServer_ReceivesCode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use port 0 to let the OS assign an ephemeral port.
	// We'll use the busy-port fallback path by specifying a port that works.
	resultCh := make(chan *CallbackResult, 1)
	errCh := make(chan error, 1)

	port := 0 // will trigger the ephemeral fallback since port 0 is special
	// Actually, let's use a high port that's likely free.
	port = 19876

	go func() {
		result, err := RunCallbackServer(ctx, port, logger)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	// Give the server a moment to start.
	time.Sleep(100 * time.Millisecond)

	// Simulate the OAuth redirect.
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/?code=test-code-123&state=test-state-456", port)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback returned status %d", resp.StatusCode)
	}

	select {
	case result := <-resultCh:
		if result.Code != "test-code-123" {
			t.Errorf("Code = %q, want %q", result.Code, "test-code-123")
		}
		if result.State != "test-state-456" {
			t.Errorf("State = %q, want %q", result.State, "test-state-456")
		}
	case err := <-errCh:
		t.Fatalf("callback server error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for callback result")
	}
}

func TestCallbackServer_HandlesOAuthError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	port := 19877
	errCh := make(chan error, 1)

	go func() {
		_, err := RunCallbackServer(ctx, port, logger)
		errCh <- err
	}()

	time.Sleep(100 * time.Millisecond)

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/?error=access_denied&error_description=User+denied+access", port)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error for OAuth error callback")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for error")
	}
}

func TestCallbackURL(t *testing.T) {
	url := CallbackURL(3334)
	if url != "http://127.0.0.1:3334" {
		t.Errorf("CallbackURL = %q, want %q", url, "http://127.0.0.1:3334")
	}
}
