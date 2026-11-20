// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// MDL-WIDGET39 (CE2421) and MDL-WIDGET40 (CE0582): two page errors only mxbuild
// reported. `check --references` passed a textbox bound to an enumeration and a
// classic drop-down on a React-client project; `mx check` then failed both.

func typeCheckCtx(t *testing.T, optimizedClient string) *ExecContext {
	t.Helper()
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-sales")}, Name: "Sales"}
	attr := func(name string, typ domainmodel.AttributeType) *domainmodel.Attribute {
		return &domainmodel.Attribute{Name: name, Type: typ}
	}
	b := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-sales")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-thing")}, Name: "Thing",
						Attributes: []*domainmodel.Attribute{
							attr("Name", &domainmodel.StringAttributeType{Length: 200}),
							attr("Pw", &domainmodel.HashedStringAttributeType{}),
							attr("Color", &domainmodel.EnumerationAttributeType{EnumerationRef: "Sales.Color"}),
							attr("Flag", &domainmodel.BooleanAttributeType{}),
							attr("WhenAt", &domainmodel.DateTimeAttributeType{}),
							attr("Num", &domainmodel.IntegerAttributeType{}),
							attr("LongNum", &domainmodel.LongAttributeType{}),
							attr("Amount", &domainmodel.DecimalAttributeType{}),
							attr("Seq", &domainmodel.AutoNumberAttributeType{}),
							attr("Blob", &domainmodel.BinaryAttributeType{}),
						}},
					{BaseElement: model.BaseElement{ID: model.ID("e-other")}, Name: "Other",
						Attributes: []*domainmodel.Attribute{
							attr("Shade", &domainmodel.EnumerationAttributeType{EnumerationRef: "Sales.Color"}),
							attr("Txt", &domainmodel.StringAttributeType{}),
						}},
					{BaseElement: model.BaseElement{ID: model.ID("e-sub")}, Name: "SubThing",
						GeneralizationRef: "Sales.Thing",
						Attributes:        []*domainmodel.Attribute{attr("Extra", &domainmodel.StringAttributeType{})}},
				},
				Associations: []*domainmodel.Association{{
					BaseElement: model.BaseElement{ID: model.ID("a-thing-other")}, Name: "Thing_Other",
					ParentID: model.ID("e-thing"), ChildID: model.ID("e-other"),
				}},
			}}, nil
		},
	}
	if optimizedClient != "" {
		b.GetProjectSettingsFunc = func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{WebUI: &model.WebUISettings{UseOptimizedClient: optimizedClient}}, nil
		}
	}
	ctx, _ := newMockCtx(t, withBackend(b))
	ctx.widgetRegistry = &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{}, byWidgetID: map[string]*WidgetDefinition{}}
	ctx.widgetRegistryLoaded = true
	return ctx
}

func typeCheckPage(t *testing.T, body string) *ast.CreatePageStmtV3 {
	t.Helper()
	src := `create page Sales.P (title: 'P', layout: Atlas_Core.Atlas_Default,
  params: ($Thing: Sales.Thing, $Sub: Sales.SubThing)) {
` + body + `
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	return s
}

func typeErrs(t *testing.T, ctx *ExecContext, body string, sc *scriptContext) []string {
	t.Helper()
	s := typeCheckPage(t, body)
	if sc == nil {
		sc = newScriptContext()
	}
	return validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, allPageWidgets(s), sc)
}

// The reported case: a text box on an enumeration attribute.
func TestCheck_TextBoxOnEnumerationIsCE2421(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) { textbox tbColor (label: 'C', attribute: Color) }`, nil)
	if len(errs) != 1 {
		t.Fatalf("want one error, got %q", errs)
	}
	for _, want := range []string{"MDL-WIDGET39", "CE2421", "tbColor", "Sales.Thing.Color", "Enumeration",
		"Hashed string, Integer, Long, String, Decimal, AutoNumber"} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("error should mention %q, got: %s", want, errs[0])
		}
	}
}

