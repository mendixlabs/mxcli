// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// CE0295 "Association '…' is not allowed." on an object mapping element whose
// association does not connect the entity it sits under to its own entity.
// Measured on MxBuild 11.16.1, export AND import mappings, every variant built
// and rejected by `mx check` while `check` and `exec` accepted it:
//
//	association touches neither entity            CE0295
//	touches the parent, not the element's entity  CE0295
//	touches the element's entity, not the parent  CE0295
//	connects the two (either direction)           clean   <- control
const mapHead = `mdl 1;
create module Mp;
create json structure Mp.JS sample '{"id": 1, "customer": {"name": "A"}}';
create non-persistent entity Mp.Root (Id: integer);
create non-persistent entity Mp.Cust (Name: string(50));
create non-persistent entity Mp.SubCust extends Mp.Cust (Extra: string(50));
create non-persistent entity Mp.Other (Name: string(50));
create non-persistent entity Mp.Other2 (Name: string(50));
create association Mp.Cust_Root from Mp.Cust to Mp.Root owner both;
create association Mp.Other_Other2 from Mp.Other to Mp.Other2;
create association Mp.Cust_Other from Mp.Cust to Mp.Other;
`

func exportMap(assoc string) string {
	return "create export mapping Mp.EMM with json structure Mp.JS {\n  Mp.Root {\n    id = Id,\n    " +
		assoc + " as customer { name = Name }\n  }\n};\n"
}

func importMap(assoc string) string {
	return "create import mapping Mp.IMM with json structure Mp.JS {\n  create Mp.Root {\n    Id = id,\n    create " +
		assoc + " = customer { Name = name }\n  }\n};\n"
}

func mapProgram(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs, src)
	}
	return prog
}

func TestValidateMappingAssociations_Script(t *testing.T) {
	cases := []struct {
		name  string
		assoc string
		want  bool // refused
	}{
		{"the association connects parent and element (control)", "Mp.Cust_Root/Mp.Cust", false},
		{"neither end is the parent or the element", "Mp.Other_Other2/Mp.Cust", true},
		{"touches the parent, not the element's entity", "Mp.Cust_Root/Mp.Other", true},
		{"touches the element's entity, not the parent", "Mp.Cust_Other/Mp.Cust", true},
		// The association's FROM is Mp.Cust, which the element's entity extends.
		{"an end is a generalization of the element's entity (control)", "Mp.Cust_Root/Mp.SubCust", false},
	}
	for _, kind := range []struct {
		name string
		body func(string) string
		rule string
	}{{"export", exportMap, "export mapping"}, {"import", importMap, "import mapping"}} {
		for _, c := range cases {
			t.Run(kind.name+": "+c.name, func(t *testing.T) {
				v := ValidateMappingAssociations(mapProgram(t, mapHead+kind.body(c.assoc)), nil)
				if got := len(v) > 0; got != c.want {
					t.Fatalf("violations=%v, want refused=%v", v, c.want)
				}
				if c.want {
					if v[0].RuleID != mappingAssociationRule || !strings.Contains(v[0].Message, "CE0295") ||
						v[0].Location.DocumentType != kind.rule {
						t.Errorf("rule, CE code or location wrong: %+v", v[0])
					}
				}
			})
		}
	}
}

// An entity-less group node passes the enclosing entity through, as the export
// builder does: the association below it still has to connect that entity.
func TestValidateMappingAssociations_GroupPassesTheParentThrough(t *testing.T) {
	wrap := func(assoc string) string {
		return "create export mapping Mp.EMM with json structure Mp.JS {\n  Mp.Root {\n    id = Id,\n    group AS wrapper {\n      " +
			assoc + " as customer { name = Name }\n    }\n  }\n};\n"
	}
	if v := ValidateMappingAssociations(mapProgram(t, mapHead+wrap("Mp.Cust_Root/Mp.Cust")), nil); len(v) != 0 {
		t.Errorf("control refused: %v", v)
	}
	if v := ValidateMappingAssociations(mapProgram(t, mapHead+wrap("Mp.Other_Other2/Mp.Cust")), nil); len(v) != 1 {
		t.Errorf("a wrong association under a group was not refused: %v", v)
	}
}

// An association the script does not declare and no project supplies is not
// looked at: refusing what cannot be read would be a false report.
func TestValidateMappingAssociations_UnknownAssociationIsNeverRefused(t *testing.T) {
	src := strings.Replace(mapHead, "create association Mp.Cust_Root from Mp.Cust to Mp.Root owner both;\n", "", 1) +
		exportMap("Mp.Cust_Root/Mp.Cust")
	if v := ValidateMappingAssociations(mapProgram(t, src), nil); len(v) != 0 {
		t.Errorf("an association that cannot be read was refused: %v", v)
	}
}

// The association and the entities live in the project, not the script.
func TestValidateMappingAssociations_ProjectAssociation(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	ent := func(id, name, gen string) *domainmodel.Entity {
		return &domainmodel.Entity{BaseElement: model.BaseElement{ID: model.ID(id)}, ContainerID: "dm-pj", Name: name, GeneralizationRef: gen}
	}
	root, cust, other := ent("e-root", "Root", ""), ent("e-cust", "Cust", ""), ent("e-other", "Other", "")
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-pj"}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{root, cust, other},
		Associations: []*domainmodel.Association{
			{BaseElement: model.BaseElement{ID: "a1"}, Name: "Cust_Root", ParentID: cust.ID, ChildID: root.ID},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	open := func() backend.FullBackend { return mb }
	mapping := func(el string) string {
		return "mdl 1;\ncreate json structure Pj.JS sample '{\"id\": 1, \"customer\": {\"name\": \"A\"}}';\n" +
			"create export mapping Pj.EMM with json structure Pj.JS {\n  Pj.Root {\n    id = Id,\n    " + el +
			" as customer { name = Name }\n  }\n};\n"
	}
	if v := ValidateMappingAssociations(mapProgram(t, mapping("Pj.Cust_Root/Pj.Cust")), open); len(v) != 0 {
		t.Errorf("control: the project association connects the two, got %v", v)
	}
	if v := ValidateMappingAssociations(mapProgram(t, mapping("Pj.Cust_Root/Pj.Other")), open); len(v) != 1 {
		t.Errorf("Cust_Root does not reach Other: want one violation, got %v", v)
	}
}

// Measured clean on MxBuild 11.16.1: the mapping's root is a specialization of
// the association's end (the parent side of the chain).
func TestValidateMappingAssociations_ParentSpecialization(t *testing.T) {
	head := strings.Replace(mapHead, "create non-persistent entity Mp.Cust ",
		"create non-persistent entity Mp.SubRoot extends Mp.Root (SubV: integer);\ncreate non-persistent entity Mp.Cust ", 1)
	mapping := func(assoc string) string {
		return "create export mapping Mp.EMM with json structure Mp.JS {\n  Mp.SubRoot {\n    id = Id,\n    " +
			assoc + " as customer { name = Name }\n  }\n};\n"
	}
	if v := ValidateMappingAssociations(mapProgram(t, head+mapping("Mp.Cust_Root/Mp.Cust")), nil); len(v) != 0 {
		t.Errorf("control: an association ending on the root's generalization was refused: %v", v)
	}
	if v := ValidateMappingAssociations(mapProgram(t, head+mapping("Mp.Cust_Other/Mp.Cust")), nil); len(v) != 1 {
		t.Errorf("an association that reaches neither the root nor what it extends: want one violation, got %v", v)
	}
}
