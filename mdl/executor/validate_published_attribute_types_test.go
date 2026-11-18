// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// CE5016 "Attribute X has type T, but is published as Y." Measured on MxBuild
// 11.16.1: changing a published attribute's type to another EDM type with
// `alter entity … modify attribute` leaves the published type stale, and `mx
// check` reports one CE5016 per attribute (String to Integer, Decimal to
// Integer, Boolean to String, DateTime to String, String to Decimal); `check`
// and `exec` accepted all of them. String(50) to String(80) is clean. The
// recovery path is `create or modify published odata service` AFTER the alter,
// which re-derives the types (0 errors); BEFORE the alter it is CE5016 again.
const pubTypesHead = `mdl 1;
create module Pt;
create persistent entity Pt.E (
  Code: integer unique error message 'u' required error message 'r',
  Label: string(50),
  Amount: decimal,
  Flag: boolean,
  Stamp: datetime
);
`

const pubTypesService = `create or modify published odata service Pt.Svc (
  path: 'odata/pt/', version: '1.0.0', ODataVersion: OData4, namespace: 'Pt.S', ServiceName: 'Svc',
  Authentication: (basic, session)
)
{
  publish entity Pt.E as 'Items' (ReadMode: source, InsertMode: not_supported, UpdateMode: not_supported, DeleteMode: not_supported)
  expose (Code as 'id' (KEY), Label, Amount, Flag, Stamp);
};
`

func pubTypeHits(t *testing.T, src string, open func() backend.FullBackend) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	var out []string
	for _, v := range ValidatePublishedAttributeTypes(prog, open) {
		if v.RuleID != publishedAttrTypeRule {
			t.Fatalf("unexpected rule %s", v.RuleID)
		}
		out = append(out, v.Message)
	}
	return out
}

func TestValidatePublishedAttributeTypes_Script(t *testing.T) {
	alter := func(attr, typ string) string {
		return "alter entity Pt.E modify attribute " + attr + ": " + typ + ";\n"
	}
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"string to integer after publishing", pubTypesHead + pubTypesService + alter("Label", "integer"), 1},
		{"decimal to integer", pubTypesHead + pubTypesService + alter("Amount", "integer"), 1},
		{"boolean to string", pubTypesHead + pubTypesService + alter("Flag", "string(10)"), 1},
		{"datetime to string", pubTypesHead + pubTypesService + alter("Stamp", "string(30)"), 1},
		{"string to decimal", pubTypesHead + pubTypesService + alter("Label", "decimal"), 1},
		{"same type, longer string (control)", pubTypesHead + pubTypesService + alter("Label", "string(80)"), 0},
		{"integer to long is Int64 both (control)", pubTypesHead + pubTypesService + alter("Code", "long"), 0},
		{"alter before publishing (control)", pubTypesHead + alter("Label", "integer") + pubTypesService, 0},
		{"alter then re-publish with create or modify (control, measured clean)",
			pubTypesHead + pubTypesService + alter("Label", "integer") + pubTypesService, 0},
		{"re-publish then alter (measured CE5016)",
			pubTypesHead + alter("Label", "integer") + pubTypesService + alter("Label", "string(20)"), 1},
		{"an attribute the service does not expose (control)",
			pubTypesHead + strings.Replace(pubTypesService, ", Flag, Stamp", "", 1) + alter("Flag", "string(10)"), 0},
		{"a service dropped before the alter (control)",
			pubTypesHead + pubTypesService + "drop odata service Pt.Svc;\n" + alter("Label", "integer"), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hits := pubTypeHits(t, c.src, nil)
			if len(hits) != c.want {
				t.Fatalf("violations=%v, want %d", hits, c.want)
			}
			if c.want > 0 && (!strings.Contains(hits[0], "CE5016") || !strings.Contains(hits[0], "Pt.Svc")) {
				t.Errorf("message should name CE5016 and the service: %s", hits[0])
			}
		})
	}
}

