// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#613, the residual half of #509. A key the theme declares for SOME
// OTHER widget type passed every mxcli gate and failed the build. Measured on a
// blank 11.12.2 project (Forms$DynamicText dt and Forms$ListView lvH on one page):
//
//	alter styling on page MyFirstModule.HideOn widget dt set 'Row size' = 'Small';
//	  mxcli check --references -> Check passed!   (no MDL-WIDGET11)
//	  mxcli docker check       -> [CE6083] "Design property Row size is not
//	                              supported by your theme." at Text 'dt'
//
// Control: the same key on lvH (a ListView) is 0 errors.
//
// The stored widget's $Type is what decides, so the reference pass opens the
// document (the pageProbe capability) and resolves the key against the widget's
// OWN type via the routing helper ALTER PAGE SET already uses. These tests use a
// mock backend, which — unlike the validators' read-only modelsdk connection —
// does open a page for mutation; that difference is why the judgement lives in
// the reference pass, and only a real project run shows it.

// The pageProbe capability now carries the widget-type accessor, and the
// concrete BSON mutator is what satisfies it.
var _ pageProbe = (*pagemutator.Mutator)(nil)

func storedTextAndListPage() bson.D {
	appearance := func() bson.D {
		return bson.D{
			{Key: "$Type", Value: "Forms$Appearance"},
			{Key: "Class", Value: ""},
			{Key: "DesignProperties", Value: bson.A{int32(3)}},
			{Key: "DynamicClasses", Value: ""},
			{Key: "Style", Value: ""},
		}
	}
	dt := bson.D{
		{Key: "$Type", Value: "Forms$DynamicText"},
		{Key: "Name", Value: "dt"},
		{Key: "Appearance", Value: appearance()},
	}
	lv := bson.D{
		{Key: "$Type", Value: "Forms$ListView"},
		{Key: "Name", Value: "lvH"},
		{Key: "Appearance", Value: appearance()},
	}
	tb := bson.D{
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "Name", Value: "tb"},
		{Key: "Appearance", Value: appearance()},
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: bson.A{int32(2), dt, lv, tb}}},
			}},
		}},
	}
}

