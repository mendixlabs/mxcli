// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	mcpbackend "github.com/JordtenBulte-OLC/mxcli/mdl/backend/mcp"
	modelsdkbackend "github.com/JordtenBulte-OLC/mxcli/mdl/backend/modelsdk"
	mprbackend "github.com/JordtenBulte-OLC/mxcli/mdl/backend/mpr"
)

// engineKind selects which model engine backs the local FullBackend.
//
// This is the single selection seam for the engine swap described in
// docs/plans/2026-06-05-adopt-modelsdk-engine.md. The codec engine ("modelsdk")
// is now the default; "legacy" (sdk/mpr) remains an explicit fallback for the few
// constructs the codec path doesn't yet write (notably SOAP web services).
// "compare" is recognised so the contract is stable but still fails fast.
type engineKind string

const (
	engineLegacy   engineKind = "legacy"   // sdk/mpr write path (explicit fallback)
	engineModelSDK engineKind = "modelsdk" // roundtrip codec engine (default)
	engineCompare  engineKind = "compare"  // run both engines, diff BSON (not yet wired)
)

// globalEngineFlag holds the value of the --engine flag; it overrides the
// MXCLI_ENGINE environment variable. Set in PersistentPreRun.
var globalEngineFlag string

// resolveEngine reads the active engine from --engine, then MXCLI_ENGINE,
// defaulting to modelsdk. An unrecognised value is fatal so typos are loud.
func resolveEngine() engineKind {
	v := strings.TrimSpace(globalEngineFlag)
	if v == "" {
		v = strings.TrimSpace(os.Getenv("MXCLI_ENGINE"))
	}
	switch engineKind(strings.ToLower(v)) {
	case "", engineModelSDK:
		return engineModelSDK
	case engineLegacy:
		return engineLegacy
	case engineCompare:
		return engineCompare
	default:
		fmt.Fprintf(os.Stderr, "mxcli: unknown MXCLI_ENGINE %q (expected: modelsdk, legacy, compare)\n", v)
		os.Exit(2)
		return engineModelSDK // unreachable
	}
}

// newBackendFactory returns the FullBackend factory for the active engine.
//
// modelsdk (default) runs the codec engine: complete reads and writes, validated
// at parity with legacy across the doctype suite. The legacy (sdk/mpr) engine is
// the explicit fallback for the few constructs the codec path doesn't yet write
// (notably SOAP web services) — select it with --engine legacy or
// MXCLI_ENGINE=legacy. compare needs the run-both diff harness (not yet wired)
// and fails fast. An unknown value was already rejected by resolveEngine.
func newBackendFactory() func() backend.FullBackend {
	// The MCP backend (live Studio Pro) is selected by --mcp / --mcp-dial,
	// independent of MXCLI_ENGINE: writes route to Studio Pro, reads come from -p.
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
	switch resolveEngine() {
	case engineLegacy:
		return func() backend.FullBackend { return mprbackend.New() }
	case engineCompare:
		fmt.Fprintln(os.Stderr, "mxcli: engine 'compare' requires the run-both diff harness (Phase 2 of "+
			"docs/plans/2026-06-05-adopt-modelsdk-engine.md). Use 'modelsdk' (default) or 'legacy'.")
		os.Exit(2)
	}
	// default: the codec (modelsdk) engine
	return func() backend.FullBackend { return modelsdkbackend.New() }
}
