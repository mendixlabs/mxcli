// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestMicroflowActionToGen_LogAndVariableActions guards against the CE0008
// ("No action defined") / CE0109 ("Undefined variable") gap: log/declare/set
// actions must convert to a non-nil gen element with the correct storage $Type.
// An unhandled action returns nil → the ActionActivity is written without an
// Action → Studio Pro rejects the microflow.
func TestMicroflowActionToGen_LogAndVariableActions(t *testing.T) {
	cases := []struct {
		name   string
		action microflows.MicroflowAction
		typ    string
	}{
		{"log", &microflows.LogMessageAction{
			LogLevel:        "Info",
			MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "hi"}},
		}, "Microflows$LogMessageAction"},
		{"declare", &microflows.CreateVariableAction{
			VariableName: "X", InitialValue: "0", DataType: &microflows.DecimalType{},
		}, "Microflows$CreateVariableAction"},
		{"set", &microflows.ChangeVariableAction{
			VariableName: "X", Value: "5",
		}, "Microflows$ChangeVariableAction"},
		// List / aggregate / cast / validation actions — storage $Type differs
		// from the Go type name for several of these (verified vs the legacy
		// serializer's case): AggregateListAction → Microflows$AggregateAction,
		// ListOperationAction → Microflows$ListOperationsAction.
		{"cast", &microflows.CastAction{OutputVariable: "O"}, "Microflows$CastAction"},
		{"aggregate", &microflows.AggregateListAction{
			InputVariable: "L", OutputVariable: "S", Function: "Sum", AttributeQualifiedName: "M.E.A",
		}, "Microflows$AggregateAction"},
		{"createlist", &microflows.CreateListAction{
			EntityQualifiedName: "M.E", OutputVariable: "L",
		}, "Microflows$CreateListAction"},
		{"changelist", &microflows.ChangeListAction{
			ChangeVariable: "L", Type: "Add", Value: "$x",
		}, "Microflows$ChangeListAction"},
		{"validationfeedback", &microflows.ValidationFeedbackAction{
			ObjectVariable: "P", AttributeName: "M.E.Code",
			Template: &model.Text{Translations: map[string]string{"en_US": "req"}},
		}, "Microflows$ValidationFeedbackAction"},
		{"listop", &microflows.ListOperationAction{
			OutputVariable: "Out", Operation: &microflows.HeadOperation{ListVariable: "L"},
		}, "Microflows$ListOperationsAction"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := microflowActionToGen(c.action)
			if g == nil {
				t.Fatalf("%s: microflowActionToGen returned nil (would emit an actionless activity → CE0008)", c.name)
			}
			if g.TypeName() != c.typ {
				t.Errorf("%s: $Type = %q, want %q", c.name, g.TypeName(), c.typ)
			}
		})
	}
}

// TestMicroflowActionToGen_ClientAndRestActions guards the same CE0008/CE0109 gap
// for the page/message/REST actions: each must convert to a non-nil gen element with
// the correct storage $Type. The $Type differs from the Go type name for the page
// actions (verified vs the legacy serializer): ShowPageAction → Microflows$ShowFormAction,
// ClosePageAction → Microflows$CloseFormAction.
func TestMicroflowActionToGen_ClientAndRestActions(t *testing.T) {
	cases := []struct {
		name   string
		action microflows.MicroflowAction
		typ    string
	}{
		{"showpage", &microflows.ShowPageAction{
			PageName: "M.Home",
			PageParameterMappings: []*microflows.PageParameterMapping{
				{Parameter: "M.Home.Obj", Argument: "$Obj"},
			},
		}, "Microflows$ShowFormAction"},
		{"closepage", &microflows.ClosePageAction{NumberOfPages: 1}, "Microflows$CloseFormAction"},
		{"showhomepage", &microflows.ShowHomePageAction{}, "Microflows$ShowHomePageAction"},
		{"showmessage", &microflows.ShowMessageAction{
			Type: "Information", Blocking: true,
			Template:           &model.Text{Translations: map[string]string{"en_US": "done {1}"}},
			TemplateParameters: []string{"$x"},
		}, "Microflows$ShowMessageAction"},
		{"restcall", &microflows.RestCallAction{
			OutputVariable: "Resp",
			HttpConfiguration: &microflows.HttpConfiguration{
				HttpMethod:       microflows.HttpMethodGet,
				LocationTemplate: "https://api/{1}",
				LocationParams:   []string{"$id"},
				CustomHeaders: []*microflows.HttpHeader{
					{Name: "Accept", Value: "application/json"},
				},
			},
			ResultHandling: &microflows.ResultHandlingString{VariableName: "Resp"},
		}, "Microflows$RestCallAction"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := microflowActionToGen(c.action)
			if g == nil {
				t.Fatalf("%s: microflowActionToGen returned nil (would emit an actionless activity → CE0008)", c.name)
			}
			if g.TypeName() != c.typ {
				t.Errorf("%s: $Type = %q, want %q", c.name, g.TypeName(), c.typ)
			}
			// The directly-built REST/FormSettings subtrees must also encode without
			// error (guards a missing list marker / mistyped property panic).
			if _, err := (&codec.Encoder{}).Encode(g); err != nil {
				t.Errorf("%s: encode: %v", c.name, err)
			}
		})
	}
}

