// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// javaActionFixture builds a catalog holding one module, two Java actions (one
// documented, one not) and three parameters (one documented, two not).
//
// Rows are inserted directly rather than round-tripped through a reader because
// the subject here is the query + Starlark surface, not the MPR parser.
func javaActionFixture(t *testing.T) *catalog.Catalog {
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
			t.Fatalf("exec %s: %v", q, err)
		}
	}

	exec(`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		"mod-1", "Formula1Backend", "default", "s1")

	insertAction := func(id, name, doc string) {
		exec(`INSERT INTO java_actions_data
			(Id, Name, QualifiedName, ModuleName, Folder, Documentation, ExportLevel,
			 ReturnType, ParameterCount, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			id, name, "Formula1Backend."+name, "Formula1Backend", "", doc, "Hidden",
			"String", 0, "default", "s1")
	}
	insertParam := func(id, actionID, actionName, name, desc string, ordinal int) {
		exec(`INSERT INTO java_action_parameters_data
			(Id, JavaActionId, JavaActionName, QualifiedName, ModuleName, Name,
			 Description, ParameterType, IsRequired, Ordinal, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, actionID, actionName, "Formula1Backend."+actionName+"."+name,
			"Formula1Backend", name, desc, "String", 1, ordinal, "default", "s1")
	}

	insertAction("ja-1", "ParseTop", "")                         // undocumented
	insertAction("ja-2", "FormatLapTime", "Renders a lap time.") // documented
	insertParam("p-1", "ja-1", "ParseTop", "pUri", "", 0)        // undocumented
	insertParam("p-2", "ja-1", "ParseTop", "pDefault", "", 1)    // undocumented
	insertParam("p-3", "ja-2", "FormatLapTime", "pMillis", "Elapsed milliseconds.", 0)

	return cat
}

// The catalog stored only a parameter COUNT, so nothing downstream could see a
// parameter's description — the field a caller reads in Studio Pro's action
// dialog. This asserts the rows land and arrive attached to their action, in
// declaration order.
func TestJavaActions_CarryTheirParameters(t *testing.T) {
	ctx := linter.NewLintContext(javaActionFixture(t), &minimalReader{})

	byName := map[string]linter.JavaAction{}
	for ja := range ctx.JavaActions() {
		byName[ja.Name] = ja
	}
	if len(byName) != 2 {
		t.Fatalf("got %d Java actions, want 2: %v", len(byName), byName)
	}

	parse := byName["ParseTop"]
	if parse.Documentation != "" {
		t.Errorf("ParseTop documentation = %q, want empty", parse.Documentation)
	}
	if len(parse.Parameters) != 2 {
		t.Fatalf("ParseTop has %d parameters, want 2", len(parse.Parameters))
	}
	// Ordinal, not alphabetical: a signature read out of order is misleading.
	if parse.Parameters[0].Name != "pUri" || parse.Parameters[1].Name != "pDefault" {
		t.Errorf("parameters out of declaration order: %v", parse.Parameters)
	}
	if parse.Parameters[0].ParameterType != "String" || !parse.Parameters[0].IsRequired {
		t.Errorf("parameter metadata lost: %+v", parse.Parameters[0])
	}

	fmtLap := byName["FormatLapTime"]
	if fmtLap.Documentation == "" {
		t.Error("FormatLapTime documentation was dropped")
	}
	if len(fmtLap.Parameters) != 1 || fmtLap.Parameters[0].Description == "" {
		t.Errorf("FormatLapTime parameter description lost: %+v", fmtLap.Parameters)
	}
}

// Marketplace and system modules carry code the user did not write, and every
// other document iterator here excludes them. Reporting "the Community Commons
// Java actions are undocumented" would bury the findings that are actionable.
func TestJavaActions_ExcludeMarketplaceModules(t *testing.T) {
	cat := javaActionFixture(t)
	db := cat.CatalogDB()
	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, Source, ProjectId, SnapshotId) VALUES (?,?,?,?,?)`,
		"mod-2", "CommunityCommons", "Marketplace", "default", "s1"); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO java_actions_data
		(Id, Name, QualifiedName, ModuleName, Folder, Documentation, ExportLevel,
		 ReturnType, ParameterCount, ProjectId, SnapshotId)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		"ja-9", "StringUtil", "CommunityCommons.StringUtil", "CommunityCommons", "",
		"", "Hidden", "String", 0, "default", "s1"); err != nil {
		t.Fatalf("insert action: %v", err)
	}

	ctx := linter.NewLintContext(cat, &minimalReader{})
	for ja := range ctx.JavaActions() {
		if ja.ModuleName == "CommunityCommons" {
			t.Errorf("a Marketplace module's Java action was reported: %s", ja.QualifiedName)
		}
	}
}

