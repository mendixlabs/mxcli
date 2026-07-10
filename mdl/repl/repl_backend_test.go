// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// TestSetBackendFactoryIsUsedByConnect guards the fix for the REPL silently
// ignoring --mcp: the interactive REPL defaulted to the local .mpr backend and
// had no way to override it, so writes never reached Studio Pro. SetBackendFactory
// must steer CONNECT to the injected backend instead of the built-in default.
func TestSetBackendFactoryIsUsedByConnect(t *testing.T) {
	var connectedPath string
	injected := &mock.MockBackend{
		ConnectFunc: func(path string) error {
			connectedPath = path
			return nil
		},
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{ProductVersion: "11.11.0"}
		},
	}

	r := New(strings.NewReader(""), io.Discard)
	r.SetBackendFactory(func() backend.FullBackend { return injected })

	if err := r.ExecuteString("CONNECT LOCAL 'some-project.mpr';"); err != nil {
		t.Fatalf("ExecuteString(CONNECT): %v", err)
	}

	if connectedPath != "some-project.mpr" {
		t.Fatalf("injected backend factory was not used by CONNECT: ConnectFunc got %q (want %q) — the REPL is still defaulting to its built-in backend",
			connectedPath, "some-project.mpr")
	}
}
