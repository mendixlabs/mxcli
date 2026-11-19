// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A custom object handler feeds a JSON member to a microflow parameter, and the
// two types must agree. Measured on MxBuild 11.16.1, ONE import mapping per project
// (CE1613 hides every other error), against a sample holding a string, an integer,
// a decimal, a boolean and a date-time. Every pairing not listed as clean is CE0282:
//
//	String parameter         clean for every member type
//	Integer parameter        clean for an Integer member only
//	Long parameter           clean for an Integer member (Long, Decimal, String members: CE0282)
//	Decimal parameter        clean for an Integer or a Decimal member
//	Boolean, DateTime param  clean for a member of the same type only
const handlerTypesSample = `{"name": "n", "items": [{"code": "c", "idx": 1, "d": 1.5, "b": true, "dt": "2020-01-01T00:00:00Z"}]}`

func handlerTypesScript(paramType, member string) string {
	return `mdl 1;
create persistent entity Ht.Root (Name: string(50));
create persistent entity Ht.Child (Code: string(50));
create association Ht.Child_Root from Ht.Child to Ht.Root type reference;
create json structure Ht.JS sample '` + handlerTypesSample + `';
create microflow Ht.MF ($P: ` + paramType + `) returns Ht.Child begin
  return empty;
end;
create import mapping Ht.IMM with json structure Ht.JS {
  create Ht.Root {
    Name = name,
    find Ht.Child_Root/Ht.Child by Ht.MF ( P: ` + member + ` ) = items { Code = code }
  }
};
`
}

func TestValidateMappingHandlerTypes_Matrix(t *testing.T) {
	members := map[string]string{"String": "code", "Integer": "idx", "Decimal": "d", "Boolean": "b", "DateTime": "dt"}
	clean := map[string][]string{
		"string":   {"String", "Integer", "Decimal", "Boolean", "DateTime"},
		"integer":  {"Integer"},
		"long":     {"Integer"},
		"decimal":  {"Integer", "Decimal"},
		"boolean":  {"Boolean"},
		"datetime": {"DateTime"},
	}
	for param, ok := range clean {
		for mtype, member := range members {
			want := 1
			for _, c := range ok {
				if c == mtype {
					want = 0
				}
			}
			got := ValidateMappingHandlerTypes(mustBuild(t, handlerTypesScript(param, member)), nil)
			if len(got) != want {
				t.Errorf("%s parameter fed a %s member: violations=%v, want %d", param, mtype, rulesOf(got), want)
				continue
			}
			if want == 1 {
				v := got[0]
				if v.RuleID != mappingHandlerTypeRule || !strings.Contains(v.Message, "CE0282") ||
					!strings.Contains(v.Message, "Ht.MF") || v.Location.DocumentName != "IMM" {
					t.Errorf("%s/%s: rule, CE code, microflow or location wrong: %+v", param, mtype, v)
				}
			}
		}
	}
}

// What is not judged: a member that is not in the structure, a parameter that is
// an object, a source other than a JSON member, and a microflow that is not known.
func TestValidateMappingHandlerTypes_NotJudged(t *testing.T) {
	cases := map[string]string{
		"a member the structure does not have": handlerTypesScript("integer", "missing"),
		"a parent source":                      handlerTypesScript("integer", "parent"),
		"an object parameter":                  handlerTypesScript("Ht.Child", "idx"),
	}
	for name, src := range cases {
		if got := ValidateMappingHandlerTypes(mustBuild(t, src), nil); len(got) != 0 {
			t.Errorf("%s: want no violation, got %v", name, rulesOf(got))
		}
	}
	unknown := strings.Replace(handlerTypesScript("integer", "code"), "by Ht.MF", "by Other.Elsewhere", 1)
	if got := ValidateMappingHandlerTypes(mustBuild(t, unknown), nil); len(got) != 0 {
		t.Errorf("a microflow that is not known here: want none, got %v", rulesOf(got))
	}
}

// The same key at two places with two types cannot be judged without following
// the mapping's path, so it is left alone rather than guessed.
func TestValidateMappingHandlerTypes_AmbiguousMember(t *testing.T) {
	src := strings.Replace(handlerTypesScript("integer", "code"),
		`"items": [`, `"other": {"code": 5}, "items": [`, 1)
	if got := ValidateMappingHandlerTypes(mustBuild(t, src), nil); len(got) != 0 {
		t.Errorf("`code` is a string under items and an integer under other: want none, got %v", rulesOf(got))
	}
	// A nested member reached through its parent is judged on the whole path.
	nested := strings.Replace(handlerTypesScript("integer", "meta/k"),
		`"items": [`, `"items": [{"meta": {"k": "s"}}, `, 1)
	if got := ValidateMappingHandlerTypes(mustBuild(t, nested), nil); len(got) != 1 {
		t.Errorf("meta/k is a string: want one violation, got %v", rulesOf(got))
	}
}

// The structure and the microflow both live in the project.
func TestValidateMappingHandlerTypes_Project(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	mf := &microflows.Microflow{
		BaseElement: model.BaseElement{ID: "mf1"}, ContainerID: mod.ID, Name: "Resolve",
		Parameters: []*microflows.MicroflowParameter{{Name: "Index", Type: &microflows.IntegerType{}}},
	}
	js := &types.JsonStructure{Name: "JS", JsonSnippet: `{"items": [{"code": "c", "idx": 1}]}`}
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		GetJsonStructureByQualifiedNameFunc: func(m, n string) (*types.JsonStructure, error) {
			if m == "Pj" && n == "JS" {
				elems, err := types.BuildJsonElementsFromSnippet(js.JsonSnippet, nil, nil)
				if err != nil {
					return nil, err
				}
				js.Elements = elems
				return js, nil
			}
			return nil, nil
		},
	}
	open := func() backend.FullBackend { return mb }
	src := func(member string) string {
		return `mdl 1;
create import mapping Pj.IMM with json structure Pj.JS {
  create Pj.Root {
    find Pj.Child_Root/Pj.Child by Pj.Resolve ( Index: ` + member + ` ) = items { Code = code }
  }
};
`
	}
	if got := ValidateMappingHandlerTypes(mustBuild(t, src("idx")), open); len(got) != 0 {
		t.Errorf("an Integer member into an Integer parameter (control): want none, got %v", rulesOf(got))
	}
	if got := ValidateMappingHandlerTypes(mustBuild(t, src("code")), open); len(got) != 1 {
		t.Errorf("a String member into an Integer parameter: want one, got %v", rulesOf(got))
	}
}
