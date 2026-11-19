// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A custom object handler (`find … by M.MF ( P: member )`) must name exactly the
// microflow's parameters. Measured on MxBuild 11.16.1, ONE import mapping per
// project (a shared project is wrong: CE1613 is an unresolved reference and hides
// every other error, which first made two of these look clean):
//
//	the microflow's own parameter names, all of them   clean
//	a name it does not have (Nope)                     CE1613   <- check and exec accepted it
//	the right name in another case (index)             CE1613   (names are case-sensitive)
//	a valid name plus an unknown one (Extra)           CE1613
//	one of two parameters left out                     CE0277   "needs to be refreshed"
//	none named on a one-parameter microflow           CE0277
//	none named on a microflow with no parameters       clean
//	a name on a microflow with no parameters           CE1613
//
// `check` and `exec` accepted every rejected case and wrote the mapping. The
// build reports CE1613 before CE0277, so a handler with an unknown name is judged
// on that alone.
const handlerParamsScript = `mdl 1;
create module Hp;
create persistent entity Hp.Root (Name: string(50));
create persistent entity Hp.Child (Code: string(50));
create association Hp.Child_Root from Hp.Child to Hp.Root type reference;
create json structure Hp.JS sample '{"name": "n", "items": [{"code": "c", "idx": 1}]}';
create microflow Hp.ByIndex ($Index: integer) returns Hp.Child begin
  return empty;
end;
create microflow Hp.Two ($A: integer, $B: integer) returns Hp.Child begin
  return empty;
end;
create microflow Hp.NoParams () returns Hp.Child begin
  return empty;
end;
`

func handlerMapping(mf, params string) string {
	return handlerParamsScript + `create import mapping Hp.IMM with json structure Hp.JS {
  create Hp.Root {
    Name = name,
    find Hp.Child_Root/Hp.Child by ` + mf + ` ( ` + params + ` ) = items { Code = code }
  }
};
`
}

func TestValidateMappingHandlerParams_Script(t *testing.T) {
	cases := []struct {
		name, mf, params string
		want             int
	}{
		{"the microflow's own parameter (control)", "Hp.ByIndex", "Index: idx", 0},
		{"a name the microflow does not have", "Hp.ByIndex", "Nope: idx", 1},
		{"the right name in another case", "Hp.ByIndex", "index: idx", 1},
		{"a valid name plus an unknown one", "Hp.ByIndex", "Index: idx, Extra: idx", 1},
		{"one of two parameters left out", "Hp.Two", "A: idx", 1},
		{"none named on a one-parameter microflow", "Hp.ByIndex", "", 1},
		{"none named on a microflow with no parameters (control)", "Hp.NoParams", "", 0},
		{"a name on a microflow with no parameters", "Hp.NoParams", "Extra: idx", 1},
		{"an unknown name AND a missing parameter: judged on the unknown name alone", "Hp.Two", "Nope: idx", 1},
		{"both parameters named (control)", "Hp.Two", "A: idx, B: idx", 0},
		{"two wrong names, two violations", "Hp.Two", "X: idx, Y: idx", 2},
		{"a microflow that is not known here (control)", "Other.Elsewhere", "Whatever: idx", 0},
		{"parent source judged by name too", "Hp.ByIndex", "Nope: parent", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidateMappingHandlerParams(mustBuild(t, handlerMapping(c.mf, c.params)), nil)
			if len(got) != c.want {
				t.Fatalf("violations=%v, want %d", rulesOf(got), c.want)
			}
			if c.want > 0 {
				v := got[0]
				wantCode := "CE1613"
				if c.name == "one of two parameters left out" || c.name == "none named on a one-parameter microflow" {
					wantCode = "CE0277"
				}
				if v.RuleID != mappingHandlerParamRule || !strings.Contains(v.Message, wantCode) ||
					!strings.Contains(v.Message, c.mf) || v.Location.DocumentName != "IMM" {
					t.Errorf("rule, CE code, microflow or location wrong: %+v", v)
				}
			}
		})
	}
}

// The handler on a nested element and on the item of a two-level array is the
// same handler.
func TestValidateMappingHandlerParams_NestedElements(t *testing.T) {
	src := handlerParamsScript + `create import mapping Hp.IMM2 with json structure Hp.JS {
  create Hp.Root {
    Name = name,
    create Hp.Child_Root/Hp.Child = items {
      Code = code,
      find Hp.Child_Root/Hp.Child by Hp.ByIndex ( Nope: idx ) = meta { Code = code }
    }
  }
};
`
	if got := ValidateMappingHandlerParams(mustBuild(t, src), nil); len(got) != 1 {
		t.Errorf("a handler two levels down: want one violation, got %v", rulesOf(got))
	}
}

// The microflow lives in the project, not the script.
func TestValidateMappingHandlerParams_ProjectMicroflow(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	mf := &microflows.Microflow{
		BaseElement: model.BaseElement{ID: "mf1"}, ContainerID: mod.ID, Name: "Resolve",
		Parameters: []*microflows.MicroflowParameter{{Name: "Index"}},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}
	open := func() backend.FullBackend { return mb }
	src := func(params string) string {
		return `mdl 1;
create import mapping Pj.IMM with json structure Pj.JS {
  create Pj.Root {
    Name = name,
    find Pj.Child_Root/Pj.Child by Pj.Resolve ( ` + params + ` ) = items { Code = code }
  }
};
`
	}
	if got := ValidateMappingHandlerParams(mustBuild(t, src("Nope: idx")), open); len(got) != 1 {
		t.Errorf("an unknown parameter of a project microflow: want one violation, got %v", rulesOf(got))
	}
	if got := ValidateMappingHandlerParams(mustBuild(t, src("")), open); len(got) != 1 || !strings.Contains(got[0].Message, "CE0277") {
		t.Errorf("a project microflow's parameter left unnamed: want one CE0277, got %v", rulesOf(got))
	}
	if got := ValidateMappingHandlerParams(mustBuild(t, src("Index: idx")), open); len(got) != 0 {
		t.Errorf("control: the project microflow's own parameter was reported: %v", rulesOf(got))
	}
	if got := ValidateMappingHandlerParams(mustBuild(t, src("Nope: idx")), nil); len(got) != 0 {
		t.Errorf("no project: a microflow nothing knows must not be judged: %v", rulesOf(got))
	}
}
