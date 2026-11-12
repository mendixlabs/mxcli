// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// entitiesScript is n entities in MyFirstModule, each referring to the one
// before it. extra, when not empty, is an attribute added to the entity at
// index changed (1-based), to make exactly one statement a change.
func entitiesScript(n, changed int, extra string) string {
	var sb strings.Builder
	sb.WriteString("mdl 1;\n")
	for i := 1; i <= n; i++ {
		more := ""
		if i == changed && extra != "" {
			more = ",\n  " + extra
		}
		fmt.Fprintf(&sb, "create or modify persistent entity MyFirstModule.RunProbe%02d (\n  Name: string(200) not null,\n  Amount: integer default 0%s\n);\n", i, more)
		if i > 1 {
			fmt.Fprintf(&sb, "create or modify association MyFirstModule.RunProbe%02d_RunProbe%02d from MyFirstModule.RunProbe%02d to MyFirstModule.RunProbe%02d;\n", i, i-1, i, i-1)
		}
	}
	return sb.String()
}

func runScript(t *testing.T, exec *Executor, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	return exec.ExecuteProgram(prog)
}

func unitReconciles(t *testing.T, e *Executor) int {
	t.Helper()
	b, ok := e.Backend().(interface{ UnitReconciles() int })
	if !ok {
		t.Fatalf("backend %T does not count its reconciles", e.Backend())
	}
	return b.UnitReconciles()
}

// ako/mxcli#2160, the entity stage: every CREATE ENTITY and CREATE ASSOCIATION
// rewrote its module's whole domain model and reconciled the rewrite against
// the stored unit, so a script building a domain model paid for the model once
// per statement. A run of them is now held and reconciled against storage when
// it ends.
//
// Six entities must cost no more reconciles than two.
func TestEntityRun_AddedStatementsDoNotReconcileTheDomainModelAgain(t *testing.T) {
	reconcilesFor := func(entities int) int {
		exec, out, _ := openPedAppCopy(t)
		before := unitReconciles(t, exec)
		if err := runScript(t, exec, entitiesScript(entities, 0, "")); err != nil {
			t.Fatalf("exec: %v\n%s", err, out.String())
		}
		// What the run held must be in the project afterwards.
		for i := 1; i <= entities; i++ {
			name := fmt.Sprintf("MyFirstModule.RunProbe%02d", i)
			if err := agreeExec(t, exec, "describe entity "+name+";"); err != nil {
				t.Fatalf("%s is not in the project after the run: %v", name, err)
			}
		}
		return unitReconciles(t, exec) - before
	}
	two, six := reconcilesFor(2), reconcilesFor(6)
	t.Logf("reconciles: %d for 2 entities, %d for 6", two, six)
	if six > two {
		t.Errorf("%d reconciles for 6 entities against %d for 2: each statement reconciles the domain model", six, two)
	}
}

// Judging a held write compared it with the write the run already held, and
// digested both sides to do it: the whole domain model twice per statement, a
// fifth of the time of a script that builds one. The held write's digest is
// now kept, so a statement digests only what it wrote.
//
// The first later write of a unit digests the held one once; that is the same
// for both sizes and cancels.
func TestEntityRun_AStatementDigestsOnlyItsOwnWrite(t *testing.T) {
	digestsFor := func(entities int) int {
		exec, out, _ := openPedAppCopy(t)
		b, ok := exec.Backend().(interface{ UnitJudgedDigests() int })
		if !ok {
			t.Fatalf("backend %T does not count its judged digests", exec.Backend())
		}
		before := b.UnitJudgedDigests()
		if err := runScript(t, exec, entitiesScript(entities, 0, "")); err != nil {
			t.Fatalf("exec: %v\n%s", err, out.String())
		}
		return b.UnitJudgedDigests() - before
	}
	two, six := digestsFor(2), digestsFor(6)
	// Four more entities and four more associations: eight more held writes.
	perWrite := float64(six-two) / 8
	t.Logf("digests: %d for 2 entities, %d for 6 — %.1f per added statement", two, six, perWrite)
	if six == two {
		t.Fatal("control: the added statements digested nothing, so the count is not measuring them")
	}
	if perWrite > 1 {
		t.Errorf("%.1f digests per added statement, want 1: the held write is digested again for each", perWrite)
	}
}

