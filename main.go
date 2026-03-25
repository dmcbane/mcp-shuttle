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
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

	// Remote side: build transport based on mode.
	remote := buildRemoteTransport(cfg, httpClient, logger)

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

func buildRemoteTransport(cfg *cli.Config, httpClient *http.Client, logger *slog.Logger) mcp.Transport {
	streamable := &mcp.StreamableClientTransport{
		Endpoint:   cfg.ServerURL,
		HTTPClient: httpClient,
	}
	sse := &mcp.SSEClientTransport{
		Endpoint:   cfg.ServerURL,
		HTTPClient: httpClient,
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
