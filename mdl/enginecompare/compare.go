// SPDX-License-Identifier: Apache-2.0

// Package enginecompare runs the same MDL read query through both the legacy
// (sdk/mpr) and the modelsdk engines in-process and compares their rendered
// output. It is the read-side of the dual-engine comparison harness described in
// docs/plans/2026-06-05-adopt-modelsdk-engine.md — automating the manual
// `--engine legacy` vs `--engine modelsdk` diffs into a repeatable parity gate.
//
// (Write/BSON comparison with an ID-canonicalizer comes in Phase 2, once the
// modelsdk backend can write.)
package enginecompare

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	modelsdkbackend "github.com/JordtenBulte-OLC/mxcli/mdl/backend/modelsdk"
	mprbackend "github.com/JordtenBulte-OLC/mxcli/mdl/backend/mpr"
	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// Engine selects which backend implementation to run a query against.
type Engine string

const (
	Legacy   Engine = "legacy"
	ModelSDK Engine = "modelsdk"
)

func factory(e Engine) func() backend.FullBackend {
	if e == ModelSDK {
		return func() backend.FullBackend { return modelsdkbackend.New() }
	}
	return func() backend.FullBackend { return mprbackend.New() }
}

// Run connects to projectPath with the chosen engine, executes a single read
// query, and returns the rendered output.
func Run(eng Engine, projectPath, query string) (string, error) {
	var buf bytes.Buffer
	exec := executor.New(&buf)
	exec.SetBackendFactory(factory(eng))
	defer exec.Close()

	prog, errs := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'; %s", projectPath, query))
	if len(errs) > 0 {
		return "", fmt.Errorf("parse %q: %v", query, errs[0])
	}
	if err := exec.ExecuteProgram(prog); err != nil {
		return "", fmt.Errorf("execute %q on %s: %w", query, eng, err)
	}
	return buf.String(), nil
}

// NormalizeTable reduces a rendered markdown-ish table to comparable rows:
// it keeps table rows, trims per-cell padding (so column widths don't matter),
// drops separator rows, applies an optional row filter, and sorts (so row order
// doesn't matter). The result is what two engines must agree on.
func NormalizeTable(out string, keep func(string) bool) []string {
	var rows []string
	for line := range strings.SplitSeq(out, "\n") {
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|--") || strings.HasPrefix(line, "| --") {
			continue
		}
		cells := strings.Split(line, "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		norm := strings.Join(cells, "|")
		if keep != nil && !keep(norm) {
			continue
		}
		rows = append(rows, norm)
	}
	sort.Strings(rows)
	return rows
}

// DiffRows returns a human-readable summary of rows present in only one side.
func DiffRows(legacy, modelsdk []string) string {
	ls := map[string]bool{}
	for _, r := range legacy {
		ls[r] = true
	}
	ms := map[string]bool{}
	for _, r := range modelsdk {
		ms[r] = true
	}
	var b strings.Builder
	for _, r := range legacy {
		if !ms[r] {
			fmt.Fprintf(&b, "  - legacy-only:   %s\n", r)
		}
	}
	for _, r := range modelsdk {
		if !ls[r] {
			fmt.Fprintf(&b, "  + modelsdk-only: %s\n", r)
		}
	}
	return b.String()
}
