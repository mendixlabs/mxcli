// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/internal/pathutil"
	"go.lsp.dev/protocol"
)

// projectElement represents a single element from the catalog.
type projectElement struct {
	ObjectType    string // ENTITY, MICROFLOW, PAGE, etc.
	QualifiedName string // Module.Name
}

// stdioReadWriteCloser wraps stdin/stdout into a single io.ReadWriteCloser.
type stdioReadWriteCloser struct{}

func (s stdioReadWriteCloser) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (s stdioReadWriteCloser) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (s stdioReadWriteCloser) Close() error                { return nil }

// uriToPath converts a file:// URI to a filesystem path.
// Deprecated: use pathutil.URIToPath instead.
func uriToPath(rawURI string) string {
	return pathutil.URIToPath(rawURI)
}

// pullConfiguration requests the "mdl" configuration section from the client.
func (s *mdlServer) pullConfiguration(ctx context.Context) {
	items, err := s.client.Configuration(ctx, &protocol.ConfigurationParams{
		Items: []protocol.ConfigurationItem{
			{Section: "mdl"},
		},
	})
	if err != nil || len(items) == 0 {
		return
	}

	raw, err := json.Marshal(items[0])
	if err != nil {
		return
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil {
		return
	}
	if v, ok := cfg["mprPath"].(string); ok && v != "" {
		s.mprPath = v
	}
	if v, ok := cfg["mxcliPath"].(string); ok && v != "" {
		s.mxcliPath = v
	}
}

// findMprPath returns the configured .mpr path, or auto-discovers one in the workspace.
func (s *mdlServer) findMprPath() string {
	if s.mprPath != "" {
		return s.mprPath
	}
	if s.workspaceRoot == "" {
		return ""
	}
	// Glob for *.mpr in the workspace root
	matches, err := filepath.Glob(filepath.Join(s.workspaceRoot, "*.mpr"))
	if err != nil || len(matches) == 0 {
		// Try one level deep
		matches, err = filepath.Glob(filepath.Join(s.workspaceRoot, "*", "*.mpr"))
		if err != nil || len(matches) == 0 {
			return ""
		}
	}
	return matches[0]
}

// runMxcli spawns the mxcli subprocess with the given arguments.
// Returns ("", nil) if no mpr path is available — graceful degradation.
func (s *mdlServer) runMxcli(ctx context.Context, args ...string) (string, error) {
	mprPath := s.findMprPath()
	if mprPath == "" {
		return "", nil
	}

	// Determine mxcli executable path
	mxcli := s.mxcliPath
	if mxcli == "" {
		var err error
		mxcli, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("cannot determine mxcli path: %w", err)
		}
	}

	// Prepend -p <mprPath>
	fullArgs := append([]string{"-p", mprPath}, args...)

	// Apply timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, mxcli, fullArgs...)
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Strip "Connected to:" lines from output
	output := stdout.String()
	var filtered []string
	for line := range strings.SplitSeq(output, "\n") {
		if !strings.HasPrefix(line, "Connected to:") {
			filtered = append(filtered, line)
		}
	}
	result := strings.TrimSpace(strings.Join(filtered, "\n"))

	if err != nil {
		// Return stderr output as the error detail, but also return stdout
		if result == "" {
			result = strings.TrimSpace(stderr.String())
		}
		return result, err
	}

	return result, nil
}

// getProjectElements returns the cached list of project elements from the catalog.
// On cache miss, it queries CATALOG.OBJECTS via runMxcli and caches for 5 minutes.
func (s *mdlServer) getProjectElements(ctx context.Context) []projectElement {
	const cacheKey = "catalog:elements"

	if cached, ok := s.cache.Get(cacheKey); ok {
		var elems []projectElement
		if json.Unmarshal([]byte(cached), &elems) == nil {
			return elems
		}
	}

	output, err := s.runMxcli(ctx, "-c", "SELECT ObjectType, QualifiedName FROM CATALOG.OBJECTS ORDER BY QualifiedName")
	if err != nil || output == "" {
		return nil
	}

	elems := parseTableOutput(output)
	if len(elems) == 0 {
		return nil
	}

	if data, err := json.Marshal(elems); err == nil {
		s.cache.Set(cacheKey, string(data), 5*time.Minute)
	}

	return elems
}

// parseTableOutput parses markdown table output from mxcli catalog queries.
// Expected format:
//
//	Found N result(s)
//	| ObjectType | QualifiedName       |
//	|------------|---------------------|
//	| ENTITY     | MyModule.Customer   |
//	...
func parseTableOutput(output string) []projectElement {
	var elems []projectElement
	headerSeen := false
	separatorSeen := false

	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Found ") {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}

		// Split on | and trim
		parts := strings.Split(line, "|")
		// Expect at least 4 parts: ["", "col1", "col2", ""]
		if len(parts) < 4 {
			continue
		}

		col1 := strings.TrimSpace(parts[1])
		col2 := strings.TrimSpace(parts[2])

		// Skip header row
		if !headerSeen {
			headerSeen = true
			continue
		}

		// Skip separator row (e.g., |----------|-----------|)
		if !separatorSeen {
			separatorSeen = true
			continue
		}

		if col1 != "" && col2 != "" {
			elems = append(elems, projectElement{
				ObjectType:    col1,
				QualifiedName: col2,
			})
		}
	}
	return elems
}