// The statements of a run still report for themselves. Re-running a settled
// script reports every statement unchanged and changes no file; and when one
// statement of the script does change something, it alone is reported as a
// change. Before the run judged its held writes, the re-run reported every
// entity "Modified" although nothing reached storage.
func TestEntityRun_EachStatementReportsWhetherItChangedAnything(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const n = 5
	if err := runScript(t, exec, entitiesScript(n, 0, "")); err != nil {
		t.Fatalf("first run: %v\n%s", err, out.String())
	}
	if got := strings.Count(out.String(), "Created entity: "); got != n {
		t.Fatalf("the first run reported %d created entities, want %d\n%s", got, n, out.String())
	}

	// Settled: nothing is a change.
	out.Reset()
	before, files := currentWriteStats(exec.Backend()), pedAppUnitDigest(t, dir)
	if err := runScript(t, exec, entitiesScript(n, 0, "")); err != nil {
		t.Fatalf("re-run: %v\n%s", err, out.String())
	}
	after := currentWriteStats(exec.Backend())
	if after.Offered == before.Offered {
		t.Fatal("control: the re-run offered no write, so 'none landed' proves nothing")
	}
	if after.Written != before.Written {
		t.Errorf("the settled re-run counted %d writes as landed", after.Written-before.Written)
	}
	if strings.Contains(out.String(), "Modified") || strings.Contains(out.String(), "Created") {
		t.Errorf("the settled re-run reported a change:\n%s", out.String())
	}
	if pedAppUnitDigest(t, dir) != files {
		t.Error("the settled re-run changed a unit file")
	}

	// One statement changes its entity: it is the only one reported.
	out.Reset()
	if err := runScript(t, exec, entitiesScript(n, 3, "Remark: string(50)")); err != nil {
		t.Fatalf("run with one change: %v\n%s", err, out.String())
	}
	if got := strings.Count(out.String(), "Modified entity: "); got != 1 || !strings.Contains(out.String(), "Modified entity: MyFirstModule.RunProbe03") {
		t.Errorf("want exactly RunProbe03 reported as modified, got:\n%s", out.String())
	}
	if pedAppUnitDigest(t, dir) == files {
		t.Error("the changed entity did not reach storage")
	}
}

// A statement that fails in the middle of a run does not take the earlier ones
// with it: what they held is written when the run ends, as it was when each
// wrote for itself.
func TestEntityRun_AFailingStatementLeavesTheEarlierOnesWritten(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	script := entitiesScript(2, 0, "") +
		"create association MyFirstModule.RunProbe02_Missing from MyFirstModule.RunProbe02 to MyFirstModule.NoSuchEntity;\n" +
		"create persistent entity MyFirstModule.RunProbeNever ( Name: string(10) );\n"
	if err := runScript(t, exec, script); err == nil {
		t.Fatalf("an association to a missing entity did not fail\n%s", out.String())
	}
	for _, name := range []string{"RunProbe01", "RunProbe02"} {
		if err := agreeExec(t, exec, "describe entity MyFirstModule."+name+";"); err != nil {
			t.Errorf("%s, created before the failing statement, is not in the project: %v", name, err)
		}
	}
	if err := agreeExec(t, exec, "describe entity MyFirstModule.RunProbeNever;"); err == nil {
		t.Error("the entity after the failing statement was created")
	}
}

// Any other statement ends the run first, so it reads and writes a project
// that holds what the run did. Here a microflow takes an entity the run has
// just created, and a second run follows it.
func TestEntityRun_AnotherStatementSeesWhatTheRunHeld(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	script := entitiesScript(2, 0, "") + `
create microflow MyFirstModule.RunProbeFlow ($Item: MyFirstModule.RunProbe02)
returns boolean
begin
  return $Item/Name != empty;
end;
create persistent entity MyFirstModule.RunProbeAfter ( Name: string(10) );
create association MyFirstModule.RunProbeAfter_RunProbe01 from MyFirstModule.RunProbeAfter to MyFirstModule.RunProbe01;
`
	if err := runScript(t, exec, script); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	for _, stmt := range []string{
		"describe microflow MyFirstModule.RunProbeFlow;",
		"describe entity MyFirstModule.RunProbeAfter;",
		"describe association MyFirstModule.RunProbeAfter_RunProbe01;",
	} {
		if err := agreeExec(t, exec, stmt); err != nil {
			t.Errorf("%s %v", stmt, err)
		}
	}
}

