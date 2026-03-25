package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProxy_ForwardsMessages(t *testing.T) {
	// Create two pairs of in-memory transports.
	// localClient <-> localServer (proxy local side)
	// remoteClient (proxy remote side) <-> remoteServer
	localClient, localServer := mcp.NewInMemoryTransports()
	remoteClient, remoteServer := mcp.NewInMemoryTransports()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	p := &Proxy{Logger: logger}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run the proxy in the background.
	errCh := make(chan error, 1)
	go func() {
		errCh <- p.Run(ctx, localServer, remoteClient)
	}()

	// Connect the endpoints we'll use to send/receive test messages.
	localConn, err := localClient.Connect(ctx)
	if err != nil {
		t.Fatalf("connect local client: %v", err)
	}
	defer localConn.Close()

	remoteConn, err := remoteServer.Connect(ctx)
	if err != nil {
		t.Fatalf("connect remote server: %v", err)
	}
	defer remoteConn.Close()

	// Send a JSON-RPC request from local → remote.
	reqID, err := jsonrpc.MakeID(float64(1))
	if err != nil {
		t.Fatalf("make ID: %v", err)
	}
	req := &jsonrpc.Request{
		ID:     reqID,
		Method: "tools/list",
		Params: json.RawMessage(`{}`),
	}

	if err := localConn.Write(ctx, req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// Read the forwarded request on the remote side.
	msg, err := remoteConn.Read(ctx)
	if err != nil {
		t.Fatalf("read request on remote: %v", err)
	}

	got, ok := msg.(*jsonrpc.Request)
	if !ok {
		t.Fatalf("expected *jsonrpc.Request, got %T", msg)
	}
	if got.Method != "tools/list" {
		t.Errorf("got method=%q, want %q", got.Method, "tools/list")
	}

	// Send a response back from remote → local.
	resp := &jsonrpc.Response{
		ID:     reqID,
		Result: json.RawMessage(`{"tools":[]}`),
	}
	if err := remoteConn.Write(ctx, resp); err != nil {
		t.Fatalf("write response: %v", err)
	}

	// Read the forwarded response on the local side.
	msg, err = localConn.Read(ctx)
	if err != nil {
		t.Fatalf("read response on local: %v", err)
	}

	gotResp, ok := msg.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected *jsonrpc.Response, got %T", msg)
	}
	if string(gotResp.Result) != `{"tools":[]}` {
		t.Errorf("got result=%s, want %s", gotResp.Result, `{"tools":[]}`)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Logf("proxy exited: %v", err) // context.Canceled is expected
	}
}
