package proxy

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessageFilter is a function that can inspect or modify messages in flight.
// Return the message to forward it, or return nil to drop it.
// Returning a non-nil error sends a JSON-RPC error response (for requests).
type MessageFilter func(msg any) (any, error)

// Proxy shuttles JSON-RPC messages between a local and remote MCP connection.
type Proxy struct {
	Logger *slog.Logger

	// Optional filters for messages flowing in each direction.
	ToRemote MessageFilter
	ToLocal  MessageFilter
}

// Run connects both transports and forwards messages bidirectionally until
// the context is cancelled or either connection closes.
func (p *Proxy) Run(ctx context.Context, local, remote mcp.Transport) error {
	logger := p.Logger
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger.Info("connecting local transport")
	localConn, err := local.Connect(ctx)
	if err != nil {
		return errors.Join(errors.New("failed to connect local transport"), err)
	}
	defer localConn.Close()

	logger.Info("connecting remote transport")
	remoteConn, err := remote.Connect(ctx)
	if err != nil {
		return errors.Join(errors.New("failed to connect remote transport"), err)
	}
	defer remoteConn.Close()

	logger.Info("proxy connected", "session_id", remoteConn.SessionID())

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		errCh <- p.forward(ctx, localConn, remoteConn, "local→remote", logger)
		cancel()
	}()
	go func() {
		defer wg.Done()
		errCh <- p.forward(ctx, remoteConn, localConn, "remote→local", logger)
		cancel()
	}()

	wg.Wait()
	close(errCh)

	// Collect errors, ignoring context cancellation (normal shutdown).
	var errs []error
	for e := range errCh {
		if e != nil && !errors.Is(e, context.Canceled) {
			errs = append(errs, e)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (p *Proxy) forward(ctx context.Context, src, dst mcp.Connection, direction string, logger *slog.Logger) error {
	for {
		msg, err := src.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Error("read error", "direction", direction, "error", err)
			return err
		}

		logger.Debug("forwarding message", "direction", direction)

		if err := dst.Write(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Error("write error", "direction", direction, "error", err)
			return err
		}
	}
}
