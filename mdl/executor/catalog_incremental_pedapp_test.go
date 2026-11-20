// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// ako/mxcli#1804: a refresh rebuilt every catalog table from every unit after
// any save. It now rebuilds the changed units and the documents that name
// them, and copies every other unit's rows from the cached catalog. That is
// only safe if it never changes the answer, so every step below compares
// every table of the incremental build with a forced full one.

// catalogRows builds the catalog, incremental unless force, and returns every
// row of every table by table, with how the build went.
func catalogRows(t *testing.T, exec *Executor, full, force bool) (map[string][]string, catalog.BuildStats) {
	t.Helper()
	var stats catalog.BuildStats
	catalogBuildHook = func(s catalog.BuildStats) { stats = s }
	defer func() { catalogBuildHook = nil }()

	ctx := exec.newExecContext(context.Background())
	var out bytes.Buffer
	ctx.Output = &out
	ctx.catalogForce = force
	if err := buildCatalog(ctx, full, false, false, 0); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	defer func() { ctx.Catalog.Close(); ctx.Catalog = nil }()
	return dumpCatalog(t, ctx.Catalog), stats
}

// dumpCatalog reads every row of every table the build writes. The rowid
// (an INTEGER PRIMARY KEY column) is left out: it numbers rows in the order
// they were written, and a carried row is written before a rebuilt one.
func dumpCatalog(t *testing.T, cat *catalog.Catalog) map[string][]string {
	t.Helper()
	tables, err := cat.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	isShadow := func(name string) bool {
		for _, fts := range []string{"strings_", "source_"} {
			if rest, ok := strings.CutPrefix(name, fts); ok {
				switch rest {
				case "data", "idx", "content", "docsize", "config":
					return true
				}
			}
		}
		return false
	}
	rows := map[string][]string{}
	for _, tr := range tables.Rows {
		name := fmt.Sprint(tr[0])
		// What records when and how the build ran, not what it found.
		if name == "catalog_meta" || name == "snapshots" || name == "projects" ||
			strings.HasPrefix(name, "catalog_") || isShadow(name) {
			continue
		}
		info, err := cat.Query(`SELECT name, type, pk FROM pragma_table_info('` + name + `')`)
		if err != nil {
			t.Fatal(err)
		}
		var cols []string
		for _, c := range info.Rows {
			if fmt.Sprint(c[2]) == "1" && strings.EqualFold(fmt.Sprint(c[1]), "INTEGER") {
				continue
			}
			cols = append(cols, `"`+fmt.Sprint(c[0])+`"`)
		}
		res, err := cat.Query("SELECT " + strings.Join(cols, ", ") + " FROM " + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, r := range res.Rows {
			rows[name] = append(rows[name], fmt.Sprint(r...))
		}
		sort.Strings(rows[name])
	}
	return rows
}

// diffTables reports every table whose rows differ, with a few of the rows.
func diffTables(t *testing.T, step string, got, want map[string][]string) {
	t.Helper()
	names := map[string]bool{}
	for n := range got {
		names[n] = true
	}
	for n := range want {
		names[n] = true
	}
	var sorted []string
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		g, w := got[n], want[n]
		if strings.Join(g, "\n") == strings.Join(w, "\n") {
			continue
		}
		missing, extra := multisetDiff(w, g), multisetDiff(g, w)
		t.Errorf("%s: table %s differs from a full build (%d rows, want %d)\n  missing: %s\n  extra:   %s",
			step, n, len(g), len(w), sample(missing), sample(extra))
	}
}

// multisetDiff is the rows of a that b does not have, counting duplicates.
func multisetDiff(a, b []string) []string {
	have := map[string]int{}
	for _, r := range b {
		have[r]++
	}
	var out []string
	for _, r := range a {
		if have[r] > 0 {
			have[r]--
			continue
		}
		out = append(out, r)
	}
	return out
}

func sample(rows []string) string {
	if len(rows) > 3 {
		return fmt.Sprintf("%q … (%d more)", rows[:3], len(rows)-3)
	}
	return fmt.Sprintf("%q", rows)
}

func TestCatalog_IncrementalBuildEqualsFullBuild(t *testing.T) {
	incrementalSteps(t, true)
}

// The fast build (a plain REFRESH CATALOG) writes no refs, so it has no
// dependents to follow: every row it writes must come from its own unit.
func TestCatalog_IncrementalFastBuildEqualsFullBuild(t *testing.T) {
	incrementalSteps(t, false)
}

