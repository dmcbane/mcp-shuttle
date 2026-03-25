# mcp-shuttle Design Document

## Overview

mcp-shuttle is a stdio-to-remote transport proxy for the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/). MCP clients like Claude Desktop, Claude Code, and Cursor spawn it as a local subprocess. It bridges the local stdio transport to a remote MCP server over HTTP Streamable or SSE transport, with optional OAuth 2.1 authentication and tool filtering.

## Architecture

```
MCP Client (Claude Desktop, Claude Code, Cursor, etc.)
    │
    │ spawns as subprocess
    ▼
┌─────────────────────────────────────────────────────┐
│  mcp-shuttle                                        │
│                                                     │
│  stdin/stdout ←──→ Proxy ←──→ HTTP/SSE Transport    │
│  (StdioTransport)    │       (StreamableClient or   │
│                      │        SSEClient Transport)  │
│                      │                              │
│              Message Interceptors                   │
│              (tool filtering)                       │
│                                                     │
│              OAuthHandler                           │
│              (token management,                     │
│               browser flow,                         │
│               disk persistence)                     │
└─────────────────────────────────────────────────────┘
    │
    │ HTTPS
    ▼
Remote MCP Server
```

## Key design decisions

### 1. Connection-level proxy, not semantic proxy

**Decision:** Forward raw JSON-RPC messages without parsing MCP semantics (tools, resources, prompts).

**Why:** The proxy doesn't need to understand what tools or resources exist — it just needs to shuttle messages between two connections. This keeps the code simple, reduces the surface area for bugs, and means we automatically support any future MCP features without code changes.

**Exception:** Tool filtering (`--ignore-tool`) intercepts `tools/list` responses and `tools/call` requests. This is the only place we parse message content.

### 2. Go MCP SDK for transport layer

**Decision:** Use `github.com/modelcontextprotocol/go-sdk` for all transport implementations rather than implementing transports from scratch.

**Why:** The SDK (maintained by Google and the MCP team, v1.4.x, 4k+ stars) provides production-quality implementations of `StdioTransport`, `StreamableClientTransport`, and `SSEClientTransport` with proper session management, SSE parsing, reconnection logic, and OAuth integration. Reimplementing these would be error-prone and create a maintenance burden tracking spec changes.

**Trade-off:** We depend on the `mcp_go_client_oauth` build tag for OAuth-related types. This is an acceptable coupling since the SDK is the canonical Go implementation.

### 3. Embed SDK's AuthorizationCodeHandler for OAuth

**Decision:** Embed `*auth.AuthorizationCodeHandler` in our handler struct rather than reimplementing the OAuth flow.

**Why:** The SDK handler implements the full MCP Authorization spec:
- Protected Resource Metadata discovery (RFC 9728) with three-tier fallback
- Authorization Server Metadata discovery (RFC 8414) with OpenID Connect fallback
- Dynamic Client Registration (RFC 7591)
- PKCE S256 via `oauth2.GenerateVerifier()`
- Token exchange and refresh via `golang.org/x/oauth2`

Reimplementing this would be hundreds of lines of security-critical code. By embedding, we inherit the `isOAuthHandler()` marker method (which is unexported, preventing external interface implementation) and can override `TokenSource()` and `Authorize()` to add disk persistence.

**Trade-off:** We can't intercept the inner handler's token exchange to persist tokens at the exact moment they're received. Instead, we call `TokenSource().Token()` after `Authorize()` completes to extract and persist. This means a narrow window where a crash could lose newly-acquired tokens, requiring re-authentication.

### 4. Conditional compilation for OAuth

**Decision:** OAuth code lives behind the `mcp_go_client_oauth` build tag, wired via a `main_oauth.go` init function that sets a package-level `setupOAuth` variable.

**Why:** This matches the SDK's own build tag strategy and lets users build a smaller, dependency-light binary if they only need header-based auth. The `main.go` checks `setupOAuth != nil` to decide whether OAuth is available — clean separation with no import-time panics.

### 5. stdlib over third-party dependencies

**Decision:** Use standard library for everything except the MCP SDK: `flag` for CLI, `net/http` for the callback server, `log/slog` for logging, `crypto/*` for hashing, `os/exec` for browser launching, `syscall.Flock` for lockfiles.

