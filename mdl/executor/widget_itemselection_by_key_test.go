// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// A selection-typed property written by its widget KEY.
//
//	datagrid dg (DataSource: database Shop.Ord, itemSelection: Single) { column (Attribute: Code) }
//
// passed check, exec reported "Created page" and mx check found nothing, but the
// grid had NO selection: the stored itemSelection read `PrimitiveValue 'Single'`
// and `Selection 'None'`. A selection-typed property keeps its value in the
// WidgetValue's `Selection` field, not in `PrimitiveValue`, so `itemSelection:`
// was written into a field the widget never reads. The mapped spelling,
// `Selection: Single`, stored it correctly.
//
// Two code paths disagreed about the same property. The mapping pass reads a
// selection mapping from its SOURCE keyword (`Selection:`) alone; the
// explicit-key pass skips what the mapping pass handled by comparing the
// authored name against the mappings' SOURCE names — so a name that is the
// property's own KEY (`itemSelection`) matched neither, and fell to the
// scalar branch, which writes every non-attribute type as a primitive.

// selectionEngine builds over Shop.Ord (Code), page-level: the widget's own
// DataSource decides the entity.
func selectionEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-shop")}, Name: "Shop"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-shop")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-ord")}, Name: "Ord",
						Attributes: []*domainmodel.Attribute{{Name: "Code"}}},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

// datagridDef is the definition a project gets for Data grid 2: generated from
// the installed package, not embedded. The test reads the package from testdata,
// so the mapping is the generator's real output.
func datagridDef(t *testing.T) *WidgetDefinition {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "expr-checker", "widgets", "com.mendix.widget.web.Datagrid.mpk")
	defs, err := mpk.ParseMPKAll(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(defs) == 0 {
		t.Fatalf("%s holds no widget", path)
	}
	return GenerateDefJSON(defs[0], "DATAGRID")
}

func galleryDef(t *testing.T) *WidgetDefinition {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := reg.Get("gallery")
	if !ok {
		t.Fatal("gallery is not registered")
	}
	return def
}

// selectionWidget is `<kind> w (DataSource: database Shop.Ord, <props>)`.
func selectionWidget(kind string, props map[string]any) *ast.WidgetV3 {
	all := map[string]any{"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Shop.Ord"}}
	for k, v := range props {
		all[k] = v
	}
	return &ast.WidgetV3{Name: "w", Type: kind, Properties: all}
}

func buildSelection(t *testing.T, def *WidgetDefinition, kind string, props map[string]any) (*pages.CustomWidget, error) {
	t.Helper()
	e := selectionEngine(t)
	return e.Build(def, selectionWidget(kind, props))
}

func mustBuildSelection(t *testing.T, def *WidgetDefinition, kind string, props map[string]any) *pages.CustomWidget {
	t.Helper()
	cw, err := buildSelection(t, def, kind, props)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return cw
}

func storedSelection(t *testing.T, cw *pages.CustomWidget, key string) string {
	t.Helper()
	s, _ := bsonField(customWidgetValue(t, cw, key), "Selection").(string)
	return s
}

// storedWithoutIDs drops what differs between two builds of the same widget by
// construction — freshly minted element ids and the pointers to them — so what
// is left is the stored configuration.
func storedWithoutIDs(v any) any {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, 0, len(x))
		for _, e := range x {
			if e.Key == "$ID" || e.Key == "TypePointer" {
				continue
			}
			out = append(out, bson.E{Key: e.Key, Value: storedWithoutIDs(e.Value)})
		}
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = storedWithoutIDs(e)
		}
		return out
	}
	return v
}

// sameStoredObject fails unless the two widgets store the same configuration,
// every property, ids aside. The visibility rules that hang off itemSelection
// (itemSelectionMethod, onSelectionChange, singleSelectionColumnLabel…) would
// show a divergence there even where itemSelection itself matched.
func sameStoredObject(t *testing.T, a, b *pages.CustomWidget) {
	t.Helper()
	if av, bv := storedWithoutIDs(a.RawObject), storedWithoutIDs(b.RawObject); !reflect.DeepEqual(av, bv) {
		t.Errorf("the widgets are stored differently:\n  by key:  %v\n  mapped:  %v", av, bv)
	}
}

// sameStoredProperty fails unless the property is stored identically (ids
// aside) in both widgets.
func sameStoredProperty(t *testing.T, a, b *pages.CustomWidget, key string) {
	t.Helper()
	av, bv := storedWithoutIDs(customWidgetValue(t, a, key)), storedWithoutIDs(customWidgetValue(t, b, key))
	if !reflect.DeepEqual(av, bv) {
		t.Errorf("%s is stored differently:\n  by key:  %v\n  mapped:  %v", key, av, bv)
	}
}

