// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// systemModuleID is the sentinel Mendix gives the built-in System module.
// Restated here rather than exported from the linter: a test that reads the
// constant under test cannot detect that constant being wrong.
const systemModuleID = "00000000-0000-0000-0000-000000000001"

// systemLeakFixture builds a catalog holding one user module and the System
// module, each with one element in every table the LintContext iterates.
//
// The System rows carry an EMPTY Source, which is the whole point: that column
// cannot distinguish System from a user module, so a filter written only against
// it lets all of System through.
func systemLeakFixture(t *testing.T) *catalog.Catalog {
	t.Helper()

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}

	exec(`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source, ProjectId, SnapshotId)
	      VALUES (?,?,?,?,?,?,?)`, "mod-user", "Racing", "Racing", "Racing", "", "default", "s1")
	exec(`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source, ProjectId, SnapshotId)
	      VALUES (?,?,?,?,?,?,?)`, systemModuleID, "System", "System", "System", "", "default", "s1")
	exec(`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source, ProjectId, SnapshotId)
	      VALUES (?,?,?,?,?,?,?)`, "mod-mp", "CommunityCommons", "CommunityCommons",
		"CommunityCommons", "Marketplace v1.0", "default", "s1")

	// One row per module in each iterated table. Names are suffixed with the
	// module so a leak is identifiable from the value alone.
	for _, mod := range []string{"Racing", "System", "CommunityCommons"} {
		exec(`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, EntityType, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?,?)`, "e-"+mod, "Entity"+mod, mod+".Entity"+mod, mod, "PERSISTENT", "default", "s1")
		exec(`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?,?)`, "mf-"+mod, "MF"+mod, mod+".MF"+mod, mod, "MICROFLOW", "default", "s1")
		exec(`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?)`, "pg-"+mod, "Page"+mod, mod+".Page"+mod, mod, "default", "s1")
		exec(`INSERT INTO enumerations_data (Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?)`, "en-"+mod, "Enum"+mod, mod+".Enum"+mod, mod, "default", "s1")
		exec(`INSERT INTO constants_data (Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?)`, "c-"+mod, "Const"+mod, mod+".Const"+mod, mod, "default", "s1")
		exec(`INSERT INTO snippets_data (Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?)`, "sn-"+mod, "Snip"+mod, mod+".Snip"+mod, mod, "default", "s1")
		exec(`INSERT INTO java_actions_data (Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?)`, "ja-"+mod, "JA"+mod, mod+".JA"+mod, mod, "default", "s1")
		exec(`INSERT INTO widgets_data (Id, Name, WidgetType, ModuleName, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?)`, "w-"+mod, "Widget"+mod, "TextBox", mod, "default", "s1")
		exec(`INSERT INTO workflows_data (Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?)`, "wf-"+mod, "WF"+mod, mod+".WF"+mod, mod, "default", "s1")
		exec(`INSERT INTO workflow_activities_data (Id, WorkflowId, WorkflowQualifiedName, ModuleName, Name, ActivityType, ProjectId, SnapshotId)
		      VALUES (?,?,?,?,?,?,?,?)`, "wa-"+mod, "wf-"+mod, mod+".WF"+mod, mod, "Task", "SystemTask", "default", "s1")
	}

	return cat
}

// Every LintContext iterator must exclude both platform sources. System is the
// one that Source-based filtering misses, and it is by far the larger leak: a
// blank project ships ~40 System entities, so a rule over entities reported 40
// findings the user could do nothing about.
func TestIterators_ExcludePlatformModules(t *testing.T) {
	cat := systemLeakFixture(t)
	ctx := linter.NewLintContext(cat, &minimalReader{})

	// name -> the qualified names that iterator yielded.
	got := map[string][]string{}
	collect := func(name string, qn ...string) { got[name] = append(got[name], qn...) }

	for e := range ctx.Entities() {
		collect("Entities", e.QualifiedName)
	}
	for mf := range ctx.Microflows() {
		collect("Microflows", mf.QualifiedName)
	}
	for p := range ctx.Pages() {
		collect("Pages", p.QualifiedName)
	}
	for en := range ctx.Enumerations() {
		collect("Enumerations", en.QualifiedName)
	}
	for c := range ctx.Constants() {
		collect("Constants", c.QualifiedName)
	}
	for s := range ctx.Snippets() {
		collect("Snippets", s.QualifiedName)
	}
	for ja := range ctx.JavaActions() {
		collect("JavaActions", ja.QualifiedName)
	}
	for w := range ctx.Widgets() {
		collect("Widgets", w.ModuleName+".w")
	}
	for w := range ctx.Workflows() {
		collect("Workflows", w.QualifiedName)
	}
	for a := range ctx.WorkflowActivities() {
		collect("WorkflowActivities", a.WorkflowQualifiedName)
	}
	for d := range ctx.DocumentableElements() {
		collect("DocumentableElements", d.QualifiedName)
	}
	for _, kind := range []string{"entity", "microflow", "page"} {
		collect("FindUnused("+kind+")", ctx.FindUnused(kind)...)
	}

	if len(got) == 0 {
		t.Fatal("no iterators were exercised")
	}

	for name, names := range got {
		joined := strings.Join(names, " ")
		if strings.Contains(joined, "System") {
			t.Errorf("%s leaked a System element: %v", name, names)
		}
		if strings.Contains(joined, "CommunityCommons") {
			t.Errorf("%s leaked a Marketplace element: %v", name, names)
		}
		// Guard against the trivial pass: an iterator that returns nothing
		// satisfies both assertions above.
		if !strings.Contains(joined, "Racing") {
			t.Errorf("%s returned no user elements at all (%v) — the assertions above "+
				"would pass even with the filter broken", name, names)
		}
	}
}
