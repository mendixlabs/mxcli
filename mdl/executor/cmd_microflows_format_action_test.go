// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/langver"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	mdltypes "github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// =============================================================================
// formatAction — CRUD actions
// =============================================================================

func TestFormatAction_CreateObject_Simple(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CreateObjectAction{
		EntityQualifiedName: "MyModule.Customer",
		OutputVariable:      "NewCustomer",
	}
	got := e.formatAction(action, nil, nil)
	if got != "$NewCustomer = create MyModule.Customer;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_CreateObject_WithMembers(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CreateObjectAction{
		EntityQualifiedName: "MyModule.Customer",
		OutputVariable:      "NewCustomer",
		InitialMembers: []*microflows.MemberChange{
			{AttributeQualifiedName: "MyModule.Customer.Name", Value: "'John'"},
			{AttributeQualifiedName: "MyModule.Customer.Age", Value: "25"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "$NewCustomer = create MyModule.Customer (Name = 'John', Age = 25);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_CreateObject_WithAssociationMember(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CreateObjectAction{
		EntityQualifiedName: "MyModule.Order",
		OutputVariable:      "NewOrder",
		InitialMembers: []*microflows.MemberChange{
			{AssociationQualifiedName: "MyModule.Order_Customer", Value: "$Customer"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "$NewOrder = create MyModule.Order (Order_Customer = $Customer);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_CreateObject_FallbackEntityID(t *testing.T) {
	e := newTestExecutor()
	entityNames := map[model.ID]string{mkID("e1"): "MyModule.Product"}
	action := &microflows.CreateObjectAction{
		EntityID:       mkID("e1"),
		OutputVariable: "NewProduct",
	}
	got := e.formatAction(action, entityNames, nil)
	if got != "$NewProduct = create MyModule.Product;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeObject(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeObjectAction{
		ChangeVariable: "Customer",
		Changes: []*microflows.MemberChange{
			{AttributeQualifiedName: "MyModule.Customer.Name", Value: "'Jane'"},
		},
	}
	got := e.formatAction(action, nil, nil)
	if got != "change $Customer (Name = 'Jane');" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeObject_NoChanges(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeObjectAction{ChangeVariable: "Obj"}
	got := e.formatAction(action, nil, nil)
	if got != "change $Obj;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeObject_WithRefresh(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeObjectAction{
		ChangeVariable:  "Customer",
		RefreshInClient: true,
		Changes: []*microflows.MemberChange{
			{AttributeQualifiedName: "MyModule.Customer.Name", Value: "'Jane'"},
		},
	}
	got := e.formatAction(action, nil, nil)
	if got != "change $Customer (Name = 'Jane') refresh;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeObject_NoChangesWithRefresh(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeObjectAction{ChangeVariable: "Obj", RefreshInClient: true}
	got := e.formatAction(action, nil, nil)
	if got != "change $Obj refresh;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_DeleteObject(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.DeleteObjectAction{DeleteVariable: "Customer"}
	got := e.formatAction(action, nil, nil)
	if got != "delete $Customer;" {
		t.Errorf("got %q", got)
	}
}

// TestFormatAction_CommitObjects covers all four stored combinations of the two
// Commit-activity flags.
//
// The asymmetry is the point (#895): events ON is Mendix's default and renders
// as nothing, events OFF is the deviation and must be spelled out. Rendering it
// the other way round is what let a Studio Pro default commit describe as
// something that re-executed into its opposite.
func TestFormatAction_CommitObjects(t *testing.T) {
	e := newTestExecutor()
	tests := []struct {
		name   string
		action *microflows.CommitObjectsAction
		want   string
	}{
		{
			name:   "default (events on) renders bare",
			action: &microflows.CommitObjectsAction{CommitVariable: "Order", WithEvents: true},
			want:   "commit $Order;",
		},
		{
			name:   "events off is spelled out",
			action: &microflows.CommitObjectsAction{CommitVariable: "Order"},
			want:   "commit $Order without events;",
		},
		{
			name:   "events on with refresh",
			action: &microflows.CommitObjectsAction{CommitVariable: "Order", WithEvents: true, RefreshInClient: true},
			want:   "commit $Order refresh;",
		},
		{
			name:   "events off with refresh",
			action: &microflows.CommitObjectsAction{CommitVariable: "Order", RefreshInClient: true},
			want:   "commit $Order without events refresh;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.formatAction(tt.action, nil, nil); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatAction_RefreshInClient_AllActivities guards the #407 sibling: the
// REFRESH modifier existed on change and rollback but not on delete or create,
// so a Studio Pro activity carrying the flag described without it and lost it on
// the next exec. Measured before the fix on a real 11.13 project: stored
// refresh=true -> `delete $Obj;` -> re-exec -> refresh=false.
func TestFormatAction_RefreshInClient_AllActivities(t *testing.T) {
	e := newTestExecutor()
	tests := []struct {
		name   string
		action microflows.MicroflowAction
		want   string
	}{
		{"delete", &microflows.DeleteObjectAction{DeleteVariable: "Customer", RefreshInClient: true}, "delete $Customer refresh;"},
		{"rollback", &microflows.RollbackObjectAction{RollbackVariable: "Order", RefreshInClient: true}, "rollback $Order refresh;"},
		{
			"create",
			&microflows.CreateObjectAction{OutputVariable: "New", EntityQualifiedName: "Mod.Order", RefreshInClient: true},
			"$New = create Mod.Order refresh;",
		},
		{
			"change",
			&microflows.ChangeObjectAction{ChangeVariable: "Order", RefreshInClient: true},
			"change $Order refresh;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.formatAction(tt.action, nil, nil); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatAction_RollbackObject(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RollbackObjectAction{RollbackVariable: "Order"}
	got := e.formatAction(action, nil, nil)
	if got != "rollback $Order;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_RollbackObject_Refresh(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RollbackObjectAction{RollbackVariable: "Order", RefreshInClient: true}
	got := e.formatAction(action, nil, nil)
	if got != "rollback $Order refresh;" {
		t.Errorf("got %q", got)
	}
}

// =============================================================================
// formatAction — Variable actions
// =============================================================================

func TestFormatAction_CreateVariable(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CreateVariableAction{
		VariableName: "Counter",
		DataType:     &microflows.IntegerType{},
		InitialValue: "0",
	}
	got := e.formatAction(action, nil, nil)
	if got != "declare $Counter Integer = 0;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_CreateVariable_NoInitial(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CreateVariableAction{
		VariableName: "Name",
		DataType:     &microflows.StringType{},
	}
	got := e.formatAction(action, nil, nil)
	if got != "declare $Name String = empty;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeVariable_Simple(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeVariableAction{
		VariableName: "Counter",
		Value:        "$Counter + 1",
	}
	got := e.formatAction(action, nil, nil)
	if got != "set $Counter = $Counter + 1;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeVariable_WithDollarPrefix(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeVariableAction{
		VariableName: "$Counter",
		Value:        "42",
	}
	got := e.formatAction(action, nil, nil)
	if got != "set $Counter = 42;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeVariable_XPath(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeVariableAction{
		VariableName: "$Product/Price",
		Value:        "9.99",
	}
	got := e.formatAction(action, nil, nil)
	if got != "change $Product (Price = 9.99);" {
		t.Errorf("got %q", got)
	}
}

// =============================================================================
// formatAction — List actions
// =============================================================================

func TestFormatAction_CreateList(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CreateListAction{
		EntityQualifiedName: "MyModule.Order",
		OutputVariable:      "OrderList",
	}
	got := e.formatAction(action, nil, nil)
	if got != "$OrderList = create list of MyModule.Order;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeList_Add(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeListAction{
		ChangeVariable: "OrderList",
		Type:           microflows.ChangeListTypeAdd,
		Value:          "$NewOrder",
	}
	got := e.formatAction(action, nil, nil)
	if got != "add $NewOrder to $OrderList;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeList_Remove(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeListAction{
		ChangeVariable: "OrderList",
		Type:           microflows.ChangeListTypeRemove,
		Value:          "$OldOrder",
	}
	got := e.formatAction(action, nil, nil)
	if got != "remove $OldOrder from $OrderList;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeList_Clear(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeListAction{
		ChangeVariable: "OrderList",
		Type:           microflows.ChangeListTypeClear,
	}
	got := e.formatAction(action, nil, nil)
	if got != "clear $OrderList;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ChangeList_Set(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeListAction{
		ChangeVariable: "OrderList",
		Type:           microflows.ChangeListTypeSet,
		Value:          "$OtherList",
	}
	got := e.formatAction(action, nil, nil)
	if got != "set $OrderList = $OtherList;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_AggregateList_Count(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.AggregateListAction{
		InputVariable:  "Orders",
		OutputVariable: "Total",
		Function:       microflows.AggregateFunctionCount,
	}
	got := e.formatAction(action, nil, nil)
	if got != "$Total = count $Orders;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_AggregateList_Sum(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.AggregateListAction{
		InputVariable:          "Orders",
		OutputVariable:         "TotalAmount",
		Function:               microflows.AggregateFunctionSum,
		AttributeQualifiedName: "MyModule.Order.Amount",
	}
	got := e.formatAction(action, nil, nil)
	if got != "$TotalAmount = sum $Orders by Amount;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_AggregateList_SumExpression(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.AggregateListAction{
		InputVariable:  "Orders",
		OutputVariable: "TotalTax",
		Function:       microflows.AggregateFunctionSum,
		UseExpression:  true,
		Expression:     "$currentObject/Amount * 0.21",
	}
	got := e.formatAction(action, nil, nil)
	if got != "$TotalTax = sum $Orders of $currentObject/Amount * 0.21;" {
		t.Errorf("got %q", got)
	}
}

