package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessageInterceptor can inspect and modify messages in flight.
// It receives the raw message and returns:
//   - the message to forward (possibly modified)
//   - a response to send back to the source instead of forwarding (for blocking)
//   - an error if something went wrong
//
// To forward unchanged: return (msg, nil, nil)
// To block with a response: return (nil, response, nil)
// To drop silently: return (nil, nil, nil)
type MessageInterceptor func(msg jsonrpc.Message) (forward jsonrpc.Message, respond jsonrpc.Message, err error)

// Proxy shuttles JSON-RPC messages between a local and remote MCP connection.
type Proxy struct {
	Logger *slog.Logger

	// OnLocalToRemote is called for every message from local to remote.
	OnLocalToRemote MessageInterceptor
	// OnRemoteToLocal is called for every message from remote to local.
	OnRemoteToLocal MessageInterceptor
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
		errCh <- p.forward(ctx, localConn, remoteConn, "local→remote", p.OnLocalToRemote, logger)
		cancel()
	}()
	go func() {
		defer wg.Done()
		errCh <- p.forward(ctx, remoteConn, localConn, "remote→local", p.OnRemoteToLocal, logger)
		cancel()
	}()

	wg.Wait()
	close(errCh)

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

func (p *Proxy) forward(ctx context.Context, src, dst mcp.Connection, direction string, intercept MessageInterceptor, logger *slog.Logger) error {
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

		if intercept != nil {
			fwd, respond, ierr := intercept(msg)
			if ierr != nil {
				logger.Error("interceptor error", "direction", direction, "error", ierr)
				continue
			}
			if respond != nil {
				if err := src.Write(ctx, respond); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					logger.Error("write response error", "direction", direction, "error", err)
				}
				continue
			}
			if fwd == nil {
				continue
			}
			msg = fwd
		}

		if err := dst.Write(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Error("write error", "direction", direction, "error", err)
			return err
		}
	}
}

// idKey returns a string key for a jsonrpc.ID, suitable for use as a map key.
func idKey(id jsonrpc.ID) string {
	if !id.IsValid() {
		return "<nil>"
	}
	return fmt.Sprintf("%v", id.Raw())
}

// ToolFilterInterceptors returns interceptors that filter tools based on
// --ignore-tool wildcard patterns. Returns nil, nil if patterns is empty.
//
// Uses request-ID correlation: the local→remote interceptor tracks IDs of
// tools/list requests, and the remote→local interceptor only filters responses
// that match a tracked ID. This prevents accidentally filtering unrelated
// responses that happen to contain a "tools" key.
func ToolFilterInterceptors(patterns []string, logger *slog.Logger) (localToRemote, remoteToLocal MessageInterceptor) {
	if len(patterns) == 0 {
		return nil, nil
	}

	// Track pending tools/list request IDs for precise response filtering.
	var mu sync.Mutex
	pendingListIDs := make(map[string]bool)

	localToRemote = func(msg jsonrpc.Message) (jsonrpc.Message, jsonrpc.Message, error) {
		req, ok := msg.(*jsonrpc.Request)
		if !ok {
			return msg, nil, nil
		}

		// Track tools/list request IDs so we can filter the matching response.
		if req.Method == "tools/list" && req.ID.IsValid() {
			key := idKey(req.ID)
			mu.Lock()
			pendingListIDs[key] = true
			mu.Unlock()
		}

		if req.Method != "tools/call" {
			return msg, nil, nil
		}

		var call struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Params, &call); err != nil {
			return msg, nil, nil
		}
		if matchesAny(patterns, call.Name) {
			logger.Info("blocked tool call", "tool", call.Name)
			resp := &jsonrpc.Response{
				ID: req.ID,
				Error: &jsonrpc.Error{
					Code:    jsonrpc.CodeMethodNotFound,
					Message: "tool not found: " + call.Name,
				},
			}
			return nil, resp, nil
		}
		return msg, nil, nil
	}

	remoteToLocal = func(msg jsonrpc.Message) (jsonrpc.Message, jsonrpc.Message, error) {
		resp, ok := msg.(*jsonrpc.Response)
		if !ok || resp.Result == nil {
			return msg, nil, nil
		}

		// Only filter responses that correspond to a tracked tools/list request.
		key := idKey(resp.ID)
		mu.Lock()
		isToolsList := pendingListIDs[key]
		if isToolsList {
			delete(pendingListIDs, key)
		}
		mu.Unlock()

		if !isToolsList {
			return msg, nil, nil
		}

		// Parse and filter the tools array.
		var result map[string]json.RawMessage
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return msg, nil, nil
		}
		toolsRaw, ok := result["tools"]
		if !ok {
			return msg, nil, nil
		}

		var tools []json.RawMessage
		if err := json.Unmarshal(toolsRaw, &tools); err != nil {
			return msg, nil, nil
		}

		filtered := make([]json.RawMessage, 0, len(tools))
		for _, raw := range tools {
			var entry struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(raw, &entry); err != nil {
				filtered = append(filtered, raw)
				continue
			}
			if !matchesAny(patterns, entry.Name) {
				filtered = append(filtered, raw)
			} else {
				logger.Debug("filtered tool from list", "tool", entry.Name)
			}
		}

		result["tools"], _ = json.Marshal(filtered)
		resp.Result, _ = json.Marshal(result)
		return resp, nil, nil
	}

	return localToRemote, remoteToLocal
}

func matchesAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if matched, _ := path.Match(p, name); matched {
			return true
		}
	}
	return false
}
