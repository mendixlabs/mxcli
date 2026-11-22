// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

func attr(entity, name, dataType, def string) linter.Attribute {
	return linter.Attribute{
		Name:                name,
		EntityQualifiedName: entity,
		ModuleName:          "Shop",
		DataType:            dataType,
		DefaultValue:        def,
	}
}

func TestFixedGuardAttributeRule_Metadata(t *testing.T) {
	r := NewFixedGuardAttributeRule()
	if r.ID() != "MPR017" {
		t.Errorf("ID = %q, want MPR017", r.ID())
	}
	if r.Name() != "FixedGuardAttribute" {
		t.Errorf("Name = %q, want FixedGuardAttribute", r.Name())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q, want correctness", r.Category())
	}
}

// The defect this rule exists for, as the benchmark's MxScout stage found it in
// all three apps it read:
//
//	"The tick, the housekeeping and the scheduled reset check a flag so that
//	 they do not work during the data reset, but no flow ever sets it to true;
//	 the reset only sets it to false."
//
// The flag IS written. A rule looking for "read but never written" would not
// see this at all, which is why the condition is a value set of one: every
// reachable value is false, so every decision on it has a fixed outcome.
func TestFixedGuards_FlagWrittenOnlyToOneConstant(t *testing.T) {
	facts := map[string]*attrFacts{
		"Simulation.SimulationSettings.IsResetInProgress": {
			readInCondition: true,
			literals:        map[string]bool{"false": true},
		},
	}
	got := fixedGuards([]linter.Attribute{
		attr("Simulation.SimulationSettings", "IsResetInProgress", "Boolean", "false"),
	}, facts)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].values != 1 {
		t.Errorf("values = %d, want 1", got[0].values)
	}
}

// The control that makes the rule mean something: a flag written to BOTH values
// is a working guard and must not be reported.
func TestFixedGuards_FlagWrittenToBothValuesIsFine(t *testing.T) {
	facts := map[string]*attrFacts{
		"Shop.Settings.IsOpen": {
			readInCondition: true,
			literals:        map[string]bool{"true": true, "false": true},
		},
	}
	if got := fixedGuards([]linter.Attribute{
		attr("Shop.Settings", "IsOpen", "Boolean", "false"),
	}, facts); len(got) != 0 {
		t.Fatalf("got %d findings, want 0: %+v", len(got), got)
	}
}

// A default of true with the only write false still has two reachable values,
// so the guard can change state once and is not dead. Getting this wrong is the
// easy mistake: counting writes alone would report it.
func TestFixedGuards_DefaultCountsAsAReachableValue(t *testing.T) {
	facts := map[string]*attrFacts{
		"Shop.Settings.IsFirstRun": {
			readInCondition: true,
			literals:        map[string]bool{"false": true},
		},
	}
	if got := fixedGuards([]linter.Attribute{
		attr("Shop.Settings", "IsFirstRun", "Boolean", "true"),
	}, facts); len(got) != 0 {
		t.Fatalf("got %d findings, want 0 (default true + write false = two values)", len(got))
	}
}

// Never written at all, and no default: still one value (empty), still a fixed
// guard. This is the "read but never written" case the issue led with.
func TestFixedGuards_NeverWrittenAtAll(t *testing.T) {
	facts := map[string]*attrFacts{
		"Shop.Settings.IsPaused": {readInCondition: true, literals: map[string]bool{}},
	}
	if got := fixedGuards([]linter.Attribute{
		attr("Shop.Settings", "IsPaused", "Boolean", ""),
	}, facts); len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
}

// Four reasons not to report, each a false positive the rule would otherwise
// produce on an ordinary app.
func TestFixedGuards_Exclusions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		a     linter.Attribute
		facts *attrFacts
	}{
		{
			// A user can type into it, so its value is not the model's to know.
			// This is the exclusion the issue called decisive: without it the
			// rule fires on half of a normal app.
			"bound to a page widget",
			attr("Shop.Settings", "IsOpen", "Boolean", "false"),
			&attrFacts{readInCondition: true, widgetBound: true, literals: map[string]bool{"false": true}},
		},
		{
			// Written from an expression, not a literal: the value set is
			// unknown, so nothing can be concluded.
			"written from an expression",
			attr("Shop.Settings", "IsOpen", "Boolean", "false"),
			&attrFacts{readInCondition: true, unknownWrite: true, literals: map[string]bool{"false": true}},
		},
		{
			// Never consulted in a condition, so there is no guard to be dead.
			// The written-but-never-read case is a separate verdict, not this one.
			"not read in any condition",
			attr("Shop.Settings", "IsOpen", "Boolean", "false"),
			&attrFacts{literals: map[string]bool{"false": true}},
		},
		{
			// Mendix computes it; the model does not assign it.
			"calculated",
			func() linter.Attribute {
				a := attr("Shop.Settings", "IsOpen", "Boolean", "")
				a.IsCalculated = true
				return a
			}(),
			&attrFacts{readInCondition: true, literals: map[string]bool{}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qn := tc.a.EntityQualifiedName + "." + tc.a.Name
			got := fixedGuards([]linter.Attribute{tc.a}, map[string]*attrFacts{qn: tc.facts})
			if len(got) != 0 {
				t.Fatalf("got %d findings, want 0: %+v", len(got), got)
			}
		})
	}
}