func incrementalSteps(t *testing.T, full bool) {
	exec, out, dir := openPedAppCopy(t)
	setupIncrementalProbes(t, exec, out)
	cachePath := getCachePath(exec.newExecContext(context.Background()))
	base, _ := catalogRows(t, exec, full, true)
	if full && (len(base["refs"]) < 50 || len(base["activities_data"]) < 50) {
		t.Fatalf("control: the fixture has %d refs and %d activities", len(base["refs"]), len(base["activities_data"]))
	}
	if len(base["entities_data"]) < 20 || len(base["microflows_data"]) < 20 {
		t.Fatalf("control: the fixture has %d entities and %d flows", len(base["entities_data"]), len(base["microflows_data"]))
	}

	// Each step edits the project the way an agent does between two
	// refreshes. full says the step must not be incremental: it changes a
	// unit no per-unit pass reads.
	steps := []struct {
		name, mdl string
		full      bool
		rounds    int // 2: the plan must widen its scope once
	}{
		{name: "no change"},
		{name: "edit a flow", mdl: strings.Replace(redeclareChain(2, "create or modify"), "length($Title) + $Prev", "length($Title) * 2 + $Prev", 1)},
		{name: "callee returns another type", mdl: `mdl 1;
create or modify microflow MyFirstModule.MF_Chain001 ($Title: String) returns Boolean as $B
begin
  declare $B Boolean = length($Title) > 3;
  return $B;
end;
`},
		{name: "data source flow returns another entity", mdl: `mdl 1;
create or modify microflow MyFirstModule.GetProbe () returns MyFirstModule.ProbeB as $B
begin
  $B = create MyFirstModule.ProbeB (Name = 'b');
  return $B;
end;
`},
		{name: "add a flow", mdl: `mdl 1;
create microflow MyFirstModule.MF_Added ($Title: String) returns Integer as $N
begin
  $N = call microflow MyFirstModule.MF_Chain004(Title = $Title);
  return $N;
end;
`},
		{name: "rename a flow", mdl: "rename microflow MyFirstModule.MF_Chain004 to MF_Chain004b;"},
		{name: "alter an entity", mdl: "alter entity FeedbackModule.Feedback add attribute IncrementalProbe: String;"},
		{name: "rename an inherited attribute", mdl: "alter entity MyFirstModule.ProbeA rename attribute Name to Label;"},
		// Nothing in the flow changes, and what it deletes does: the
		// variable's entity is the association's other end.
		{name: "repoint an association", mdl: `drop association MyFirstModule.ProbeLink;
create association MyFirstModule.ProbeLink from MyFirstModule.ProbeD to MyFirstModule.ProbeA;`},
		{name: "add an association", mdl: "create association MyFirstModule.ProbeA_ProbeB from MyFirstModule.ProbeA to MyFirstModule.ProbeB;"},
		{name: "drop an association", mdl: "drop association MyFirstModule.ProbeA_ProbeB;"},
		{name: "rename an entity", mdl: "rename entity MyFirstModule.ProbeB to ProbeC;"},
		{name: "add an enumeration value", mdl: "alter enumeration MyFirstModule.ProbeColor add value Blue caption 'Blue';"},
		{name: "edit a page", mdl: `mdl 1;
create or modify page MyFirstModule.ProbePage (Title: 'P2', Layout: Atlas_Core.Atlas_Default) {
  dataview dataView1 (DataSource: microflow MyFirstModule.GetProbe) {
    textbox txtName (Attribute: Name)
    actionbutton btnGo (Caption: 'Go', Action: microflow MyFirstModule.MF_Chain001)
  }
};
`},
		{name: "move a flow to a new folder", mdl: "move microflow MyFirstModule.MF_Chain001 to folder 'Probes';", full: true},
		{name: "move a flow into that folder", mdl: "move microflow MyFirstModule.MF_Chain002 to folder 'Probes';"},
		{name: "drop a flow", mdl: "drop microflow MyFirstModule.MF_Added;"},
		// The page still names the flow it no longer finds. Created again,
		// the flow is a unit the previous catalog never knew, so only the
		// built catalog shows the page naming it: the plan widens once.
		{name: "drop a called flow", mdl: "drop microflow MyFirstModule.GetProbe;"},
		{name: "create it again", mdl: `mdl 1;
create microflow MyFirstModule.GetProbe () returns MyFirstModule.ProbeA as $A
begin
  $A = create MyFirstModule.ProbeA (Label = 'a');
  return $A;
end;
`, rounds: 2},
	}

	incremental := 0
	for _, s := range steps {
		if s.mdl != "" {
			if err := agreeExec(t, exec, s.mdl); err != nil {
				t.Fatalf("%s: %v\n%s", s.name, err, out.String())
			}
		}
		got, stats := catalogRows(t, exec, full, false)
		// A second refresh with nothing changed carries from the first.
		again, againStats := catalogRows(t, exec, full, false)
		if !againStats.Incremental || againStats.Rebuilt != 0 {
			t.Errorf("%s: a refresh with nothing changed rebuilt %d units (%+v)", s.name, againStats.Rebuilt, againStats)
		}
		// The forced full build is the reference. It replaces the cache, so
		// the incremental one is put back: the next step carries from it, and
		// every build in the sequence is incremental on incremental.
		saved, err := os.ReadFile(cachePath)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := catalogRows(t, exec, full, true)
		if err := os.WriteFile(cachePath, saved, 0o644); err != nil {
			t.Fatal(err)
		}

		diffTables(t, s.name, got, want)
		diffTables(t, s.name+" (again)", again, want)
		switch {
		case s.full && stats.Incremental:
			t.Errorf("%s: the build was incremental, want a full one", s.name)
		case !s.full && !stats.Incremental:
			t.Errorf("%s: the build was full (%s), want an incremental one", s.name, stats.Reason)
		}
		if full && s.rounds != 0 && stats.Rounds != s.rounds {
			t.Errorf("%s: the build took %d rounds, want %d", s.name, stats.Rounds, s.rounds)
		}
		if stats.Incremental {
			incremental++
		}
		t.Logf("%s: %+v", s.name, stats)
	}
	if incremental == 0 {
		t.Fatalf("control: no build was incremental, so the comparison proves nothing about carrying rows")
	}
	_ = dir
}

