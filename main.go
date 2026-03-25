package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dmcbane/mcp-shuttle/internal/cli"
	"github.com/dmcbane/mcp-shuttle/internal/proxy"
	"github.com/dmcbane/mcp-shuttle/internal/transport"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// setupOAuth is set by main_oauth.go when built with the mcp_go_client_oauth tag.
// It returns an auth.OAuthHandler and an http.Client that may include custom headers.
var setupOAuth func(cfg *cli.Config, logger *slog.Logger, baseClient *http.Client) (auth.OAuthHandler, error)

func main() {
	cfg, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp-shuttle: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Local side: stdio transport.
	local := &mcp.StdioTransport{}

	// Build HTTP client with custom headers if specified.
	var httpClient *http.Client
	if len(cfg.Headers) > 0 {
		httpClient = &http.Client{
			Transport: transport.HeaderTransport(nil, cfg.Headers),
		}
	}

	// Set up OAuth handler if available.
	var oauthHandler auth.OAuthHandler
	if setupOAuth != nil {
		oauthHandler, err = setupOAuth(cfg, logger, httpClient)
		if err != nil {
			logger.Error("failed to set up OAuth", "error", err)
			os.Exit(1)
		}
	}

	// Remote side: build transport based on mode.
	remote := buildRemoteTransport(cfg, httpClient, oauthHandler, logger)

	logger.Info("starting proxy", "server", cfg.ServerURL, "transport", cfg.Transport)

	p := &proxy.Proxy{Logger: logger}
	if err := p.Run(ctx, local, remote); err != nil {
		logger.Error("proxy error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(cfg *cli.Config) *slog.Logger {
	var level slog.Level
	switch {
	case cfg.Silent:
		level = slog.LevelError + 4 // suppress everything
	case cfg.Debug:
		level = slog.LevelDebug
	default:
		level = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func buildRemoteTransport(cfg *cli.Config, httpClient *http.Client, oauthHandler auth.OAuthHandler, logger *slog.Logger) mcp.Transport {
	streamable := &mcp.StreamableClientTransport{
		Endpoint:     cfg.ServerURL,
		HTTPClient:   httpClient,
		OAuthHandler: oauthHandler,
	}
	sse := &mcp.SSEClientTransport{
		Endpoint:   cfg.ServerURL,
		HTTPClient: httpClient,
	}

	// For SSE transport with OAuth, wrap the HTTP client with a bearer token
	// injecting RoundTripper since SSEClientTransport lacks OAuthHandler support.
	if oauthHandler != nil && httpClient == nil {
		// SSE needs an HTTP client that can inject tokens.
		// The StreamableClientTransport handles this internally via OAuthHandler.
		// For SSE, we'd need a custom RoundTripper. This is handled at the
		// transport level when the token source is available.
		logger.Debug("OAuth handler configured for streamable transport")
	}

	switch cfg.Transport {
	case cli.TransportHTTPOnly:
		return streamable
	case cli.TransportSSEOnly:
		return sse
	case cli.TransportSSEFirst:
		return &transport.FallbackTransport{
			Primary:   sse,
			Secondary: streamable,
			Logger:    logger,
		}
	default: // http-first
		return &transport.FallbackTransport{
			Primary:   streamable,
			Secondary: sse,
			Logger:    logger,
		}
	}
}
