// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1928 (mendixlabs/mxcli#1726): two microflows of one qualified name,
// one excluded — which Mendix accepts (CE0122 counts active documents only).
// `check --references` validated a call against the excluded twin's parameters
// and refused a correct call. The validators index stored documents by name in
// a map, so the twin listed LAST won. Each index must hold the live one, in
// either list order.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

type twinOrder struct {
	name          string
	excludedFirst bool
}

var twinOrders = []twinOrder{{"excluded twin listed last", false}, {"excluded twin listed first", true}}

func twinPair[T any](live, excluded T, excludedFirst bool) []T {
	if excludedFirst {
		return []T{excluded, live}
	}
	return []T{live, excluded}
}

func newExcludedTwinCtx(t *testing.T, order twinOrder) *ExecContext {
	t.Helper()
	mod := mkModule("Examples")
	live := mkMicroflow(mod.ID, "Test")
	live.Parameters = []*microflows.MicroflowParameter{{Name: "param1", Type: &microflows.StringType{}}}
	live.ReturnType = &microflows.BooleanType{}
	excl := mkMicroflow(mod.ID, "Test")
	excl.Excluded = true
	excl.Parameters = []*microflows.MicroflowParameter{{Name: "param2", Type: &microflows.StringType{}}}
	excl.ReturnType = &microflows.StringType{}

	nlive := mkNanoflow(mod.ID, "NTest")
	nlive.Parameters = []*microflows.MicroflowParameter{{Name: "param1", Type: &microflows.StringType{}}}
	nexcl := mkNanoflow(mod.ID, "NTest")
	nexcl.Excluded = true
	nexcl.Parameters = []*microflows.MicroflowParameter{{Name: "param2", Type: &microflows.StringType{}}}

	plive := mkPage(mod.ID, "Edit")
	plive.Parameters = []*pages.PageParameter{{Name: "param1", EntityName: "Examples.Live"}}
	pexcl := mkPage(mod.ID, "Edit")
	pexcl.Excluded = true
	pexcl.Parameters = []*pages.PageParameter{{Name: "param2", EntityName: "Examples.Excluded"}}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return twinPair(live, excl, order.excludedFirst), nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return twinPair(nlive, nexcl, order.excludedFirst), nil
		},
		ListPagesFunc: func() ([]*pages.Page, error) {
			return twinPair(plive, pexcl, order.excludedFirst), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

func TestExcludedTwin_ValidatorIndexesHoldTheLiveDocument(t *testing.T) {
	for _, order := range twinOrders {
		t.Run(order.name, func(t *testing.T) {
			ctx := newExcludedTwinCtx(t, order)

			sigs := buildFlowSignatures(ctx)
			for _, key := range []string{"examples.test", "examples.ntest"} {
				if got := sigs[key].paramNames(); len(got) != 1 || got[0] != "param1" {
					t.Errorf("buildFlowSignatures[%s] params = %v, want the live [param1]", key, got)
				}
			}
			if got := buildMicroflowReturnTypes(ctx)["Examples.Test"]; got != (&microflows.BooleanType{}).GetTypeName() {
				t.Errorf("buildMicroflowReturnTypes = %q, want the live flow's Boolean", got)
			}
			if got := buildPageParamNames(ctx)["examples.edit"]; len(got) != 1 || got[0] != "param1" {
				t.Errorf("buildPageParamNames = %v, want the live [param1]", got)
			}
			if got := buildPageParamEntities(ctx)["examples.edit"]; len(got) != 1 || got[0] != "Examples.Live" {
				t.Errorf("buildPageParamEntities = %v, want the live [Examples.Live]", got)
			}
			mfs, err := microflowsByQualifiedName(ctx)
			if err != nil || mfs["examples.test"] == nil || mfs["examples.test"].Excluded {
				t.Errorf("microflowsByQualifiedName picked the excluded twin (err %v)", err)
			}
		})
	}
}

// The reported script: a call written against the live flow's parameter.
func TestExcludedTwin_CallAgainstLiveFlowPasses(t *testing.T) {
	arg := func(name string) []ast.CallArgument {
		return []ast.CallArgument{{Name: name, Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "test"}}}
	}
	for _, order := range twinOrders {
		t.Run(order.name, func(t *testing.T) {
			ctx := newExcludedTwinCtx(t, order)
			ok := []ast.MicroflowStatement{&ast.CallMicroflowStmt{MicroflowName: qn("Examples", "Test"), Arguments: arg("param1")}}
			if errs := validateFlowBodyReferences(ctx, ok, newScriptContext()); len(errs) != 0 {
				t.Fatalf("call with the live flow's parameter refused: %v", errs)
			}
			// Control: the excluded twin's parameter is not one the app has.
			bad := []ast.MicroflowStatement{&ast.CallMicroflowStmt{MicroflowName: qn("Examples", "Test"), Arguments: arg("param2")}}
			errs := validateFlowBodyReferences(ctx, bad, newScriptContext())
			if len(errs) != 1 || !strings.Contains(errs[0], `has no parameter "param2" (declared parameters: param1)`) {
				t.Fatalf("errors = %v, want the excluded twin's parameter refused against the live one", errs)
			}
		})
	}
}
