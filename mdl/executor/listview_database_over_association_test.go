// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#721 L5. A List View whose "Database" source is reached from a
// context object over an association is stored as a Forms$ListViewXPathSource
// whose EntityRef is an IndirectEntityRef — a database retrieve, with an XPath
// constraint, a sort bar and a search bar. Describe read the steps and printed
// `$Ctx/Assoc`, the spelling of a Forms$AssociationSource: an in-memory retrieve
// over the association that has none of those three. Executing the description
// re-typed the source and dropped sort and search without a word, on seven
// WorkflowCommons snippets of the Studio Pro-authored TestApp.
//
// The stored shape, from TestApp's Snip_TaskAssignment_UserTask_Assignees. The
// DestinationEntity is Administration.Account, a specialization of the
// association's own System.User end — resolving it from the association would
// write the wrong entity, so describe must print it.
func storedListViewXPathOverAssociation() map[string]any {
	return dsBSON(dsTypeListViewXPath,
		"EntityRef", map[string]any{
			"$Type": "DomainModels$IndirectEntityRef",
			"Steps": []any{map[string]any{
				"$Type":             "DomainModels$EntityRefStep",
				"Association":       "System.WorkflowUserTask_Assignees",
				"DestinationEntity": "Administration.Account",
			}},
		},
		"SourceVariable", map[string]any{"$Type": "Forms$PageVariable", "SnippetParameter": "WorkflowUserTask"},
		"XPathConstraint", "[Active = true()]",
		"SortBar", map[string]any{"SortItems": []any{map[string]any{
			"AttributeRef": map[string]any{"Attribute": "Administration.Account.FullName"},
			"SortOrder":    "Ascending",
		}}},
		"Search", map[string]any{"SearchRefs": []any{map[string]any{"Attribute": "Administration.Account.FullName"}}},
	)
}

func TestDescribeListViewDatabaseOverAssociationKeepsItsKind(t *testing.T) {
	got := dataSourceExpr(nil, parseDataSource(storedListViewXPathOverAssociation()))
	want := "database from $WorkflowUserTask/System.WorkflowUserTask_Assignees/Administration.Account" +
		" where [Active = true()] sort by FullName asc search by FullName"
	if got != want {
		t.Errorf("describe printed\n  %s\nwant\n  %s", got, want)
	}
}

// Control: a real Forms$AssociationSource with the same steps still describes as
// the association spelling — the fix is keyed on the stored $Type, not on the
// presence of steps.
func TestDescribeAssociationSourceIsUnchanged(t *testing.T) {
	ds := storedListViewXPathOverAssociation()
	ds["$Type"] = dsTypeAssociation
	if got, want := dataSourceExpr(nil, parseDataSource(ds)), "$WorkflowUserTask/System.WorkflowUserTask_Assignees"; got != want {
		t.Errorf("association source printed %q, want %q", got, want)
	}
}

// With no SourceVariable the steps start from the enclosing data container's
// object, which MDL spells $currentObject, as the association form does.
func TestDescribeListViewDatabaseOverAssociationFromCurrentObject(t *testing.T) {
	ds := storedListViewXPathOverAssociation()
	delete(ds, "SourceVariable")
	delete(ds, "XPathConstraint")
	delete(ds, "Search")
	delete(ds, "SortBar")
	got := dataSourceExpr(nil, parseDataSource(ds))
	if want := "database from $currentObject/System.WorkflowUserTask_Assignees/Administration.Account"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The description must parse back onto a database source that remembers the
// context variable and the path — otherwise exec has nothing to write the
// ListViewXPathSource from.
func TestDatabaseOverAssociationParses(t *testing.T) {
	prog, errs := visitor.Build(`create snippet W.S (Params: ($Task: System.WorkflowUserTask)) {
  listview lv (DataSource: database from $Task/System.WorkflowUserTask_Assignees/Administration.Account where [Active = true()] sort by FullName asc search by FullName) {
    dynamictext t (Content: 'x')
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var ds *ast.DataSourceV3
	for _, w := range prog.Statements[0].(*ast.CreateSnippetStmtV3).Widgets {
		if strings.EqualFold(w.Type, "listview") {
			ds = w.GetDataSource()
		}
	}
	if ds == nil {
		t.Fatal("no list view datasource")
	}
	if ds.Type != "database" || ds.ContextVariable != "Task" ||
		ds.AssociationPath != "System.WorkflowUserTask_Assignees/Administration.Account" {
		t.Errorf("got Type=%q ContextVariable=%q AssociationPath=%q Reference=%q",
			ds.Type, ds.ContextVariable, ds.AssociationPath, ds.Reference)
	}
	if ds.Where == "" || len(ds.OrderBy) != 1 || len(ds.SearchAttributes) != 1 {
		t.Errorf("where/sort/search lost: %+v", ds)
	}
}

// Control: the plain association sugar still means an association source.
func TestAssociationSugarStillParsesAsAssociation(t *testing.T) {
	prog, errs := visitor.Build(`create page W.P ( Title: 'P' ) {
  listview lv (DataSource: $currentObject/W.A_B) { dynamictext t (Content: 'x') }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if ds := firstListViewDataSource(t, prog); ds.Type != "association" {
		t.Errorf("Type = %q, want association", ds.Type)
	}
}

// A list view and the pluggable engine store the path; any other caller of
// buildDataSourceV3 (a chart series, a dynamic image) would write the entity
// alone — every row instead of the context's — so it refuses.
func TestDatabaseOverAssociationRefusedOutsideListViewAndPluggable(t *testing.T) {
	pb := pluggableAssocPageBuilder(11, 12)
	ds := &ast.DataSourceV3{Type: "database", ContextVariable: "currentObject", AssociationPath: "Probe.Item_Parent/Probe.Item"}
	if _, _, err := pb.buildDataSourceV3(ds); err == nil {
		t.Error("buildDataSourceV3 accepted `database from $currentObject/…`")
	}
	if _, _, err := pb.buildPluggableDataSourceV3(ds, ""); err != nil {
		t.Errorf("a pluggable widget was refused: %v", err)
	}
}
