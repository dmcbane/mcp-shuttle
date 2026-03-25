package transport

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FallbackTransport tries the primary transport first. If the connection
// fails with an HTTP 404 (indicating the transport type is not supported),
// it falls back to the secondary transport.
type FallbackTransport struct {
	Primary   mcp.Transport
	Secondary mcp.Transport
	Logger    *slog.Logger
}

// Connect tries Primary first, then Secondary on failure.
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

// shouldFallback returns true if the error indicates we should try the other
// transport (e.g., 404 Not Found means the endpoint doesn't exist).
func shouldFallback(err error) bool {
	// The SDK wraps HTTP errors; check for common fallback indicators.
	// For now, always fall back on any connection error. We can refine
	// this to only fall back on 404/405 once we see the SDK's error types.
	_ = http.StatusNotFound
	return true
}