// pedAppUnitDigest is one digest over every unit file of the project in dir:
// equal before and after means no unit file was added, removed or rewritten
// with different bytes.
func pedAppUnitDigest(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(filepath.Join(dir, "mprcontents"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %x\n", path, sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// A storage GUID is the database's identity for an entity and its members, and
// a rewrite that re-mints one drops data on the next deploy. A run reconciles
// against storage once, at its end, where each statement used to; the stored
// GUIDs must come through that just the same.
//
// The subject is a Studio Pro-authored domain model: in one mxcli created, a
// GUID equals its $ID from birth and a re-mint would reproduce it unseen. The
// control checks that.
func TestEntityRun_KeepsTheStoredGUIDs(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	mod, err := exec.Backend().GetModuleByName("NanoflowCommons")
	if err != nil || mod == nil {
		t.Fatalf("module NanoflowCommons: %v", err)
	}
	dm, err := exec.Backend().GetDomainModel(mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := unitGUIDs(t, exec.Backend(), dm.ID)
	distinct := 0
	for id, guid := range before {
		if id != guid {
			distinct++
		}
	}
	if distinct == 0 {
		t.Fatal("control: every GUID in this domain model equals its $ID, so a re-mint could not be told from a carry")
	}

	// One run, two statements on the same domain model: the stored entity
	// re-declared with a member added, and a new entity beside it.
	script := `mdl 1;
create or modify non-persistent entity NanoflowCommons.Position (
  Latitude: String(200),
  Longitude: String(200),
  Altitude: String(50)
);
create non-persistent entity NanoflowCommons.RunProbeBeside ( Name: String(20) );
`
	if err := runScript(t, exec, script); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	after := unitGUIDs(t, exec.Backend(), dm.ID)
	if len(after) <= len(before) {
		t.Fatalf("control: the run added no GUID-bearing element (%d before, %d after), so it did not rewrite the domain model", len(before), len(after))
	}
	assertGUIDsKept(t, before, after)
}

// Both kinds of run hold their writes in the same place, so the one a
// statement does not belong to must be ended before the one it does belong to
// is opened. Otherwise the second opens over the first, and the first's end
// flushes the second's writes and closes its deferral under it.
func TestStepRuns_EndsOneRunBeforeOpeningTheOther(t *testing.T) {
	fake := &runOrderBackend{MockBackend: &mock.MockBackend{IsConnectedFunc: func() bool { return true }}}
	e := New(io.Discard)
	e.backend = fake
	var rules accessRuleRun
	var entities entityRun
	for _, stmt := range []ast.Statement{
		&ast.CreateEntityStmt{},
		&ast.CreateAssociationStmt{},
		&ast.GrantEntityAccessStmt{},
		&ast.CreateEntityStmt{},
		&ast.CreateMicroflowStmt{},
	} {
		if err := stepRuns(e, &rules, &entities, stmt); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"judged", "flush", "plain", "flush", "judged", "flush"}
	if !slices.Equal(fake.calls, want) {
		t.Errorf("deferral calls %v, want %v: each run opened, and flushed before the next opened", fake.calls, want)
	}

	// A flush that fails is the run's error, and says what did not happen.
	fake.flushErr = errors.New("storage GUID changed")
	if err := stepRuns(e, &rules, &entities, &ast.CreateEntityStmt{}); err != nil {
		t.Fatal(err)
	}
	err := entities.end()
	if err == nil || !strings.Contains(err.Error(), "did not take effect") || !strings.Contains(err.Error(), "storage GUID changed") {
		t.Errorf("a failed flush returned %v, want an error saying the statements did not take effect and why", err)
	}
}

// runOrderBackend records the order of deferral calls.
type runOrderBackend struct {
	*mock.MockBackend
	calls    []string
	flushErr error
}

func (b *runOrderBackend) DeferUnitWrites()       { b.calls = append(b.calls, "plain") }
func (b *runOrderBackend) DeferUnitWritesJudged() { b.calls = append(b.calls, "judged") }
func (b *runOrderBackend) FlushDeferredWrites() error {
	b.calls = append(b.calls, "flush")
	return b.flushErr
}
