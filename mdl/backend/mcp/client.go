// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
)

// Client is a minimal MCP "streamable HTTP" client for Studio Pro's PED server.
//
// It handles the three things a plain HTTP client cannot: the server answers a
// POST with either application/json or a text/event-stream (SSE) body, it
// assigns a session via the Mcp-Session-Id response header that must be echoed
// on subsequent calls, and Studio Pro's DNS-rebinding guard rejects any request
// whose Host header is not exactly "localhost". The client is derived from
// cmd/mcpprobe, which proved this handshake against Studio Pro 11.x.
type Client struct {
	http     *http.Client
	endpoint string // URL the server sees; drives the Host header (must resolve to host "localhost")

	sessionID string
	protocol  string
	nextID    int

	// trace, when set, reports each CallTool invocation for --mcp-verbose /
	// --mcp-trace. nil is a no-op (Tracer methods guard their receiver).
	trace *backend.Tracer
}

// ClientOptions configures a Client.
type ClientOptions struct {
	// URL is the MCP endpoint as the server must see it. Its host drives the
	// HTTP Host header and so must be "localhost" (Studio Pro rejects others).
	// Example: http://localhost:7782/mcp or http://localhost/mcp.
	URL string
	// Dial overrides the TCP address actually connected to, independent of the
	// URL host. From a devcontainer this is typically host.docker.internal:7782.
	// When empty it defaults to the URL host, or to host.docker.internal:<port>
	// when the URL host is localhost/127.0.0.1.
	Dial string
	// Protocol is the MCP protocol version advertised (default 2025-06-18).
	Protocol string
	// Timeout is the per-request timeout (default 30s).
	Timeout time.Duration
}

// NewClient creates an MCP client. It does not connect; call Initialize first.
func NewClient(opts ClientOptions) (*Client, error) {
	u, err := url.Parse(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid MCP URL %q: %w", opts.URL, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("MCP URL %q has no host", opts.URL)
	}
	dial := opts.Dial
	if dial == "" {
		dial = defaultDial(u.Host)
	}
	protocol := opts.Protocol
	if protocol == "" {
		protocol = "2025-06-18"
	}
	timeout := opts.Timeout
	if timeout == 0 {
		// Some writes (microflow create + validate) can take tens of seconds to
		// round-trip through Studio Pro; keep the default generous.
		timeout = 90 * time.Second
	}
	return &Client{
		http:     newHTTPClient(dial, timeout),
		endpoint: opts.URL,
		protocol: protocol,
	}, nil
}

// defaultDial maps a localhost endpoint to the Docker host gateway so the client
// works from inside a devcontainer, while the Host header stays localhost. The
// rewrite happens only when host.docker.internal is actually reachable — i.e.
// inside a container. On the host (e.g. macOS, where host.docker.internal does
// not resolve) it dials localhost directly, so `--mcp http://localhost:PORT/mcp`
// works without an explicit --mcp-dial.
func defaultDial(host string) string {
	return dialFor(host, hostDockerInternalReachable())
}

// dialFor is the pure decision behind defaultDial: rewrite a localhost endpoint to
// the docker host gateway only when that gateway is reachable. Split out so both
// branches are unit-testable without depending on the runtime environment.
func dialFor(host string, dockerReachable bool) string {
	h, port, err := net.SplitHostPort(host)
	if err != nil { // no port
		h, port = host, "80"
	}
	if (h == "localhost" || h == "127.0.0.1") && dockerReachable {
		return net.JoinHostPort(dockerHostName, port)
	}
	return host
}

// dockerHostName is the gateway hostname Docker injects for reaching the host.
const dockerHostName = "host.docker.internal"

var dockerHostOnce struct {
	sync.Once
	reachable bool
}

// hostDockerInternalReachable reports whether host.docker.internal resolves
// (true inside a Docker/devcontainer with the host-gateway mapping, false on a
// bare host). Resolved once and cached.
func hostDockerInternalReachable() bool {
	dockerHostOnce.Do(func() {
		_, err := net.LookupHost(dockerHostName)
		dockerHostOnce.reachable = err == nil
	})
	return dockerHostOnce.reachable
}

func newHTTPClient(dialAddr string, timeout time.Duration) *http.Client {
	// Pin every connection to dialAddr regardless of the request URL host, so
	// the URL (and thus the Host header) can stay "localhost" while we actually
	// connect to the host gateway.
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, dialAddr)
		},
		ForceAttemptHTTP2: false,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// ServerInfo describes the connected MCP server (from the initialize result).
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ServerInfo      ServerInfo `json:"serverInfo"`
}

