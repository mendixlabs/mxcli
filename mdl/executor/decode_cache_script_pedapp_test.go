// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// pagesScript is n pages that each call a microflow from a button: the
// statement whose check resolves the flow's signature, and so lists the
// project's microflows.
func pagesScript(n int, flow string) string {
	var sb strings.Builder
	sb.WriteString("mdl 1;\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, `
create or modify page MyFirstModule.DecodeBudget%02d
(
  title: 'Decode budget %d',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid grid1 {
    row {
      column (desktopwidth: 12) {
        actionbutton btnRun (caption: 'Run', action: call microflow %s)
      }
    }
  }
};
`, i, i, flow)
	}
	return sb.String()
}

// ako/mxcli#2160: checking and executing a script cost statements × project,
// because each statement listed whole document types and each listing decoded
// every unit. Six statements must not decode much more than two do: what a
// statement adds is its own work, not another pass over the project.
//
// The control runs the same two scripts with the decode cache off, where each
// added statement decodes at least the project's microflows again. Without it
// the bound would pass against a build that never had the cache.
func TestScript_AddedStatementsDoNotDecodeTheProjectAgain(t *testing.T) {
	decodesFor := func(t *testing.T, statements int) (decoded int64, microflows int) {
		exec, out, _ := openPedAppCopy(t)
		mfs, err := exec.Backend().ListMicroflows()
		if err != nil || len(mfs) < 10 {
			t.Fatalf("control: the fixture has %d microflows (%v); the bounds below would prove nothing", len(mfs), err)
		}
		h, err := getHierarchy(exec.newExecContext(t.Context()))
		if err != nil {
			t.Fatal(err)
		}
		var flow string
		for _, mf := range mfs {
			if len(mf.Parameters) == 0 {
				flow = h.GetQualifiedName(mf.ContainerID, mf.Name)
				break
			}
		}
		if flow == "" {
			t.Fatal("control: the fixture has no microflow without parameters to call from a button")
		}
		prog, errs := visitor.Build(pagesScript(statements, flow))
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs[0])
		}
		before := mprread.UnitsDecoded()
		// What `mxcli exec` does: the check, then the statements.
		if verrs, _ := exec.ValidateProgramWithWarnings(prog); len(verrs) > 0 {
			t.Fatalf("check: %v", verrs[0])
		}
		if err := exec.ExecuteProgram(prog); err != nil {
			t.Fatalf("exec: %v\n%s", err, out.String())
		}
		return mprread.UnitsDecoded() - before, len(mfs)
	}

	two, microflows := decodesFor(t, 2)
	six, _ := decodesFor(t, 6)
	perStatement := (six - two) / 4
	t.Logf("cache on: %d decodes for 2 statements, %d for 6 — %d per added statement (%d microflows in the project)", two, six, perStatement, microflows)
	if perStatement >= int64(microflows) {
		t.Errorf("each added statement decoded %d units; the project has %d microflows, so a statement is decoding the project again", perStatement, microflows)
	}

	defer mprread.SetDecodeCacheEnabled(false)()
	twoOff, _ := decodesFor(t, 2)
	sixOff, _ := decodesFor(t, 6)
	perStatementOff := (sixOff - twoOff) / 4
	t.Logf("cache off: %d and %d — %d per added statement", twoOff, sixOff, perStatementOff)
	if perStatementOff < int64(microflows) {
		t.Errorf("control: with the cache off each added statement decoded %d units, fewer than the %d microflows — the script no longer lists them per statement, so the bound above has stopped measuring the cache", perStatementOff, microflows)
	}
}

// A unit this process rewrites is read back as rewritten: the cache is keyed
// by a unit's stored bytes, so the write needs no invalidation and cannot be
// missed. Listed three times first, so that the stored form is the one being
// served when the write lands.
func TestDecodeCache_AWriteIsReadBack(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const name = "DecodeCacheProbe"
	documentation := func() string {
		t.Helper()
		mfs, err := exec.Backend().ListMicroflows()
		if err != nil {
			t.Fatal(err)
		}
		for _, mf := range mfs {
			if mf.Name == name {
				return mf.Documentation
			}
		}
		t.Fatalf("microflow %s not found", name)
		return ""
	}
	write := func(doc string) {
		t.Helper()
		src := fmt.Sprintf("mdl 1;\n/** %s */\ncreate or modify microflow MyFirstModule.%s ()\nreturns boolean\nbegin\n  return true;\nend;", doc, name)
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
	}

	write("first")
	for range 3 {
		if got := documentation(); got != "first" {
			t.Fatalf("documentation %q, want %q", got, "first")
		}
	}
	hits := mprread.DecodeCacheHits()
	if documentation(); mprread.DecodeCacheHits() == hits {
		t.Fatal("control: the listing is not being served from the cache, so reading a write back proves nothing about it")
	}

	write("second")
	if got := documentation(); got != "second" {
		t.Errorf("after the rewrite the listing still reads %q, want %q", got, "second")
	}
}

// ako/mxcli#2160, the layer under decoding: a statement that writes emptied
// the reader's whole byte cache, and a script run did not hold that cache at
// all, so every listing after every write read every unit file of its type
// again — `create page` lists the project's pages, and the next one read them
// all from disk. A write now drops the one unit it rewrote.
//
// Measured on this fixture: 120 file reads per added page statement before
// the change, 1 after. The bound is the number of pages in the project: at or above it,
// a statement is reading the listing again.
func TestScript_AddedStatementsDoNotReadTheProjectsFilesAgain(t *testing.T) {
	readsFor := func(statements int) (reads int64, pages int) {
		exec, out, _ := openPedAppCopy(t)
		ps, err := exec.Backend().ListPages()
		if err != nil || len(ps) < 10 {
			t.Fatalf("control: the fixture has %d pages (%v); the bound below would prove nothing", len(ps), err)
		}
		mfs, err := exec.Backend().ListMicroflows()
		if err != nil {
			t.Fatal(err)
		}
		h, err := getHierarchy(exec.newExecContext(t.Context()))
		if err != nil {
			t.Fatal(err)
		}
		var flow string
		for _, mf := range mfs {
			if len(mf.Parameters) == 0 {
				flow = h.GetQualifiedName(mf.ContainerID, mf.Name)
				break
			}
		}
		prog, errs := visitor.Build(pagesScript(statements, flow))
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs[0])
		}
		before := unitFileReads(t, exec)
		if err := exec.ExecuteProgram(prog); err != nil {
			t.Fatalf("exec: %v\n%s", err, out.String())
		}
		return unitFileReads(t, exec) - before, len(ps)
	}

	two, pages := readsFor(2)
	six, _ := readsFor(6)
	perStatement := (six - two) / 4
	t.Logf("%d file reads for 2 statements, %d for 6 — %d per added statement (%d pages in the project)", two, six, perStatement, pages)
	if perStatement >= int64(pages) {
		t.Errorf("each added statement read %d unit files; the project has %d pages, so a statement is reading the listing from disk again", perStatement, pages)
	}
}