**Why:** Minimizing dependencies reduces supply chain risk, simplifies auditing, and keeps the binary small. The functionality we need is well-served by stdlib:
- `flag` is sufficient for our CLI (13 flags, one positional arg)
- `net/http` handles the OAuth callback server (one endpoint, no middleware needed)
- `slog` (stdlib since Go 1.21) provides structured logging with levels

### 6. Token storage compatible with mcp-remote's directory

**Decision:** Store tokens in `~/.mcp-auth/` (same directory as mcp-remote) but with a different file naming scheme.

**Why:** Using the same directory makes migration feel natural and avoids a second config directory. We use `sha256(serverURL)_tokens.json` as the file name (mcp-remote uses a similar scheme but with different hashing). Files are written with `0600` permissions for security.

**Trade-off:** Existing mcp-remote tokens are not reused — users re-authenticate once after switching. Full compatibility would require reverse-engineering mcp-remote's exact hashing and file format, which is fragile and not worth the complexity.

### 7. Lockfile-based OAuth coordination

**Decision:** Use advisory file locking (`syscall.Flock`) to prevent multiple mcp-shuttle instances from opening duplicate browser windows for the same server.

**Why:** MCP clients may spawn multiple server instances (e.g., reconnecting after a crash). Without coordination, each instance would independently trigger the OAuth flow, causing multiple browser tabs and confusing the user. The lockfile pattern:
1. First instance acquires the lock and opens the browser
2. Subsequent instances detect the lock and poll `~/.mcp-auth/` for the token
3. Lock includes PID and timestamp for staleness detection (>30 minutes or dead process)

**Trade-off:** This only works for instances on the same machine (shared filesystem). Cross-machine coordination would require a different mechanism, but that's an unusual scenario for a stdio proxy.

### 8. Transport fallback strategy

**Decision:** Default to `http-first` (try HTTP Streamable, fall back to SSE on any connection error).

**Why:** HTTP Streamable is the current MCP spec (2025-03-26) and supports session management and resumability. SSE is the legacy transport (2024-11-05). Most new servers implement HTTP Streamable, but many existing servers still only support SSE. Trying HTTP first with automatic fallback gives the best compatibility without user configuration.

**Trade-off:** The fallback currently triggers on any connection error, not just HTTP 404/405. This is intentionally broad — we'd rather successfully connect via fallback than fail on an unexpected error type. This can be tightened as we see real-world error patterns.

### 9. Message interceptors for extensibility

**Decision:** The proxy accepts `MessageInterceptor` functions for each direction (local→remote and remote→local) that can inspect, modify, block, or respond to messages.

**Why:** Tool filtering needs to intercept messages in both directions (block `tools/call` requests and filter `tools/list` responses). Rather than hardcoding this into the forwarding loop, the interceptor pattern makes the proxy extensible for future features (rate limiting, logging, message transformation) without modifying core code.

## Project structure

```
mcp-shuttle/
├── main.go                     # Entry point, CLI parsing, transport wiring
├── main_oauth.go               # OAuth setup (behind build tag)
├── Makefile                    # Build/test with correct tags
│
├── internal/
│   ├── cli/
│   │   ├── cli.go              # Config struct, flag parsing
│   │   └── cli_test.go
│   │
│   ├── proxy/
│   │   ├── proxy.go            # Bidirectional forwarding, interceptors, tool filter
│   │   └── proxy_test.go
│   │
│   ├── transport/
│   │   ├── fallback.go         # Try-primary-then-secondary transport
│   │   ├── fallback_test.go
│   │   ├── headers.go          # HTTP RoundTripper that injects custom headers
│   │   └── headers_test.go
│   │
│   ├── oauth/                  # All behind mcp_go_client_oauth build tag
│   │   ├── handler.go          # Persistent OAuthHandler wrapping SDK
│   │   ├── callback.go         # Local HTTP server for OAuth redirect
│   │   ├── callback_test.go
│   │   ├── browser.go          # Platform-specific browser launch
│   │   ├── storage.go          # Token persistence to ~/.mcp-auth/
│   │   └── storage_test.go
│   │
│   ├── filter/
│   │   ├── tool.go             # Wildcard matching for tool names
│   │   └── tool_test.go
│   │
│   └── lockfile/
│       ├── lockfile.go         # Advisory file locking for OAuth coordination
│       └── lockfile_test.go
│
├── DESIGN.md                   # This document
├── README.md                   # User-facing documentation
├── LICENSE                     # MIT
├── go.mod
└── go.sum
```

## Standards implemented

