// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ---------------------------------------------------------------------------
// A stored page carrying one pluggable widget
// ---------------------------------------------------------------------------

// storedGridPage builds the raw document of a page holding a pluggable widget
// named dgProducts with the template key `pageSize` — the shape #1069 was about,
// and the one a `SET` has to be resolved against, since no widget definition on
// disk can say what THIS project's installed grid declares.
func storedGridPage() bson.D {
	typeID := bson.Binary{Subtype: 0x04, Data: []byte{
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00,
	}}
	grid := bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "dgProducts"},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{
						{Key: "$ID", Value: typeID},
						{Key: "PropertyKey", Value: "pageSize"},
					},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "TypePointer", Value: typeID},
					{Key: "Value", Value: bson.D{{Key: "PrimitiveValue", Value: "20"}}},
				},
			}},
		}},
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: bson.A{int32(2), grid}}},
			}},
		}},
	}
}

// countingDeps records the writes a validation pass must not make.
type countingDeps struct{ saves int }

func (d *countingDeps) SerializeWidget(pages.Widget) bson.D                 { return nil }
func (d *countingDeps) SerializeClientAction(pages.ClientAction) bson.D     { return nil }
func (*countingDeps) SerializeFormDataSource(pages.DataSource, bool) bson.D { return nil }
func (d *countingDeps) SerializeClientTemplate() bson.D                     { return nil }

func (d *countingDeps) SerializeCustomWidgetDataSource(pages.DataSource) bson.D {
	return nil
}
func (d *countingDeps) BuildDataGrid2Column(*backend.DataGridColumnSpec, string, map[string]pages.PropertyTypeIDEntry) (bson.D, error) {
	return nil, nil
}
func (d *countingDeps) SerializeParameter(backend.ContainerKind, backend.PageParameterSpec) (bson.D, error) {
	return nil, nil
}
func (d *countingDeps) SaveUnit(string, []byte) error { d.saves++; return nil }

// gridPageCtx wires a project holding exactly one page, MyModule.P_Grid, whose
// stored document is storedGridPage().
func gridPageCtx(t *testing.T) (*ExecContext, *countingDeps, *int) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "P_Grid")
	deps := &countingDeps{}
	opens := 0
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			opens++
			return pagemutator.New(storedGridPage(), unitID, deps), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, deps, &opens
}

// parseMDL builds a program from real MDL, so the test exercises the AST the
// visitor actually produces rather than one written to suit the validator.
func parseMDL(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	if prog == nil {
		t.Fatalf("parse %q: nil program", src)
	}
	return prog
}

func checkAlterSet(t *testing.T, ctx *ExecContext, src string) []error {
	t.Helper()
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	return validateAlterSetProperties(ctx, prog, sc)
}

// ---------------------------------------------------------------------------
// The gap
// ---------------------------------------------------------------------------

// TestAlterSet_UnknownPluggableProperty is the regression test for the reported
// gap: measured on a real 11.13.0 project, `set NoSuchProperty = 10 on
// dgProducts` passed `check -p --references` and was then refused by `exec` with
// `pluggable property "NoSuchProperty" not found`.
func TestAlterSet_UnknownPluggableProperty(t *testing.T) {
	ctx, deps, _ := gridPageCtx(t)

	errs := checkAlterSet(t, ctx, `alter page MyModule.P_Grid { set NoSuchProperty = 10 on dgProducts; }`)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	for _, want := range []string{"MyModule.P_Grid", "NoSuchProperty", "dgProducts"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %q", msg, want)
		}
	}
	// The hint names what the STORED widget declares, so the author is not left
	// guessing a spelling — the mistake #1069 was made of.
	if !strings.Contains(msg, "pageSize") {
		t.Errorf("error %q does not name the widget's own property keys", msg)
	}
	if deps.saves != 0 {
		t.Errorf("validation wrote to storage %d times, want 0", deps.saves)
	}
}

// TestAlterSet_UnknownWidget — the same inversion one level up. Both spellings
// of the mistake used to pass check.
func TestAlterSet_UnknownWidget(t *testing.T) {
	ctx, _, _ := gridPageCtx(t)

	errs := checkAlterSet(t, ctx, `alter page MyModule.P_Grid { set pageSize = 12 on noSuchWidget; }`)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "noSuchWidget") {
		t.Errorf("error %q does not name the widget", errs[0])
	}
}

// TestAlterSet_ReportsEveryBadProperty — a statement setting several properties
// reports all the bad ones, not just the first. exec stops at the first, which
// is right for a run and wrong for a pre-flight: the point of checking is to
// hand back the whole list once.
func TestAlterSet_ReportsEveryBadProperty(t *testing.T) {
	ctx, _, _ := gridPageCtx(t)

	errs := checkAlterSet(t, ctx, `alter page MyModule.P_Grid {
		set NoSuchProperty = 10 on dgProducts;
		set AlsoMissing = 11 on dgProducts;
	}`)
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(errs), errs)
	}
}

// ---------------------------------------------------------------------------
// Controls — the ways this could block a script that works
// ---------------------------------------------------------------------------

// TestAlterSet_ValidPropertyInAnyCasing is the control that matters most for
// this area: #1069 was a case-sensitive resolver, and a check that reintroduced
// one would refuse the spelling DESCRIBE PAGE prints.
func TestAlterSet_ValidPropertyInAnyCasing(t *testing.T) {
	for _, spelling := range []string{"pageSize", "PageSize", "pagesize", "PAGESIZE"} {
		t.Run(spelling, func(t *testing.T) {
			ctx, _, _ := gridPageCtx(t)
			errs := checkAlterSet(t, ctx,
				`alter page MyModule.P_Grid { set `+spelling+` = 12 on dgProducts; }`)
			if len(errs) != 0 {
				t.Fatalf("valid property %s reported: %v", spelling, errs)
			}
		})
	}
}