// The reported case, per widget and per value. The mapped spelling is the
// control: it already worked, and the by-key one must store exactly what it does.
func TestItemSelectionByKey_StoresTheSelectionField(t *testing.T) {
	// `Multiple` is the mapped keyword's spelling for what the widget calls Multi.
	cases := []struct{ byKey, mapped, want string }{
		{"Single", "Single", "Single"},
		{"Multi", "Multiple", "Multi"},
		{"None", "None", "None"},
	}
	for _, w := range []struct {
		kind string
		def  func(*testing.T) *WidgetDefinition
	}{{"datagrid", datagridDef}, {"gallery", galleryDef}} {
		for _, c := range cases {
			t.Run(w.kind+" "+c.byKey, func(t *testing.T) {
				def := w.def(t)
				byKey := mustBuildSelection(t, def, w.kind, map[string]any{"itemSelection": c.byKey})
				mapped := mustBuildSelection(t, def, w.kind, map[string]any{"Selection": c.mapped})

				if got := storedSelection(t, byKey, "itemSelection"); got != c.want {
					t.Errorf("by key: stored Selection = %q, want %q", got, c.want)
				}
				if got := storedPrimitive(t, byKey, "itemSelection"); got != storedPrimitive(t, mapped, "itemSelection") {
					t.Errorf("by key: PrimitiveValue = %q — a selection property keeps its value in Selection, "+
						"and the primitive field must stay what the mapped spelling leaves (%q)",
						got, storedPrimitive(t, mapped, "itemSelection"))
				}
				sameStoredProperty(t, byKey, mapped, "itemSelection")
				sameStoredObject(t, byKey, mapped)
			})
		}
	}
}

// The default the mapping supplies must not survive an explicit by-key value —
// a gallery selects Single when told nothing, and `itemSelection: None` is the
// way to turn that off.
func TestItemSelectionByKey_OverridesTheMappingDefault(t *testing.T) {
	def := galleryDef(t)
	if got := storedSelection(t, mustBuildSelection(t, def, "gallery", nil), "itemSelection"); got != "Single" {
		t.Fatalf("control: an unconfigured gallery stores Selection %q, want Single", got)
	}
	cw := mustBuildSelection(t, def, "gallery", map[string]any{"itemSelection": "None"})
	if got := storedSelection(t, cw, "itemSelection"); got != "None" {
		t.Errorf("gallery `itemSelection: None` stored Selection %q, want None", got)
	}
}

// A value the widget does not declare is refused, not stored. Case is the
// author's: the mapped keyword accepts any case and so does the key.
func TestItemSelectionByKey_InvalidValueRefusedByExec(t *testing.T) {
	for _, kind := range []string{"datagrid", "gallery"} {
		t.Run(kind, func(t *testing.T) {
			def := datagridDef(t)
			if kind == "gallery" {
				def = galleryDef(t)
			}
			_, err := buildSelection(t, def, kind, map[string]any{"itemSelection": "Bogus"})
			if err == nil {
				t.Fatal("`itemSelection: Bogus` was written")
			}
			for _, want := range []string{"itemSelection", "Bogus", "None", "Single", "Multi"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal should mention %q, got: %v", want, err)
				}
			}
		})
	}
	// The control: a valid value in another case is fine.
	if _, err := buildSelection(t, datagridDef(t), "datagrid", map[string]any{"itemSelection": "multi"}); err != nil {
		t.Errorf("`itemSelection: multi` should be accepted: %v", err)
	}
}

// Both spellings given: the mapped keyword and the key name the same property,
// so they must agree. Disagreement is refused — either would be a guess, and
// "the last one wins" depends on the order of two passes.
func TestItemSelectionByKey_ConflictingSpellingsRefusedByExec(t *testing.T) {
	for _, kind := range []string{"datagrid", "gallery"} {
		t.Run(kind, func(t *testing.T) {
			def := datagridDef(t)
			if kind == "gallery" {
				def = galleryDef(t)
			}
			_, err := buildSelection(t, def, kind, map[string]any{"Selection": "Single", "itemSelection": "Multi"})
			if err == nil {
				t.Fatal("`Selection: Single` with `itemSelection: Multi` was written")
			}
			for _, want := range []string{"Selection", "itemSelection", "Single", "Multi"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal should mention %q, got: %v", want, err)
				}
			}
		})
	}
	// The controls: the same value twice, however spelled, is one statement.
	for _, c := range []struct{ mapped, byKey string }{{"Single", "single"}, {"Multiple", "Multi"}} {
		cw, err := buildSelection(t, datagridDef(t), "datagrid", map[string]any{"Selection": c.mapped, "itemSelection": c.byKey})
		if err != nil {
			t.Errorf("Selection: %s with itemSelection: %s agree and must be accepted: %v", c.mapped, c.byKey, err)
			continue
		}
		if got := storedSelection(t, cw, "itemSelection"); got != canonicalSelection(c.byKey) {
			t.Errorf("Selection: %s with itemSelection: %s stored %q", c.mapped, c.byKey, got)
		}
	}
}

