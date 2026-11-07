// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1796, after #1769 and #1777: a fragment spliced into a stored
// flow starts from the variables storedVariables types, and five outputs
// `create microflow` types were left untyped there — a created list, a cast
// in a type split's case, a nanoflow call's result, an import mapping's and a
// REST call's mapped result. A member of one was refused ("cannot qualify
// member …") and nothing was written. And of two direct origins of one name
// that give different types, the last one won.

// stored1796Fixture is a stored flow over Mod.Order (Code), its
// specialization Mod.SpecialOrder (Code, Rank) and Mod.OrderLine (Code,
// Status). Mod.FirstOrder and Mod.AllLines are nanoflows returning an Order
// and a list of lines.
func stored1796Fixture(t *testing.T) (*ExecContext, *alterFlowContext) {
	t.Helper()
	mod := mkModule("Mod")
	attr := func(name string) *domainmodel.Attribute {
		return &domainmodel.Attribute{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: name}
	}
	order := mkEntity(mod.ID, "Order")
	order.Attributes = []*domainmodel.Attribute{attr("Code")}
	special := mkEntity(mod.ID, "SpecialOrder")
	special.Attributes = []*domainmodel.Attribute{attr("Code"), attr("Rank")}
	line := mkEntity(mod.ID, "OrderLine")
	line.Attributes = []*domainmodel.Attribute{attr("Code"), attr("Status")}
	tag := mkEntity(mod.ID, "Tag")
	tag.Attributes = []*domainmodel.Attribute{attr("Label")}
	dm := mkDomainModel(mod.ID, order, special, line, tag)
	nanoflows := []*microflows.Nanoflow{
		{ContainerID: mod.ID, Name: "FirstOrder", ReturnType: &microflows.ObjectType{EntityQualifiedName: "Mod.Order"}},
		{ContainerID: mod.ID, Name: "AllLines", ReturnType: &microflows.ListType{EntityQualifiedName: "Mod.OrderLine"}},
		{ContainerID: mod.ID, Name: "Label", ReturnType: &microflows.StringType{}},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Mod" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListNanoflowsFunc:  func() ([]*microflows.Nanoflow, error) { return nanoflows, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	activity := func(id string, a microflows.MicroflowAction) *microflows.ActionActivity {
		return &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(id)}}}, Action: a}
	}
	act := func(out string, a microflows.MicroflowAction) mfmutator.Candidate {
		return mfmutator.Candidate{OutputVariable: out, Object: activity("act-"+out+string(nextID("")), a)}
	}
	newList := func(out, entity string) mfmutator.Candidate {
		return act(out, &microflows.CreateListAction{OutputVariable: out, EntityQualifiedName: entity})
	}
	nanoflow := func(out, qn string) mfmutator.Candidate {
		return act(out, &microflows.NanoflowCallAction{NanoflowCall: &microflows.NanoflowCall{Nanoflow: qn},
			OutputVariableName: out, UseReturnVariable: true})
	}
	mapped := func(entity string, single bool, out string) *microflows.ResultHandlingMapping {
		return &microflows.ResultHandlingMapping{ResultEntityID: model.ID(entity), ResultVariable: out, SingleObject: single}
	}
	imported := func(out, entity string, single bool) mfmutator.Candidate {
		return act(out, &microflows.ImportXmlAction{ResultHandling: mapped(entity, single, out)})
	}
	fetched := func(out, entity string, single bool) mfmutator.Candidate {
		return act(out, &microflows.RestCallAction{OutputVariable: out, UseReturnVariable: true, ResultHandling: mapped(entity, single, out)})
	}
	dbList := func(out, entity string) mfmutator.Candidate {
		return act(out, &microflows.RetrieveAction{OutputVariable: out,
			Source: &microflows.DatabaseRetrieveSource{EntityQualifiedName: entity}})
	}

	// $Order split by type: its Mod.SpecialOrder case casts it to $Special;
	// a cast reached from no case is not typed.
	split := &microflows.InheritanceSplit{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: "split1"}}, VariableName: "Order"}
	castSpecial := activity("cast1", &microflows.CastAction{ObjectVariable: "Order", OutputVariable: "Special"})
	castLoose := activity("cast2", &microflows.CastAction{ObjectVariable: "Order", OutputVariable: "Loose"})
	collection := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{split, castSpecial, castLoose},
		Flows: []*microflows.SequenceFlow{
			{OriginID: "split1", DestinationID: "cast1", CaseValue: &microflows.InheritanceCase{EntityQualifiedName: "Mod.SpecialOrder"}},
		},
	}

	a := &alterFlowContext{
		stmt: &ast.AlterFlowStmt{},
		mf: &microflows.Microflow{
			Parameters: []*microflows.MicroflowParameter{
				{Name: "Order", Type: &microflows.ObjectType{EntityQualifiedName: "Mod.Order"}},
			},
			ObjectCollection: collection,
		},
		entityNames: map[model.ID]string{order.ID: "Mod.Order", special.ID: "Mod.SpecialOrder", line.ID: "Mod.OrderLine", tag.ID: "Mod.Tag"},
		cands: []mfmutator.Candidate{
			newList("Fresh", "Mod.OrderLine"),
			act("FreshByID", &microflows.CreateListAction{OutputVariable: "FreshByID", EntityID: line.ID}),
			{OutputVariable: "Special", Object: castSpecial},
			{OutputVariable: "Loose", Object: castLoose},
			nanoflow("NfOrder", "Mod.FirstOrder"),
			nanoflow("NfLines", "Mod.AllLines"),
			nanoflow("NfLabel", "Mod.Label"),
			imported("Imported", "Mod.Order", true),
			imported("ImportedLines", "Mod.OrderLine", false),
			fetched("Fetched", "Mod.OrderLine", false),
			fetched("FetchedOne", "Mod.Order", true),
			// Two direct origins that give one type are one.
			newList("Same", "Mod.OrderLine"),
			dbList("Same", "Mod.OrderLine"),
			// Two direct origins that give different types: which one a
			// fragment means depends on where it goes.
			newList("Twice", "Mod.OrderLine"),
			newList("Twice", "Mod.Tag"),
			dbList("DbVsCall", "Mod.Order"),
			nanoflow("DbVsCall", "Mod.AllLines"),
			// A direct origin against a dependent one.
			act("ObjVsHead", &microflows.CreateObjectAction{OutputVariable: "ObjVsHead", EntityQualifiedName: "Mod.Order"}),
			act("ObjVsHead", &microflows.ListOperationAction{OutputVariable: "ObjVsHead", Operation: &microflows.HeadOperation{ListVariable: "Fresh"}}),
			// An entity origin against one that holds no entity.
			newList("ListVsText", "Mod.OrderLine"),
			nanoflow("ListVsText", "Mod.Label"),
		},
	}
	return ctx, a
}

