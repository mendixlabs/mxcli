// SPDX-License-Identifier: Apache-2.0

// A quoted `Editable: '<expression>'` on an input widget.
//
// `Visible: '$currentObject/Locked = false'` is stored as a conditional
// visibility: pages.StaticVisibleExpression hands any string that is not true or
// false back as the expression. `Editable` took the same grammar slot but a
// different consumer, pages.CanonicalEditability, which knows only the enum words
// Always | Never | Conditional and answers "not one of mine" for the rest. The
// builder's `else if` then simply did not fire, so the widget kept the default
// Always: check clean, exec "Created page", and a field the author meant to lock
// stayed editable.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

const lockedExpr = "$currentObject/Locked = false"

// buildInputFromMDL runs the real parser and page builder over one widget.
func buildInputFromMDL(t *testing.T, widget string) pages.Widget {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n  " + widget + "\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", widget, errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	w, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		t.Fatalf("building %q: %v", widget, err)
	}
	return w
}

func baseWidgetOf(t *testing.T, w pages.Widget) *pages.BaseWidget {
	t.Helper()
	g, ok := w.(interface{ GetBaseWidget() *pages.BaseWidget })
	if !ok {
		t.Fatalf("%T has no BaseWidget", w)
	}
	return g.GetBaseWidget()
}

// storedEditability is what the writers store for a built widget: the enum and
// the condition beside it.
func storedEditability(t *testing.T, w pages.Widget) (enum, expr string) {
	t.Helper()
	bw := baseWidgetOf(t, w)
	if bw.ConditionalEditability != nil {
		expr = bw.ConditionalEditability.Expression
	}
	return pages.WidgetEditability(bw), expr
}

func storedVisibility(t *testing.T, w pages.Widget) string {
	t.Helper()
	bw := baseWidgetOf(t, w)
	if bw.ConditionalVisibility == nil {
		return ""
	}
	return bw.ConditionalVisibility.Expression
}

var editableInputs = []struct{ name, tmpl string }{
	{"checkbox", `CHECKBOX w (Label: 'A', Attribute: Mod.E.Flag, %s)`},
	{"textbox", `TEXTBOX w (Label: 'C', Attribute: Mod.E.Code, %s)`},
}

func inputWith(tmpl, prop string) string { return strings.Replace(tmpl, "%s", prop, 1) }

// The quoted spelling stores exactly what the unquoted one stores.
func TestQuotedEditableExpression_EqualsUnquotedSpelling(t *testing.T) {
	for _, in := range editableInputs {
		t.Run(in.name, func(t *testing.T) {
			quoted := buildInputFromMDL(t, inputWith(in.tmpl, `Editable: '`+lockedExpr+`'`))
			unquoted := buildInputFromMDL(t, inputWith(in.tmpl, `Editable: `+lockedExpr))

			wantEnum, wantExpr := storedEditability(t, unquoted)
			if wantEnum != "Conditional" || wantExpr != lockedExpr {
				t.Fatalf("control: the unquoted spelling stored (%q, %q), want (Conditional, %q)",
					wantEnum, wantExpr, lockedExpr)
			}
			gotEnum, gotExpr := storedEditability(t, quoted)
			if gotEnum != wantEnum || gotExpr != wantExpr {
				t.Errorf("quoted Editable stored (%q, %q), want what the unquoted spelling stores (%q, %q) — "+
					"the quoted expression was dropped and the field stays editable",
					gotEnum, gotExpr, wantEnum, wantExpr)
			}
		})
	}
}

// A string literal inside the expression is written with its quotes doubled in
// the quoted spelling and must come out as the unquoted spelling's text.
func TestQuotedEditableExpression_EscapedQuotes(t *testing.T) {
	quoted := buildInputFromMDL(t, `TEXTBOX w (Attribute: Mod.E.Code, Editable: '$currentObject/Code != ''x''')`)
	unquoted := buildInputFromMDL(t, `TEXTBOX w (Attribute: Mod.E.Code, Editable: $currentObject/Code != 'x')`)
	_, want := storedEditability(t, unquoted)
	enum, got := storedEditability(t, quoted)
	if want != "$currentObject/Code != 'x'" {
		t.Fatalf("control: unquoted stored %q", want)
	}
	if enum != "Conditional" || got != want {
		t.Errorf("quoted stored (%q, %q), want (Conditional, %q)", enum, got, want)
	}
}