// A selection-typed property no mapping covers still has a value type, and the
// explicit-key pass has to honour it — a definition edited by hand, or one
// generated before the property existed, must not turn the same key back into a
// primitive write.
func TestItemSelectionByKey_UnmappedSelectionPropertyIsWrittenAsSelection(t *testing.T) {
	def := datagridDef(t)
	var kept []PropertyMapping
	for _, m := range def.PropertyMappings {
		if m.PropertyKey != "itemSelection" {
			kept = append(kept, m)
		}
	}
	if len(kept) == len(def.PropertyMappings) {
		t.Fatal("test setup: the generated definition has no itemSelection mapping to remove")
	}
	def.PropertyMappings = kept

	cw := mustBuildSelection(t, def, "datagrid", map[string]any{"itemSelection": "Multi"})
	if got := storedSelection(t, cw, "itemSelection"); got != "Multi" {
		t.Errorf("unmapped itemSelection stored Selection %q, want Multi", got)
	}
	if got := storedPrimitive(t, cw, "itemSelection"); got != "" {
		t.Errorf("unmapped itemSelection wrote PrimitiveValue %q", got)
	}
	if _, err := buildSelection(t, def, "datagrid", map[string]any{"itemSelection": "Bogus"}); err == nil {
		t.Error("an unmapped itemSelection: Bogus was written")
	}
}

// Properties that ARE scalars keep their route. These are the controls for the
// change: a fix that rerouted by-key writes wholesale would show here first.
func TestItemSelectionByKey_ScalarKeysUnchanged(t *testing.T) {
	cw := mustBuildSelection(t, datagridDef(t), "datagrid", map[string]any{"refreshInterval": 30, "pageSize": 7})
	if got := storedPrimitive(t, cw, "refreshInterval"); got != "30" {
		t.Errorf("datagrid refreshInterval = %q, want 30", got)
	}
	if got := storedPrimitive(t, cw, "pageSize"); got != "7" {
		t.Errorf("datagrid pageSize = %q, want 7", got)
	}
	g := mustBuildSelection(t, galleryDef(t), "gallery", map[string]any{"pageSize": 30, "pagination": "virtualScrolling"})
	if got := storedPrimitive(t, g, "pageSize"); got != "30" {
		t.Errorf("gallery pageSize = %q, want 30", got)
	}
	if got := storedPrimitive(t, g, "pagination"); got != "virtualScrolling" {
		t.Errorf("gallery pagination = %q, want virtualScrolling", got)
	}
}

// A mapped constant written by its key.
//
// The gallery definition maps `advanced` to the constant "false", but the
// Gallery widget (3.x) declares no such property — the definition names a key an
// older version had. The mapping pass wrote the author's `advanced: true` over
// the constant, the builder skipped the unknown key without a word, and the
// refusal that exists for exactly this (refuseUndeclaredMappedKey) exempted every
// mapping with a constant, authored or not. Nothing was stored and nothing was said.
func TestMappedConstantByKey_UndeclaredKeyIsRefusedWhenAuthored(t *testing.T) {
	def := galleryDef(t)
	_, err := buildSelection(t, def, "gallery", map[string]any{"advanced": true})
	if err == nil {
		t.Fatal("gallery `advanced: true` was accepted and dropped: the widget has no such property")
	}
	for _, want := range []string{"advanced", "does not declare"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal should mention %q, got: %v", want, err)
		}
	}
	// The controls. The definition's own constant is not the author's data, and
	// an authored value equal to it drops nothing the definition would not have.
	for name, props := range map[string]map[string]any{"unset": nil, "equal to the constant": {"advanced": false}} {
		if _, err := buildSelection(t, def, "gallery", props); err != nil {
			t.Errorf("gallery with advanced %s was refused: %v", name, err)
		}
	}
}

