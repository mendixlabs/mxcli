// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func TestDocumentsCountsWhatIsGenerated(t *testing.T) {
	p := Params{Prefix: "Seed", Modules: 3, Entities: 4}
	if got, want := p.Documents(), 3*(1+4*4); got != want {
		t.Fatalf("Documents() = %d, want %d", got, want)
	}
	if got := strings.Count(Microflows(p), "create or modify microflow "); got != 3*4*2 {
		t.Errorf("%d microflows generated, want two per entity", got)
	}
	if got := strings.Count(Pages(p), "create or modify page "); got != 3*4*2 {
		t.Errorf("%d pages generated, want two per entity", got)
	}
	if got := strings.Count(Domain(p), " persistent entity "); got != 3*4 {
		t.Errorf("%d entities generated", got)
	}
	// One module is named by the bare prefix: the session's scripts and its
	// read steps both say `Bench.Item01`.
	if got := (Params{Prefix: "Bench", Modules: 1}).ModuleName(0); got != "Bench" {
		t.Errorf("single module named %q", got)
	}
}

// A bulk change must change every microflow and nothing about its identity:
// same names, different bodies.
func TestBulkAlterChangesEveryMicroflowBody(t *testing.T) {
	p := Params{Prefix: "Bench", Modules: 1, Entities: 3}
	before, after := Microflows(p), BulkAlter(p)
	if before == after {
		t.Fatal("the bulk change is the script it changes")
	}
	names := func(src string) []string {
		var out []string
		for _, line := range strings.Split(src, "\n") {
			if strings.HasPrefix(line, "create or modify microflow ") {
				out = append(out, line)
			}
		}
		return out
	}
	b, a := names(before), names(after)
	if len(b) != 6 || strings.Join(b, "\n") != strings.Join(a, "\n") {
		t.Errorf("the bulk change renamed or dropped a microflow:\n%v\n%v", b, a)
	}
}

// The benchmark must not time a failing script. This runs the session's
// scripts, and a two-module seed, through the real executor on the PedApp
// fixture — the project the nightly job starts from — and then checks the
// property the re-exec step asserts: a settled script lands no write.
func TestGeneratedScriptsExecuteAndSettleOnPedApp(t *testing.T) {
	src := filepath.Join("..", "..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Fatalf("the PedApp fixture is the benchmark's base project and is missing: %v", err)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(filepath.Join(dir, ".mxcli"))

	out := &bytes.Buffer{}
	exec := executor.New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: filepath.Join(dir, "PedApp.mpr")}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })

	run := func(name, script string) {
		t.Helper()
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatalf("%s does not parse: %v", name, errs[0])
		}
		if err := exec.ExecuteProgram(prog); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out.String())
		}
	}
	written := func() int {
		t.Helper()
		r, ok := exec.Backend().(backend.WriteStatsReporter)
		if !ok {
			t.Fatalf("backend %T does not report its writes", exec.Backend())
		}
		return r.WriteStats().Written
	}

	session := Params{Prefix: "Bench", Modules: 1, Entities: 2}
	run("domain", Domain(session))
	run("microflows", Microflows(session))
	run("pages", Pages(session))
	run("single-edit", SingleEdit(session))

	before := written()
	run("bulk-alter", BulkAlter(session))
	if written() == before {
		t.Fatal("control: the bulk change landed no write, so the settled re-run below would prove nothing")
	}
	before = written()
	run("re-exec", BulkAlter(session))
	if got := written() - before; got != 0 {
		t.Errorf("re-running the settled bulk change landed %d writes, want 0\n%s", got, out.String())
	}

	// The structural edit of the running-app session, and its re-run: the
	// attribute is there the second time, and the statement says so itself.
	before = written()
	run("entity-edit", EntityEdit(session))
	if written() == before {
		t.Error("adding an attribute landed no write")
	}
	before = written()
	run("entity-edit again", EntityEdit(session))
	if got := written() - before; got != 0 {
		t.Errorf("adding the same attribute again landed %d writes", got)
	}

	seed := Params{Prefix: "Seed", Modules: 2, Entities: 2}
	run("seed-domain", Domain(seed))
	run("seed-microflows", Microflows(seed))
	run("seed-pages", Pages(seed))
}

// The test suite must name what the session authored, or every test in it
// fails for a reason that has nothing to do with speed.
func TestTestsNameTheFirstEntitysValidation(t *testing.T) {
	p := Params{Prefix: "Bench", Modules: 1, Entities: 2}
	suite := Tests(p)
	if got := strings.Count(suite, "@test "); got != 2 {
		t.Errorf("%d tests in the suite, want 2", got)
	}
	for _, want := range []string{"create Bench.Item01 (", "call microflow Bench.VAL_Item01_Save("} {
		if !strings.Contains(suite, want) {
			t.Errorf("the suite does not contain %q", want)
		}
	}
	if !strings.Contains(Microflows(p), "create or modify microflow Bench.VAL_Item01_Save (") {
		t.Error("the microflow the suite calls is not one the session creates")
	}
}