// Only Boolean attributes in v1. A String or Integer read in a condition and
// written to one literal is the same shape of defect, but "the literals I could
// see" is a far weaker claim for those types, so they are left out rather than
// reported at low confidence.
func TestFixedGuards_BooleanOnlyForNow(t *testing.T) {
	for _, dt := range []string{"String", "Integer", "Decimal", "DateTime", "Enum"} {
		facts := map[string]*attrFacts{
			"Shop.Settings.Mode": {readInCondition: true, literals: map[string]bool{"'a'": true}},
		}
		if got := fixedGuards([]linter.Attribute{
			attr("Shop.Settings", "Mode", dt, ""),
		}, facts); len(got) != 0 {
			t.Errorf("%s: got %d findings, want 0", dt, len(got))
		}
	}
}

// Reads are matched by attribute name against expression text, which cannot
// resolve which entity a variable holds. That makes the match deliberately
// over-broad: a read of SomeOther.IsOpen marks every IsOpen as read, so the
// rule under-reports rather than reporting something false.
func TestAttributeReadInExpression(t *testing.T) {
	for _, tc := range []struct {
		expr, name string
		want       bool
	}{
		{"$Settings/IsResetInProgress = true", "IsResetInProgress", true},
		{"$Settings/Shop.Settings_Order/IsOpen", "IsOpen", true},
		{"not $S/IsOpen", "IsOpen", true},
		{"$S/IsOpenToday", "IsOpen", false}, // prefix, not the member
		{"$S/WasIsOpen", "IsOpen", false},   // suffix, not the member
		{"'IsOpen'", "IsOpen", false},       // a string literal, not a member read
		{"$IsOpen", "IsOpen", false},        // a variable of that name, not a member
		{"", "IsOpen", false},
	} {
		if got := attributeReadInExpression(tc.expr, tc.name); got != tc.want {
			t.Errorf("attributeReadInExpression(%q, %q) = %v, want %v", tc.expr, tc.name, got, tc.want)
		}
	}
}

// A reader that cannot supply microflows yields no findings rather than
// reporting every attribute as unwritten — the failure mode that would make
// this rule dangerous, since "I could not see the writes" must never read as
// "there are no writes".
func TestFixedGuardAttributeRule_NoReaderReportsNothing(t *testing.T) {
	if got := NewFixedGuardAttributeRule().Check(linter.NewLintContextFromDB(nil)); len(got) != 0 {
		t.Fatalf("got %d findings with no reader, want 0", len(got))
	}
}

// The rule must declare that it needs a full catalog.
//
// Regression guard, not a hypothetical: without it the lint entry point builds
// a FAST catalog, the `activities` table is not written, and the rule finds no
// decision expressions — so it reports nothing on a project that does have the
// defect. Measured before the declaration: a fresh catalog gave "No issues
// found" while a second run against the cached (full) catalog reported the
// defect correctly. linter.CatalogRequirer's own doc warns about exactly this
// ("silently returning empty results").
func TestFixedGuardAttributeRule_RequiresFullCatalog(t *testing.T) {
	r := NewFixedGuardAttributeRule()
	req, ok := any(r).(linter.CatalogRequirer)
	if !ok {
		t.Fatal("rule does not implement linter.CatalogRequirer, so lint will build a fast catalog and the rule will silently find nothing")
	}
	if got := req.RequiredCatalogMode(); got != linter.CatalogFull {
		t.Errorf("RequiredCatalogMode = %v, want CatalogFull (it reads activities, widgets and xpath_expressions)", got)
	}
	// And the computed mode for a run including this rule must be full.
	if got := linter.RequiredCatalogMode([]linter.Rule{r}); got != linter.CatalogFull {
		t.Errorf("RequiredCatalogMode([rule]) = %v, want CatalogFull", got)
	}
}

