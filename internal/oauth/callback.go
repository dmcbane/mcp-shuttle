//go:build mcp_go_client_oauth

package oauth

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// callbackPath is the specific path the OAuth callback server listens on.
// Using a dedicated path instead of "/" reduces the attack surface.
const callbackPath = "/oauth/callback"

// CallbackResult holds the authorization code and state from the OAuth redirect.
type CallbackResult struct {
	Code  string
	State string
}

// RunCallbackServer starts a local HTTP server that listens for the OAuth
// redirect and captures the authorization code. It returns the result once
// the callback is received or the context is cancelled.
//
// Security hardening:
//   - Only responds on the /oauth/callback path (404 for all others)
//   - Only accepts GET requests (405 for POST, etc.)
//   - Sanitizes reflected error parameters to prevent XSS
//   - Sets Content-Security-Policy on HTML responses
func RunCallbackServer(ctx context.Context, port int, logger *slog.Logger) (*CallbackResult, error) {
	resultCh := make(chan *CallbackResult, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		// Only accept GET requests.
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		if code == "" {
			errMsg := r.URL.Query().Get("error")
			errDesc := r.URL.Query().Get("error_description")
			if errMsg != "" {
				// Sanitize before logging and reflecting to prevent log injection and XSS.
				safeMsg := html.EscapeString(truncate(errMsg, 200))
				safeDesc := html.EscapeString(truncate(errDesc, 500))
				logger.Error("OAuth error callback", "error", safeMsg, "description", safeDesc)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				http.Error(w, fmt.Sprintf("Authorization failed: %s - %s", safeMsg, safeDesc), http.StatusBadRequest)
				errCh <- fmt.Errorf("OAuth authorization error: %s (%s)", safeMsg, safeDesc)
				return
			}
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}

		logger.Info("received authorization code")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
<h1>Authorization successful!</h1>
<p>You can close this window and return to your application.</p>
<script>window.close()</script>
</body></html>`)

		resultCh <- &CallbackResult{Code: code, State: state}
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Try an ephemeral port if the specified one is taken.
		logger.Warn("specified port unavailable, using ephemeral port", "port", port, "error", err)
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("failed to start callback server: %w", err)
		}
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	logger.Info("callback server listening", "port", actualPort)

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server error: %w", err)
		}
	}()

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CallbackURL returns the redirect URL for the callback server.
func CallbackURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, callbackPath)
}

// truncate limits a string to maxLen characters to prevent log flooding
// or response inflation from malicious OAuth error parameters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
