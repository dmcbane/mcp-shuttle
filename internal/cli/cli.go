package cli

import (
	"flag"
	"fmt"
	"strings"
)

// TransportMode controls how the proxy connects to the remote server.
type TransportMode string

const (
	TransportHTTPFirst TransportMode = "http-first"
	TransportSSEFirst  TransportMode = "sse-first"
	TransportHTTPOnly  TransportMode = "http-only"
	TransportSSEOnly   TransportMode = "sse-only"
)

// headerList is a flag.Value that collects repeated --header flags.
type headerList []string

func (h *headerList) String() string { return strings.Join(*h, ", ") }
func (h *headerList) Set(val string) error {
	*h = append(*h, val)
	return nil
}

// Config holds all CLI configuration for mcp-shuttle.
type Config struct {
	ServerURL    string
	CallbackPort int
	Headers      map[string]string
	Transport    TransportMode
	AllowHTTP    bool
	Debug        bool
	Silent       bool
	IgnoreTools  []string
	Resource     string
}

// Parse parses CLI arguments and returns a Config. The args slice should not
// include the program name (i.e., pass os.Args[1:]).
//
// Supports both "mcp-shuttle <url> [flags]" and "mcp-shuttle [flags] <url>"
// since flag.Parse stops at the first non-flag argument.
func Parse(args []string) (*Config, error) {
	// Separate positional args from flags so order doesn't matter.
	var flagArgs []string
	var positional []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flagArgs = append(flagArgs, args[i])
			// If it's a flag that takes a value (not a boolean), consume the next arg too.
			// We check by looking at known value-bearing flags.
			switch args[i] {
			case "--header", "--transport", "--port", "--ignore-tool", "--resource":
				if i+1 < len(args) {
					i++
					flagArgs = append(flagArgs, args[i])
				}
			}
		} else {
			positional = append(positional, args[i])
		}
	}

	fs := flag.NewFlagSet("mcp-shuttle", flag.ContinueOnError)

	var headers headerList
	var ignoreTools headerList
	fs.Var(&headers, "header", "Custom header in 'Name: Value' format (repeatable)")
	fs.Var(&ignoreTools, "ignore-tool", "Tool name pattern to hide (wildcard, repeatable)")

	transport := fs.String("transport", "http-first", "Transport strategy: http-first, sse-first, http-only, sse-only")
	callbackPort := fs.Int("port", 3334, "OAuth callback port")
	allowHTTP := fs.Bool("allow-http", false, "Allow unencrypted HTTP connections")
	debug := fs.Bool("debug", false, "Enable debug logging to stderr")
	silent := fs.Bool("silent", false, "Suppress all logging output")
	resource := fs.String("resource", "", "Resource identifier for OAuth session isolation")

	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}

	if len(positional) < 1 {
		return nil, fmt.Errorf("usage: mcp-shuttle <server-url> [options]")
	}

	serverURL := positional[0]

	mode := TransportMode(*transport)
	switch mode {
	case TransportHTTPFirst, TransportSSEFirst, TransportHTTPOnly, TransportSSEOnly:
	default:
		return nil, fmt.Errorf("invalid transport mode: %q", *transport)
	}

	if !*allowHTTP && strings.HasPrefix(serverURL, "http://") {
		return nil, fmt.Errorf("refusing unencrypted HTTP connection to %s (use --allow-http to override)", serverURL)
	}

	parsedHeaders := make(map[string]string, len(headers))
	for _, h := range headers {
		name, value, ok := strings.Cut(h, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header format %q, expected 'Name: Value'", h)
		}
		parsedHeaders[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}

	return &Config{
		ServerURL:    serverURL,
		CallbackPort: *callbackPort,
		Headers:      parsedHeaders,
		Transport:    mode,
		AllowHTTP:    *allowHTTP,
		Debug:        *debug,
		Silent:       *silent,
		IgnoreTools:  ignoreTools,
		Resource:     *resource,
	}, nil
}
