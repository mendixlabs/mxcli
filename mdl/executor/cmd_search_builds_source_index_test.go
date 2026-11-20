// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// ako/mxcli#2004 (mendixlabs/mxcli#1771): on a project whose source index had
// never been built, `search` answered "No matches found." with exit code 0 for
// a name used only in a flow body: only string literals and documentation were
// searched, and the one hint was a warning on stderr. `callers` found the use.
//
// A search now builds the source index the first time it needs it, saying so
// once; the index is kept up to date incrementally after that. A caller that
// must not pay for that build asks for NoBuild, and is told the answer is
// incomplete instead of being handed an empty one.

const searchCallerSetup = `mdl 1;
create microflow MyFirstModule.SUB_Do2004 () begin end;
create microflow MyFirstModule.Caller2004 () begin call microflow MyFirstModule.SUB_Do2004(); end;`

// searchSetup is a PedApp copy holding a microflow that only another flow's
// body names, and no source index: the reporter's fresh clone.
func searchSetup(t *testing.T) (*Executor, *ExecContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, searchCallerSetup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	ctx := exec.newExecContext(t.Context())
	diag := &bytes.Buffer{}
	ctx.Diagnostics = diag
	if mode := cachedCatalogMode(ctx); mode == "source" {
		t.Fatalf("control: the copy already has a source index")
	}
	out.Reset()
	return exec, ctx, out, diag
}

func TestSearch_BuildsTheSourceIndexOnFirstUse(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			_, ctx, out, diag := searchSetup(t)
			res, err := searchReport(ctx, "SUB_Do2004", format, SearchOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Caller2004") {
				t.Fatalf("search does not find the call in Caller2004's body (hits %d):\n%s\nstderr:\n%s", res.Hits, out.String(), diag.String())
			}
			if res.Incomplete {
				t.Error("a search that built the source index reports itself incomplete")
			}
			if !strings.Contains(diag.String(), "Building the source index") {
				t.Errorf("the one-time build is not announced:\n%s", diag.String())
			}
			if strings.Contains(diag.String(), "is not built") {
				t.Errorf("warned that the source index is missing after building it:\n%s", diag.String())
			}
			if mode := cachedCatalogMode(ctx); mode != "source" {
				t.Errorf("the source index was not kept: cached catalog mode %q", mode)
			}

			// A second search reuses it and says nothing about building.
			out.Reset()
			diag.Reset()
			if _, err := searchReport(ctx, "SUB_Do2004", format, SearchOptions{}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Caller2004") || strings.Contains(diag.String(), "Building the source index") {
				t.Errorf("second search: want the hit and no build notice\nout:\n%s\nstderr:\n%s", out.String(), diag.String())
			}
		})
	}
}

// NoBuild never builds the source index. Without one the answer is marked
// incomplete; with one (the control) it is a complete answer like any other.
func TestSearch_NoBuildSaysTheAnswerIsIncomplete(t *testing.T) {
	_, ctx, out, diag := searchSetup(t)
	res, err := searchReport(ctx, "SUB_Do2004", "json", SearchOptions{NoBuild: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Incomplete {
		t.Errorf("no source index, NoBuild: want the answer marked incomplete\nout:\n%s\nstderr:\n%s", out.String(), diag.String())
	}
	if mode := cachedCatalogMode(ctx); mode == "source" {
		t.Error("NoBuild built the source index")
	}
	if strings.Contains(diag.String(), "Building the source index") {
		t.Errorf("NoBuild announced a build:\n%s", diag.String())
	}

	// Control: once the index exists, NoBuild answers from it in full.
	if _, err := searchReport(ctx, "SUB_Do2004", "json", SearchOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	res, err = searchReport(ctx, "SUB_Do2004", "json", SearchOptions{NoBuild: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Incomplete || !strings.Contains(out.String(), "Caller2004") {
		t.Errorf("with a source index, NoBuild: want a complete answer with the hit (incomplete %v)\n%s", res.Incomplete, out.String())
	}
}
