//go:build mcp_go_client_oauth

package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/dmcbane/mcp-shuttle/internal/lockfile"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// HandlerConfig configures the OAuth handler.
type HandlerConfig struct {
	ServerURL    string
	CallbackPort int
	Logger       *slog.Logger
	Storage      *Storage
	HTTPClient   *http.Client
	Resource     string // optional resource identifier for session isolation

	// Static OAuth credentials for pre-registered clients.
	// When set, PreregisteredClientConfig is used instead of Dynamic Client Registration.
	OAuthClientID     string
	OAuthClientSecret string
}

// storageKey returns the key used for token storage, incorporating
// the resource identifier if set for multi-tenant isolation.
func (c *HandlerConfig) storageKey() string {
	if c.Resource != "" {
		return c.ServerURL + ":" + c.Resource
	}
	return c.ServerURL
}

// Handler implements auth.OAuthHandler by wrapping the SDK's
// AuthorizationCodeHandler and adding disk-based token persistence.
type Handler struct {
	// Embed to inherit the unexported isOAuthHandler() marker method.
	*auth.AuthorizationCodeHandler

	config      *HandlerConfig
	mu          sync.Mutex
	tokenSource oauth2.TokenSource
}

// NewHandler creates an OAuth handler with persistent token storage and
// browser-based authorization.
func NewHandler(cfg *HandlerConfig) (*Handler, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Storage == nil {
		var err error
		cfg.Storage, err = NewStorage("", cfg.Logger)
		if err != nil {
			return nil, fmt.Errorf("creating token storage: %w", err)
		}
	}

	h := &Handler{config: cfg}

	redirectURL := CallbackURL(cfg.CallbackPort)

	handlerCfg := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL:              redirectURL,
		AuthorizationCodeFetcher: h.fetchAuthorizationCode,
		Client:                   cfg.HTTPClient,
	}

	if cfg.OAuthClientID != "" {
		// Use pre-registered client credentials.
		cfg.Logger.Info("using pre-registered OAuth client", "client_id", cfg.OAuthClientID)
		handlerCfg.PreregisteredClientConfig = &auth.PreregisteredClientConfig{
			ClientSecretAuthConfig: &auth.ClientSecretAuthConfig{
				ClientID:     cfg.OAuthClientID,
				ClientSecret: cfg.OAuthClientSecret,
			},
		}
	} else {
		// Use dynamic client registration (default).
		handlerCfg.DynamicClientRegistrationConfig = &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				RedirectURIs:           []string{redirectURL},
				TokenEndpointAuthMethod: "none",
				GrantTypes:             []string{"authorization_code", "refresh_token"},
				ResponseTypes:          []string{"code"},
				ClientName:             "mcp-shuttle",
				SoftwareID:             "mcp-shuttle",
			},
		}
	}

	inner, err := auth.NewAuthorizationCodeHandler(handlerCfg)
	if err != nil {
		return nil, fmt.Errorf("creating authorization code handler: %w", err)
	}

	h.AuthorizationCodeHandler = inner

	// Try to load a saved token from disk.
	if token, err := cfg.Storage.LoadToken(cfg.storageKey()); err != nil {
		cfg.Logger.Warn("failed to load saved token", "error", err)
	} else if token != nil && token.Valid() {
		cfg.Logger.Info("loaded saved token from disk")
		h.tokenSource = oauth2.StaticTokenSource(token)
	} else if token != nil && token.RefreshToken != "" {
		cfg.Logger.Info("saved token expired but has refresh token, will refresh on first use")
		// Create a token source that will auto-refresh using the refresh token.
		// We need the OAuth config for this, which we don't have yet (it's determined
		// during the Authorize flow). Store the token for now; if refresh fails,
		// the transport will call Authorize.
		h.tokenSource = oauth2.StaticTokenSource(token)
	}

	return h, nil
}

// TokenSource returns a token source for outgoing requests.
// It first checks for a persisted token, then falls back to the inner handler.
func (h *Handler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.tokenSource != nil {
		return h.tokenSource, nil
	}

	// Delegate to the inner handler (may return nil if no auth has happened yet).
	return h.AuthorizationCodeHandler.TokenSource(ctx)
}

// Authorize performs the OAuth flow and persists the resulting token.
func (h *Handler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	h.config.Logger.Info("starting OAuth authorization flow")

	// Clear any cached token source so we get fresh tokens.
	h.mu.Lock()
	h.tokenSource = nil
	h.mu.Unlock()

	// Delegate to the SDK's authorization flow.
	if err := h.AuthorizationCodeHandler.Authorize(ctx, req, resp); err != nil {
		return err
	}

	// The inner handler now has a token source. Get a token and persist it.
	ts, err := h.AuthorizationCodeHandler.TokenSource(ctx)
	if err != nil {
		return fmt.Errorf("getting token source after authorize: %w", err)
	}
	if ts != nil {
		token, err := ts.Token()
		if err != nil {
			h.config.Logger.Warn("failed to get token for persistence", "error", err)
		} else {
			if err := h.config.Storage.SaveToken(h.config.storageKey(), token); err != nil {
				h.config.Logger.Warn("failed to persist token", "error", err)
			} else {
				h.config.Logger.Info("token persisted to disk")
			}
		}

		h.mu.Lock()
		h.tokenSource = ts
		h.mu.Unlock()
	}

	return nil
}

// fetchAuthorizationCode implements auth.AuthorizationCodeFetcher.
// It uses a lockfile to coordinate with other mcp-shuttle instances —
// only one instance opens the browser; others wait for the token on disk.
func (h *Handler) fetchAuthorizationCode(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
	lockName := keyHash(h.config.storageKey())
	lock, existing, err := lockfile.Acquire(h.config.Storage.dir, lockName)
	if err != nil {
		h.config.Logger.Warn("failed to acquire auth lock, proceeding anyway", "error", err)
	}

	if existing != nil && !existing.IsStale(30*time.Minute) {
		// Another instance is handling the OAuth flow. Wait for it.
		h.config.Logger.Info("another instance is handling OAuth, waiting for token",
			"pid", existing.PID)
		return h.waitForToken(ctx)
	}

	if lock != nil {
		defer lock.Release()
	}

	h.config.Logger.Info("opening browser for authorization", "url", args.URL)

	if err := OpenBrowser(args.URL); err != nil {
		h.config.Logger.Warn("failed to open browser automatically", "error", err)
		fmt.Fprintf(os.Stderr,
			"Please open this URL in your browser to authorize:\n%s\n",
			args.URL,
		)
	}

	result, err := RunCallbackServer(ctx, h.config.CallbackPort, h.config.Logger)
	if err != nil {
		return nil, fmt.Errorf("waiting for authorization callback: %w", err)
	}

	return &auth.AuthorizationResult{
		Code:  result.Code,
		State: result.State,
	}, nil
}

// waitForToken polls disk for a token saved by another instance.
func (h *Handler) waitForToken(ctx context.Context) (*auth.AuthorizationResult, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("timed out waiting for another instance to complete OAuth")
		case <-ticker.C:
			token, err := h.config.Storage.LoadToken(h.config.storageKey())
			if err != nil {
				continue
			}
			if token != nil && token.Valid() {
				h.config.Logger.Info("found token saved by another instance")
				h.mu.Lock()
				h.tokenSource = oauth2.StaticTokenSource(token)
				h.mu.Unlock()
				// Return a sentinel that will cause the SDK to skip the
				// code exchange. We've already got the token.
				return nil, fmt.Errorf("token obtained from another instance")
			}
		}
	}
}