// describe prints a stored condition as `Editable: <expr>`; that has to rebuild
// the same condition, and so does the quoted spelling of it.
func TestQuotedEditableExpression_DescribeRoundTrip(t *testing.T) {
	w := buildInputFromMDL(t, `CHECKBOX w (Attribute: Mod.E.Flag, Editable: '`+lockedExpr+`')`)
	_, expr := storedEditability(t, w)
	if expr == "" {
		t.Fatal("the quoted expression was not stored, so there is nothing for describe to print")
	}
	printed := widgetConditionMDL("Editable", expr)
	again := buildInputFromMDL(t, `CHECKBOX w (Attribute: Mod.E.Flag, `+printed+`)`)
	enum, got := storedEditability(t, again)
	if enum != "Conditional" || got != expr {
		t.Errorf("describe line %q rebuilt as (%q, %q), want (Conditional, %q)", printed, enum, got, expr)
	}
}

// Controls: the enum words keep mapping to the enum, in any case, quoted or not.
func TestQuotedEditable_EnumWordsStillMapToTheEnum(t *testing.T) {
	cases := []struct{ prop, want string }{
		{`Editable: 'Never'`, "Never"},
		{`Editable: 'never'`, "Never"},
		{`Editable: Never`, "Never"},
		{`Editable: 'Always'`, "Always"},
		{`Editable: 'ALWAYS'`, "Always"},
		{`Editable: Always`, "Always"},
	}
	for _, c := range cases {
		t.Run(c.prop, func(t *testing.T) {
			enum, expr := storedEditability(t, buildInputFromMDL(t, `CHECKBOX w (Attribute: Mod.E.Flag, `+c.prop+`)`))
			if enum != c.want || expr != "" {
				t.Errorf("%s stored (%q, %q), want (%q, no condition)", c.prop, enum, expr, c.want)
			}
		})
	}
}

// A boolean said what it meant and was dropped too: GetStringProp answers ""
// for a bool, so `Editable: false` on a text box left it editable. false is the
// enum's Never, true its Always.
func TestEditableBoolean_IsNotDropped(t *testing.T) {
	cases := []struct{ prop, want string }{
		{`Editable: false`, "Never"},
		{`Editable: 'false'`, "Never"},
		{`Editable: true`, "Always"},
		{`Editable: 'TRUE'`, "Always"},
	}
	for _, in := range editableInputs {
		for _, c := range cases {
			t.Run(in.name+"/"+c.prop, func(t *testing.T) {
				enum, expr := storedEditability(t, buildInputFromMDL(t, inputWith(in.tmpl, c.prop)))
				if enum != c.want || expr != "" {
					t.Errorf("%s stored (%q, %q), want (%q, no condition)", c.prop, enum, expr, c.want)
				}
			})
		}
	}
}

// Control: nothing authored, nothing stored.
func TestQuotedEditable_NothingAuthoredStoresNothing(t *testing.T) {
	enum, expr := storedEditability(t, buildInputFromMDL(t, `CHECKBOX w (Attribute: Mod.E.Flag)`))
	if enum != "Always" || expr != "" {
		t.Errorf("no Editable stored (%q, %q), want (Always, no condition)", enum, expr)
	}
}

// Visible is unchanged: its quoted expression is a conditional visibility, false
// is the constant condition "false", true stores nothing.
func TestQuotedVisible_Unchanged(t *testing.T) {
	cases := []struct{ prop, want string }{
		{`Visible: '` + lockedExpr + `'`, lockedExpr},
		{`Visible: ` + lockedExpr, lockedExpr},
		{`Visible: false`, "false"},
		{`Visible: 'false'`, "false"},
		{`Visible: true`, ""},
	}
	for _, c := range cases {
		t.Run(c.prop, func(t *testing.T) {
			w := buildInputFromMDL(t, `CHECKBOX w (Attribute: Mod.E.Flag, `+c.prop+`)`)
			if got := storedVisibility(t, w); got != c.want {
				t.Errorf("%s stored visibility %q, want %q", c.prop, got, c.want)
			}
			if enum, expr := storedEditability(t, w); enum != "Always" || expr != "" {
				t.Errorf("%s changed editability to (%q, %q)", c.prop, enum, expr)
			}
		})
	}
}

