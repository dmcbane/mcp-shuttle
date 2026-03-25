# mcp-shuttle

A standalone MCP (Model Context Protocol) stdio-to-remote transport proxy written in Go. No Node.js required.

Drop-in replacement for [mcp-remote](https://github.com/geelen/mcp-remote) — a single static binary with zero runtime dependencies.

## Why mcp-shuttle?

`mcp-remote` requires Node.js, which means startup latency from `npx`, a heavyweight runtime for what is essentially a thin bidirectional proxy, and a class of troubleshooting issues tied to Node.js version mismatches, npm caching, and VPN certificate handling. `mcp-shuttle` is a single ~12MB Go binary that starts instantly and has zero runtime dependencies.

## Install

```bash
go install -tags mcp_go_client_oauth github.com/dmcbane/mcp-shuttle@latest
```

Or build from source:

```bash
git clone https://github.com/dmcbane/mcp-shuttle.git
cd mcp-shuttle
make build
```

The `mcp_go_client_oauth` build tag enables OAuth 2.1 support. Without it, you get a smaller binary that only supports header-based authentication (API keys, bearer tokens).

## Quick start

Add to your MCP client configuration:

```json
{
  "mcpServers": {
    "my-server": {
      "command": "mcp-shuttle",
      "args": ["https://mcp.example.com"]
    }
  }
}
```

That's it. mcp-shuttle will connect using HTTP Streamable transport (falling back to SSE if the server doesn't support it) and handle OAuth automatically if the server requires it.

## Client configuration examples

### Claude Code

In `~/.claude/settings.json` (global) or `.mcp.json` (per-project):

```json
{
  "mcpServers": {
    "my-server": {
      "command": "mcp-shuttle",
      "args": ["https://mcp.example.com"]
    }
  }
}
```

### Claude Desktop

macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "my-server": {
      "command": "/full/path/to/mcp-shuttle",
      "args": ["https://mcp.example.com"]
    }
  }
}
```

> **Note:** Claude Desktop may not have `$GOPATH/bin` in its PATH. Use the full path to the binary, or symlink it to `/usr/local/bin/mcp-shuttle`.

### Cursor

In `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "my-server": {
      "command": "mcp-shuttle",
      "args": ["https://mcp.example.com"]
    }
  }
}
```

## Authentication

### API key / Bearer token

Pass static credentials via `--header`:

```json
{
  "command": "mcp-shuttle",
  "args": [
    "https://mcp.example.com",
    "--header", "Authorization: Bearer your-api-key"
  ]
}
```

You can use environment variables in your MCP client config to avoid hardcoding secrets:

```json
{
  "command": "mcp-shuttle",
  "args": [
    "https://mcp.example.com",
    "--header", "Authorization: Bearer ${MCP_API_KEY}"
  ],
  "env": {
    "MCP_API_KEY": "your-api-key"
  }
}
```

### OAuth 2.1

For servers that require OAuth, mcp-shuttle handles the entire flow automatically:

1. Detects a `401 Unauthorized` response from the server
2. Discovers the authorization server via [Protected Resource Metadata (RFC 9728)](https://www.rfc-editor.org/rfc/rfc9728)
3. Registers as a client via [Dynamic Client Registration (RFC 7591)](https://www.rfc-editor.org/rfc/rfc7591)
4. Opens your browser for authorization (with [PKCE S256](https://datatracker.ietf.org/doc/html/rfc7636))
5. Runs a local callback server on `127.0.0.1:3334` to receive the auth code
6. Exchanges the code for tokens and stores them in `~/.mcp-auth/`
7. Auto-refreshes expired tokens on subsequent requests

No OAuth configuration is needed — just point at the server URL:

```json
{
  "command": "mcp-shuttle",
  "args": ["https://oauth-protected-server.example.com"]
}
```

**OAuth options:**

| Flag | Description |
|------|-------------|
| `--port 3334` | OAuth callback port (default 3334, auto-selects if busy) |
| `--resource tenant-123` | Isolates OAuth sessions for multi-tenant servers |

**Token storage:** Tokens are stored as JSON in `~/.mcp-auth/` with `0600` permissions, keyed by a SHA-256 hash of the server URL (and resource identifier, if set). Tokens persist across process restarts.

**Multi-instance coordination:** If multiple mcp-shuttle instances connect to the same OAuth server simultaneously, only one will open the browser. Others wait for the first instance to complete the flow and then read the stored token from disk.

## Transport modes

mcp-shuttle supports two MCP transport protocols:

| Mode | Flag | Behavior |
|------|------|----------|
| **HTTP Streamable** (default) | `--transport http-first` | Modern MCP transport (spec 2025-03-26). Sends JSON-RPC via HTTP POST, receives responses as JSON or SSE streams. Supports session management and resumability. |
| **SSE** (legacy) | `--transport sse-first` | Legacy transport (spec 2024-11-05). Uses Server-Sent Events for server→client messages and HTTP POST for client→server. |

The default `http-first` mode tries HTTP Streamable first and falls back to SSE if the server returns a 404. Use `--transport sse-only` or `--transport http-only` to force a specific transport.

## Tool filtering

Hide specific tools from the MCP client using wildcard patterns:

```json
{
  "command": "mcp-shuttle",
  "args": [
    "https://mcp.example.com",
    "--ignore-tool", "delete*",
    "--ignore-tool", "*admin*",
    "--ignore-tool", "dangerous_tool"
  ]
}
```

Patterns use [Go's `path.Match`](https://pkg.go.dev/path#Match) syntax:
- `*` matches any sequence of characters
- `?` matches any single character
- `[abc]` matches any character in the set

Filtered tools are removed from `tools/list` responses. If a client somehow calls a filtered tool, it receives a JSON-RPC `MethodNotFound` error.

## All options

```
Usage: mcp-shuttle <server-url> [options]

