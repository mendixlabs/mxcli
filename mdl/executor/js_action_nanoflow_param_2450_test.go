// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2450 (mendixlabs/mxcli#1904): a JavaScript action argument for a
// Nanoflow-typed parameter is stored by Studio Pro as
// Nanoflows$NanoflowParameterValue { Nanoflow: "<Module.Nanoflow>" } (blank: "").
// mxcli wrote a Basic value with the name as its expression — check and exec
// passed, mx check CE0115 — and describe left the argument out, so editing a
// Studio Pro call dropped every nanoflow argument.

func gcdBuilder(t *testing.T) *flowBuilder {
	return &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		backend: &mock.MockBackend{
			ReadJavaScriptActionByNameFunc: func(string) (*types.JavaScriptAction, error) {
				return &types.JavaScriptAction{Name: "GetCreateDevice", Parameters: []*types.JavaActionParameter{
					{Name: "deviceName", ParameterType: &types.StringType{}},
					{Name: "initialize", ParameterType: &types.NanoflowType{}},
					{Name: "onConnect", ParameterType: &types.NanoflowType{}},
				}}, nil
			},
		},
	}
}

func parseJSCall(t *testing.T, call string) *ast.CallJavaScriptActionStmt {
	t.Helper()
	prog, errs := visitor.Build("create nanoflow M.N () begin\n" + call + "\nend;")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, s := range prog.Statements[0].(*ast.CreateNanoflowStmt).Body {
		if c, ok := s.(*ast.CallJavaScriptActionStmt); ok {
			return c
		}
	}
	t.Fatal("no call javascript action")
	return nil
}

func TestJSActionNanoflowParameterWritesNanoflowValue_Issue2450(t *testing.T) {
	fb := gcdBuilder(t)
	action := jsActionActivity(t, fb, parseJSCall(t, `call javascript action SC.GetCreateDevice (
    deviceName = 'Scanner 1',
    initialize = SC.Initialize
,
    onConnect = empty
  );`))
	got := map[string]microflows.CodeActionParameterValue{}
	for _, m := range action.ParameterMappings {
		got[m.Parameter[strings.LastIndex(m.Parameter, ".")+1:]] = m.Value
	}
	if v, ok := got["initialize"].(*microflows.NanoflowParameterValue); !ok || v.Nanoflow != "SC.Initialize" {
		t.Errorf("initialize = %#v, want NanoflowParameterValue SC.Initialize", got["initialize"])
	}
	if v, ok := got["onConnect"].(*microflows.NanoflowParameterValue); !ok || v.Nanoflow != "" {
		t.Errorf("onConnect = %#v, want a blank NanoflowParameterValue", got["onConnect"])
	}
	// CONTROL: a String parameter keeps its expression.
	if _, ok := got["deviceName"].(*microflows.BasicCodeActionParameterValue); !ok {
		t.Errorf("deviceName = %T, want Basic", got["deviceName"])
	}
	if len(fb.errors) != 0 {
		t.Errorf("unexpected errors: %v", fb.errors)
	}
}

// A Nanoflow parameter takes a name, never an expression, as a Java action's
// Microflow parameter does (mendixlabs/mxcli#1210).
func TestJSActionNanoflowParameterRefusesAnExpression_Issue2450(t *testing.T) {
	fb := gcdBuilder(t)
	jsActionActivity(t, fb, parseJSCall(t, `call javascript action SC.GetCreateDevice (initialize = 'x' + 'y');`))
	if len(fb.errors) == 0 || !strings.Contains(strings.Join(fb.errors, " "), "Nanoflow") {
		t.Errorf("errors = %v, want a refusal naming the Nanoflow type", fb.errors)
	}
}

// Describe prints the nanoflow argument, so describe → exec carries it.
func TestDescribeJSActionNanoflowArgument_Issue2450(t *testing.T) {
	ctx, _ := newMockCtx(t)
	a := &microflows.JavaScriptActionCallAction{JavaScriptAction: "SC.GetCreateDevice", ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
		{Parameter: "SC.GetCreateDevice.initialize", Value: &microflows.NanoflowParameterValue{Nanoflow: "SC.Initialize"}},
		{Parameter: "SC.GetCreateDevice.onDisconnect", Value: &microflows.NanoflowParameterValue{}},
	}}
	out := formatAction(ctx, a, nil, nil)
	for _, want := range []string{"initialize = SC.Initialize", "onDisconnect = empty"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe lacks %q:\n%s", want, out)
		}
	}
}