// Initialize performs the MCP handshake (initialize + notifications/initialized)
// and returns the server identity. It must be called before any tool call.
func (c *Client) Initialize() (ServerInfo, error) {
	res, err := c.call("initialize", map[string]any{
		"protocolVersion": c.protocol,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mxcli", "version": "0.1.0"},
	})
	if err != nil {
		return ServerInfo{}, err
	}
	if res.Error != nil {
		return ServerInfo{}, res.Error
	}
	var ir initializeResult
	_ = json.Unmarshal(res.Result, &ir)
	if err := c.notify("notifications/initialized", nil); err != nil {
		return ir.ServerInfo, fmt.Errorf("notifications/initialized: %w", err)
	}
	return ir.ServerInfo, nil
}

// ToolResult is the decoded result of a tools/call.
type ToolResult struct {
	IsError bool
	// Text is the concatenation of all text content blocks.
	Text string
}

type toolCallResult struct {
	IsError bool `json:"isError"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// CallTool invokes an MCP tool by name with the given arguments and returns the
// decoded text content. A transport/protocol failure is returned as an error;
// a tool that ran but reported a model-level problem is returned with
// ToolResult.IsError set (so callers can surface the server's message).
// ListTools returns the names of the tools the connected MCP server exposes
// (a live tools/list probe — the tool-presence half of the capability model).
func (c *Client) ListTools() ([]string, error) {
	res, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	var r struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(res.Result, &r); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	names := make([]string, 0, len(r.Tools))
	for _, t := range r.Tools {
		names = append(names, t.Name)
	}
	return names, nil
}

func (c *Client) CallTool(name string, arguments any) (*ToolResult, error) {
	if c.trace.Enabled() {
		target, detail := summarizeToolCall(name, arguments)
		c.trace.Call(name, target, detail)
	}
	res, err := c.call("tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	var tc toolCallResult
	if err := json.Unmarshal(res.Result, &tc); err != nil {
		return nil, fmt.Errorf("decode %s result: %w", name, err)
	}
	var sb strings.Builder
	for i, blk := range tc.Content {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(blk.Text)
	}
	return &ToolResult{IsError: tc.IsError, Text: sb.String()}, nil
}

// call sends a JSON-RPC request and returns the matching response, handling
// both application/json and text/event-stream reply framing.
func (c *Client) call(method string, params any) (*rpcResponse, error) {
	c.nextID++
	id := c.nextID
	resp, err := c.send(rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	c.captureSession(resp)

	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	switch {
	case strings.HasPrefix(ct, "text/event-stream"):
		return c.readSSEResponse(resp.Body, id)
	case strings.HasPrefix(ct, "application/json"):
		var r rpcResponse
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			return nil, fmt.Errorf("decode json response: %w", err)
		}
		return &r, nil
	default:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("unexpected Content-Type %q, body: %s", ct, strings.TrimSpace(string(b)))
	}
}

// notify sends a JSON-RPC notification (no id); the server replies 202 with no
// JSON-RPC payload, which we drain.
func (c *Client) notify(method string, params any) error {
	resp, err := c.send(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.captureSession(resp)
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return nil
}

func (c *Client) send(payload rpcRequest) (*http.Response, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", c.protocol)
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	return c.http.Do(req)
}

func (c *Client) captureSession(resp *http.Response) {
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" && c.sessionID == "" {
		c.sessionID = id
	}
}

// readSSEResponse scans an SSE stream for the first JSON-RPC message whose id
// matches wantID (interleaved server-to-client messages are skipped).
func (c *Client) readSSEResponse(r io.Reader, wantID int) (*rpcResponse, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var data strings.Builder
	flush := func() (*rpcResponse, bool, error) {
		if data.Len() == 0 {
			return nil, false, nil
		}
		raw := data.String()
		data.Reset()
		var probe struct {
			ID *int `json:"id"`
		}
		_ = json.Unmarshal([]byte(raw), &probe)
		if probe.ID != nil && *probe.ID == wantID {
			var resp rpcResponse
			if err := json.Unmarshal([]byte(raw), &resp); err != nil {
				return nil, false, fmt.Errorf("decode sse json: %w", err)
			}
			return &resp, true, nil
		}
		return nil, false, nil
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "": // event boundary
			if resp, done, err := flush(); err != nil || done {
				return resp, err
			}
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if resp, done, err := flush(); err != nil || done {
		return resp, err
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read sse: %w", err)
	}
	return nil, fmt.Errorf("stream ended without a response for id %d", wantID)
}