// =============================================================================
// formatAction — Call actions
// =============================================================================

func TestFormatAction_MicroflowCall_WithResult(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.MicroflowCallAction{
		ResultVariableName: "Result",
		UseReturnVariable:  true,
		MicroflowCall: &microflows.MicroflowCall{
			Microflow: "MyModule.ProcessOrder",
			ParameterMappings: []*microflows.MicroflowCallParameterMapping{
				{Parameter: "MyModule.ProcessOrder.Order", Argument: "$Order"},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "$Result = call microflow MyModule.ProcessOrder(Order = $Order);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_MicroflowCall_NoResult(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.MicroflowCallAction{
		MicroflowCall: &microflows.MicroflowCall{
			Microflow: "MyModule.DoSomething",
		},
	}
	got := e.formatAction(action, nil, nil)
	if got != "call microflow MyModule.DoSomething();" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_JavaActionCall(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaActionCallAction{
		JavaAction:         "MyModule.SendEmail",
		ResultVariableName: "Success",
		UseReturnVariable:  true,
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{
				Parameter: "MyModule.SendEmail.To",
				Value: &microflows.ExpressionBasedCodeActionParameterValue{
					Expression: "$Customer/Email",
				},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "$Success = call java action MyModule.SendEmail(To = $Customer/Email);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaActionCall_EmptyParameterValues(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaActionCallAction{
		JavaAction: "MyModule.Recalculate",
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{
				Parameter: "MyModule.Recalculate.CompanyId",
				Value:     &microflows.BasicCodeActionParameterValue{Argument: ""},
			},
			{
				Parameter: "MyModule.Recalculate.RecalculateAll",
				Value:     &microflows.BasicCodeActionParameterValue{Argument: "true"},
			},
			{
				Parameter: "MyModule.Recalculate.Callback",
				Value:     &microflows.MicroflowParameterValue{Microflow: ""},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "call java action MyModule.Recalculate(CompanyId = empty, RecalculateAll = true, Callback = empty);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_CallExternal(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CallExternalAction{
		ConsumedODataService: "MyModule.OrderService",
		Name:                 "GetOrders",
		ResultVariableName:   "Orders",
		UseReturnVariable:    true,
	}
	got := e.formatAction(action, nil, nil)
	want := "$Orders = call external action MyModule.OrderService.GetOrders();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// =============================================================================
// formatAction — UI actions
// =============================================================================

func TestFormatAction_ShowPage(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ShowPageAction{
		PageName: "MyModule.CustomerEdit",
	}
	got := e.formatAction(action, nil, nil)
	if got != "show page MyModule.CustomerEdit;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ShowPage_WithParams(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ShowPageAction{
		PageName: "MyModule.OrderDetail",
		PageParameterMappings: []*microflows.PageParameterMapping{
			{Parameter: "MyModule.OrderDetail.Order", Argument: "$Order"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "show page MyModule.OrderDetail(Order = $Order);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A page title override is described, so describe -> exec keeps it and taking
// it out of a create or modify is a difference (ako/mxcli#869).
func TestFormatAction_ShowPage_WithTitle(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ShowPageAction{
		PageName: "MyModule.OrderDetail",
		PageParameterMappings: []*microflows.PageParameterMapping{
			{Parameter: "MyModule.OrderDetail.Order", Argument: "$Order"},
		},
		OverridePageTitle: &model.Text{Translations: map[string]string{"en_US": "Order's details"}},
	}
	got := e.formatAction(action, nil, nil)
	want := "show page MyModule.OrderDetail(Order = $Order) with title = 'Order''s details';"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// An override with no text states nothing the statement can carry.
	action.OverridePageTitle = &model.Text{}
	if got := e.formatAction(action, nil, nil); got != "show page MyModule.OrderDetail(Order = $Order);" {
		t.Errorf("empty override: got %q", got)
	}
}

// "Number of pages to close" is described, so an action that closes the
// current page before opening reads differently from one that closes none,
// and describe -> exec keeps it (mendixlabs/mxcli#1617).
func TestFormatAction_ShowPage_NumberOfPagesToClose(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ShowPageAction{PageName: "MyFirstModule.Booking_Overview", NumberOfPagesToClose: "1"}
	if got, want := e.formatAction(action, nil, nil),
		"show page MyFirstModule.Booking_Overview with NumberOfPagesToClose = 1;"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// With a title, both settings share the one `with` list.
	action.OverridePageTitle = &model.Text{Translations: map[string]string{"en_US": "Bookings"}}
	action.NumberOfPagesToClose = "$Count + 1"
	if got, want := e.formatAction(action, nil, nil),
		"show page MyFirstModule.Booking_Overview with title = 'Bookings', NumberOfPagesToClose = $Count + 1;"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// CONTROL: the stored default, "", closes nothing and says nothing.
	action.OverridePageTitle, action.NumberOfPagesToClose = nil, ""
	if got := e.formatAction(action, nil, nil); got != "show page MyFirstModule.Booking_Overview;" {
		t.Errorf("no pages to close: got %q", got)
	}
}

func TestFormatAction_ClosePage(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ClosePageAction{NumberOfPages: 1}
	got := e.formatAction(action, nil, nil)
	if got != "close page;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ClosePage_Multiple(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ClosePageAction{NumberOfPages: 3}
	got := e.formatAction(action, nil, nil)
	if got != "close page 3;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ShowMessage(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ShowMessageAction{
		Type: microflows.MessageTypeWarning,
		Template: &model.Text{
			Translations: map[string]string{"en_US": "Order saved"},
		},
	}
	got := e.formatAction(action, nil, nil)
	if got != "show message 'Order saved' type Warning;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ShowMessage_EscapesQuotes(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ShowMessageAction{
		Type: microflows.MessageTypeInformation,
		Template: &model.Text{
			Translations: map[string]string{"en_US": "It's done"},
		},
	}
	got := e.formatAction(action, nil, nil)
	if got != "show message 'It''s done' type Information;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_ShowMessage_EscapesMultiline(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ShowMessageAction{
		Type: microflows.MessageTypeInformation,
		Template: &model.Text{
			Translations: map[string]string{"en_US": "Line 1\nLine 2\tTabbed"},
		},
	}
	// mdl 1, the default since the freeze: the break is in the literal.
	if got := e.formatAction(action, nil, nil); got != "show message 'Line 1\nLine 2\tTabbed' type Information;" {
		t.Errorf("mdl 1: got %q", got)
	}
	// Asked for mdl 0 (`describe --mdl 0`): the mdl 0 escapes.
	e.SetDescribeLanguage(langver.V0)
	if got := e.formatAction(action, nil, nil); got != "show message 'Line 1\\nLine 2\\tTabbed' type Information;" {
		t.Errorf("mdl 0: got %q", got)
	}
}

func TestFormatAction_DownloadFile(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.DownloadFileAction{
		FileDocument:  "GeneratedReport",
		ShowInBrowser: true,
	}

	got := e.formatAction(action, nil, nil)
	want := "download file $GeneratedReport show in browser;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_DownloadFileWithoutBrowserFlag(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.DownloadFileAction{
		FileDocument: "GeneratedReport",
	}

	got := e.formatAction(action, nil, nil)
	want := "download file $GeneratedReport;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_ValidationFeedback(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ValidationFeedbackAction{
		ObjectVariable: "Customer",
		AttributeName:  "MyModule.Customer.Email",
		Template: &model.Text{
			Translations: map[string]string{"en_US": "Email is required"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "validation feedback $Customer/Email message 'Email is required';"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// What describe writes for a validation feedback on an attribute named Error
// must parse back: `$E/Error message` used to lex as the `error message` clause
// keyword, so describe output for such a microflow failed `check` with "missing
// MESSAGE" (mendixlabs/mxcli#1720).
func TestFormatAction_ValidationFeedback_MemberNamedErrorReparses(t *testing.T) {
	e := newTestExecutor()
	for _, attr := range []string{"Error", "error", "Message", "ErrorMessage"} {
		action := &microflows.ValidationFeedbackAction{
			ObjectVariable: "Entity2",
			AttributeName:  "ExamplesModule.Entity2." + attr,
			Template: &model.Text{
				Translations: map[string]string{"en_US": "not good"},
			},
		}
		line := e.formatAction(action, nil, nil)
		src := "create or modify microflow ExamplesModule.ACT ($Entity2: ExamplesModule.Entity2)\nbegin\n  " +
			line + "\n  return;\nend;"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("%s: describe output does not parse: %v\n%s", attr, errs, src)
		}
		vf := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.ValidationFeedbackStmt)
		segs := vf.AttributePath.Segments
		if len(segs) != 1 || segs[0].Name != attr {
			t.Errorf("%s: read back as %v", attr, segs)
		}
	}
}

func TestFormatAction_ValidationFeedback_ObjectOnlyTarget(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ValidationFeedbackAction{
		ObjectVariable: "Customer",
		Template: &model.Text{
			Translations: map[string]string{"en_US": "Select a customer"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "validation feedback $Customer message 'Select a customer';"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_ValidationFeedback_AssociationTarget(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ValidationFeedbackAction{
		ObjectVariable:  "OrderForm",
		AssociationName: "Sales.OrderForm_Customer",
		Template: &model.Text{
			Translations: map[string]string{"en_US": "Select a customer"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "validation feedback $OrderForm/Sales.OrderForm_Customer message 'Select a customer';"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_LogMessage(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.LogMessageAction{
		LogLevel:    microflows.LogLevelWarning,
		LogNodeName: "'OrderService'",
		MessageTemplate: &model.Text{
			Translations: map[string]string{"en_US": "Processing order"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "log warning node 'OrderService' 'Processing order';"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_LogMessage_WithTemplateParams(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.LogMessageAction{
		LogLevel:    microflows.LogLevelInfo,
		LogNodeName: "'App'",
		MessageTemplate: &model.Text{
			Translations: map[string]string{"en_US": "Order {1} for {2}"},
		},
		TemplateParameters: []string{"$OrderNumber", "$CustomerName"},
	}
	got := e.formatAction(action, nil, nil)
	want := "log node 'App' 'Order {1} for {2}' with ({1} = $OrderNumber, {2} = $CustomerName);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_LogMessage_EscapesMultiline(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.LogMessageAction{
		LogLevel:    microflows.LogLevelInfo,
		LogNodeName: "'App'",
		MessageTemplate: &model.Text{
			Translations: map[string]string{"en_US": "Line 1\nLine 2"},
		},
	}
	if got, want := e.formatAction(action, nil, nil), "log node 'App' 'Line 1\nLine 2';"; got != want {
		t.Errorf("mdl 1: got %q, want %q", got, want)
	}
	e.SetDescribeLanguage(langver.V0)
	if got, want := e.formatAction(action, nil, nil), "log node 'App' 'Line 1\\nLine 2';"; got != want {
		t.Errorf("mdl 0: got %q, want %q", got, want)
	}
}

func TestFormatAction_LogMessage_NodeExpression(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.LogMessageAction{
		LogLevel:    microflows.LogLevelInfo,
		LogNodeName: "@MyModule.SecurityLogNode",
		MessageTemplate: &model.Text{
			Translations: map[string]string{"en_US": "User added"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "log node @MyModule.SecurityLogNode 'User added';"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_UnknownAction(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.UnknownAction{TypeName: "SomeNewAction"}
	got := e.formatAction(action, nil, nil)
	if got != "-- Unsupported action type: SomeNewAction" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_Nil(t *testing.T) {
	e := newTestExecutor()
	got := e.formatAction(nil, nil, nil)
	if got != "-- Empty action" {
		t.Errorf("got %q", got)
	}
}

// =============================================================================
// formatAction — Retrieve actions
// =============================================================================

func TestFormatAction_Retrieve_Database(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RetrieveAction{
		OutputVariable: "Customers",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "MyModule.Customer",
		},
	}
	got := e.formatAction(action, nil, nil)
	if got != "retrieve $Customers from MyModule.Customer;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_Retrieve_WithXPath(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RetrieveAction{
		OutputVariable: "ActiveCustomers",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "MyModule.Customer",
			XPathConstraint:     "[IsActive = true()]",
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "retrieve $ActiveCustomers from MyModule.Customer\n    where IsActive = true();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_Retrieve_WithLimit(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RetrieveAction{
		OutputVariable: "First",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "MyModule.Customer",
			Range:               &microflows.Range{RangeType: microflows.RangeTypeFirst},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "retrieve $First from MyModule.Customer\n    first;" // the object range (#734)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_Retrieve_WithSorting(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RetrieveAction{
		OutputVariable: "Sorted",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "MyModule.Customer",
			Sorting: []*microflows.SortItem{
				{AttributeQualifiedName: "MyModule.Customer.Name", Direction: microflows.SortDirectionAscending},
				{AttributeQualifiedName: "MyModule.Customer.Age", Direction: microflows.SortDirectionDescending},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "retrieve $Sorted from MyModule.Customer\n    sort by MyModule.Customer.Name asc, MyModule.Customer.Age desc;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_Retrieve_Association(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RetrieveAction{
		OutputVariable: "Address",
		Source: &microflows.AssociationRetrieveSource{
			StartVariable:            "Customer",
			AssociationQualifiedName: "MyModule.Customer_Address",
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "retrieve $Address from $Customer/MyModule.Customer_Address;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A DatabaseRetrieveSource always renders in database form, even when its XPath
// is a reverse-association constraint — it must NOT collapse to the association
// shorthand `from $var/Assoc`, which is the in-memory retrieve form. Rendering a
// database retrieve that way made the two indistinguishable (issue #726).
func TestFormatAction_Retrieve_ReverseAssociationDatabaseSourceUsesDatabaseForm(t *testing.T) {
	e := newTestExecutor()
	e.backend = reverseAssociationBackend(t)
	action := &microflows.RetrieveAction{
		OutputVariable: "Domains",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "SampleRuntime.Domain",
			XPathConstraint:     "[SampleRuntime.Domain_Runtime = $Runtime]",
			Range:               &microflows.Range{RangeType: microflows.RangeTypeAll},
		},
	}

	got := e.formatAction(action, nil, nil)
	want := "retrieve $Domains from SampleRuntime.Domain\n    where SampleRuntime.Domain_Runtime = $Runtime;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_Retrieve_ReverseAssociationRequiresSimpleAllRange(t *testing.T) {
	e := newTestExecutor()
	e.backend = reverseAssociationBackend(t)
	action := &microflows.RetrieveAction{
		OutputVariable: "Domains",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "SampleRuntime.Domain",
			XPathConstraint:     "[SampleRuntime.Domain_Runtime = $Runtime]",
			Range:               &microflows.Range{RangeType: microflows.RangeTypeFirst},
		},
	}

	got := e.formatAction(action, nil, nil)
	want := "retrieve $Domains from SampleRuntime.Domain\n    where SampleRuntime.Domain_Runtime = $Runtime\n    first;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_Retrieve_ReverseAssociationRequiresMatchingEntity(t *testing.T) {
	e := newTestExecutor()
	e.backend = reverseAssociationBackend(t)
	action := &microflows.RetrieveAction{
		OutputVariable: "Domains",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "SampleRuntime.Runtime",
			XPathConstraint:     "[SampleRuntime.Domain_Runtime = $Runtime]",
		},
	}

	got := e.formatAction(action, nil, nil)
	want := "retrieve $Domains from SampleRuntime.Runtime\n    where SampleRuntime.Domain_Runtime = $Runtime;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseReverseAssociationXPathRejectsComplexPredicates(t *testing.T) {
	tests := []string{
		"[SampleRuntime.Domain_Runtime = $Runtime][Active = true]",
		"[SampleRuntime.Domain_Runtime != $Runtime]",
		"[SampleRuntime.Domain_Runtime = $Runtime/Other.Assoc]",
		"[SampleRuntime.Domain_Runtime = 'literal']",
		"[_SampleRuntime.Domain_Runtime = $Runtime]",
		"[SampleRuntime._Domain_Runtime = $Runtime]",
		"[SampleRuntime.Domain_Runtime = $_Runtime]",
		"SampleRuntime.Domain_Runtime = $Runtime",
	}

	for _, tt := range tests {
		if assoc, start, ok := parseReverseAssociationXPath(tt); ok {
			t.Fatalf("parseReverseAssociationXPath(%q) = %q, %q, true; want false", tt, assoc, start)
		}
	}
}

func reverseAssociationBackend(t *testing.T) *mock.MockBackend {
	t.Helper()
	moduleID := model.ID("sample-runtime-module")
	return &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name != "SampleRuntime" {
				return nil, nil
			}
			return &model.Module{
				BaseElement: model.BaseElement{ID: moduleID},
				Name:        "SampleRuntime",
			}, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id != moduleID {
				return nil, nil
			}
			return &domainmodel.DomainModel{
				ContainerID: moduleID,
				Entities: []*domainmodel.Entity{
					{
						BaseElement: model.BaseElement{ID: "domain-entity"},
						Name:        "Domain",
					},
					{
						BaseElement: model.BaseElement{ID: "runtime-entity"},
						Name:        "Runtime",
					},
				},
				Associations: []*domainmodel.Association{
					{
						Name:     "Domain_Runtime",
						ParentID: "domain-entity",
						ChildID:  "runtime-entity",
						Type:     domainmodel.AssociationTypeReference,
					},
				},
			}, nil
		},
	}
}

// --- OBS-6: Numeric return values should not get $ prefix ---

func TestFormatActivity_ReturnNumericLiteral(t *testing.T) {
	e := newTestExecutor()
	activity := &microflows.EndEvent{
		ReturnValue: "42",
	}
	got := e.formatActivity(activity, nil, nil)
	want := "return 42;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatActivity_ReturnNegativeNumericLiteral(t *testing.T) {
	e := newTestExecutor()
	activity := &microflows.EndEvent{
		ReturnValue: "-1",
	}
	got := e.formatActivity(activity, nil, nil)
	want := "return -1;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatActivity_ReturnDecimalLiteral(t *testing.T) {
	e := newTestExecutor()
	activity := &microflows.EndEvent{
		ReturnValue: "3.14",
	}
	got := e.formatActivity(activity, nil, nil)
	want := "return 3.14;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatActivity_ReturnIdentifier(t *testing.T) {
	e := newTestExecutor()
	activity := &microflows.EndEvent{
		ReturnValue: "MyVar",
	}
	got := e.formatActivity(activity, nil, nil)
	want := "return $MyVar;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- OBS-6: isNumericLiteral ---

func TestIsNumericLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"42", true},
		{"-1", true},
		{"3.14", true},
		{"-0.5", true},
		{"0", true},
		{"", false},
		{"-", false},
		{"abc", false},
		{"$42", false},
		{"1.2.3", false},
		{"42abc", false},
		{".", false},
		{"-.", false},
		{"5.", false},
		{".5", true},
		{"-.5", true},
	}
	for _, tt := range tests {
		got := isNumericLiteral(tt.input)
		if got != tt.want {
			t.Errorf("isNumericLiteral(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// --- OBS-10: getActionErrorHandlingType with NanoflowCallAction ---

func TestGetActionErrorHandlingType_NanoflowCallAction(t *testing.T) {
	activity := &microflows.ActionActivity{
		Action: &microflows.NanoflowCallAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeContinue,
		},
	}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeContinue {
		t.Errorf("got %q, want %q", got, microflows.ErrorHandlingTypeContinue)
	}
}

func TestGetActionErrorHandlingType_NanoflowCallAction_Abort(t *testing.T) {
	activity := &microflows.ActionActivity{
		Action: &microflows.NanoflowCallAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeAbort,
		},
	}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeAbort {
		t.Errorf("got %q, want %q", got, microflows.ErrorHandlingTypeAbort)
	}
}

// =============================================================================
// formatAction — JavaScript action call
// =============================================================================

func TestFormatAction_JavaScriptActionCall_Simple(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "MyModule.MyJSAction",
	}
	got := e.formatAction(action, nil, nil)
	want := "call javascript action MyModule.MyJSAction();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_WithReturn(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction:   "MyModule.MyJSAction",
		OutputVariableName: "Result",
		UseReturnVariable:  true,
	}
	got := e.formatAction(action, nil, nil)
	want := "$Result = call javascript action MyModule.MyJSAction();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_WithParams(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "MyModule.MyJSAction",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{
				Parameter: "MyModule.MyJSAction.Input",
				Value: &microflows.ExpressionBasedCodeActionParameterValue{
					Expression: "$MyVar",
				},
			},
		},
		OutputVariableName: "Result",
		UseReturnVariable:  true,
	}
	got := e.formatAction(action, nil, nil)
	want := "$Result = call javascript action MyModule.MyJSAction(Input = $MyVar);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_NilParamValue(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "MyModule.MyJSAction",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{
				Parameter: "MyModule.MyJSAction.Input",
				Value:     nil,
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "call javascript action MyModule.MyJSAction();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_EmptyName(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "",
	}
	got := e.formatAction(action, nil, nil)
	want := "-- JavaScriptAction: missing action reference"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_EmptyNameWithParams(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{Parameter: "Mod.Action.P1"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "-- JavaScriptAction: missing action reference (1 param)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_EmptyParamValues(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "MyModule.MyJSAction",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{
				Parameter: "MyModule.MyJSAction.Input",
				Value:     &microflows.BasicCodeActionParameterValue{Argument: ""},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "call javascript action MyModule.MyJSAction();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_EmptyExpressionParam(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "MyModule.MyJSAction",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{
				Parameter: "MyModule.MyJSAction.Token",
				Value:     &microflows.ExpressionBasedCodeActionParameterValue{Expression: ""},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "call javascript action MyModule.MyJSAction();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_EmptyEntityParam(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "MyModule.MyJSAction",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{
				Parameter: "MyModule.MyJSAction.EntityType",
				Value:     &microflows.EntityTypeCodeActionParameterValue{Entity: ""},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "call javascript action MyModule.MyJSAction();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_MixedEmptyAndPopulatedParams(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction: "MyModule.MyJSAction",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{
				Parameter: "MyModule.MyJSAction.URL",
				Value:     &microflows.ExpressionBasedCodeActionParameterValue{Expression: "'https://example.com'"},
			},
			{
				Parameter: "MyModule.MyJSAction.UseAuthToken",
				Value:     &microflows.BasicCodeActionParameterValue{Argument: ""},
			},
			{
				Parameter: "MyModule.MyJSAction.Timeout",
				Value:     &microflows.ExpressionBasedCodeActionParameterValue{Expression: "30"},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "call javascript action MyModule.MyJSAction(URL = 'https://example.com', Timeout = 30);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_JavaScriptActionCall_WithOutputAndEmptyParam(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.JavaScriptActionCallAction{
		JavaScriptAction:   "MyModule.MyJSAction",
		OutputVariableName: "Result",
		UseReturnVariable:  true,
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{
				Parameter: "MyModule.MyJSAction.Input",
				Value:     &microflows.BasicCodeActionParameterValue{Argument: ""},
			},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "$Result = call javascript action MyModule.MyJSAction();"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetActionErrorHandlingType_JavaScriptActionCallAction(t *testing.T) {
	activity := &microflows.ActionActivity{
		Action: &microflows.JavaScriptActionCallAction{
			ErrorHandlingType: microflows.ErrorHandlingTypeContinue,
		},
	}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeContinue {
		t.Errorf("got %q, want %q", got, microflows.ErrorHandlingTypeContinue)
	}
}

// TestFormatAction_WebServiceCallRendersStoredQualifiedNames pins that DESCRIBE
// prints the three names a SOAP call stores, verbatim, and consults nothing.
//
// All three are BY_NAME_REFERENCE properties — qualified-name strings, never
// element ids. Measured on ako/TestApp (11.14.0), Clients.GetOrders stores
// ImportedService "Clients.OrderSoapClient" and ReturnValueMapping
// "Clients.SoapOrdersImportMapping"; modelsdk/gen agrees, registering
// ImportedWebService and ImportMappingCall.Mapping as codec.RefByName.
//
// Three resolvers used to sit here turning those strings into qualified names
// by looking each one up in the project. They could not match on two counts at
// once — the id compared was already a qualified name, and the unit type asked
// for ("WebServices$ImportedWebService") is stored as
// "WebServices$ImportedServiceImpl" — so every call fell through to its
// fallback, which returned the stored string. The right answer, by accident.
//
// The backend below fails the test if it is asked anything: a lookup returning
// the correct value is indistinguishable from no lookup at all in the output,
// which is precisely how this survived.
func TestFormatAction_WebServiceCallRendersStoredQualifiedNames(t *testing.T) {
	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListRawUnitsByTypeFunc: func(typePrefix string) ([]*mdltypes.RawUnit, error) {
			t.Fatalf("DESCRIBE consulted the project for %q; the stored names are already qualified", typePrefix)
			return nil, nil
		},
		ListExportMappingsFunc: func() ([]*model.ExportMapping, error) {
			t.Fatal("DESCRIBE listed export mappings; the send mapping is stored by qualified name")
			return nil, nil
		},
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) {
			t.Fatal("DESCRIBE listed import mappings; the receive mapping is stored by qualified name")
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(backend), withHierarchy(mkHierarchy()))

	action := &microflows.WebServiceCallAction{
		ServiceID:         "Clients.OrderSoapClient",
		OperationName:     "GetOrder",
		SendMappingID:     "Clients.SoapOrderExportMapping",
		ReceiveMappingID:  "Clients.SoapOrdersImportMapping",
		OutputVariable:    "Orders",
		UseReturnVariable: true,
	}
	got := formatAction(ctx, action, nil, nil)
	want := "$Orders = call web service Clients.OrderSoapClient\noperation GetOrder\n" +
		"send mapping Clients.SoapOrderExportMapping\nreceive mapping Clients.SoapOrdersImportMapping;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatAction_WebServiceCallQuotesNamesThatAreNotIdentifiers — a dangling
// or malformed reference is still printed, quoted so the line parses back.
func TestFormatAction_WebServiceCallQuotesNamesThatAreNotIdentifiers(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return true }}),
		withHierarchy(mkHierarchy()))

	action := &microflows.WebServiceCallAction{
		ServiceID:        "dangling-service-id",
		OperationName:    "FetchOrders",
		SendMappingID:    "dangling-send-id",
		ReceiveMappingID: "dangling-receive-id",
		OutputVariable:   "Root",
	}
	got := formatAction(ctx, action, nil, nil)
	want := "$Root = call web service 'dangling-service-id'\noperation FetchOrders\nsend mapping 'dangling-send-id'\nreceive mapping 'dangling-receive-id';"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_WebServiceCallRaw(t *testing.T) {
	raw, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: "soap-action"},
		{Key: "$Type", Value: "Microflows$CallWebServiceAction"},
		{Key: "OperationName", Value: "FetchOrders"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := formatAction(nil, &microflows.WebServiceCallAction{
		OutputVariable: "Root",
		RawBSON:        raw,
	}, nil, nil)
	if !strings.HasPrefix(got, "$Root = call web service raw '") {
		t.Fatalf("got %q", got)
	}
}

// TestFormatAction_WebServiceCallArgumentsAndSendMapping — DESCRIBE renders both
// request-body forms, and renders them so they parse back.
//
// The argument list shows only the parameter NAME; the stored ParameterPath
// ("http%3A//www.example.com/:GetOrder|OrderId") is rebuilt from the operation
// document on the way back in. Putting the path in the script would fail every
// readability test the language is held to — and `send rest request` already
// makes the same move, showing `code` where the model holds Mod.Svc.Op.code.
func TestFormatAction_WebServiceCallArgumentsAndSendMapping(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return true }}),
		withHierarchy(mkHierarchy()))

	args := formatAction(ctx, &microflows.WebServiceCallAction{
		ServiceID:     "Clients.OrderSoapClient",
		OperationName: "GetOrder",
		Arguments: []microflows.WebServiceArgument{
			{Name: "OrderId", Path: "http%3A//www.example.com/:GetOrder|OrderId", Expression: "$Customer/OrderId", Checked: true},
			{Name: "Verbose", Path: "http%3A//www.example.com/:GetOrder|Verbose", Expression: "true", Checked: true},
		},
		ReceiveMappingID: "Clients.SoapOrdersImportMapping",
		OutputVariable:   "Orders",
	}, nil, nil)
	want := "$Orders = call web service Clients.OrderSoapClient\n" +
		"operation GetOrder (OrderId = $Customer/OrderId, Verbose = true)\n" +
		"receive mapping Clients.SoapOrdersImportMapping;"
	if args != want {
		t.Errorf("arguments form:\n got %q\nwant %q", args, want)
	}

	send := formatAction(ctx, &microflows.WebServiceCallAction{
		ServiceID:           "Clients.OrderSoapClient",
		OperationName:       "SaveOrder",
		SendMappingID:       "Clients.SoapOrderExportMapping",
		SendMappingVariable: "NewSaveOrder",
	}, nil, nil)
	wantSend := "call web service Clients.OrderSoapClient\n" +
		"operation SaveOrder\n" +
		"send mapping Clients.SoapOrderExportMapping from $NewSaveOrder;"
	if send != wantSend {
		t.Errorf("send mapping form:\n got %q\nwant %q", send, wantSend)
	}
}

// TestFormatAction_WebServiceCallArgumentWithoutAName renders no argument list
// at all rather than a partial one.
//
// A ParameterPath with no "|" yields no name on the way in — a shape no
// reference document carries — and emitting `operation X ( = expr)` would write
// a DIFFERENT path back. The action keeps the raw fallback instead, which is
// decided by the reader; this is the belt to that braces.
func TestFormatAction_WebServiceCallArgumentWithoutAName(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return true }}),
		withHierarchy(mkHierarchy()))

	got := formatAction(ctx, &microflows.WebServiceCallAction{
		ServiceID:     "M.S",
		OperationName: "Op",
		Arguments:     []microflows.WebServiceArgument{{Path: "no-separator", Expression: "1"}},
	}, nil, nil)
	if strings.Contains(got, "(") {
		t.Errorf("emitted a partial argument list: %q", got)
	}
}

// TestFormatAction_ShowMessageBlocking — DESCRIBE has to emit `blocking`, or the
// round trip turns a blocking message box into a non-blocking one.
//
// The model carried Blocking on both engines all along; the loss was here, in
// the one layer that had no word for it.
func TestFormatAction_ShowMessageBlocking(t *testing.T) {
	e := newTestExecutor()
	msg := func(blocking bool) *microflows.ShowMessageAction {
		return &microflows.ShowMessageAction{
			Type:     microflows.MessageTypeInformation,
			Blocking: blocking,
			Template: &model.Text{Translations: map[string]string{"en_US": "Saved."}},
		}
	}
	if got := e.formatAction(msg(true), nil, nil); got != "show message 'Saved.' type Information blocking;" {
		t.Errorf("blocking message = %q", got)
	}
	if got := e.formatAction(msg(false), nil, nil); got != "show message 'Saved.' type Information;" {
		t.Errorf("non-blocking message = %q", got)
	}
}

// ako/mxcli#859 (rehearsal S8): a lone XPath group with a predicate on a path
// step was described without its outer brackets, `where A/B[Name = empty]`,
// which describe's own parser rejects — so create or modify could not compare
// the stored flow and refused it under mdl 1 on an identical re-run. Such a
// group keeps its brackets; one without a nested predicate still does not.
func TestFormatAction_Retrieve_NestedPredicateReparses(t *testing.T) {
	for _, c := range []struct{ xpath, where string }{
		{"[M.Timeline_Emp/M.Timeline[Name = empty]]", "where [M.Timeline_Emp/M.Timeline[Name = empty]]"},
		{"[M.A_B/M.B[Name = 'x[1]'] and Active = true()]", "where [M.A_B/M.B[Name = 'x[1]'] and Active = true()]"},
		// Controls: brackets only inside a string, and no brackets at all.
		{"[Name = 'x[1]']", "where Name = 'x[1]'"},
		{"[IsActive = true()]", "where IsActive = true()"},
	} {
		e := newTestExecutor()
		action := &microflows.RetrieveAction{
			OutputVariable: "Rows",
			Source: &microflows.DatabaseRetrieveSource{
				EntityQualifiedName: "M.Emp",
				XPathConstraint:     c.xpath,
			},
		}
		got := e.formatAction(action, nil, nil)
		if !strings.Contains(got, "\n    "+c.where+";") {
			t.Errorf("%s: got %q, want %q", c.xpath, got, c.where)
		}
		for _, header := range []string{"", "mdl 1;\n"} {
			src := header + "create microflow M.F ()\nbegin\n  " + got + "\nend;\n"
			if _, errs := visitor.Build(src); len(errs) > 0 {
				t.Errorf("%s: the description does not parse (header %q): %v\n%s", c.xpath, header, errs[0], src)
			}
		}
	}
}

// ako/mxcli#950 item 2: the `$` prefix is for a bare variable name a legacy
// writer stored without one, and nothing else. It used to go on anything
// without + ' " ( ), so `[%CurrentUser%]` became `$[%CurrentUser%]`, which does
// not parse — and so did an `if … then … else …` or a bare `and` expression.
// Each case below must describe as stored and parse back as a return value.
func TestFormatActivity_ReturnExpressionIsNotPrefixed(t *testing.T) {
	e := newTestExecutor()
	for _, v := range []string{
		"[%CurrentUser%]",
		"[%CurrentDateTime%]",
		"if $Ok then 1 else 2",
		"$A and $B",
		"empty",
		"MyModule.Status.Open",
	} {
		got := e.formatActivity(&microflows.EndEvent{ReturnValue: v}, nil, nil)
		if want := "return " + v + ";"; got != want {
			t.Errorf("ReturnValue %q: got %q, want %q", v, got, want)
		}
		script := "create microflow M.F () returns String begin\n  " + got + "\nend;"
		if _, errs := visitor.Build(script); len(errs) > 0 {
			t.Errorf("%q does not parse: %v", got, errs[0])
		}
	}
	// The control: a bare (legacy) variable name is still prefixed, and a
	// bare one with an attribute path too.
	for in, want := range map[string]string{"MyVar": "return $MyVar;", "Order/Total": "return $Order/Total;"} {
		if got := e.formatActivity(&microflows.EndEvent{ReturnValue: in}, nil, nil); got != want {
			t.Errorf("ReturnValue %q: got %q, want %q", in, got, want)
		}
	}
}

// mendixlabs/mxcli#1288: a stored Add/Remove member change (Studio Pro's Add
// and Remove buttons on a reference set) used to describe as `Assoc = $X`,
// which re-executes as a set assignment and drops the rest of the set.
func TestFormatAction_ChangeObject_AssociationAddRemove(t *testing.T) {
	e := newTestExecutor()
	cases := []struct {
		name   string
		action *microflows.ChangeObjectAction
		want   string
	}{
		{"add", &microflows.ChangeObjectAction{
			ChangeVariable: "Order",
			Changes: []*microflows.MemberChange{
				{AssociationQualifiedName: "Sales.Order_Line", Type: microflows.MemberChangeTypeAdd, Value: "$Line"},
			},
		}, "add $Line to $Order/Sales.Order_Line;"},
		{"remove with modifiers", &microflows.ChangeObjectAction{
			ChangeVariable:  "Order",
			Commit:          microflows.CommitTypeYesWithoutEvents,
			RefreshInClient: true,
			Changes: []*microflows.MemberChange{
				{AssociationQualifiedName: "Sales.Order_Line", Type: microflows.MemberChangeTypeRemove, Value: "$Line"},
			},
		}, "remove $Line from $Order/Sales.Order_Line commit without events refresh;"},
		{"set stays a change", &microflows.ChangeObjectAction{
			ChangeVariable: "Order",
			Changes: []*microflows.MemberChange{
				{AssociationQualifiedName: "Sales.Order_Line", Type: microflows.MemberChangeTypeSet, Value: "$Line"},
			},
		}, "change $Order (Sales.Order_Line = $Line);"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.formatAction(tc.action, nil, nil); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// mendixlabs/mxcli#1473: an activity mixing Add/Remove with other members
// used to describe its Add as `=` plus a trailing WARNING, because the only
// Add/Remove spelling was the sole-member `add … to $Obj/Assoc`. `+=` and `-=`
// are the member operators, so the activity describes as one statement that
// re-executes to the same Add/Remove members.
func TestFormatAction_ChangeObject_MixedAddRemoveUsesOperators(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeObjectAction{
		ChangeVariable: "Order",
		Changes: []*microflows.MemberChange{
			{AttributeQualifiedName: "Sales.Order.Number", Type: microflows.MemberChangeTypeSet, Value: "1"},
			{AssociationQualifiedName: "Sales.Order_Line", Type: microflows.MemberChangeTypeAdd, Value: "$Line"},
			{AssociationQualifiedName: "Sales.Order_OldLine", Type: microflows.MemberChangeTypeRemove, Value: "$Old"},
		},
	}
	got := e.formatAction(action, nil, nil)
	want := "change $Order (Number = 1, Sales.Order_Line += $Line, Sales.Order_OldLine -= $Old);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A sole Remove whose value is not a bare variable has no `remove … from`
// spelling (that form takes a variable); it describes with `-=`, not `=`.
func TestFormatAction_ChangeObject_RemoveExpressionUsesOperator(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeObjectAction{
		ChangeVariable: "Order",
		Changes: []*microflows.MemberChange{
			{AssociationQualifiedName: "Sales.Order_Line", Type: microflows.MemberChangeTypeRemove, Value: "$Lines"},
		},
	}
	if got := e.formatAction(action, nil, nil); got != "remove $Lines from $Order/Sales.Order_Line;" {
		t.Errorf("control: bare-variable remove = %q", got)
	}
	action.Changes[0].Value = "$Batch/Sales.Batch_Line"
	got := e.formatAction(action, nil, nil)
	if want := "change $Order (Sales.Order_Line -= $Batch/Sales.Batch_Line);"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