// The measured matrix (mxbuild 11.14.0, every pair built on a fresh app): for
// each built-in input widget, the attribute types it accepts. Every other pair
// in the table was CE2421. Written out here independently of the rule's own
// table so the test is a second statement of the measurement, not a copy.
func TestCheck_InputWidgetAttributeTypeMatrix(t *testing.T) {
	ctx := typeCheckCtx(t, "No")
	attrs := map[string]string{ // attribute -> kind
		"Name": "String", "Pw": "HashedString", "Color": "Enumeration", "Flag": "Boolean",
		"WhenAt": "DateTime", "Num": "Integer", "LongNum": "Long", "Amount": "Decimal",
		"Seq": "AutoNumber", "Blob": "Binary",
	}
	accepts := map[string]map[string]bool{
		"textbox":      {"String": true, "HashedString": true, "Integer": true, "Long": true, "Decimal": true, "AutoNumber": true},
		"textarea":     {"String": true},
		"datepicker":   {"DateTime": true},
		"checkbox":     {"Boolean": true},
		"radiobuttons": {"Boolean": true, "Enumeration": true},
		"dropdown":     {"Enumeration": true},
	}
	for widget, ok := range accepts {
		for attrName, kind := range attrs {
			body := fmt.Sprintf(`dataview dv (datasource: $Thing) { %s w1 (label: 'x', attribute: %s) }`, widget, attrName)
			errs := typeErrs(t, ctx, body, nil)
			flagged := len(errs) > 0
			if kind == "HashedString" {
				// Not measured: MDL cannot author one, so it is never judged.
				if flagged {
					t.Errorf("%s on %s: an unmeasured kind was judged: %q", widget, attrName, errs)
				}
				continue
			}
			if flagged == ok[kind] {
				t.Errorf("%s on %s (%s): flagged=%v, want %v (%q)", widget, attrName, kind, flagged, !ok[kind], errs)
			}
		}
	}
}

// An attribute declared on a generalization is judged by its declared type.
func TestCheck_InheritedEnumerationOnTextBoxIsCE2421(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Sub) {
    textbox tbColor (label: 'C', attribute: Color)
    textbox tbName (label: 'N', attribute: Name)
  }`, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "tbColor") || !strings.Contains(errs[0], "Sales.Thing.Color") {
		t.Fatalf("want one error on tbColor naming the declaring entity, got %q", errs)
	}
}

// Over an association path the final attribute's type is what counts — mxbuild
// reported `Thing_OtherE/Shade` against `OtherE.Shade`.
func TestCheck_AssociationPathIsJudgedAtItsEnd(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) {
    textbox tbShade (label: 'S', attribute: Thing_Other/Shade)
    textbox tbTxt (label: 'T', attribute: Thing_Other/Txt)
  }`, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "tbShade") || !strings.Contains(errs[0], "Sales.Other.Shade") {
		t.Fatalf("want one error on tbShade, got %q", errs)
	}
}

// A bare association where an attribute belongs: mxbuild answers CE1613 (the
// page builder qualifies it as an attribute that does not exist), and that
// error stops `mx check` from reporting anything else.
func TestCheck_InputBoundToAnAssociationIsCE1613(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) { textbox tbAssoc (label: 'A', attribute: Thing_Other) }`, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "CE1613") || !strings.Contains(errs[0], "Sales.Thing_Other") {
		t.Fatalf("want one CE1613 error naming the association, got %q", errs)
	}
}

// The script's own entities are judged by the types it declares — the common
// shape is one script with the entity and the page.
func TestCheck_ScriptDeclaredEntityAttributeTypes(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	prog, perrs := visitor.Build(`create enumeration Sales.Size (S 'S', L 'L');
create persistent entity Sales.Shirt (Label: String(50), Size: Enumeration(Sales.Size));
create association Sales.Shirt_Other from Sales.Shirt to Sales.Other;
create page Sales.P2 (title: 'P', layout: Atlas_Core.Atlas_Default, params: ($Shirt: Sales.Shirt)) {
  dataview dv (datasource: $Shirt) {
    textbox tbSize (label: 'S', attribute: Size)
    textbox tbLabel (label: 'L', attribute: Label)
    textbox tbShade (label: 'X', attribute: Shirt_Other/Shade)
  }
}`)
	if len(perrs) > 0 {
		t.Fatalf("parse: %v", perrs)
	}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	s := prog.Statements[3].(*ast.CreatePageStmtV3)
	errs := validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, allPageWidgets(s), sc)
	if len(errs) != 2 || !strings.Contains(strings.Join(errs, "\n"), "tbSize") || !strings.Contains(strings.Join(errs, "\n"), "tbShade") {
		t.Fatalf("want errors on tbSize and tbShade only, got %q", errs)
	}
}

// What the walk cannot place is not judged: an association data source, a page
// variable. An attribute the entity does not have is not a TYPE error — there is
// no type to judge — but it is the CE1613 of ako/mxcli#555, reported once.
func TestCheck_AttributeTypeUnjudgedWhenUnknown(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) {
    dataview dv2 (datasource: association Thing_Other) { textbox tb1 (label: 'x', attribute: Shade) }
    textbox tb2 (label: 'x', attribute: NoSuchAttribute)
  }`, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "widget `tb2`") ||
		!strings.Contains(errs[0], "has no attribute NoSuchAttribute") || strings.Contains(errs[0], widgetAttributeTypeRule) {
		t.Fatalf("want only tb2's missing-attribute CE1613, no type error, got %q", errs)
	}
}

