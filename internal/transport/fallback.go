package transport

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FallbackTransport tries the primary transport first. If the connection
// fails with an HTTP 404 or 405 (indicating the transport type is not
// supported), it falls back to the secondary transport.
//
// Security: only protocol-mismatch errors (404/405) trigger fallback.
// TLS errors, authentication failures, and other errors are never masked
// by silently trying a different transport.
type FallbackTransport struct {
	Primary   mcp.Transport
	Secondary mcp.Transport
	Logger    *slog.Logger
}

// Connect tries Primary first, then Secondary on 404/405 failure.
func (f *FallbackTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	logger := f.Logger
	if logger == nil {
		logger = slog.Default()
	}

	conn, err := f.Primary.Connect(ctx)
	if err != nil {
		if shouldFallback(err) {
			logger.Info("primary transport failed, trying fallback", "error", err)
			return f.Secondary.Connect(ctx)
		}
		return nil, err
	}
	return conn, nil
}

// shouldFallback returns true only if the error indicates a protocol mismatch
// (HTTP 404 Not Found or 405 Method Not Allowed), meaning the server exists
// but doesn't support this transport type.
//
// Returns false for TLS errors, authentication failures, network errors,
// and all other error types to prevent security-sensitive errors from being
// silently masked by transport fallback.
func shouldFallback(err error) bool {
	if err == nil {
		return false
	}
	_ = http.StatusNotFound // reference for clarity

	errStr := err.Error()
	return strings.Contains(errStr, "404") ||
		strings.Contains(errStr, "405") ||
		strings.Contains(errStr, "Not Found") ||
		strings.Contains(errStr, "Method Not Allowed")
}