// TestListOperationToGen_StorageNames guards the list-operation sub-element
// storage names and (critically) BSON keys: the gen ListOperation types use the
// wrong keys (ListVariableName/SecondListOrObjectVariableName), so these are
// built directly with the verified legacy keys ListName / SecondListOrObjectName
// under the wrapper's "NewOperation" / "ResultVariableName".
func TestListOperationToGen_StorageNames(t *testing.T) {
	cases := []struct {
		name string
		op   microflows.ListOperation
		typ  string
	}{
		{"head", &microflows.HeadOperation{ListVariable: "L"}, "Microflows$Head"},
		{"tail", &microflows.TailOperation{ListVariable: "L"}, "Microflows$Tail"},
		{"find", &microflows.FindOperation{ListVariable: "L", Expression: "x"}, "Microflows$FindByExpression"},
		{"filter", &microflows.FilterOperation{ListVariable: "L", Expression: "x"}, "Microflows$FilterByExpression"},
		{"sort", &microflows.SortOperation{ListVariable: "L"}, "Microflows$Sort"},
		{"union", &microflows.UnionOperation{ListVariable1: "A", ListVariable2: "B"}, "Microflows$Union"},
		{"intersect", &microflows.IntersectOperation{ListVariable1: "A", ListVariable2: "B"}, "Microflows$Intersect"},
		{"subtract", &microflows.SubtractOperation{ListVariable1: "A", ListVariable2: "B"}, "Microflows$Subtract"},
		{"contains", &microflows.ContainsOperation{ListVariable: "L", ObjectVariable: "O"}, "Microflows$Contains"},
		{"equals", &microflows.EqualsOperation{ListVariable1: "A", ListVariable2: "B"}, "Microflows$Equals"},
		{"findby", &microflows.FindByAttributeOperation{ListVariable: "L", Attribute: "M.E.A"}, "Microflows$Find"},
		{"filterby", &microflows.FilterByAttributeOperation{ListVariable: "L", Attribute: "M.E.A"}, "Microflows$Filter"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := listOperationToGen(c.op)
			if e == nil {
				t.Fatalf("%s: listOperationToGen returned nil", c.name)
			}
			if e.TypeName() != c.typ {
				t.Errorf("%s: $Type = %q, want %q", c.name, e.TypeName(), c.typ)
			}
			// The BSON key must be ListName, never the gen ListVariableName.
			var hasListName, hasWrong bool
			for _, p := range e.Properties() {
				switch p.Name() {
				case "ListName":
					hasListName = true
				case "ListVariableName", "SecondListOrObjectVariableName":
					hasWrong = true
				}
			}
			if !hasListName {
				var names []string
				for _, p := range e.Properties() {
					names = append(names, p.Name())
				}
				t.Errorf("%s: missing ListName property (got %v)", c.name, names)
			}
			if hasWrong {
				t.Errorf("%s: emits gen storage key instead of verified key", c.name)
			}
		})
	}
}

// TestMicroflowActionToGen_ExecuteDatabaseQuery guards 05: EXECUTE DATABASE QUERY
// must convert to a DatabaseConnector$ExecuteDatabaseQueryAction (else CE0008).
func TestMicroflowActionToGen_ExecuteDatabaseQuery(t *testing.T) {
	g := microflowActionToGen(&microflows.ExecuteDatabaseQueryAction{
		Query:              "DbTest.Conn.GetAll",
		OutputVariableName: "Rows",
		ParameterMappings:  []*microflows.DatabaseQueryParameterMapping{{ParameterName: "p", Value: "$x"}},
	})
	if g == nil {
		t.Fatal("nil action (CE0008 regression)")
	}
	if g.TypeName() != "DatabaseConnector$ExecuteDatabaseQueryAction" {
		t.Errorf("$Type = %q, want DatabaseConnector$ExecuteDatabaseQueryAction", g.TypeName())
	}
}
