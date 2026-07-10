// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/spf13/cobra"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Start MDL Language Server Protocol server",
	Long: `Start an LSP server for MDL files.

The server communicates via JSON-RPC over stdin/stdout (--stdio mode)
and provides the following features:

  - Parse diagnostics (real-time as you type)
  - Semantic diagnostics on save (reference validation via mxcli check)
  - Context-aware code completion (keywords, snippets)
  - Hover information for qualified names (via mxcli describe)
  - Go-to-definition for qualified names (opens virtual MDL document)
  - Document symbols (outline of CREATE/ALTER/DROP statements)
  - Folding ranges (BEGIN/END, IF/END IF, braces, parens, comments)

Project-aware features (hover, definition, semantic diagnostics) require
a .mpr file, configured via initializationOptions or the mdl.mprPath
setting. When no project is available, these features degrade silently.

Examples:
  mxcli lsp --stdio
`,
	Run: func(cmd *cobra.Command, args []string) {
		runLSPServer()
	},
}

// mdlServer implements protocol.Server for MDL language support.
type mdlServer struct {
	client        protocol.Client
	mu            sync.Mutex
	docs          map[uri.URI]string
	mprPath       string    // Path to .mpr file
	mxcliPath     string    // Path to mxcli binary (default: os.Executable())
	workspaceRoot string    // Workspace folder path (filesystem path, not URI)
	cache         *lspCache // Subprocess result cache

	// Widget completion cache (lazily populated)
	widgetCompletionsOnce sync.Once
	widgetCompletionItems []protocol.CompletionItem
	widgetRegistry        *executor.WidgetRegistry // populated by the same Once
}

func newMDLServer(client protocol.Client) *mdlServer {
	return &mdlServer{
		client: client,
		docs:   make(map[uri.URI]string),
		cache:  newLSPCache(),
	}
}

// lspCache provides a simple TTL cache for subprocess results.
type lspCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	value     string
	timestamp time.Time
	ttl       time.Duration
}

func newLSPCache() *lspCache {
	return &lspCache{
		entries: make(map[string]*cacheEntry),
	}
}

func (c *lspCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Since(entry.timestamp) > entry.ttl {
		delete(c.entries, key)
		return "", false
	}
	return entry.value, true
}

func (c *lspCache) Set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cacheEntry{
		value:     value,
		timestamp: time.Now(),
		ttl:       ttl,
	}
}

func (c *lspCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// Initialize handles the initialize request.
func (s *mdlServer) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	// Extract workspace root
	if len(params.WorkspaceFolders) > 0 {
		s.workspaceRoot = uriToPath(string(params.WorkspaceFolders[0].URI))
	} else if params.RootURI != "" {
		s.workspaceRoot = uriToPath(string(params.RootURI))
	}

	// Extract initialization options
	if params.InitializationOptions != nil {
		if raw, err := json.Marshal(params.InitializationOptions); err == nil {
			var opts map[string]string
			if json.Unmarshal(raw, &opts) == nil {
				if v := opts["mprPath"]; v != "" {
					s.mprPath = v
				}
				if v := opts["mxcliPath"]; v != "" {
					s.mxcliPath = v
				}
			}
		}
	}

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.TextDocumentSyncKindFull,
				Save: &protocol.SaveOptions{
					IncludeText: true,
				},
			},
			CompletionProvider:     &protocol.CompletionOptions{},
			DocumentSymbolProvider: true,
			FoldingRangeProvider:   true,
			HoverProvider:          true,
			DefinitionProvider:     true,
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "mdl-language-server",
			Version: version,
		},
	}, nil
}

// Initialized handles the initialized notification.
func (s *mdlServer) Initialized(ctx context.Context, params *protocol.InitializedParams) error {
	s.pullConfiguration(ctx)
	go s.getProjectElements(ctx) // pre-warm catalog element cache
	return nil
}

// Shutdown handles the shutdown request.
func (s *mdlServer) Shutdown(ctx context.Context) error {
	return nil
}

// Exit handles the exit notification.
func (s *mdlServer) Exit(ctx context.Context) error {
	os.Exit(0)
	return nil
}

// DidChangeConfiguration handles workspace/didChangeConfiguration notifications.
func (s *mdlServer) DidChangeConfiguration(ctx context.Context, params *protocol.DidChangeConfigurationParams) error {
	s.pullConfiguration(ctx)
	s.cache.Invalidate()
	return nil
}

func runLSPServer() {
	// Redirect stderr for logging (stdout is used by LSP protocol)
	logger, _ := zap.NewDevelopment(zap.ErrorOutput(os.Stderr))
	defer logger.Sync()

	ctx := context.Background()

	// Create JSON-RPC stream on stdin/stdout
	rwc := io.ReadWriteCloser(stdioReadWriteCloser{})
	stream := jsonrpc2.NewStream(rwc)
	conn := jsonrpc2.NewConn(stream)

	// Create client dispatcher (for sending diagnostics to client)
	client := protocol.ClientDispatcher(conn, logger)

	// Create our server implementation
	server := newMDLServer(client)

	// Wire up the handler
	handler := protocol.ServerHandler(server, jsonrpc2.MethodNotFoundHandler)
	handler = protocol.Handlers(handler)

	// Start the connection handler
	conn.Go(ctx, handler)

	fmt.Fprintln(os.Stderr, "MDL Language Server started")

	// Wait for the connection to close
	<-conn.Done()
	if err := conn.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "LSP connection error: %v\n", err)
	}
}

func init() {
	lspCmd.Flags().Bool("stdio", true, "Use stdio transport (default)")
}
