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
go build -o mcp-shuttle .
```

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

## Status

Phase 1 (core proxy) is complete. OAuth support (Phase 2) and tool filtering (Phase 3) are in progress.

## License

MIT
