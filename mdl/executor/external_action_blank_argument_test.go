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
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1915: Studio Pro 11.16 stores an optional OData action parameter
// left blank as `Argument: ""` (11.14 stored the expression `empty`, measured
// for #1073). DESCRIBE printed it as `additional = )`, which does not parse, so
// every `mdl 1;` rewrite of the microflow was refused with MDL-V1-REBUILD
// "the description does not parse".
//
// The two stored states are distinct and both real, so each has to come back
// as itself. MDL already has the spelling for the first: `Param = nothing`, the
// blank argument of call microflow / call nanoflow, which this extends to call
// external action. `empty` stays `= empty`, and omitting a parameter still
// writes no mapping at all.

// storedBlankArgumentCall is the reference shape from Odata.IncludeAssociationsMF
// (ako/TestApp, 11.16.0): `additional` blank, beside a required `command` and,
// as the control, a nullable `note` holding the expression `empty`.
func storedBlankArgumentCall() *microflows.CallExternalAction {
	return &microflows.CallExternalAction{
		ConsumedODataService: "Odata.Bug1073",
		Name:                 "RunCommand",
		ParameterMappings: []*microflows.ExternalActionParameterMapping{
			{ParameterName: "command", Argument: "$Cmd", ParameterDataType: "String", CanBeEmpty: false},
			{ParameterName: "additional", Argument: "", ParameterDataType: "String", CanBeEmpty: true},
			{ParameterName: "note", Argument: "empty", ParameterDataType: "String", CanBeEmpty: true},
		},
	}
}

// buildExternalCallFrom parses one `call external action` statement and runs
// the real builder on it against canBeEmptyMetadata's contract.
func buildExternalCallFrom(t *testing.T, stmt string) (*ast.CallExternalActionStmt, *microflows.CallExternalAction, []string) {
	t.Helper()
	svcID, modID := model.ID("svc-1"), model.ID("mod-1")
	mb := &mock.MockBackend{
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{{
				BaseElement: model.BaseElement{ID: svcID},
				ContainerID: modID,
				Name:        "Bug1073",
				Metadata:    canBeEmptyMetadata,
			}}, nil
		},
	}
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing, backend: mb,
		hierarchy:    &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "Odata"}},
		varTypes:     map[string]string{"Cmd": "String"},
		declaredVars: map[string]string{"Cmd": "String"},
	}
	prog, errs := visitor.Build("create microflow Odata.ACT_Probe ($Cmd: String)\nbegin\n  " + stmt + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("the statement does not parse — the reported symptom: %v\n%s", errs, stmt)
	}
	call := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.CallExternalActionStmt)
	fb.addCallExternalActionAction(call)
	for _, obj := range fb.objects {
		if act, ok := obj.(*microflows.ActionActivity); ok {
			if built, ok := act.Action.(*microflows.CallExternalAction); ok {
				return call, built, fb.GetErrors()
			}
		}
	}
	t.Fatal("the builder produced no CallExternalAction")
	return nil, nil, nil
}

func runCommandContract(t *testing.T) *types.EdmAction {
	t.Helper()
	doc, err := types.ParseEdmx(canBeEmptyMetadata)
	if err != nil || len(doc.Actions) != 1 {
		t.Fatalf("parsing the contract: %v", err)
	}
	return doc.Actions[0]
}

func externalMappingShape(pms []*microflows.ExternalActionParameterMapping) string {
	var parts []string
	for _, pm := range pms {
		parts = append(parts, pm.ParameterName+"="+`"`+pm.Argument+`"`)
		if pm.CanBeEmpty {
			parts[len(parts)-1] += "?"
		}
	}
	return strings.Join(parts, ", ")
}

func TestCallExternalAction_BlankArgumentRoundTrips(t *testing.T) {
	stored := storedBlankArgumentCall()
	described := newTestExecutor().formatAction(stored, nil, nil)

	if strings.Contains(described, "= )") || strings.Contains(described, "= ,") {
		t.Fatalf("a blank argument is described with no value, which does not parse: %s", described)
	}
	if !strings.Contains(described, "additional = nothing") {
		t.Errorf("the blank argument is not described as `= nothing`: %s", described)
	}
	// Control: the `empty` expression is a value and is still spelled.
	if !strings.Contains(described, "note = empty") {
		t.Errorf("the `empty` mapping lost its spelling: %s", described)
	}

	stmt, rebuilt, errs := buildExternalCallFrom(t, described)
	if len(errs) > 0 {
		t.Fatalf("rebuilding the description reported errors: %v", errs)
	}
	if got, want := externalMappingShape(rebuilt.ParameterMappings), externalMappingShape(stored.ParameterMappings); got != want {
		t.Errorf("describe -> build changed the mappings\n got: %s\nwant: %s\ndescription: %s", got, want, described)
	}

	// The described statement names every parameter, so the contract check
	// that `check --references` and exec run accepts it.
	action := runCommandContract(t)
	if err := checkExternalActionParameters(externalCall{stmt: stmt, flow: "Odata.ACT_Probe"}, action); err != nil {
		t.Errorf("the described call is refused against its own contract: %v", err)
	}
}

// Control: the 11.14 form, every unfilled argument the expression `empty`,
// describes and rebuilds byte-identically, and omitting a parameter is still
// not the blank: a required one left out is refused as before.
func TestCallExternalAction_EmptyArgumentAndRequiredStillHold(t *testing.T) {
	stored := storedBlankArgumentCall()
	stored.ParameterMappings[1].Argument = "empty"
	described := newTestExecutor().formatAction(stored, nil, nil)
	if !strings.Contains(described, "additional = empty") {
		t.Fatalf("the `empty` argument is not described as `= empty`: %s", described)
	}
	_, rebuilt, _ := buildExternalCallFrom(t, described)
	if got, want := externalMappingShape(rebuilt.ParameterMappings), externalMappingShape(stored.ParameterMappings); got != want {
		t.Errorf("describe -> build changed the mappings\n got: %s\nwant: %s", got, want)
	}

	stmt, _, _ := buildExternalCallFrom(t, "call external action Odata.Bug1073.RunCommand(additional = empty, note = empty);")
	action := runCommandContract(t)
	if err := checkExternalActionParameters(externalCall{stmt: stmt, flow: "Odata.ACT_Probe"}, action); err == nil {
		t.Error("leaving the required `command` out was accepted")
	}
}