// A list view's and a grid column's Editable is a plain bool built from
// GetBoolProp (ako/mxcli#510) — a different property from the input widgets'
// editability enum. A bool there must not also be read as Never/Always on the
// widget's BaseWidget.
func TestEditableBoolean_PlainBoolTypesKeepTheirOwnProperty(t *testing.T) {
	for _, typ := range []string{"listview", "column"} {
		for _, v := range []any{true, false, "false"} {
			w := &ast.WidgetV3{Type: typ, Name: "w", Properties: map[string]any{"Editable": v}}
			lv := &pages.ListView{}
			applyConditionalSettings(lv, w)
			if got := lv.BaseWidget.Editable; got != "" {
				t.Errorf("%s with Editable: %v set the editability enum to %q", typ, v, got)
			}
			if lv.ConditionalEditability != nil {
				t.Errorf("%s with Editable: %v stored a condition %q", typ, v, lv.ConditionalEditability.Expression)
			}
		}
	}
}

// ---- check ----------------------------------------------------------------

func checkMessages(t *testing.T, widget, rule string) []string {
	t.Helper()
	src := "mdl 1;\ncreate page Mod.P (Title: 'T', Layout: Atlas_Core.Atlas_Default, Params: ($E: Mod.E)) {\n" +
		"  dataview dvE (DataSource: $E) {\n    " + widget + "\n  }\n};"
	prog := parseMDL(t, src)
	var out []string
	for _, v := range ValidateWidgetProperties(prog, "") {
		if v.RuleID == rule {
			out = append(out, v.Message+" | "+v.Suggestion)
		}
	}
	for _, v := range ValidatePageButtonContext(prog) {
		if v.RuleID == rule {
			out = append(out, v.Message+" | "+v.Suggestion)
		}
	}
	return out
}

// check refuses what exec cannot store, naming the accepted spellings, and
// stays quiet on what it can.
func TestMDLWIDGET19_EditableValues(t *testing.T) {
	cases := []struct {
		name   string
		prop   string
		refuse bool
	}{
		{"quoted expression", `Editable: '` + lockedExpr + `'`, false},
		{"quoted expression with a literal", `Editable: '$currentObject/Code != ''x'''`, false},
		{"unquoted expression", `Editable: ` + lockedExpr, false},
		{"Never", `Editable: Never`, false},
		{"quoted Always", `Editable: 'Always'`, false},
		{"false", `Editable: false`, false},
		// Strings that are neither an enum word nor an expression the grammar reads.
		{"misspelt enum word", `Editable: Neever`, true},
		{"misspelt enum word, quoted", `Editable: 'ReadOnly'`, true},
		{"malformed quoted expression", `Editable: '$currentObject/Locked = = false'`, true},
		{"Conditional without a condition", `Editable: Conditional`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkMessages(t, `CHECKBOX w (Attribute: Mod.E.Flag, `+c.prop+`)`, "MDL-WIDGET19")
			if !c.refuse {
				if len(got) != 0 {
					t.Fatalf("MDL-WIDGET19 refused a value exec stores: %v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want one MDL-WIDGET19, got %v", got)
			}
			for _, want := range []string{"Always", "Never"} {
				if !strings.Contains(got[0], want) {
					t.Errorf("message should name the accepted spellings (%s missing): %s", want, got[0])
				}
			}
		})
	}
}

// A quoted Editable expression gets the expression checks the unquoted one gets.
func TestQuotedEditableExpression_GetsTheExpressionChecks(t *testing.T) {
	const assoc = `$currentObject/Mod.E_E/Code = 'x'`
	unquoted := checkMessages(t, `CHECKBOX w (Attribute: Mod.E.Flag, Editable: `+assoc+`)`, "MDL-WIDGET13")
	if len(unquoted) != 1 {
		t.Fatalf("control: the unquoted spelling should earn one MDL-WIDGET13, got %v", unquoted)
	}
	quoted := checkMessages(t, `CHECKBOX w (Attribute: Mod.E.Flag, Editable: '$currentObject/Mod.E_E/Code = ''x''')`, "MDL-WIDGET13")
	if len(quoted) != 1 {
		t.Errorf("the quoted spelling of the same expression earned %d MDL-WIDGET13, want 1: %v", len(quoted), quoted)
	}

	// The widget's own data container is not a variable in its own context.
	unquoted = checkMessages(t, `CHECKBOX w (Attribute: Mod.E.Flag, Editable: $dvE/Locked = false)`, "MDL-BUTTON02")
	if len(unquoted) != 1 {
		t.Fatalf("control: the unquoted spelling should earn one MDL-BUTTON02, got %v", unquoted)
	}
	quoted = checkMessages(t, `CHECKBOX w (Attribute: Mod.E.Flag, Editable: '$dvE/Locked = false')`, "MDL-BUTTON02")
	if len(quoted) != 1 {
		t.Errorf("the quoted spelling of the same expression earned %d MDL-BUTTON02, want 1: %v", len(quoted), quoted)
	}
}