// CE0582: the classic drop-down under the React client, and only there —
// measured on 11.14.0: UseOptimizedClient=Yes reports it, No and MigrationMode
// do not.
func TestCheck_ClassicDropDownUnderReactIsCE0582(t *testing.T) {
	body := `dataview dv (datasource: $Thing) { dropdown ddColor (label: 'C', attribute: Color) }`
	errs := typeErrs(t, typeCheckCtx(t, "Yes"), body, nil)
	if len(errs) != 1 {
		t.Fatalf("want one error under React, got %q", errs)
	}
	for _, want := range []string{"MDL-WIDGET40", "CE0582", "ddColor", "combobox"} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("error should mention %q, got: %s", want, errs[0])
		}
	}
	for _, mode := range []string{"No", "MigrationMode", ""} {
		if errs := typeErrs(t, typeCheckCtx(t, mode), body, nil); len(errs) != 0 {
			t.Errorf("UseOptimizedClient=%q: want no error, got %q", mode, errs)
		}
	}
}

// Wired into the reference pass a page goes through.
func TestCheck_AttributeTypeIsPartOfThePageReferencePass(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	sc := newScriptContext()
	sc.modules["Sales"] = true
	err := validateWithContext(ctx, typeCheckPage(t, `dataview dv (datasource: $Thing) { textbox tbColor (label: 'C', attribute: Color) }`), sc)
	if err == nil || !strings.Contains(err.Error(), "MDL-WIDGET39") {
		t.Fatalf("want the page refused for MDL-WIDGET39, got %v", err)
	}
}

// ako/mxcli#953: CE0582 is the React client's, and a native page is not
// rendered by it. Measured on mxbuild 11.13.0 (UseOptimizedClient Yes): a
// classic drop-down on a NativePhone_Default page builds clean, the same
// drop-down on an Atlas_Default page is CE0582 (the control). MDL-WIDGET39 is
// NOT React-only — a textbox on an enumeration is CE2421 on the native page
// too — so it must keep firing there.
func TestCheck_ClassicDropDownOnNativePageIsClean(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	b := ctx.Backend.(*mock.MockBackend)
	atlas := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-atlas")}, Name: "Atlas_Core"}
	sales := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-sales")}, Name: "Sales"}
	b.ListModulesFunc = func() ([]*model.Module, error) { return []*model.Module{sales, atlas}, nil }
	layout := func(id, name string, native bool) *pages.Layout {
		l := &pages.Layout{ContainerID: atlas.ID, Name: name, Native: native}
		l.ID = model.ID(id)
		return l
	}
	b.ListLayoutsFunc = func() ([]*pages.Layout, error) {
		return []*pages.Layout{layout("l-web", "Atlas_Default", false), layout("l-nat", "NativePhone_Default", true)}, nil
	}
	body := `dataview dv (datasource: $Thing) { dropdown ddColor (label: 'C', attribute: Color) }`
	s := typeCheckPage(t, body)

	if errs := validatePluggableAttributeScopes(ctx, "Atlas_Core.Atlas_Default", s.Parameters, allPageWidgets(s), newScriptContext()); len(errs) != 1 || !strings.Contains(errs[0], "MDL-WIDGET40") {
		t.Fatalf("control: web page should report MDL-WIDGET40, got %q", errs)
	}
	if errs := validatePluggableAttributeScopes(ctx, "Atlas_Core.NativePhone_Default", s.Parameters, allPageWidgets(s), newScriptContext()); len(errs) != 0 {
		t.Fatalf("native page: want no error, got %q", errs)
	}
	enum := typeCheckPage(t, `dataview dv (datasource: $Thing) { textbox tbColor (label: 'C', attribute: Color) }`)
	if errs := validatePluggableAttributeScopes(ctx, "Atlas_Core.NativePhone_Default", enum.Parameters, allPageWidgets(enum), newScriptContext()); len(errs) != 1 || !strings.Contains(errs[0], "MDL-WIDGET39") {
		t.Fatalf("native page: MDL-WIDGET39 (CE2421) still applies, got %q", errs)
	}
}

