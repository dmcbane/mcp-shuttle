# mcp-shuttle

A standalone MCP (Model Context Protocol) stdio-to-remote transport proxy written in Go. No Node.js required.

Drop-in replacement for [mcp-remote](https://github.com/geelen/mcp-remote) — a single static binary with zero runtime dependencies.

## Install

```bash
go install github.com/dmcbane/mcp-shuttle@latest
```

Or build from source:

```bash
git clone https://github.com/dmcbane/mcp-shuttle.git
cd mcp-shuttle
make build
```

This builds with OAuth support enabled. You can also use `go build -tags mcp_go_client_oauth -o mcp-shuttle .` directly.

## Usage

Configure in your MCP client (Claude Desktop, Claude Code, Cursor, etc.):

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

### With API key authentication

```json
{
  "mcpServers": {
    "my-server": {
      "command": "mcp-shuttle",
      "args": [
        "https://mcp.example.com",
        "--header", "Authorization: Bearer your-api-key"
      ]
    }
  }
}
```

### Options

```
  -header value
        Custom header in 'Name: Value' format (repeatable)
  -transport string
        Transport strategy: http-first, sse-first, http-only, sse-only (default "http-first")
  -port int
        OAuth callback port (default 3334)
  -allow-http
        Allow unencrypted HTTP connections
  -debug
        Enable debug logging to stderr
  -silent
        Suppress all logging output
```

### Transport modes

| Mode | Behavior |
|------|----------|
| `http-first` (default) | Try HTTP Streamable, fall back to SSE |
| `sse-first` | Try SSE, fall back to HTTP Streamable |
| `http-only` | HTTP Streamable only |
| `sse-only` | SSE only |

### OAuth authentication

For servers that require OAuth 2.1, mcp-shuttle handles the entire flow automatically:

1. Opens your browser for authorization
2. Runs a local callback server to receive the auth code
3. Exchanges it for tokens via Dynamic Client Registration (RFC 7591)
4. Stores tokens in `~/.mcp-auth/` (0600 permissions)
5. Auto-refreshes expired tokens on subsequent runs

No configuration needed — just point at the server:

```json
{
  "mcpServers": {
    "my-oauth-server": {
      "command": "mcp-shuttle",
      "args": ["https://mcp.example.com"]
    }
  }
}
```

Use `--port` to change the OAuth callback port (default 3334).

## Status

Phase 1 (core proxy) and Phase 2 (OAuth 2.1) are complete. Tool filtering (Phase 3) is in progress.

## License

MIT
