//go:build mcp_go_client_oauth

package main

import (
	"log/slog"
	"net/http"

	"github.com/dmcbane/mcp-shuttle/internal/cli"
	"github.com/dmcbane/mcp-shuttle/internal/oauth"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func init() {
	setupOAuth = func(cfg *cli.Config, logger *slog.Logger, baseClient *http.Client) (auth.OAuthHandler, error) {
		storage, err := oauth.NewStorage("")
		if err != nil {
			return nil, err
		}

		handler, err := oauth.NewHandler(&oauth.HandlerConfig{
			ServerURL:    cfg.ServerURL,
			CallbackPort: cfg.CallbackPort,
			Logger:       logger,
			Storage:      storage,
			HTTPClient:   baseClient,
		})
		if err != nil {
			return nil, err
		}

		return handler, nil
	}
}
