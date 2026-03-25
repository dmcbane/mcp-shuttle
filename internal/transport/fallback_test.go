package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// errTransport always fails to connect.
type errTransport struct{ err error }

func (e *errTransport) Connect(context.Context) (mcp.Connection, error) {
	return nil, e.err
}

func TestFallbackTransport_UsePrimaryWhenAvailable(t *testing.T) {
	primary, _ := mcp.NewInMemoryTransports()
	secondary, _ := mcp.NewInMemoryTransports()

	ft := &FallbackTransport{
		Primary:   primary,
		Secondary: secondary,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	ctx := context.Background()
	conn, err := ft.Connect(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer conn.Close()
}

func TestFallbackTransport_FallsBackOnPrimaryFailure(t *testing.T) {
	primary := &errTransport{err: fmt.Errorf("HTTP 404: Not Found")}
	secondary, _ := mcp.NewInMemoryTransports()

	ft := &FallbackTransport{
		Primary:   primary,
		Secondary: secondary,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	ctx := context.Background()
	conn, err := ft.Connect(ctx)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	defer conn.Close()
}

func TestFallbackTransport_BothFail(t *testing.T) {
	primary := &errTransport{err: errors.New("primary failed")}
	secondary := &errTransport{err: errors.New("secondary failed")}

	ft := &FallbackTransport{
		Primary:   primary,
		Secondary: secondary,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	ctx := context.Background()
	_, err := ft.Connect(ctx)
	if err == nil {
		t.Fatal("expected error when both transports fail")
	}
}

func TestShouldFallback_OnlyOn404And405(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"404 Not Found", fmt.Errorf("HTTP 404: Not Found"), true},
		{"405 Method Not Allowed", fmt.Errorf("HTTP 405: Method Not Allowed"), true},
		{"status 404 in wrapped error", fmt.Errorf("request failed: %w", fmt.Errorf("status 404")), true},
		{"status 405 in wrapped error", fmt.Errorf("request failed: %w", fmt.Errorf("status 405")), true},
		{"TLS handshake error", fmt.Errorf("tls: handshake failure"), false},
		{"x509 certificate error", fmt.Errorf("x509: certificate signed by unknown authority"), false},
		{"certificate keyword", fmt.Errorf("certificate verify failed"), false},
		{"401 Unauthorized", fmt.Errorf("HTTP 401: Unauthorized"), false},
		{"403 Forbidden", fmt.Errorf("HTTP 403: Forbidden"), false},
		{"connection refused", fmt.Errorf("dial tcp 127.0.0.1:8080: connection refused"), false},
		{"DNS failure", fmt.Errorf("lookup nonexistent.example.com: no such host"), false},
		{"timeout", fmt.Errorf("context deadline exceeded"), false},
		{"generic error", errors.New("something went wrong"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFallback(tt.err)
			if got != tt.want {
				t.Errorf("shouldFallback(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestFallbackTransport_NoFallbackOnTLSError(t *testing.T) {
	primary := &errTransport{err: fmt.Errorf("tls: handshake failure")}
	secondary, _ := mcp.NewInMemoryTransports()

	ft := &FallbackTransport{
		Primary:   primary,
		Secondary: secondary,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	ctx := context.Background()
	_, err := ft.Connect(ctx)
	if err == nil {
		t.Fatal("expected TLS error to NOT trigger fallback")
	}
}

func TestFallbackTransport_NoFallbackOnAuthError(t *testing.T) {
	primary := &errTransport{err: fmt.Errorf("HTTP 401: Unauthorized")}
	secondary, _ := mcp.NewInMemoryTransports()

	ft := &FallbackTransport{
		Primary:   primary,
		Secondary: secondary,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	ctx := context.Background()
	_, err := ft.Connect(ctx)
	if err == nil {
		t.Fatal("expected auth error to NOT trigger fallback")
	}
}