// A combo box in enumeration mode (no options data source) binds its attribute
// to `attributeEnumeration`, which takes only an Enumeration. Measured on fresh
// 11.14.0 and 11.13.0 projects, one combo box per attribute type: every type but
// Enumeration was CE7247 "Only attributes of type Enumeration are allowed here."
// — Boolean included, though the widget has a separate Boolean source that MDL
// does not write. `check --references` passed all eight.
func TestCheck_ComboBoxEnumerationModeAttributeTypes(t *testing.T) {
	ctx := typeCheckCtx(t, "No")
	attrs := map[string]string{
		"Name": "String", "Color": "Enumeration", "Flag": "Boolean", "WhenAt": "DateTime",
		"Num": "Integer", "LongNum": "Long", "Amount": "Decimal", "Seq": "AutoNumber", "Blob": "Binary",
	}
	for attrName, kind := range attrs {
		body := fmt.Sprintf(`dataview dv (datasource: $Thing) { combobox cb1 (label: 'x', attribute: %s) }`, attrName)
		errs := typeErrs(t, ctx, body, nil)
		if want := kind != "Enumeration"; (len(errs) > 0) != want {
			t.Errorf("combobox on %s (%s): flagged=%v, want %v (%q)", attrName, kind, len(errs) > 0, want, errs)
		}
	}
}

// The reported case, and what its message must carry.
func TestCheck_ComboBoxOnBooleanIsCE7247(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) { combobox cbFlag (label: 'F', attribute: Flag) }`, nil)
	if len(errs) != 1 {
		t.Fatalf("want one error, got %q", errs)
	}
	for _, want := range []string{"MDL-WIDGET39", "CE7247", "cbFlag", "Sales.Thing.Flag", "Boolean",
		"Only attributes of type Enumeration are allowed here.", "`checkbox`", "`radiobuttons`"} {
		if !strings.Contains(errs[0], want) {
			t.Errorf("error should mention %q, got: %s", want, errs[0])
		}
	}
	if strings.Contains(errs[0], "CE2421") {
		t.Errorf("a combo box is CE7247, not the built-in widgets' CE2421: %s", errs[0])
	}
}

// Association mode binds an association, not `attributeEnumeration`: a data
// source puts the combo box in it whichever key names the association, so the
// attribute-type rule does not apply.
func TestCheck_ComboBoxAssociationModeIsNotTypeChecked(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	errs := typeErrs(t, ctx, `dataview dv (datasource: $Thing) {
    combobox cb1 (label: 'O', attribute: Thing_Other, datasource: database Sales.Other, CaptionAttribute: Txt)
  }`, nil)
	for _, e := range errs {
		if strings.Contains(e, "MDL-WIDGET39") {
			t.Fatalf("association mode judged as an enumeration binding: %s", e)
		}
	}
}

// mendixlabs/mxcli#1583: a combo box in DATABASE mode — selected by naming its
// own datasource, `optionsSourceDatabaseDataSource` — stores the chosen value on
// `databaseAttributeString`, which takes String, Integer, Long or Enumeration.
// Judged as enumeration mode, a String target was refused with CE7247, so the
// MDL describe prints for such a combo box could not be re-executed. Measured on
// a fresh 11.12.5 project (Combo box 2.5.0), one combo box per type: Boolean,
// DateTime, Decimal, AutoNumber and Binary are CE7247 "Only attributes of type
// String, Integer, Long, Enumeration are allowed here."; the other four build.
func TestCheck_ComboBoxDatabaseModeAttributeTypes(t *testing.T) {
	ctx := typeCheckCtx(t, "Yes")
	attrs := map[string]string{
		"Name": "String", "Color": "Enumeration", "Num": "Integer", "LongNum": "Long",
		"Flag": "Boolean", "WhenAt": "DateTime", "Amount": "Decimal", "Seq": "AutoNumber", "Blob": "Binary",
	}
	accepted := map[string]bool{"String": true, "Enumeration": true, "Integer": true, "Long": true}
	for _, spelling := range []string{"Attribute", "databaseAttributeString"} {
		for attrName, kind := range attrs {
			body := fmt.Sprintf(`dataview dv (datasource: $Thing) { combobox cb1 (optionsSourceDatabaseDataSource: database from Sales.Other, `+
				`%s: %s, CaptionAttribute: Txt, optionsSourceDatabaseValueAttribute: Txt) }`, spelling, attrName)
			errs := typeErrs(t, ctx, body, nil)
			if want := !accepted[kind]; (len(errs) > 0) != want {
				t.Errorf("%s: %s (%s): flagged=%v, want %v (%q)", spelling, attrName, kind, len(errs) > 0, want, errs)
				continue
			}
			if len(errs) > 0 {
				for _, want := range []string{"MDL-WIDGET39", "CE7247", "String, Integer, Long, Enumeration"} {
					if !strings.Contains(errs[0], want) {
						t.Errorf("%s: %s: error should mention %q, got: %s", spelling, attrName, want, errs[0])
					}
				}
			}
		}
	}
}