// …and on a widget that does declare the property, the author's value replaces
// the constant, as it always did.
func TestMappedConstantByKey_DeclaredKeyIsStored(t *testing.T) {
	on := mustBuildSelection(t, datagridDef(t), "datagrid", map[string]any{"advanced": true})
	off := mustBuildSelection(t, datagridDef(t), "datagrid", nil)
	if got := storedPrimitive(t, on, "advanced"); got != "true" {
		t.Errorf("datagrid `advanced: true` stored %q, want true", got)
	}
	if got := storedPrimitive(t, off, "advanced"); got != "false" {
		t.Errorf("control: an unconfigured datagrid stores advanced = %q, want false", got)
	}
}

// --- check -------------------------------------------------------------------

func selectionRegistry(t *testing.T) *WidgetRegistry {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dg := datagridDef(t)
	dg.MDLName = "datagrid"
	reg.byMDLName["DATAGRID"] = dg
	reg.byWidgetID[dg.WidgetID] = dg
	return reg
}

func selectionPage(widget string) string {
	return `create page Shop.P ( Title: 'P' )
{
  ` + widget + `
}`
}

func checkSelection(t *testing.T, widget string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(selectionPage(widget))
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	var out []linter.Violation
	reg := selectionRegistry(t)
	for _, stmt := range prog.Statements {
		out = append(out, ValidateWidgetPropertiesForStatement(stmt, reg)...)
	}
	return GateWidgetViolations(out, prog.LanguageVersion)
}

func violationsOf(vs []linter.Violation, rule string) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == rule {
			out = append(out, v)
		}
	}
	return out
}

func TestItemSelectionByKey_InvalidValueRefusedByCheck(t *testing.T) {
	for _, widget := range []string{
		`datagrid dg (DataSource: database Shop.Ord, itemSelection: Bogus) { column (Attribute: Code) }`,
		`gallery g (DataSource: database Shop.Ord, itemSelection: Bogus) { template t { dynamictext x (Content: 'x') } }`,
	} {
		t.Run(strings.Fields(widget)[0], func(t *testing.T) {
			got := violationsOf(checkSelection(t, widget), selectionValueRule)
			if len(got) != 1 {
				t.Fatalf("got %d %s violations, want 1", len(got), selectionValueRule)
			}
			if got[0].Severity != linter.SeverityError {
				t.Errorf("severity = %v, want Error", got[0].Severity)
			}
			for _, want := range []string{"itemSelection", "Bogus", "None", "Single", "Multi"} {
				if !strings.Contains(got[0].Message, want) {
					t.Errorf("message should mention %q: %s", want, got[0].Message)
				}
			}
		})
	}
}

func TestItemSelectionByKey_ConflictRefusedByCheck(t *testing.T) {
	widget := `datagrid dg (DataSource: database Shop.Ord, Selection: Single, itemSelection: Multi) { column (Attribute: Code) }`
	got := violationsOf(checkSelection(t, widget), selectionValueRule)
	if len(got) != 1 {
		t.Fatalf("got %d %s violations, want 1", len(got), selectionValueRule)
	}
	for _, want := range []string{"Selection", "itemSelection", "Single", "Multi"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message should mention %q: %s", want, got[0].Message)
		}
	}
}

// The controls: every spelling that is right stays silent, so the rule cannot
// have been satisfied by firing on everything.
func TestItemSelectionByKey_ValidSpellingsPassCheck(t *testing.T) {
	for _, widget := range []string{
		`datagrid dg (DataSource: database Shop.Ord, itemSelection: Single) { column (Attribute: Code) }`,
		`datagrid dg (DataSource: database Shop.Ord, itemSelection: Multi) { column (Attribute: Code) }`,
		`datagrid dg (DataSource: database Shop.Ord, itemSelection: none) { column (Attribute: Code) }`,
		`datagrid dg (DataSource: database Shop.Ord, Selection: Multiple) { column (Attribute: Code) }`,
		`datagrid dg (DataSource: database Shop.Ord, Selection: Single, itemSelection: Single) { column (Attribute: Code) }`,
		`datagrid dg (DataSource: database Shop.Ord) { column (Attribute: Code) }`,
	} {
		t.Run(widget, func(t *testing.T) {
			if got := violationsOf(checkSelection(t, widget), selectionValueRule); len(got) != 0 {
				t.Errorf("a right spelling was refused: %v", got)
			}
		})
	}
}