// The rule under test is the one that ships, loaded from disk — not a copy
// inlined here. A test against an inlined copy proves the Starlark builtin works
// and says nothing about whether QUAL002 uses it.
func runMissingDocRule(t *testing.T, cat *catalog.Catalog, options map[string]any) []linter.Violation {
	t.Helper()
	rule, err := linter.LoadStarlarkRule("../../.claude/lint-rules/missing_documentation.star")
	if err != nil {
		t.Fatalf("loading the shipped QUAL002: %v", err)
	}
	if options != nil {
		rule.Configure(options)
	}
	return rule.Check(linter.NewLintContext(cat, &minimalReader{}))
}

// mxcli-formula1: entities and microflows were covered; Java actions and their
// parameters were not reachable from Starlark at all.
func TestQUAL002_FlagsUndocumentedJavaActionsAndParameters(t *testing.T) {
	vs := runMissingDocRule(t, javaActionFixture(t), nil)

	var messages []string
	for _, v := range vs {
		messages = append(messages, v.Message)
	}
	joined := strings.Join(messages, "\n")

	for _, want := range []string{
		"Java action 'ParseTop' has no documentation.",
		"Java action parameter 'ParseTop.pUri' has no description.",
		"Java action parameter 'ParseTop.pDefault' has no description.",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("QUAL002 did not report %q; got:\n%s", want, joined)
		}
	}
	// The documented action and its documented parameter must stay quiet,
	// otherwise the rule is noise rather than a signal.
	if strings.Contains(joined, "FormatLapTime") {
		t.Errorf("a documented action was flagged:\n%s", joined)
	}
}

// Each target is switchable, so a project that has decided Java actions are
// self-describing can silence just that half without losing the rest.
func TestQUAL002_TargetsAreIndividuallySwitchable(t *testing.T) {
	cat := javaActionFixture(t)

	noParams := runMissingDocRule(t, cat, map[string]any{"check_java_action_params": false})
	for _, v := range noParams {
		if strings.Contains(v.Message, "parameter") {
			t.Errorf("check_java_action_params=false still reported: %s", v.Message)
		}
	}
	if len(noParams) == 0 {
		t.Error("switching off parameters must not switch off the actions too")
	}

	noActions := runMissingDocRule(t, cat, map[string]any{"check_java_actions": false})
	for _, v := range noActions {
		if strings.Contains(v.Message, "Java action '") {
			t.Errorf("check_java_actions=false still reported: %s", v.Message)
		}
	}
}

// A guard against the fixture silently rotting: if the schema loses the
// parameters table, every assertion above would pass by reporting nothing.
func TestJavaActionParametersTableExists(t *testing.T) {
	cat := javaActionFixture(t)
	var n int
	if err := cat.CatalogDB().QueryRow(
		`SELECT COUNT(*) FROM java_action_parameters`).Scan(&n); err != nil && err != sql.ErrNoRows {
		t.Fatalf("java_action_parameters is not queryable: %v", err)
	}
	if n != 3 {
		t.Errorf("java_action_parameters holds %d rows, want 3", n)
	}
}

