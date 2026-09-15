// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	mcpbackend "github.com/mendixlabs/mxcli/mdl/backend/mcp"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// There is one local engine: the codec ("modelsdk") engine. The legacy sdk/mpr
// backend it replaced is gone — see docs/plans/2026-09-14-retire-legacy-engine.md.
//
// --engine and MXCLI_ENGINE survive as a NO-OP that warns, rather than being
// removed outright, so a script or CI job pinning `legacy` keeps working and is
// told once why its pin no longer means anything. Deleting the flag would fail
// those runs at argument parsing with "unknown flag", which says nothing about
// what changed or what to do. Remove the flag in a later release, once the
// warning has had time to be seen.
//
// This is not the seam that selects the MCP backend: --mcp / --mcp-dial route
// writes to a live Studio Pro independently of this flag, and always did.

// globalEngineFlag holds the value of the --engine flag; it overrides the
// MXCLI_ENGINE environment variable. Set in PersistentPreRun.
var globalEngineFlag string

// warnIfEngineRequested prints the deprecation once if the user asked for an
// engine by name. Any value is accepted, including one that was never valid: the
// point is to tell the user the setting is inert, and rejecting a typo in a
// setting that no longer does anything would be a worse failure than ignoring it.
func warnIfEngineRequested() {
	v := strings.TrimSpace(globalEngineFlag)
	from := "--engine"
	if v == "" {
		v, from = strings.TrimSpace(os.Getenv("MXCLI_ENGINE")), "MXCLI_ENGINE"
	}
	if v == "" || strings.EqualFold(v, "modelsdk") {
		return
	}
	fmt.Fprintf(os.Stderr,
		"mxcli: %s=%s is ignored — there is now one engine and it is always used.\n"+
			"       The legacy sdk/mpr engine was removed; drop the setting from your scripts.\n",
		from, v)
}

// newBackendFactory returns the FullBackend factory for this run.
func newBackendFactory() func() backend.FullBackend {
	// The MCP backend (live Studio Pro) is selected by --mcp / --mcp-dial:
	// writes route to Studio Pro, reads come from -p.
	if globalMCPURL != "" {
		url, dial := globalMCPURL, globalMCPDial
		concordURL, concordDial := globalMCPConcord, globalMCPConcordDial
		saveOnExit, checkOnExit, runOnExit := globalMCPSave, globalMCPCheck, globalMCPRun
		return func() backend.FullBackend {
			b := mcpbackend.New(url, dial)
			if concordURL != "" || saveOnExit || checkOnExit || runOnExit {
				b = b.WithConcord(mcpbackend.ConcordConfig{
					URL: concordURL, Dial: concordDial,
					SaveOnExit: saveOnExit, CheckOnExit: checkOnExit, RunOnExit: runOnExit,
				})
			}
			return b
		}
	}
	warnIfEngineRequested()
	return func() backend.FullBackend { return modelsdkbackend.New() }
}