// TestAlterSet_WidgetTheScriptInserts — a widget an INSERT in the same script
// puts on the page is not in the stored document, and must not be reported as
// missing.
func TestAlterSet_WidgetTheScriptInserts(t *testing.T) {
	ctx, _, _ := gridPageCtx(t)

	errs := checkAlterSet(t, ctx, `alter page MyModule.P_Grid {
		insert into dgProducts { container cNew }
		set class = 'x' on cNew;
	}`)
	if len(errs) != 0 {
		t.Fatalf("a widget the script inserts was reported: %v", errs)
	}
}

// TestAlterSet_PageTheScriptCreates — nothing is stored to dry-run against, and
// the widgets the CREATE carries are checked by ValidateWidgetProperties. The
// document must not even be opened.
func TestAlterSet_PageTheScriptCreates(t *testing.T) {
	ctx, _, opens := gridPageCtx(t)

	errs := checkAlterSet(t, ctx, `create page MyModule.P_New (Title: 'New') { container c1 { } }
		alter page MyModule.P_New { set NoSuchProperty = 10 on c1; }`)
	if len(errs) != 0 {
		t.Fatalf("a page the script creates was reported: %v", errs)
	}
	if *opens != 0 {
		t.Errorf("opened %d documents for a page that is not stored, want 0", *opens)
	}
}

// TestAlterSet_MissingPageIsLeftToTheTargetCheck — validateAlterTarget already
// reports a page that does not exist, in its own wording. Reporting it again
// here reads as two problems.
func TestAlterSet_MissingPageIsLeftToTheTargetCheck(t *testing.T) {
	ctx, _, _ := gridPageCtx(t)

	errs := checkAlterSet(t, ctx, `alter page MyModule.P_Missing { set pageSize = 12 on dgProducts; }`)
	if len(errs) != 0 {
		t.Fatalf("a missing page was reported by this pass: %v", errs)
	}
}

// TestAlterSet_BackendWithoutProbeIsLeftAlone — a backend whose mutator resolves
// properties differently (the MCP one has no pluggable path at all) is left
// unchecked rather than wrongly checked.
func TestAlterSet_BackendWithoutProbeIsLeftAlone(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "P_Grid")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{}, nil // no Probe method
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	if errs := checkAlterSet(t, ctx,
		`alter page MyModule.P_Grid { set NoSuchProperty = 10 on dgProducts; }`); len(errs) != 0 {
		t.Fatalf("a backend without the dry-run capability was checked anyway: %v", errs)
	}
}

// TestAlterSet_NotConnected — with no project there is no document, and this
// tier does not run at all.
func TestAlterSet_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return false }}
	ctx, _ := newMockCtx(t, withBackend(mb))

	if errs := checkAlterSet(t, ctx,
		`alter page MyModule.P_Grid { set NoSuchProperty = 10 on dgProducts; }`); len(errs) != 0 {
		t.Fatalf("ran without a project: %v", errs)
	}
}

// ---------------------------------------------------------------------------
// upstream #1032 — a destructive SET must be refused by check, not just by exec
// ---------------------------------------------------------------------------

// storedDataViewPage builds a page holding one DataView on a microflow source —
// the starting point #1032 reproduces from.
func storedDataViewPage() bson.D {
	dv := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dvCust"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$MicroflowSource"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Microflow", Value: "MyModule.ACT_GetCustomer"},
			}},
		}},
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: bson.A{int32(2), dv}}},
			}},
		}},
	}
}

func dataViewPageCtx(t *testing.T) (*ExecContext, *countingDeps) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "P_View")
	deps := &countingDeps{}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(storedDataViewPage(), unitID, deps), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, deps
}

// TestAlterSet_DatabaseDataSourceOnDataView — the reported statement. It used
// to pass check AND exec ("Altered page …", exit 0) while leaving the DataView
// with a Forms$DataViewSource whose SourceVariable was null: DESCRIBE showed no
// datasource and mxbuild reported CE7007 on a page mxcli had called successful.
//
// The dry run is what makes check agree with exec here — one refusal in the
// mutator, reached by both, rather than a second copy of the rule in the
// validator.
func TestAlterSet_DatabaseDataSourceOnDataView(t *testing.T) {
	ctx, deps := dataViewPageCtx(t)

	errs := checkAlterSet(t, ctx,
		`alter page MyModule.P_View { set DataSource = DATABASE MyModule.Customer on dvCust; }`)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	msg := strings.ToLower(errs[0].Error())
	for _, want := range []string{"data view", "database", "dvcust"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", errs[0], want)
		}
	}
	if deps.saves != 0 {
		t.Errorf("validation wrote to storage %d times, want 0", deps.saves)
	}
}

// Control: the sources a DataView really takes must still pass check, or the
// refusal has simply moved the failure rather than fixing it.
func TestAlterSet_ValidDataViewSourcesStillPass(t *testing.T) {
	for _, src := range []string{
		`set DataSource = MICROFLOW MyModule.ACT_GetCustomer on dvCust;`,
		`set DataSource = SELECTION lvOther on dvCust;`,
	} {
		t.Run(src, func(t *testing.T) {
			ctx, _ := dataViewPageCtx(t)
			errs := checkAlterSet(t, ctx, `alter page MyModule.P_View { `+src+` }`)
			if len(errs) != 0 {
				t.Fatalf("valid datasource reported: %v", errs)
			}
		})
	}
}