// allKindsFixture inserts exactly one undocumented element of every kind the
// generic sweep claims to cover, driven off DocumentableKinds() so a kind added
// in Go without a test row fails here rather than shipping unswept.
func allKindsFixture(t *testing.T) (*catalog.Catalog, map[string]string) {
	t.Helper()

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()

	// table -> doc column, mirroring documentableSources.
	tables := map[string]struct {
		kind   string
		docCol string
	}{
		"entities":                {"Entity", "Description"},
		"associations":            {"Association", "Description"},
		"pages":                   {"Page", "Description"},
		"snippets":                {"Snippet", "Description"},
		"building_blocks":         {"BuildingBlock", "Description"},
		"layouts":                 {"Layout", "Description"},
		"enumerations":            {"Enumeration", "Description"},
		"javascript_actions":      {"JavaScriptAction", "Description"},
		"image_collections":       {"ImageCollection", "Description"},
		"data_transformers":       {"DataTransformer", "Description"},
		"workflows":               {"Workflow", "Description"},
		"business_event_services": {"BusinessEventService", "Documentation"},
		"rest_clients":            {"RestClient", "Documentation"},
		"published_rest_services": {"PublishedRestService", "Documentation"},
		"constants":               {"Constant", "Description"},
		"json_structures":         {"JsonStructure", "Documentation"},
		"import_mappings":         {"ImportMapping", "Documentation"},
		"export_mappings":         {"ExportMapping", "Documentation"},
	}

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Description, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?)`,
		"mod-1", "Racing", "Racing", "Racing", "", "default", "s1"); err != nil {
		t.Fatalf("insert module: %v", err)
	}

	// kind -> the element name the sweep should report.
	//
	// Id is omitted deliberately: json_structures, import_mappings and
	// export_mappings declare `Id INTEGER PRIMARY KEY AUTOINCREMENT` while every
	// other table uses `Id TEXT PRIMARY KEY`, so a synthetic string id is a
	// datatype mismatch on exactly those three.
	// Module is deliberately NOT in here. A Mendix module has no documentation
	// property at all — the metamodel declares none, modelsdk/gen offers no
	// accessor, and no stored Projects$ModuleImpl carries the key — so QUAL002
	// no longer sweeps modules. The module row above still exists because every
	// other element in this fixture lives in it.
	want := map[string]string{}
	for table, meta := range tables {
		name := meta.kind + "X"
		q := fmt.Sprintf(
			`INSERT INTO %s_data (Name, QualifiedName, ModuleName, %s, ProjectId, SnapshotId)
			 VALUES (?,?,?,?,?,?)`, table, meta.docCol)
		if _, err := db.Exec(q, name, "Racing."+name, "Racing",
			"", "default", "s1"); err != nil {
			t.Fatalf("insert into %s: %v", table, err)
		}
		want[meta.kind] = name
	}
	return cat, want
}

// Every kind the Go side advertises must actually be swept. Without this, a new
// documentableSources row that the .star table does not know about is silently
// skipped by the rule's `entry == None` guard — covered in Go, invisible in
// practice.
func TestQUAL002_SweepsEveryAdvertisedDocumentType(t *testing.T) {
	cat, want := allKindsFixture(t)

	// The Go-side list and the fixture must agree, or the assertion below is
	// only as complete as whichever is shorter.
	for _, kind := range linter.DocumentableKinds() {
		if _, ok := want[kind]; !ok {
			t.Fatalf("DocumentableKinds() advertises %q but the fixture inserts no such row — "+
				"add it here, or the sweep for that kind is untested", kind)
		}
	}

	var msgs []string
	for _, v := range runMissingDocRule(t, cat, map[string]any{"check_associations": true}) {
		msgs = append(msgs, v.Message)
	}
	joined := strings.Join(msgs, "\n")

	for kind, name := range want {
		if !strings.Contains(joined, "'"+name+"'") {
			t.Errorf("kind %s (element %q) was not reported by QUAL002; got:\n%s", kind, name, joined)
		}
	}
}

// A documented element of every kind must produce silence. A sweep that reports
// regardless of content would pass the test above just as well.
func TestQUAL002_DocumentedElementsAreSilent(t *testing.T) {
	cat, _ := allKindsFixture(t)
	db := cat.CatalogDB()
	for _, tbl := range []string{"modules", "pages", "workflows", "constants"} {
		col := "Description"
		if tbl == "modules" {
			// A module's documentation is its domain model's: Mendix modules
			// have no Description of their own (ako/mxcli#2305).
			col = "DomainModelDocumentation"
		}
		if _, err := db.Exec(fmt.Sprintf(
			`UPDATE %s_data SET %s = 'Documented.'`, tbl, col)); err != nil {
			t.Fatalf("update %s: %v", tbl, err)
		}
	}

	var msgs []string
	for _, v := range runMissingDocRule(t, cat, nil) {
		msgs = append(msgs, v.Message)
	}
	joined := strings.Join(msgs, "\n")

	for _, quiet := range []string{"'Racing'", "'PageX'", "'WorkflowX'", "'ConstantX'"} {
		if strings.Contains(joined, quiet) {
			t.Errorf("a documented element %s was still flagged:\n%s", quiet, joined)
		}
	}
	// ...while the ones left blank must still be reported, proving the run
	// itself was not simply empty.
	if !strings.Contains(joined, "'LayoutX'") {
		t.Errorf("the sweep went quiet altogether; expected LayoutX:\n%s", joined)
	}
}

// Associations default OFF, like attributes: a real domain model has as many
// associations as entities and none are documented, so defaulting them on would
// double the rule's output with findings nobody asked for.
func TestQUAL002_AssociationsDefaultOff(t *testing.T) {
	cat, _ := allKindsFixture(t)

	var off []string
	for _, v := range runMissingDocRule(t, cat, nil) {
		off = append(off, v.Message)
	}
	if strings.Contains(strings.Join(off, "\n"), "AssociationX") {
		t.Error("associations were reported without being switched on")
	}

	var on []string
	for _, v := range runMissingDocRule(t, cat, map[string]any{"check_associations": true}) {
		on = append(on, v.Message)
	}
	if !strings.Contains(strings.Join(on, "\n"), "AssociationX") {
		t.Error("check_associations: true did not switch associations on")
	}
}

// The generic sweep needs its own Marketplace test: TestJavaActions_Exclude…
// covers only the Java action query, so dropping the filter from
// DocumentableElements left every other kind unguarded with a green suite.
func TestQUAL002_SweepExcludesMarketplaceModules(t *testing.T) {
	cat, _ := allKindsFixture(t)
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source, Description, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?,?)`,
		"mod-mp", "CommunityCommons", "CommunityCommons", "CommunityCommons",
		"Marketplace", "", "default", "s1"); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	// One element per shape: a page (joins modules by name) and the Marketplace
	// module row itself (filters on its own Source column — a different code
	// path, and the one an all-tables loop is most likely to forget).
	if _, err := db.Exec(
		`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, Description, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?)`,
		"pg-mp", "CommonsPage", "CommunityCommons.CommonsPage", "CommunityCommons",
		"", "default", "s1"); err != nil {
		t.Fatalf("insert page: %v", err)
	}

	var msgs []string
	for _, v := range runMissingDocRule(t, cat, map[string]any{"check_associations": true}) {
		msgs = append(msgs, v.Message)
	}
	joined := strings.Join(msgs, "\n")

	for _, leaked := range []string{"CommonsPage", "CommunityCommons"} {
		if strings.Contains(joined, leaked) {
			t.Errorf("Marketplace element %q was reported:\n%s", leaked, joined)
		}
	}
	// The user's own elements must still come through, or this passes trivially.
	if !strings.Contains(joined, "PageX") {
		t.Errorf("the sweep excluded everything, not just Marketplace:\n%s", joined)
	}
}