// The gallery's own ambiguity check reads the selection the widget will get.
// It read `Selection:` alone, so a by-key `itemSelection: None` — which stores
// None — was still reported as "a selection and an onClick".
func TestItemSelectionByKey_GalleryClickRuleSeesTheKey(t *testing.T) {
	const click = `onClick: show_page Shop.Detail`
	for _, c := range []struct {
		props   string
		refused bool
	}{
		{`itemSelection: None`, false},
		{`Selection: None`, false},
		{`itemSelection: Single`, true},
		{``, true},
	} {
		t.Run(c.props, func(t *testing.T) {
			w := `gallery g (DataSource: database Shop.Ord, ` + click + `, ` + c.props + `) { template t { dynamictext x (Content: 'x') } }`
			if c.props == "" {
				w = `gallery g (DataSource: database Shop.Ord, ` + click + `) { template t { dynamictext x (Content: 'x') } }`
			}
			got := violationsOf(checkSelection(t, w), galleryClickRefused.Code)
			if (len(got) > 0) != c.refused {
				t.Errorf("the gallery click rule fired = %v, want %v", len(got) > 0, c.refused)
			}
		})
	}
}

// The check-time twin of the build refusal above. `gallery … advanced: true`
// checked clean and then failed exec; check now says so first. The controls are
// the value that equals the constant, and a widget that does declare the key.
func TestMappedConstantByKey_UndeclaredKeyIsRefusedByCheck(t *testing.T) {
	const tmpl = `template { dynamictext t (Content: 'x') }`
	got := violationsOf(checkSelection(t,
		`gallery g (DataSource: database Shop.Ord, advanced: true) { `+tmpl+` }`), authoredConstantUndeclared)
	if len(got) != 1 {
		t.Fatalf("gallery `advanced: true`: got %d %s violations, want 1", len(got), authoredConstantUndeclared)
	}
	if got[0].Severity != linter.SeverityError || !strings.Contains(got[0].Message, "advanced") {
		t.Errorf("unexpected violation: %+v", got[0])
	}
	for name, widget := range map[string]string{
		"gallery advanced: false":  `gallery g (DataSource: database Shop.Ord, advanced: false) { ` + tmpl + ` }`,
		"gallery without advanced": `gallery g (DataSource: database Shop.Ord) { ` + tmpl + ` }`,
		"datagrid advanced: true":  `datagrid dg (DataSource: database Shop.Ord, advanced: true) { column (Attribute: Code) }`,
	} {
		if got := violationsOf(checkSelection(t, widget), authoredConstantUndeclared); len(got) != 0 {
			t.Errorf("%s was refused: %v", name, got)
		}
	}
}

// A selection-typed property that has no keyword at all — the combo box's
// `optionsSourceDatabaseItemSelection` — can only be written by its key. It goes
// through the explicit-key pass, which must store the Selection field (not a
// primitive), and `check` must not call it unpersisted (MDL-WIDGET06).
func TestItemSelectionByKey_ComboBoxSelectionWithNoKeyword(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := reg.Get("combobox")
	if !ok {
		t.Fatal("combobox is not registered")
	}
	const key = "optionsSourceDatabaseItemSelection"
	for _, m := range selectionMappingsOf(def) {
		if m.PropertyKey == key {
			t.Fatalf("test setup: %s is mapped; this test is for a selection with no keyword", key)
		}
	}

	e := selectionEngine(t)
	w := &ast.WidgetV3{Name: "cb", Type: "combobox", Properties: map[string]any{
		"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Shop.Ord"},
		key:          "Multi",
	}}
	cw, err := e.Build(def, w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := storedSelection(t, cw, key); got != "Multi" {
		t.Errorf("%s stored Selection %q, want Multi", key, got)
	}
	if got := storedPrimitive(t, cw, key); got != "" {
		t.Errorf("%s wrote PrimitiveValue %q", key, got)
	}

	// check: the key is recognised (the project's package declares it) and, being
	// written by the explicit pass, must not be reported as dropped.
	d := *def
	d.KnownProperties = append(append([]string(nil), def.KnownProperties...), key)
	reg2 := selectionRegistry(t)
	reg2.byMDLName["COMBOBOX"] = &d
	reg2.byWidgetID[d.WidgetID] = &d
	prog, errs := visitor.Build(selectionPage(`combobox cb (DataSource: database Shop.Ord, ` + key + `: Multi)`))
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	var vs []linter.Violation
	for _, stmt := range prog.Statements {
		vs = append(vs, ValidateWidgetPropertiesForStatement(stmt, reg2)...)
	}
	for _, v := range vs {
		if v.RuleID == "MDL-WIDGET06" || v.RuleID == "MDL-WIDGET01" || v.RuleID == selectionValueRule {
			t.Errorf("unexpected %s: %s", v.RuleID, v.Message)
		}
	}
}
