// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestShowFeaturesForVersionRoutesToFeatures is ako/mxcli#910: `show features
// for version 10.24` printed the session's `show version` output, because the
// bare `ctx.VERSION() != nil` branch of ExitShowStatement ran before the
// FEATURES branch and swallowed the `for version` alternative.
func TestShowFeaturesForVersionRoutesToFeatures(t *testing.T) {
	for _, src := range []string{
		"show features for version 10.24;",
		"list features for version 10.24;",
		"mdl 1;\nlist features for version 10.24;",
	} {
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Fatalf("%q: parse errors: %v", src, errs)
		}
		got := prog.Statements[len(prog.Statements)-1]
		fs, ok := got.(*ast.ShowFeaturesStmt)
		if !ok {
			t.Fatalf("%q: got %T, want *ast.ShowFeaturesStmt", src, got)
		}
		if fs.ForVersion != "10.24" {
			t.Errorf("%q: ForVersion = %q, want 10.24", src, fs.ForVersion)
		}
	}
	// Control: the bare `show version` still routes to the session version.
	prog, _ := Build("show version;")
	if s, ok := prog.Statements[0].(*ast.ShowStmt); !ok || s.ObjectType != ast.ShowVersion {
		t.Errorf("show version: got %#v, want ShowStmt{ShowVersion}", prog.Statements[0])
	}
}

// ako/mxcli#1367: versions are written major.minor.patch everywhere else, and
// `show features for version 10.20.0` was a parse error. The area names are
// lexer keywords (workflows, microflows, pages, …), and `show features in
// workflows` was a parse error too — the documented `in <area>` form failed
// for most of the areas the registry has.
func TestShowFeaturesAcceptsFullVersionsAndKeywordAreas(t *testing.T) {
	cases := map[string]ast.ShowFeaturesStmt{
		"show features for version 10.20.0;": {ForVersion: "10.20.0"},
		"show features added since 11.0.0;":  {AddedSince: "11.0.0"},
		"show features for version 10.24;":   {ForVersion: "10.24"},
		"show features in workflows;":        {InArea: "workflows"},
		"show features in microflows;":       {InArea: "microflows"},
		"show features in domain_model;":     {InArea: "domain_model"},
	}
	for src, want := range cases {
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Errorf("%q: parse errors: %v", src, errs)
			continue
		}
		got, ok := prog.Statements[0].(*ast.ShowFeaturesStmt)
		if !ok || *got != want {
			t.Errorf("%q: got %#v, want %#v", src, prog.Statements[0], want)
		}
	}
}
