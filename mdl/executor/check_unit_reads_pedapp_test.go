// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1272: `check -p` on a script that re-declares stored
// microflows with `create or modify` took 6-7x as long as v0.24.0, and the
// cost per statement grew with the project. Every lookup the verdict and
// reference passes make (ListMicroflows, GetMicroflow, GetRawUnitByName)
// listed every microflow of the project from disk, ~9 times per statement, so
// a pass read statements × microflows files. Each pass now holds the units it
// reads in memory: a pass reads a file at most once.

// unitFileReads is the connected modelsdk backend's count of unit files read.
func unitFileReads(t *testing.T, e *Executor) int64 {
	t.Helper()
	b, ok := e.Backend().(interface{ UnitFileReads() int64 })
	if !ok {
		t.Fatalf("backend %T does not count its file reads", e.Backend())
	}
	return b.UnitFileReads()
}

// unitReadsNotFromMemory is every unit read the backend did not serve from
// its in-memory cache: the files it opened and the reads its persisted store
// answered. A run that ends inside a cache scope flushes what it read to that
// store, so afterwards an uncached read of the same unit is a store read, not
// a file read — and counting files alone would call a pass that reads each
// flow nine times a pass that reads almost nothing.
func unitReadsNotFromMemory(t *testing.T, e *Executor) int64 {
	t.Helper()
	b, ok := e.Backend().(interface{ UnitStoreHits() int64 })
	if !ok {
		t.Fatalf("backend %T does not count its store reads", e.Backend())
	}
	return unitFileReads(t, e) + b.UnitStoreHits()
}

// redeclareChain is the issue's generator: n microflows, each calling the one
// before it.
func redeclareChain(n int, verb string) string {
	var sb strings.Builder
	sb.WriteString("mdl 1;\n")
	for i := 1; i <= n; i++ {
		call := "  declare $Prev Integer = 0;\n"
		if i > 1 {
			call = fmt.Sprintf("  $Prev = call microflow MyFirstModule.MF_Chain%03d(Title = $Title);\n", i-1)
		}
		fmt.Fprintf(&sb, `%s microflow MyFirstModule.MF_Chain%03d ($Title: String) returns Integer as $N
begin
%s  declare $N Integer = length($Title) + $Prev;
  return $N;
end;
`, verb, i, call)
	}
	return sb.String()
}

func TestCheckPasses_ReadEachUnitFileOnce(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const n = 12
	if err := agreeExec(t, exec, redeclareChain(n, "create")); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	units := 0
	_ = filepath.Walk(filepath.Join(dir, "mprcontents"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".mxunit") {
			units++
		}
		return nil
	})

	prog, errs := visitor.Build(redeclareChain(n, "create or modify"))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}

	// Control: the verdict pass without the cache reads the re-declared flows
	// over and over. It used to read more files than the project has; since
	// the unit index names flows without opening them (ako/mxcli#1791) it
	// reads only flows it looks up, but each of them several times — 103
	// reads for these 12 flows, against 12 with the cache.
	// Counted as reads not served from memory: the setup above ran as a
	// script, which holds the cache and leaves what it read in the persisted
	// store (ako/mxcli#2160).
	before := unitReadsNotFromMemory(t, exec)
	CheckFlowVerdicts(exec.newExecContext(context.Background()), prog)
	if reads := unitReadsNotFromMemory(t, exec) - before; reads <= 2*n {
		t.Fatalf("control: the uncached verdict pass read %d files for %d flows; the bound below would prove nothing", reads, n)
	}

	passes := []struct {
		name string
		run  func()
	}{
		{"CheckFlowVerdicts", func() { exec.CheckFlowVerdicts(prog) }},
		{"ValidateProgramWithWarnings", func() { exec.ValidateProgramWithWarnings(prog) }},
		{"CheckProjectNameClashes", func() { exec.CheckProjectNameClashes(prog) }},
		{"CheckProjectConflicts", func() { exec.CheckProjectConflicts(prog) }},
	}
	for _, p := range passes {
		before := unitFileReads(t, exec)
		p.run()
		if reads := unitFileReads(t, exec) - before; reads > int64(units) {
			t.Errorf("%s read %d unit files for %d re-declared flows; the project has %d", p.name, reads, n, units)
		}
		if reads := unitFileReads(t, exec) - before; p.name == "CheckFlowVerdicts" && reads > n {
			t.Errorf("%s read %d unit files for %d re-declared flows; each needs reading once", p.name, reads, n)
		}
	}

	before = unitFileReads(t, exec)
	if err := agreeExec(t, exec, redeclareChain(n, "create or modify")); err != nil {
		t.Fatalf("re-run: %v\n%s", err, out.String())
	}
	// exec plans each `create or modify` under the cache too: 2856 reads for
	// this re-run without it, 1816 with it. The rest is exec's per-statement
	// work outside the verdict, so the bound is the measured gap's midpoint.
	if reads := unitFileReads(t, exec) - before; reads > 2300 {
		t.Errorf("exec re-run of %d unchanged flows read %d unit files; its verdicts are not cached", n, reads)
	}
}
