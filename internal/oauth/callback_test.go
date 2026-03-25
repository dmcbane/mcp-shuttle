//go:build mcp_go_client_oauth

package oauth

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCallbackServer_ReceivesCode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *CallbackResult, 1)
	errCh := make(chan error, 1)

	port := 19876

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

	// Simulate the OAuth redirect on the correct callback path.
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/oauth/callback?code=test-code-123&state=test-state-456", port)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback returned status %d", resp.StatusCode)
	}

	// Verify CSP header is set.
	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Error("expected Content-Security-Policy header on success response")
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

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/oauth/callback?error=access_denied&error_description=User+denied+access", port)
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

func TestCallbackServer_RejectsWrongPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	port := 19878
	go func() {
		RunCallbackServer(ctx, port, logger)
	}()

	time.Sleep(100 * time.Millisecond)

	// Request on wrong path should get 404.
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/wrong/path?code=test&state=test", port)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for wrong path, got %d", resp.StatusCode)
	}
}

func TestCallbackServer_RejectsPostMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	port := 19879
	go func() {
		RunCallbackServer(ctx, port, logger)
	}()

	time.Sleep(100 * time.Millisecond)

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/oauth/callback?code=test&state=test", port)
	resp, err := http.Post(callbackURL, "text/plain", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", resp.StatusCode)
	}
}

func TestCallbackServer_SanitizesErrorReflection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	port := 19880
	errCh := make(chan error, 1)

	go func() {
		_, err := RunCallbackServer(ctx, port, logger)
		errCh <- err
	}()

	time.Sleep(100 * time.Millisecond)

	// Send an error with HTML/script injection attempt in error_description.
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/oauth/callback?error=bad&error_description=<script>alert(1)</script>", port)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Response body should NOT contain raw <script> tag.
	if strings.Contains(string(body), "<script>") {
		t.Error("error response reflects unsanitized HTML — potential XSS")
	}
}

func TestCallbackURL(t *testing.T) {
	url := CallbackURL(3334)
	if url != "http://127.0.0.1:3334/oauth/callback" {
		t.Errorf("CallbackURL = %q, want %q", url, "http://127.0.0.1:3334/oauth/callback")
	}
}
