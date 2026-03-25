package transport

import (
	"context"
	"errors"
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
	primary := &errTransport{err: errors.New("connection refused")}
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