// ako/mxcli#2501: four findings, all false, by two mechanisms plus one the
// report did not list. In each the attribute really is assigned, but not by a
// Change activity, so the rule read "nothing assigns it".
//
//   - an external entity's attributes come from the published service;
//   - a database-connection query's `Map:` assigns the attributes it maps;
//   - an import mapping assigns the attributes it maps (reproduced on a project:
//     a Boolean set from JSON `true` and tested by a decision was reported as
//     "can only ever be false").
func TestMappedAttributeNames_ImportMappingsAndDatabaseQueries(t *testing.T) {
	imports := []*model.ImportMapping{{
		Name: "IMM",
		Elements: []*model.ImportMappingElement{{
			Kind: "Object", Entity: "Shop.Standing",
			Children: []*model.ImportMappingElement{
				{Kind: "Value", Attribute: "Shop.Standing.Won"},
				{Kind: "Object", Entity: "Shop.Line", Children: []*model.ImportMappingElement{
					{Kind: "Value", Attribute: "Shop.Line.Accepted"},
				}},
				{Kind: "Value"}, // an element that maps no attribute
			},
		}},
	}}
	conns := []*model.DatabaseConnection{{
		Name: "Db",
		Queries: []*model.DatabaseQuery{{
			Name: "Q",
			TableMappings: []*model.DatabaseTableMapping{{
				Entity: "Shop.Result",
				Columns: []*model.DatabaseColumnMapping{
					{Attribute: "Shop.Result.Accepted"},
					{Attribute: ""},
				},
			}},
		}},
	}}
	got := mappedAttributeNames(imports, conns)
	want := []string{"Shop.Line.Accepted", "Shop.Result.Accepted", "Shop.Standing.Won"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if got := mappedAttributeNames(nil, nil); len(got) != 0 {
		t.Errorf("nothing mapped: want none, got %v", got)
	}
}

// An external entity is not judged: nothing in the consuming app assigns its
// attributes by construction, so "nothing ever assigns it" says nothing about the
// runtime value. A local entity is still judged (the control).
func TestFixedGuardEntity_ExternalEntitiesAreNotJudged(t *testing.T) {
	if guardableEntity(linter.Entity{QualifiedName: "F1Admin.Narrative", IsExternal: true}) {
		t.Error("an external entity must not be judged")
	}
	if !guardableEntity(linter.Entity{QualifiedName: "Shop.Settings"}) {
		t.Error("a local entity must still be judged")
	}
}

// mappedWritesReader is a reader that also lists import mappings and database
// connections, the way the production backend does (reached by assertion).
type mappedWritesReader struct {
	rawUnitReader
	imports []*model.ImportMapping
	conns   []*model.DatabaseConnection
}

func (r mappedWritesReader) ListImportMappings() ([]*model.ImportMapping, error) {
	return r.imports, nil
}
func (r mappedWritesReader) ListDatabaseConnections() ([]*model.DatabaseConnection, error) {
	return r.conns, nil
}

// Through the real entry point: a mapped attribute is an unknown write, so
// fixedGuards then leaves it alone. The unmapped one (the control) stays reportable.
func TestCollectAttributeFacts_MappedAttributesAreWrites(t *testing.T) {
	// A file-backed catalog: an in-memory one gives each pooled connection its own
	// empty database (ako/mxcli#2417).
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	for _, q := range []string{
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES ('m1', 'Shop', 'default', 's1')`,
		`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, Folder, EntityType, Description, Generalization, AttributeCount, AccessRuleCount, ValidationRuleCount, HasEventHandlers, IsExternal, ProjectId, SnapshotId) VALUES ('e1', 'Standing', 'Shop.Standing', 'Shop', '', 'PERSISTENT', '', '', 3, 0, 0, 0, 0, 'default', 's1')`,
		`INSERT INTO attributes_data (Id, Name, EntityId, EntityQualifiedName, ModuleName, DataType, Length, IsUnique, IsRequired, DefaultValue, IsCalculated, Description, ProjectId, SnapshotId) VALUES
			('a1', 'Won', 'e1', 'Shop.Standing', 'Shop', 'Boolean', 0, 0, 0, 'false', 0, '', 'default', 's1'),
			('a2', 'Accepted', 'e1', 'Shop.Standing', 'Shop', 'Boolean', 0, 0, 0, 'false', 0, '', 'default', 's1'),
			('a3', 'Local', 'e1', 'Shop.Standing', 'Shop', 'Boolean', 0, 0, 0, 'false', 0, '', 'default', 's1')`,
	} {
		if _, err := cat.CatalogDB().Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	r := mappedWritesReader{
		imports: []*model.ImportMapping{{Elements: []*model.ImportMappingElement{{Kind: "Object", Entity: "Shop.Standing",
			Children: []*model.ImportMappingElement{{Kind: "Value", Attribute: "Shop.Standing.Won"}}}}}},
		conns: []*model.DatabaseConnection{{Queries: []*model.DatabaseQuery{{TableMappings: []*model.DatabaseTableMapping{{
			Columns: []*model.DatabaseColumnMapping{{Attribute: "Shop.Standing.Accepted"}}}}}}}},
	}
	facts, why := collectAttributeFacts(linter.NewLintContext(cat, r))
	if why != "" {
		t.Fatalf("collect failed: %s", why)
	}
	for _, qn := range []string{"Shop.Standing.Won", "Shop.Standing.Accepted"} {
		if f := facts[qn]; f == nil || !f.unknownWrite {
			t.Errorf("%s is assigned by a mapping, so it must count as an unknown write: %+v", qn, f)
		}
	}
	if f := facts["Shop.Standing.Local"]; f == nil || f.unknownWrite {
		t.Errorf("control: Shop.Standing.Local is mapped by nothing and must not be marked written: %+v", f)
	}
}
