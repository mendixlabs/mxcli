// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1932 (mendixlabs/mxcli#1730): a Delete button in a Data grid 2
// control bar. Studio Pro stores Forms$DeleteClientAction with SourceVariable =
// Forms$PageVariable{Widget: "grid1"} — the grid's selection. describe printed
// it as plain `Action: delete` and exec wrote SourceVariable null, which mx check
// reports as CE7005 "No value selection has been made"; check --references
// passed. The spelling is `delete $grid1`, as `$grid1` already names the grid's
// selection in a flow argument (#1560).

func TestDeleteAction_DescribeReadsSelectionSource(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	for _, tc := range []struct {
		name   string
		stored map[string]any
		want   string
	}{
		{"grid selection", map[string]any{
			"$Type": "Forms$DeleteClientAction", "ClosePage": false, "SourceVariable": gridSelection("grid1"),
		}, "delete $grid1"},
		{"grid selection, close page", map[string]any{
			"$Type": "Forms$DeleteClientAction", "ClosePage": true, "SourceVariable": gridSelection("grid1"),
		}, "delete $grid1 close page"},
		// Control: the context object (a data view's) stores no source.
		{"no source (control)", map[string]any{
			"$Type": "Forms$DeleteClientAction", "ClosePage": false, "SourceVariable": nil,
		}, "delete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderClientActionMDL(ctx, tc.stored); got != tc.want {
				t.Errorf("described %q, want %q", got, tc.want)
			}
		})
	}
}

// buildDeleteFrom parses `Action: <mdl>` on a button and builds it with a page
// builder that knows grid1 as a widget of the document and Order as a page
// parameter.
func buildDeleteFrom(t *testing.T, mdl string, argCtx pageArgContext) (pages.ClientAction, error) {
	t.Helper()
	prog, errs := visitor.Build("create page M.P (Title: 'T', Layout: A.L) {\n" +
		"  actionbutton b1 (Caption: 'Delete', Action: " + mdl + ")\n}")
	if len(errs) > 0 {
		t.Fatalf("%q does not parse: %v", mdl, errs)
	}
	a := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Properties["Action"].(*ast.ActionV3)
	pb := &pageBuilder{
		argCtx:           argCtx,
		paramScope:       map[string]model.ID{"Order": ""},
		isDocumentWidget: func(name string) bool { return name == "grid1" },
	}
	return pb.buildClientActionV3(a)
}

func TestDeleteAction_SelectionSourceRoundTrips(t *testing.T) {
	inControlBar := enteringDataWidget(&ast.DataSourceV3{Type: "database", Reference: "M.Order"}, "M.Order").inDataWidget("grid1")
	inControlBar.selectionOf = "grid1"
	for _, tc := range []struct {
		mdl       string
		closePage bool
		want      *pages.WidgetVariable
	}{
		{"delete $grid1", false, &pages.WidgetVariable{Widget: "grid1"}},
		{"delete $grid1 close page", true, &pages.WidgetVariable{Widget: "grid1"}},
		{"delete $Order", false, &pages.WidgetVariable{Variable: "Order", Kind: "parameter"}},
	} {
		t.Run(tc.mdl, func(t *testing.T) {
			built, err := buildDeleteFrom(t, tc.mdl, inControlBar)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			d, ok := built.(*pages.DeleteClientAction)
			if !ok {
				t.Fatalf("built %T, want *pages.DeleteClientAction", built)
			}
			if d.SourceVariable == nil || *d.SourceVariable != *tc.want {
				t.Errorf("SourceVariable = %+v, want %+v", d.SourceVariable, tc.want)
			}
			if d.ClosePage != tc.closePage {
				t.Errorf("ClosePage = %v, want %v", d.ClosePage, tc.closePage)
			}
		})
	}
}

// Control: inside a data view the context object is the source, stored as no
// SourceVariable — exactly what was written before.
func TestDeleteAction_DataViewDeleteHasNoSource(t *testing.T) {
	built, err := buildDeleteFrom(t, "delete", enteringDataWidget(&ast.DataSourceV3{Type: "parameter", Reference: "$Order"}, "M.Order"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if d := built.(*pages.DeleteClientAction); d.SourceVariable != nil {
		t.Errorf("SourceVariable = %+v, want nil (the data view's object)", d.SourceVariable)
	}
}

func TestDeleteAction_UnknownSourceRefused(t *testing.T) {
	_, err := buildDeleteFrom(t, "delete $nope", atDocumentRoot())
	if err == nil || !strings.Contains(err.Error(), "$nope") {
		t.Fatalf("err = %v, want a refusal naming $nope", err)
	}
}

// A bare `delete` with no object to delete is the CE7005 button the issue
// reports. Refused at exec in a control bar and at a document's root; ALTER
// PAGE, which cannot see what encloses the button, stands down.
func TestDeleteAction_BareDeleteWithoutObjectRefused(t *testing.T) {
	inControlBar := enteringDataWidget(&ast.DataSourceV3{Type: "database", Reference: "M.Order"}, "M.Order").inDataWidget("grid1")
	inControlBar.selectionOf = "grid1"
	if _, err := buildDeleteFrom(t, "delete", inControlBar); err == nil || !strings.Contains(err.Error(), "delete $grid1") {
		t.Errorf("control bar: err = %v, want a refusal suggesting `delete $grid1`", err)
	}
	if _, err := buildDeleteFrom(t, "delete", atDocumentRoot()); err == nil || !strings.Contains(err.Error(), "CE7005") {
		t.Errorf("document root: err = %v, want a refusal naming CE7005", err)
	}
	if _, err := buildDeleteFrom(t, "delete", pageArgContext{}); err != nil {
		t.Errorf("unknown context (ALTER PAGE): err = %v, want none", err)
	}
}

// The check-time mirror: `mxcli check` reports the CE7005 button without a
// project.
func TestDeleteAction_CheckReportsBareDeleteInControlBar(t *testing.T) {
	violations := func(button string) []string {
		prog, errs := visitor.Build(`create page M.P (Title: 'T', Layout: A.L) {
  datagrid grid1 (DataSource: database from M.Order) {
    controlbar {
      actionbutton btnDelete (Caption: 'Delete', Action: ` + button + `)
    }
    column (attribute: Name, caption: 'Name')
  }
  dataview dv (DataSource: database from M.Order) {
    actionbutton btnDvDelete (Caption: 'Delete', Action: delete)
  }
}`)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		var out []string
		for _, v := range validateWidgetPropertiesOf(prog.Statements[0], LoadWidgetRegistry("")) {
			if v.RuleID == "MDL-PAGEDEL01" {
				out = append(out, v.Message)
			}
		}
		return out
	}
	if got := violations("delete"); len(got) != 1 || !strings.Contains(got[0], "btnDelete") {
		t.Errorf("bare delete in the control bar: got %v, want one MDL-PAGEDEL01 on btnDelete", got)
	}
	// Control: the selection spelled out, and the data view's button, are clean.
	if got := violations("delete $grid1"); len(got) != 0 {
		t.Errorf("delete $grid1: got %v, want none", got)
	}
}