| Standard | Implementation |
|----------|---------------|
| [MCP Streamable HTTP Transport (2025-03-26)](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#streamable-http) | Via SDK `StreamableClientTransport` |
| [MCP SSE Transport (2024-11-05)](https://modelcontextprotocol.io/specification/2024-11-05/basic/transports#http-with-sse) | Via SDK `SSEClientTransport` |
| [MCP Authorization (2025-03-26)](https://modelcontextprotocol.io/specification/draft/basic/authorization) | Via SDK `AuthorizationCodeHandler` |
| [OAuth 2.1](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1) | Via SDK + `golang.org/x/oauth2` |
| [RFC 9728 - Protected Resource Metadata](https://www.rfc-editor.org/rfc/rfc9728) | Via SDK `oauthex` package |
| [RFC 7591 - Dynamic Client Registration](https://www.rfc-editor.org/rfc/rfc7591) | Via SDK `oauthex.RegisterClient` |
| [RFC 8414 - Authorization Server Metadata](https://www.rfc-editor.org/rfc/rfc8414) | Via SDK `oauthex.GetAuthServerMeta` |
| [RFC 7636 - PKCE](https://datatracker.ietf.org/doc/html/rfc7636) | Via `oauth2.GenerateVerifier()` + S256 |
| [JSON-RPC 2.0](https://www.jsonrpc.org/specification) | Via SDK `jsonrpc` package |

### 10. Token encryption at rest

**Decision:** Encrypt stored tokens using AES-256-GCM with a key derived via HKDF from a machine-specific secret (UID + username + hostname + home directory).

**Why:** Plaintext tokens in `~/.mcp-auth/` are vulnerable to casual file access on shared systems or if backups are compromised. AES-256-GCM provides authenticated encryption. The machine-specific key means tokens encrypted on one machine/user can't be decrypted by another, providing defense-in-depth alongside the `0600` file permissions.

**Trade-off:** The key derivation uses non-secret inputs (hostname, UID, home path). This protects against casual access and cross-machine/cross-user scenarios, but not against a determined attacker with root access on the same machine. True secret-based encryption would require a user-provided passphrase or OS keyring integration, which adds UX friction.

**Backward compatibility:** Files without the `mcp-shuttle-enc:1:` prefix are treated as legacy plaintext and loaded transparently. New saves always encrypt.

### 11. Static OAuth client credentials

**Decision:** Support pre-registered OAuth clients via `--oauth-client-id` and `--oauth-client-secret` flags, using the SDK's `PreregisteredClientConfig`.

**Why:** Some OAuth servers don't support Dynamic Client Registration. Pre-registered clients are common in enterprise environments where the admin provisions client credentials ahead of time. The SDK's `AuthorizationCodeHandler` already supports both registration methods — we just need to configure it based on CLI flags.

**Validation:** The flags must be used together (both or neither). When set, `PreregisteredClientConfig` takes priority; when absent, `DynamicClientRegistrationConfig` is used as before.

### 12. Cross-platform lockfile support

**Decision:** Split lockfile implementation into platform-specific files using build tags: `lockfile_unix.go` (Flock) and `lockfile_windows.go` (LockFileEx).

**Why:** The original implementation used `syscall.Flock` which only works on Unix. Windows requires `LockFileEx` from `kernel32.dll` for advisory file locking, and `OpenProcess`/`GetExitCodeProcess` for process liveness checks. Platform-specific build tags are the idiomatic Go approach.

## Known limitations

1. **SSE transport lacks OAuth integration.** The SDK's `SSEClientTransport` does not have an `OAuthHandler` field. OAuth works with HTTP Streamable transport (the default). For SSE-only servers that require OAuth, a custom `http.RoundTripper` would need to inject bearer tokens — this is not yet implemented.

2. **Tool filter matches response shape, not method.** Since JSON-RPC responses don't carry the method name, the remote→local interceptor checks for the presence of a `"tools"` array in the response. This heuristic could theoretically match non-tools/list responses that happen to have a `"tools"` key.

3. **Client ID Metadata Documents not supported.** The SDK supports CIMD-based registration, but mcp-shuttle doesn't yet expose flags for this registration method.

## Future work

- Client ID Metadata Documents support
- SSE transport OAuth via bearer token RoundTripper
- Request ID tracking for precise tool filter response matching
- Prometheus metrics endpoint for observability
- Configurable token storage directory (`MCP_SHUTTLE_CONFIG_DIR`)
- OS keyring integration for encryption key storage
