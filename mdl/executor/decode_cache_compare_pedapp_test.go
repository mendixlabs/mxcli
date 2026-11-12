// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// settledPagesScript is n small pages in MyFirstModule, re-declared exactly.
func settledPagesScript(n int) string {
	var sb strings.Builder
	sb.WriteString("mdl 1;\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "create or modify page MyFirstModule.ComparePg%02d (title: 'P', layout: Atlas_Core.Atlas_Default) {\n  layoutgrid g { row { column (desktopwidth: 12) { dynamictext t (content: 'x') } } }\n};\n", i)
	}
	return sb.String()
}

// ako/mxcli#2160, the settled page re-run: the decode cache recognises a unit
// by the bytes it was decoded from, by identity where it can and by comparing
// them where it cannot. Each pass over the project reads into buffers of its
// own, so the first listing of a pass compares; but the cache kept the buffer
// of the pass before, so every later listing of the pass compared every unit
// of the type again — all the project's pages, twice per page statement, an
// eighth of the time of a 600-page re-run.
//
// A unit is compared once per pass. What still grows with the statements is
// the one unit each reads for itself, into a buffer of its own: eight
// statements compared four times the bytes of two, and now compare less than
// half as much again.
func TestSettledPages_AUnitsBytesAreComparedOncePerRun(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := runScript(t, exec, settledPagesScript(8)); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	comparedFor := func(statements int) int64 {
		before, hits := mprread.DecodeCacheBytesCompared(), mprread.DecodeCacheHits()
		if err := runScript(t, exec, settledPagesScript(statements)); err != nil {
			t.Fatalf("re-run: %v\n%s", err, out.String())
		}
		if mprread.DecodeCacheHits() == hits {
			t.Fatal("control: the re-run was served nothing from the decode cache")
		}
		return mprread.DecodeCacheBytesCompared() - before
	}
	two, eight := comparedFor(2), comparedFor(8)
	t.Logf("bytes compared: %d for 2 statements, %d for 8", two, eight)
	if two == 0 {
		t.Fatal("control: nothing was compared, so the count is not measuring the run")
	}
	if eight > two*3/2 {
		t.Errorf("%d bytes compared for 8 statements against %d for 2: units are compared again for each statement", eight, two)
	}
}
