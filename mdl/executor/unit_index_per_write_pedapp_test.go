// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// newMicroflowsScript is n microflows the project does not have yet: each
// statement inserts one unit.
func newMicroflowsScript(n int) string {
	var sb strings.Builder
	sb.WriteString("mdl 1;\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "create microflow MyFirstModule.IndexProbe%02d ()\nreturns boolean\nbegin\n  return true;\nend;\n", i)
	}
	return sb.String()
}

// ako/mxcli#2160, the unit index: every write marked the reader's list of
// units stale, and the next read scanned the whole Unit table to rebuild it —
// once per statement, for a write that added or changed one row. A write now
// updates the list for the unit it wrote.
//
// Six statements must scan the table no more often than two do.
func TestScript_AddedWritesDoNotRescanTheUnitTable(t *testing.T) {
	buildsFor := func(statements int) int64 {
		exec, out, _ := openPedAppCopy(t)
		b, ok := exec.Backend().(interface{ UnitIndexBuilds() int64 })
		if !ok {
			t.Fatalf("backend %T does not count its unit index builds", exec.Backend())
		}
		prog, errs := visitor.Build(newMicroflowsScript(statements))
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs[0])
		}
		before := b.UnitIndexBuilds()
		if err := exec.ExecuteProgram(prog); err != nil {
			t.Fatalf("exec: %v\n%s", err, out.String())
		}
		// The writes must be visible to a listing afterwards, or a list that
		// was never refreshed would pass the count.
		mfs, err := exec.Backend().ListMicroflows()
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, mf := range mfs {
			if strings.HasPrefix(mf.Name, "IndexProbe") {
				found++
			}
		}
		if found != statements {
			t.Fatalf("the listing after the script holds %d of the %d microflows it created", found, statements)
		}
		return b.UnitIndexBuilds() - before
	}

	two, six := buildsFor(2), buildsFor(6)
	t.Logf("unit table scans: %d for 2 statements, %d for 6", two, six)
	if six > two {
		t.Errorf("%d unit table scans for 6 statements against %d for 2: a write rescans the table", six, two)
	}
}

// Listing the modules is what rebuilding the container hierarchy starts with,
// and a script rebuilds that after every document it creates. A module's ID and
// name are in the unit index, and its Marketplace details in a unit the decode
// cache holds; it used to read every module's file twice per listing, unmarshal
// the whole document for its name and decode it again for the rest
// (ako/mxcli#2160).
func TestListModules_ReadsAndDecodesNothingOnceListed(t *testing.T) {
	exec, _, _ := openPedAppCopy(t)
	defer exec.CacheUnitReads()() // as a script run holds it

	list := func() map[string]*model.Module {
		t.Helper()
		ms, err := exec.Backend().ListModules()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]*model.Module{}
		for _, m := range ms {
			out[m.Name] = m
		}
		return out
	}
	list()
	want := list() // the second listing is the one the decode cache keeps

	for _, name := range []string{"MyFirstModule", "Administration", "Atlas_Core", "System"} {
		if want[name] == nil {
			t.Fatalf("ListModules does not list %s", name)
		}
	}
	if len(want) < 9 {
		t.Fatalf("control: %d modules listed; the fixture has eight and System", len(want))
	}
	// What the listing reads from the module's document, not from the index.
	if m := want["Atlas_Core"]; !m.FromAppStore || m.AppStoreVersion == "" {
		t.Errorf("Atlas_Core lost its Marketplace details: FromAppStore=%v version=%q", m.FromAppStore, m.AppStoreVersion)
	}
	if m := want["MyFirstModule"]; m.FromAppStore {
		t.Errorf("MyFirstModule reads as a Marketplace module")
	}

	reads, decodes := unitReadsNotFromMemory(t, exec), mprread.UnitsDecoded()
	again := list()
	if n := unitReadsNotFromMemory(t, exec) - reads; n != 0 {
		t.Errorf("listing %d modules again read %d unit files", len(again), n)
	}
	if n := mprread.UnitsDecoded() - decodes; n != 0 {
		t.Errorf("listing %d modules again decoded %d units", len(again), n)
	}
	for name, m := range want {
		got := again[name]
		if got == nil || got.ID != m.ID || got.FromAppStore != m.FromAppStore || got.AppStoreVersion != m.AppStoreVersion {
			t.Errorf("module %s differs between two listings: %+v and %+v", name, m, got)
		}
	}
}
