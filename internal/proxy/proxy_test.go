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

func TestProxy_ToolFilterBlocksCall(t *testing.T) {
	localClient, localServer := mcp.NewInMemoryTransports()
	remoteClient, remoteServer := mcp.NewInMemoryTransports()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	toRemote, toLocal := ToolFilterInterceptors([]string{"delete*"}, logger)
	p := &Proxy{
		Logger:          logger,
		OnLocalToRemote: toRemote,
		OnRemoteToLocal: toLocal,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() { p.Run(ctx, localServer, remoteClient) }()

	localConn, _ := localClient.Connect(ctx)
	defer localConn.Close()
	// We won't read from remoteServer for this test — the call should be blocked.
	_ = remoteServer

	reqID, _ := jsonrpc.MakeID(float64(1))
	req := &jsonrpc.Request{
		ID:     reqID,
		Method: "tools/call",
		Params: json.RawMessage(`{"name":"delete_user","arguments":{}}`),
	}
	if err := localConn.Write(ctx, req); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Should receive an error response back (not forwarded to remote).
	msg, err := localConn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	resp, ok := msg.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected *jsonrpc.Response, got %T", msg)
	}
	if resp.Error == nil {
		t.Fatal("expected error response for blocked tool call")
	}

	cancel()
}

func TestProxy_ToolFilterFiltersListResponse(t *testing.T) {
	localClient, localServer := mcp.NewInMemoryTransports()
	remoteClient, remoteServer := mcp.NewInMemoryTransports()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	toRemote, toLocal := ToolFilterInterceptors([]string{"delete*", "*admin*"}, logger)
	p := &Proxy{
		Logger:          logger,
		OnLocalToRemote: toRemote,
		OnRemoteToLocal: toLocal,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() { p.Run(ctx, localServer, remoteClient) }()

	localConn, _ := localClient.Connect(ctx)
	defer localConn.Close()
	remoteConn, _ := remoteServer.Connect(ctx)
	defer remoteConn.Close()

	// Send tools/list request from local.
	reqID, _ := jsonrpc.MakeID(float64(1))
	req := &jsonrpc.Request{
		ID:     reqID,
		Method: "tools/list",
		Params: json.RawMessage(`{}`),
	}
	if err := localConn.Write(ctx, req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// Read it on remote side and reply with tools.
	msg, _ := remoteConn.Read(ctx)
	_ = msg
	resp := &jsonrpc.Response{
		ID:     reqID,
		Result: json.RawMessage(`{"tools":[{"name":"read_file"},{"name":"delete_file"},{"name":"super_admin"}]}`),
	}
	if err := remoteConn.Write(ctx, resp); err != nil {
		t.Fatalf("write response: %v", err)
	}

	// Read the filtered response on local side.
	rmsg, err := localConn.Read(ctx)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	gotResp, ok := rmsg.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected *jsonrpc.Response, got %T", rmsg)
	}

	// Should only contain read_file.
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(gotResp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d: %+v", len(result.Tools), result.Tools)
	}
	if result.Tools[0].Name != "read_file" {
		t.Errorf("expected tool name 'read_file', got %q", result.Tools[0].Name)
	}

	cancel()
}

func TestProxy_MaxMessageSize_DropsOversized(t *testing.T) {
	localClient, localServer := mcp.NewInMemoryTransports()
	remoteClient, remoteServer := mcp.NewInMemoryTransports()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	p := &Proxy{
		Logger:         logger,
		MaxMessageSize: 100, // very small limit
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() { p.Run(ctx, localServer, remoteClient) }()

	localConn, _ := localClient.Connect(ctx)
	defer localConn.Close()
	remoteConn, _ := remoteServer.Connect(ctx)
	defer remoteConn.Close()

	// Send a small message — should pass through.
	smallID, _ := jsonrpc.MakeID(float64(1))
	smallReq := &jsonrpc.Request{
		ID:     smallID,
		Method: "ping",
		Params: json.RawMessage(`{}`),
	}
	if err := localConn.Write(ctx, smallReq); err != nil {
		t.Fatalf("write small: %v", err)
	}
	msg, err := remoteConn.Read(ctx)
	if err != nil {
		t.Fatalf("read small: %v", err)
	}
	if req, ok := msg.(*jsonrpc.Request); !ok || req.Method != "ping" {
		t.Fatalf("expected ping request, got %T", msg)
	}

	// Send an oversized message — should be dropped.
	bigID, _ := jsonrpc.MakeID(float64(2))
	bigPayload := make([]byte, 200)
	for i := range bigPayload {
		bigPayload[i] = 'x'
	}
	bigReq := &jsonrpc.Request{
		ID:     bigID,
		Method: "tools/call",
		Params: json.RawMessage(`{"data":"` + string(bigPayload) + `"}`),
	}
	if err := localConn.Write(ctx, bigReq); err != nil {
		t.Fatalf("write big: %v", err)
	}

	// Send another small message after — this should arrive,
	// proving the big one was dropped.
	nextID, _ := jsonrpc.MakeID(float64(3))
	nextReq := &jsonrpc.Request{
		ID:     nextID,
		Method: "pong",
		Params: json.RawMessage(`{}`),
	}
	if err := localConn.Write(ctx, nextReq); err != nil {
		t.Fatalf("write next: %v", err)
	}
	msg, err = remoteConn.Read(ctx)
	if err != nil {
		t.Fatalf("read next: %v", err)
	}
	if req, ok := msg.(*jsonrpc.Request); !ok || req.Method != "pong" {
		t.Fatalf("expected pong request (oversized msg should be dropped), got %T %v", msg, msg)
	}

	cancel()
}

func TestProxy_ToolFilterOnlyFiltersToolsListResponses(t *testing.T) {
	// Verify that responses with a "tools" key that aren't replies to
	// tools/list requests are NOT filtered (request-ID correlation).
	localClient, localServer := mcp.NewInMemoryTransports()
	remoteClient, remoteServer := mcp.NewInMemoryTransports()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	toRemote, toLocal := ToolFilterInterceptors([]string{"delete*"}, logger)
	p := &Proxy{
		Logger:          logger,
		OnLocalToRemote: toRemote,
		OnRemoteToLocal: toLocal,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() { p.Run(ctx, localServer, remoteClient) }()

	localConn, _ := localClient.Connect(ctx)
	defer localConn.Close()
	remoteConn, _ := remoteServer.Connect(ctx)
	defer remoteConn.Close()

	// Send a NON-tools/list request (e.g., some custom method).
	reqID, _ := jsonrpc.MakeID(float64(42))
	req := &jsonrpc.Request{
		ID:     reqID,
		Method: "resources/list",
		Params: json.RawMessage(`{}`),
	}
	if err := localConn.Write(ctx, req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// Read it on remote side.
	msg, _ := remoteConn.Read(ctx)
	_ = msg

	// Reply with a response that happens to have a "tools" key
	// (should NOT be filtered since it's not a tools/list response).
	resp := &jsonrpc.Response{
		ID:     reqID,
		Result: json.RawMessage(`{"tools":[{"name":"delete_file"},{"name":"read_file"}]}`),
	}
	if err := remoteConn.Write(ctx, resp); err != nil {
		t.Fatalf("write response: %v", err)
	}

	// Read the response on local side — should be UNFILTERED.
	rmsg, err := localConn.Read(ctx)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	gotResp, ok := rmsg.(*jsonrpc.Response)
	if !ok {
		t.Fatalf("expected *jsonrpc.Response, got %T", rmsg)
	}

	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(gotResp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	// Both tools should be present — no filtering for non-tools/list responses.
	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools (unfiltered), got %d: %+v", len(result.Tools), result.Tools)
	}

	cancel()
}