func TestStoredVariables_1796OriginsAreTyped(t *testing.T) {
	ctx, a := stored1796Fixture(t)
	varTypes, declared := a.storedVariables(ctx)
	want := map[string]string{
		"Fresh":         "List of Mod.OrderLine",
		"FreshByID":     "List of Mod.OrderLine",
		"Special":       "Mod.SpecialOrder",
		"NfOrder":       "Mod.Order",
		"NfLines":       "List of Mod.OrderLine",
		"Imported":      "Mod.Order",
		"ImportedLines": "List of Mod.OrderLine",
		"Fetched":       "List of Mod.OrderLine",
		"FetchedOne":    "Mod.Order",
		"Same":          "List of Mod.OrderLine",
	}
	for v, w := range want {
		if got := varTypes[v]; got != w {
			t.Errorf("$%s: typed %q, want %q", v, got, w)
		}
		if d, ok := declared[v]; ok {
			t.Errorf("$%s: typed and also declared %q", v, d)
		}
	}
	for _, v := range []string{"Loose", "NfLabel", "Twice", "DbVsCall", "ObjVsHead", "ListVsText"} {
		if got, ok := varTypes[v]; ok {
			t.Errorf("$%s: typed %q, want it left untyped", v, got)
		}
		if _, ok := declared[v]; !ok {
			t.Errorf("$%s: not declared at all", v)
		}
	}
}

// The reported shape: a member of each, unqualified, is written qualified with
// the entity the variable holds, as `create microflow` writes it.
func TestSpliceFragment_MembersOf1796OutputsAreQualified(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`$F = find $Fresh by Status = 'Open';`, "Mod.OrderLine.Status"},
		{`change $Special (Rank = 1);`, "Mod.SpecialOrder.Rank"},
		{`change $NfOrder (Code = 'B');`, "Mod.Order.Code"},
		{`$F = filter $NfLines by Status = 'Open';`, "Mod.OrderLine.Status"},
		{`change $Imported (Code = 'B');`, "Mod.Order.Code"},
		{`$S = sort $ImportedLines by Code asc;`, "Mod.OrderLine.Code"},
		{`$F = find $Fetched by Code = 'A';`, "Mod.OrderLine.Code"},
		{`$F = find $Same by Code = 'A';`, "Mod.OrderLine.Code"},
	} {
		ctx, a := stored1796Fixture(t)
		got, err := fragmentMembers(t, ctx, a, c.src)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("%s wrote %v, want [%s]", c.src, got, c.want)
		}
	}
}

// A name two direct origins give different types stays untyped, as two
// dependent ones already did, and a member of it is refused rather than
// qualified with whichever origin came last.
func TestSpliceFragment_MembersOfConflictingDirectOriginsAreRefused(t *testing.T) {
	for _, src := range []string{
		`$F = find $Twice by Code = 'A';`,
		`$F = find $DbVsCall by Code = 'A';`,
		`change $ObjVsHead (Code = 'B');`,
		`$F = find $ListVsText by Code = 'A';`,
		`change $Loose (Rank = 1);`,
	} {
		ctx, a := stored1796Fixture(t)
		if _, err := fragmentMembers(t, ctx, a, src); err == nil || !strings.Contains(err.Error(), "cannot qualify member") {
			t.Errorf("%s: want a refusal, got %v", src, err)
		}
	}
}