// atlas613Theme is the slice of Atlas (11.12.2 / 11.14.0) the issue turns on:
// the three `Widget` keys every widget inherits, the ListView group that
// declares `Row size`, and the DynamicText group, which does not. Atlas has no
// TextBox group.
func atlas613Theme() *ThemeRegistry {
	return &ThemeRegistry{WidgetProperties: map[string][]ThemeProperty{
		"Widget": {
			{Name: "Spacing", Type: "Spacing"},
			{Name: "Align self", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Left"}, {Name: "Right"}}},
			{Name: "Hide on", Type: "ToggleButtonGroup", MultiSelect: true,
				Options: []ThemeOption{{Name: "Phone"}, {Name: "Tablet"}}},
		},
		"ListView": {
			{Name: "Style", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Lined"}, {Name: "Striped"}}},
			{Name: "Hover style", Type: "Toggle", Class: "listview-hover"},
			{Name: "Row size", Type: "ToggleButtonGroup", Options: []ThemeOption{{Name: "Small"}, {Name: "Large"}}},
		},
		"DynamicText": {
			{Name: "Size", Type: "Dropdown", Options: []ThemeOption{{Name: "H1"}, {Name: "Small"}}},
			{Name: "Weight", Type: "Dropdown", Options: []ThemeOption{{Name: "Bold"}}},
		},
	}}
}

// storedStylingCtx is a connected context over one module M holding page P.
func storedStylingCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	pg := mkPage(mod.ID, "P")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(storedTextAndListPage(), unitID, &countingDeps{}), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

// stylingFindings runs ALTER STYLING's design-property check the way `check -p`
// does: the no-project any-type pass (ValidateProgram), then — when ctx is not
// nil — the reference pass that opens the stored document. Each finding is one
// string carrying its rule id, so a key reported by both passes shows twice.
func stylingFindings(t *testing.T, ctx *ExecContext, reg *ThemeRegistry, src string) []string {
	t.Helper()
	prog := parseMDL(t, src)
	var out []string
	for _, v := range validateAlterStylingDesignProps(prog, reg) {
		sev := "warning"
		if v.Severity != linter.SeverityWarning {
			sev = "NOT-A-WARNING"
		}
		out = append(out, fmt.Sprintf("%s: %s | %s [%s]", sev, v.Message, v.Suggestion, v.RuleID))
	}
	if ctx != nil {
		ctx.ThemeRegistry = reg
		sc := newScriptContext()
		sc.collectDefinitions(prog)
		validateAlterStylingStoredTypes(ctx, prog, sc)
		for _, w := range sc.warnings {
			// sc.warnings is the reference pass's warning channel: it never
			// blocks, which is what keeps this a warning.
			out = append(out, "warning: "+w)
		}
	}
	return out
}

// The reported case: `Row size` on a DynamicText.
func TestAlterStyling613_KeyOfAnotherWidgetTypeIsReported(t *testing.T) {
	got := stylingFindings(t, storedStylingCtx(t), atlas613Theme(),
		`alter styling on page M.P widget dt set 'Row size' = 'Small';`)
	if len(got) != 1 {
		t.Fatalf("want one MDL-WIDGET11 for a ListView key on a DynamicText, got %d: %q", len(got), got)
	}
	f := got[0]
	// Still a warning: a newer theme may add keys the snapshot lacks (#509).
	for _, w := range []string{"warning: ", "MDL-WIDGET11", `"dt"`, `"Row size"`, "DynamicText", "CE6083"} {
		if !strings.Contains(f, w) {
			t.Errorf("lacks %s:\n%s", w, f)
		}
	}
	// The keys valid for it, the three inherited from `Widget` included...
	_, valid, _ := strings.Cut(f, "Valid design properties for a DynamicText: ")
	for _, w := range []string{"Spacing", "Align self", "Hide on", "Size", "Weight"} {
		if !strings.Contains(valid, w) {
			t.Errorf("does not list valid key %q:\n%s", w, f)
		}
	}
	// ...and never the key just refused, nor another type's.
	for _, w := range []string{"Row size", "Hover style"} {
		if strings.Contains(valid, w) {
			t.Errorf("lists %q as valid on a DynamicText:\n%s", w, f)
		}
	}
}

// CONTROL: the same key on the widget that declares it — 0 errors in mx check.
func TestAlterStyling613_KeyOfTheWidgetsOwnTypeIsSilent(t *testing.T) {
	for _, stmt := range []string{
		`alter styling on page M.P widget lvH set 'Row size' = 'Small';`,
		// Inherited from `Widget` — resolving only the type's own group is the
		// mistake this area keeps producing.
		`alter styling on page M.P widget dt set 'Align self' = 'Right';`,
		`alter styling on page M.P widget dt set 'Size' = 'H1';`,
	} {
		if got := stylingFindings(t, storedStylingCtx(t), atlas613Theme(), stmt); len(got) != 0 {
			t.Errorf("%s\n  reported a key the widget's own type declares: %q", stmt, got)
		}
	}
}

// `Hide on` is a KNOWN key with a writer problem (#511), never an unknown one.
func TestAlterStyling613_HideOnIsKnownNotUnknown(t *testing.T) {
	got := stylingFindings(t, storedStylingCtx(t), atlas613Theme(),
		`alter styling on page M.P widget dt set 'Hide on' = 'Phone';`)
	if len(got) != 1 || !strings.Contains(got[0], "MDL-WIDGET12") {
		t.Fatalf("want exactly one MDL-WIDGET12 (multi-select), got %q", got)
	}
	if strings.Contains(got[0], "CE6083") {
		t.Errorf("Hide on was reported as unknown: %q", got[0])
	}
}

// A key no type declares is the any-type pass's finding, reported once — the
// reference pass must not say it again in other words.
func TestAlterStyling613_UndeclaredAnywhereIsReportedOnce(t *testing.T) {
	got := stylingFindings(t, storedStylingCtx(t), atlas613Theme(),
		`alter styling on page M.P widget dt set 'Remove empty text' = on;`)
	if len(got) != 1 || !strings.Contains(got[0], "no widget type") {
		t.Errorf("want the any-type finding exactly once, got %q", got)
	}
}

// CONTROL: with no project the check still runs, and still asks the any-type
// question of #509 — silent on a key some type declares, loud on one none does.
func TestAlterStyling613_NoProjectDegradesToAnyTypeCheck(t *testing.T) {
	if got := stylingFindings(t, nil, atlas613Theme(),
		`alter styling on page M.P widget dt set 'Row size' = 'Small';`); len(got) != 0 {
		t.Errorf("without a project the widget's type is unknowable, so a key some type declares must pass: %q", got)
	}
	got := stylingFindings(t, nil, atlas613Theme(),
		`alter styling on page M.P widget dt set 'Remove empty text' = on;`)
	if len(got) != 1 || !strings.Contains(got[0], "MDL-WIDGET11") || !strings.Contains(got[0], "no widget type") {
		t.Errorf("the #509 any-type case is no longer reported without a project: %q", got)
	}
	// A context that is not connected is the same as none.
	notConnected, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return false }}))
	if got := stylingFindings(t, notConnected, atlas613Theme(),
		`alter styling on page M.P widget dt set 'Row size' = 'Small';`); len(got) != 0 {
		t.Errorf("an unconnected context must degrade, not judge: %q", got)
	}
}

// CONTROL: a project with no themesource metadata reports nothing.
func TestAlterStyling613_NoThemeReportsNothing(t *testing.T) {
	for _, reg := range []*ThemeRegistry{nil, {WidgetProperties: map[string][]ThemeProperty{}}} {
		if got := stylingFindings(t, storedStylingCtx(t), reg,
			`alter styling on page M.P widget dt set 'Row size' = 'Small';`); len(got) != 0 {
			t.Errorf("claimed a key is undeclared with no theme to judge against: %q", got)
		}
	}
}

// Where the stored type cannot be known, the reference pass is silent rather
// than judging against a type it guessed.
func TestAlterStyling613_UnknowableStoredTypeIsSilent(t *testing.T) {
	cases := []struct{ name, src string }{
		// No such widget in the stored document: exec reports that itself.
		{"widget not stored", `alter styling on page M.P widget nope set 'Row size' = 'Small';`},
		// A type the theme declares no group for: its full key set is unknown.
		{"type without a theme group", `alter styling on page M.P widget tb set 'Row size' = 'Small';`},
		// A widget the same script puts on the page: the stored tree is not the
		// one exec will style — here `dt` becomes a list view.
		{"page the script grows", `alter page M.P { replace dt with { listview dt (datasource: database M.E) {} } };
alter styling on page M.P widget dt set 'Row size' = 'Small';`},
		{"page the script creates", `create or replace page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) { dynamictext dt (content: 'x') };
alter styling on page M.P widget dt set 'Row size' = 'Small';`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, f := range stylingFindings(t, storedStylingCtx(t), atlas613Theme(), c.src) {
				if strings.Contains(f, `"Row size"`) && strings.Contains(f, "MDL-WIDGET11") {
					t.Errorf("judged against a stored type that is not the one exec will style: %s", f)
				}
			}
		})
	}
}