// setupIncrementalProbes adds the documents the incremental steps edit: a
// call chain, entities with a specialization and an association, a flow
// that deletes over that association, and a page whose data source is a
// flow.
func setupIncrementalProbes(t *testing.T, exec *Executor, out *bytes.Buffer) {
	t.Helper()
	if err := agreeExec(t, exec, redeclareChain(4, "create")); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if err := agreeExec(t, exec, `mdl 1;
create persistent entity MyFirstModule.ProbeA (Name: String(50));
create persistent entity MyFirstModule.ProbeB (Name: String(50));
create persistent entity MyFirstModule.ProbeSub extends MyFirstModule.ProbeA (Extra: String(50));
create persistent entity MyFirstModule.ProbeD (Name: String(50));
create association MyFirstModule.ProbeLink from MyFirstModule.ProbeB to MyFirstModule.ProbeA;
create enumeration MyFirstModule.ProbeColor (Red 'Red', Green 'Green');
create microflow MyFirstModule.GetProbe () returns MyFirstModule.ProbeA as $A
begin
  $A = create MyFirstModule.ProbeA (Name = 'a');
  return $A;
end;
create microflow MyFirstModule.UseSub ($S: MyFirstModule.ProbeSub) returns String as $N
begin
  declare $N String = $S/Name;
  return $N;
end;
create microflow FeedbackModule.DropLinked ($A: MyFirstModule.ProbeA)
begin
  retrieve $Linked from $A/MyFirstModule.ProbeLink;
  delete $Linked;
end;
create page MyFirstModule.ProbePage (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dataview dataView1 (DataSource: microflow MyFirstModule.GetProbe) {
    textbox txtName (Attribute: Name)
  }
};
`); err != nil {
		t.Fatalf("setup probes: %v\n%s", err, out.String())
	}
	// ako/mxcli#1829: documents in another module that name a MyFirstModule
	// entity or enumeration through exactly one construct each, so no other
	// edge into its domain model covers the read. The enumeration value Blue is
	// named before "add an enumeration value" creates it.
	if err := agreeExec(t, exec, `mdl 1;
create association FeedbackModule.Feedback_ProbeA from FeedbackModule.Feedback to MyFirstModule.ProbeA;
create microflow FeedbackModule.ProbeCreateList ()
begin
  $Ds = create list of MyFirstModule.ProbeD;
end;
create microflow MyFirstModule.ProbeResultType ($Json: String)
begin
  $R = import from mapping FeedbackModule.IMM_PostResponse($Json);
end;
create microflow FeedbackModule.ProbePathEnd ($F: FeedbackModule.Feedback) returns Boolean as $B
begin
  declare $B Boolean = $F/FeedbackModule.Feedback_ProbeA/MyFirstModule.ProbeA != empty;
  return $B;
end;
create microflow FeedbackModule.ProbeMissingValue () returns Boolean as $B
begin
  declare $B Boolean = MyFirstModule.ProbeColor.Blue = MyFirstModule.ProbeColor.Blue;
  return $B;
end;
create snippet FeedbackModule.ProbeSnippet (Params: ($D: MyFirstModule.ProbeD)) {
  container ctnProbe {}
};
`); err != nil {
		t.Fatalf("setup #1829 probes: %v\n%s", err, out.String())
	}
}