// The service is stored in the project, not created by the script: its published
// type is what the project says.
func TestValidatePublishedAttributeTypes_ProjectService(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	ent := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e1"}, ContainerID: "dm-pj", Name: "E"}
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: "dm-pj"}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{ent}}
	svc := &model.PublishedODataService{
		BaseElement: model.BaseElement{ID: "svc"}, ContainerID: mod.ID, Name: "Svc",
		EntityTypes: []*model.PublishedEntityType{{
			Entity: "Pj.E",
			Members: []*model.PublishedMember{
				{Kind: "attribute", Name: "Pj.E.Label", EdmType: "Edm.String"},
				{Kind: "attribute", Name: "Pj.E.Count", EdmType: "Edm.Int64"},
				{Kind: "association", Name: "Pj.E.Link"},
			},
		}},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:                func() bool { return true },
		ListModulesFunc:                func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc:           func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetModuleByNameFunc:            func(string) (*model.Module, error) { return mod, nil },
		GetDomainModelFunc:             func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) { return []*model.PublishedODataService{svc}, nil },
	}
	open := func() backend.FullBackend { return mb }
	alter := func(a string) string { return "mdl 1;\nalter entity Pj.E modify attribute " + a + ";\n" }

	if hits := pubTypeHits(t, alter("Label: integer"), open); len(hits) != 1 || !strings.Contains(hits[0], "Pj.Svc") {
		t.Errorf("String to Integer on a published String: want one violation naming Pj.Svc, got %v", hits)
	}
	if hits := pubTypeHits(t, alter("Label: string(120)"), open); len(hits) != 0 {
		t.Errorf("control: same EDM type reported: %v", hits)
	}
	if hits := pubTypeHits(t, alter("Count: long"), open); len(hits) != 0 {
		t.Errorf("control: Integer and Long are both Edm.Int64: %v", hits)
	}
	if hits := pubTypeHits(t, alter("Other: integer"), open); len(hits) != 0 {
		t.Errorf("control: an attribute no service publishes: %v", hits)
	}
	// Re-published afterwards: the types are re-derived, nothing stale remains.
	republish := alter("Label: integer") + "create or modify published odata service Pj.Svc (path: 'odata/pj/', version: '1.0.0', ODataVersion: OData4, namespace: 'Pj.S', ServiceName: 'Svc', Authentication: (basic))\n{\n  publish entity Pj.E as 'Items' (ReadMode: source, InsertMode: not_supported, UpdateMode: not_supported, DeleteMode: not_supported)\n  expose (Label);\n};\n"
	if hits := pubTypeHits(t, republish, open); len(hits) != 0 {
		t.Errorf("control: a later create or modify re-publishes the service: %v", hits)
	}
	// No project: what only the project could say is not judged.
	if hits := pubTypeHits(t, alter("Label: integer"), nil); len(hits) != 0 {
		t.Errorf("without a project nothing stored is known: %v", hits)
	}
}

// r2 of the measured pair, over a project entity: the script re-publishes the
// service (the published types are derived from the attributes as they are in the
// project) and THEN alters one. The script never declared the attribute, so its
// type has to come from the project.
func TestValidatePublishedAttributeTypes_RepublishThenAlterProjectEntity(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	ent := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e1"}, ContainerID: "dm-pj", Name: "E",
		Attributes: []*domainmodel.Attribute{
			{BaseElement: model.BaseElement{ID: "a1"}, Name: "Label", Type: &domainmodel.StringAttributeType{Length: 50}},
		}}
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: "dm-pj"}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{ent}}
	mb := &mock.MockBackend{
		IsConnectedFunc:                func() bool { return true },
		ListModulesFunc:                func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc:           func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetModuleByNameFunc:            func(string) (*model.Module, error) { return mod, nil },
		GetDomainModelFunc:             func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListPublishedODataServicesFunc: func() ([]*model.PublishedODataService, error) { return nil, nil },
	}
	open := func() backend.FullBackend { return mb }
	svc := "create or modify published odata service Pj.Svc (path: 'odata/pj/', version: '1.0.0', ODataVersion: OData4, namespace: 'Pj.S', ServiceName: 'Svc', Authentication: (basic))\n{\n  publish entity Pj.E as 'Items' (ReadMode: source, InsertMode: not_supported, UpdateMode: not_supported, DeleteMode: not_supported)\n  expose (Label);\n};\n"
	alter := "alter entity Pj.E modify attribute Label: integer;\n"
	if hits := pubTypeHits(t, "mdl 1;\n"+svc+alter, open); len(hits) != 1 {
		t.Errorf("re-publish then alter: want one violation, got %v", hits)
	}
	if hits := pubTypeHits(t, "mdl 1;\n"+alter+svc, open); len(hits) != 0 {
		t.Errorf("control, alter then re-publish: %v", hits)
	}
	if hits := pubTypeHits(t, "mdl 1;\n"+svc+"alter entity Pj.E modify attribute Label: string(90);\n", open); len(hits) != 0 {
		t.Errorf("control, same EDM type: %v", hits)
	}
}