// A button has no editability, quoted or not (MDL-WIDGET20, #928).
func TestMDLWIDGET20_QuotedExpressionOnAButton(t *testing.T) {
	got := widgetViolations(t, buttonPage(`editable: '`+lockedExpr+`'`), "MDL-WIDGET20")
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET20 violations for a quoted expression on a button, want 1", len(got))
	}
}

// On a list view and a grid column `Editable: true` is the plain property, and
// a quoted expression is the part that is genuinely dropped.
func TestMDLWIDGET20_QuotedExpressionOnPlainEditableTypes(t *testing.T) {
	src := `create page W.P ( Title: 'P' )
{
  LISTVIEW lv (DataSource: DATABASE W.Product, Editable: '` + lockedExpr + `') {
    DYNAMICTEXT dt ( Content: 'x' )
  }
}`
	if got := widgetViolations(t, src, "MDL-WIDGET20"); len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET20 violations for a quoted expression on a list view, want 1", len(got))
	}
}

// A pluggable widget that does not declare Editability has no place to store a
// condition; the quoted spelling is refused like the unquoted one.
func TestApplyPluggableSystemSettings_QuotedExpression(t *testing.T) {
	cases := []struct {
		name     string
		declared []string
		wantErr  bool
	}{
		{"declared", []string{"Editability"}, false},
		{"undeclared", []string{"Visibility"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cw := &pages.CustomWidget{RawType: typeDeclaring(c.declared...), Editable: "Always"}
			w := &ast.WidgetV3{Type: "switch", Name: "w", Properties: map[string]any{"Editable": lockedExpr}}
			err := applyPluggableSystemSettings(cw, w)
			if c.wantErr != (err != nil) {
				t.Fatalf("err = %v, want error: %v", err, c.wantErr)
			}
			if !c.wantErr {
				if cw.Editable != "Conditional" || cw.ConditionalEditability == nil ||
					cw.ConditionalEditability.Expression != lockedExpr {
					t.Errorf("stored (%q, %v), want (Conditional, %q)", cw.Editable, cw.ConditionalEditability, lockedExpr)
				}
			}
		})
	}
}

// A pluggable widget that declares Editability takes the same values, and is
// refused the same way for the ones that cannot be stored.
func TestMDLWIDGET19_PluggableWidgetEditableValues(t *testing.T) {
	cases := []struct {
		prop   string
		refuse bool
	}{
		{`Editable: '` + lockedExpr + `'`, false},
		{`Editable: Never`, false},
		{`Editable: false`, false},
		{`Editable: 'Neever'`, true},
		{`Editable: '$currentObject/Locked = = false'`, true},
		{`Editable: Conditional`, true},
	}
	for _, c := range cases {
		t.Run(c.prop, func(t *testing.T) {
			got := checkMessages(t, `combobox c (Attribute: Status, `+c.prop+`)`, "MDL-WIDGET19")
			if c.refuse != (len(got) == 1) || len(got) > 1 {
				t.Errorf("%s: want refused=%v, got %v", c.prop, c.refuse, got)
			}
		})
	}
}

// A data view stores its static editability from its own ReadOnly flag, not from
// BaseWidget.Editable, so only the condition is carried: the quoted spelling must
// store the same ConditionalEditabilitySettings the bare one does.
func TestQuotedEditableExpression_DataViewMatchesBare(t *testing.T) {
	build := func(props map[string]any) *pages.DataView {
		dv := &pages.DataView{}
		applyConditionalSettings(dv, &ast.WidgetV3{Type: "dataview", Name: "dv", Properties: props})
		return dv
	}
	bare := build(map[string]any{"EditableIf": lockedExpr})
	quoted := build(map[string]any{"Editable": lockedExpr})
	if bare.ConditionalEditability == nil || quoted.ConditionalEditability == nil ||
		bare.ConditionalEditability.Expression != quoted.ConditionalEditability.Expression {
		t.Errorf("bare stored %v, quoted stored %v", bare.ConditionalEditability, quoted.ConditionalEditability)
	}
}