// System is the trap that Source-based filtering does not catch: the built-in
// module's Source is empty, exactly like a user module's, so a WHERE on Source
// alone lets all ~40 platform entities through. QUAL002 shipped that way before
// this sweep existed — FileDocument and HttpRequest were reported as
// undocumented on every project.
func TestQUAL002_ExcludesTheSystemModule(t *testing.T) {
	cat, _ := allKindsFixture(t)
	db := cat.CatalogDB()

	// Note the empty Source: System is indistinguishable from a user module on
	// that column alone. Only the sentinel Id separates them.
	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source, Description, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?,?)`,
		"00000000-0000-0000-0000-000000000001", "System", "System", "System",
		"", "", "default", "s1"); err != nil {
		t.Fatalf("insert System module: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, Description, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?)`,
		"sys-e1", "FileDocument", "System.FileDocument", "System",
		"", "default", "s1"); err != nil {
		t.Fatalf("insert System entity: %v", err)
	}

	var msgs []string
	for _, v := range runMissingDocRule(t, cat, map[string]any{"check_associations": true}) {
		msgs = append(msgs, v.Message)
	}
	joined := strings.Join(msgs, "\n")

	if strings.Contains(joined, "FileDocument") {
		t.Errorf("a System entity was reported:\n%s", joined)
	}
	if strings.Contains(joined, "'System'") {
		t.Errorf("the System module itself was reported:\n%s", joined)
	}
	// The positive control. Excluding System must not exclude the user's own
	// module along with it, and an entity is what carries that now — a module is
	// no longer reported at all, so its absence proves nothing here.
	if !strings.Contains(joined, "'EntityX'") {
		t.Errorf("the user's own module's contents stopped being reported:\n%s", joined)
	}
}