Options:
  -header value
        Custom header in 'Name: Value' format (repeatable)
  -ignore-tool value
        Tool name pattern to hide (wildcard, repeatable)
  -transport string
        Transport strategy: http-first, sse-first, http-only, sse-only
        (default "http-first")
  -port int
        OAuth callback port (default 3334)
  -resource string
        Resource identifier for OAuth session isolation
  -allow-http
        Allow unencrypted HTTP connections
  -debug
        Enable debug logging to stderr
  -silent
        Suppress all logging output
```

Flags can appear before or after the server URL.

## Migrating from mcp-remote

Replace:

```json
{
  "command": "npx",
  "args": ["mcp-remote", "https://mcp.example.com/sse"]
}
```

With:

```json
{
  "command": "mcp-shuttle",
  "args": ["https://mcp.example.com/sse"]
}
```

**Flag mapping:**

| mcp-remote | mcp-shuttle | Notes |
|------------|-------------|-------|
| `--header "K:V"` | `--header "K:V"` | Same syntax |
| `--transport sse-first` | `--transport sse-first` | Same values |
| `--allow-http` | `--allow-http` | Same |
| `--ignore-tool "pattern"` | `--ignore-tool "pattern"` | Same wildcard syntax |
| `--resource "id"` | `--resource "id"` | Same |
| `--debug` | `--debug` | Same |
| `--silent` | `--silent` | Same |
| Second positional arg (port) | `--port 3334` | Named flag instead of positional |
| `--host 127.0.0.1` | Not needed | Always binds to 127.0.0.1 |
| `--enable-proxy` | Not needed | Go respects `HTTP_PROXY`/`HTTPS_PROXY` by default |
| `--static-oauth-client-metadata` | Not yet supported | Dynamic registration only |
| `--static-oauth-client-info` | Not yet supported | Dynamic registration only |

**Token storage compatibility:** mcp-shuttle stores tokens in the same `~/.mcp-auth/` directory as mcp-remote, but uses a different file naming scheme. Existing mcp-remote tokens will not be reused — you'll need to re-authenticate once.

## Troubleshooting

### Enable debug logging

```json
{
  "command": "mcp-shuttle",
  "args": ["https://mcp.example.com", "--debug"]
}
```

Debug output goes to stderr (never stdout, which is the MCP transport channel). In Claude Desktop, check the MCP server logs. In Claude Code, stderr output appears in the MCP server panel.

### OAuth browser doesn't open

If the browser fails to open automatically, mcp-shuttle prints the authorization URL to stderr. Copy and paste it into your browser manually.

### Port conflict on OAuth callback

If port 3334 is in use, mcp-shuttle automatically selects an ephemeral port. You can also specify a different port:

```json
{
  "args": ["https://mcp.example.com", "--port", "9999"]
}
```

### Connection refused / timeout

- Verify the server URL is correct and reachable
- For HTTPS issues, check your system's CA certificates
- For servers on private networks, Go respects `HTTP_PROXY` and `HTTPS_PROXY` environment variables automatically
- Use `--allow-http` only for local development servers (never in production)

### Clearing stored tokens

Delete the token files in `~/.mcp-auth/`:

```bash
rm ~/.mcp-auth/*_tokens.json
```

## Building

```bash
# Full build with OAuth support
make build

# Run tests
make test

# Build without OAuth (smaller binary, header auth only)
go build -o mcp-shuttle .

# Cross-compile
GOOS=darwin GOARCH=arm64 go build -tags mcp_go_client_oauth -o mcp-shuttle-darwin-arm64 .
GOOS=windows GOARCH=amd64 go build -tags mcp_go_client_oauth -o mcp-shuttle.exe .
```

## License

MIT
