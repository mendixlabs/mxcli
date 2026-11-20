// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// gallerySortBar is the Forms$GridSortBar Studio Pro stores on a pluggable
// widget's XPath source: one ascending item on Probe.Item.Code.
func gallerySortBar() map[string]any {
	return map[string]any{
		"$Type": "Forms$GridSortBar",
		"SortItems": []any{int32(2), map[string]any{
			"$Type":         "Forms$GridSortItem",
			"AttributeRef":  map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": "Probe.Item.Code"},
			"SortDirection": "Ascending",
		}},
	}
}

// mendixlabs/mxcli#1599: a gallery whose "Database" source runs over an
// association from its context object is stored as a
// CustomWidgets$CustomWidgetXPathSource with an IndirectEntityRef and a sort
// bar. Describe read it as the association source `$currentObject/Assoc`,
// which drops the sort (and the XPath) and re-executes as a
// Forms$AssociationSource — a different retrieve.
func TestParseDataSource_PluggableXPathOverAssociationKeepsSort(t *testing.T) {
	ds := parseDataSource(map[string]any{
		"$Type":           dsTypeCustomWidgetXPath,
		"EntityRef":       indirectRef(assocStep("Probe.Item_Parent", "Probe.Item")),
		"XPathConstraint": "",
		"SortBar":         gallerySortBar(),
	})
	if ds == nil {
		t.Fatal("datasource dropped")
	}
	want := "database from $currentObject/Probe.Item_Parent/Probe.Item sort by Code asc"
	if got := dataSourceExpr(nil, ds); got != want {
		t.Errorf("dataSourceExpr = %q, want %q", got, want)
	}
	if got := dataSourceEntityContext(nil, ds); got != "Probe.Item" {
		t.Errorf("dataSourceEntityContext = %q, want Probe.Item", got)
	}
}

// The XPath constraint rides along as well, and a page-parameter start is kept.
func TestParseDataSource_PluggableXPathOverAssociationKeepsWhereAndParameter(t *testing.T) {
	ds := parseDataSource(map[string]any{
		"$Type":           dsTypeCustomWidgetXPath,
		"EntityRef":       indirectRef(assocStep("Probe.Item_Parent", "Probe.Item")),
		"SourceVariable":  pageVariable("Parent"),
		"XPathConstraint": "[Amount > 3]",
	})
	got := dataSourceExpr(nil, ds)
	want := "database from $Parent/Probe.Item_Parent/Probe.Item where [Amount > 3]"
	if got != want {
		t.Errorf("dataSourceExpr = %q, want %q", got, want)
	}
}

// Control for the change above: a hop with no stored destination cannot be
// spelled as a database path, and keeps the association reading (#941).
func TestParseDataSource_PluggableXPathWithoutDestinationStaysAssociation(t *testing.T) {
	ds := parseDataSource(dsBSON(dsTypeCustomWidgetXPath,
		"EntityRef", map[string]any{"Steps": []any{map[string]any{"Association": "Mod.Item_Bucket"}}},
	))
	if ds == nil || ds.Type != "association" {
		t.Fatalf("parseDataSource = %+v, want an association source", ds)
	}
}

// Write half: on 10+ a pluggable widget builds `database from $ctx/Assoc/Entity`
// as a database source carrying the hops, its sort and the start variable, for
// customWidgetDataSourceToGen to store as a CustomWidgetXPathSource.
func TestBuildPluggableDataSource_DatabaseOverAssociationOn10(t *testing.T) {
	pb := pluggableAssocPageBuilder(11, 12)
	prog, errs := visitor.Build(`create page Probe.P ( Title: 'P' ) {
  gallery g (DataSource: database from $currentObject/Probe.Item_Parent/Probe.Item sort by Code asc) { dynamictext t (Content: 'x') }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	built, entity, err := pb.buildPluggableDataSourceV3(w.GetDataSource(), "")
	if err != nil {
		t.Fatalf("buildPluggableDataSourceV3: %v", err)
	}
	if entity != "Probe.Item" {
		t.Errorf("entity = %q, want Probe.Item", entity)
	}
	src, ok := built.(*pages.DatabaseSource)
	if !ok {
		t.Fatalf("built %T, want *pages.DatabaseSource", built)
	}
	if src.UseConstraints {
		t.Error("10+ got the 9.x constraint-row source")
	}
	if len(src.EntitySteps) != 1 || src.EntitySteps[0].Association != "Probe.Item_Parent" || src.EntitySteps[0].DestinationEntity != "Probe.Item" {
		t.Errorf("EntitySteps = %+v, want one hop Probe.Item_Parent → Probe.Item", src.EntitySteps)
	}
	if len(src.Sorting) != 1 || !strings.HasSuffix(src.Sorting[0].AttributePath, "Code") || src.Sorting[0].Direction != pages.SortDirectionAscending {
		t.Errorf("Sorting = %+v, want Code ascending", src.Sorting)
	}
}

// pluggableAssocPageBuilder is a page builder at the given Mendix version over
// the issue's domain model: Probe.Item_Parent from Probe.Item to Probe.Parent,
// with the context object a Probe.Parent (the enclosing list view's).
func pluggableAssocPageBuilder(major, minor int) *pageBuilder {
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-probe")}, Name: "Probe"}
	b := &mock.MockBackend{
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor}
		},
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-probe")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-parent")}, Name: "Parent",
						Attributes: []*domainmodel.Attribute{{Name: "Name"}}},
					{BaseElement: model.BaseElement{ID: model.ID("e-item")}, Name: "Item",
						Attributes: []*domainmodel.Attribute{{Name: "Code"}, {Name: "Amount"}}},
				},
				Associations: []*domainmodel.Association{{
					BaseElement: model.BaseElement{ID: model.ID("a-item-parent")},
					Name:        "Item_Parent",
					ParentID:    model.ID("e-item"),
					ChildID:     model.ID("e-parent"),
				}},
			}}, nil
		},
	}
	return &pageBuilder{
		backend:          b,
		entityContext:    "Probe.Parent",
		paramScope:       map[string]model.ID{},
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
}
