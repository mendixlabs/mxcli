// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	mdltypes "github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
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

func TestFormatAction_CommitObjects_WithEvents(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CommitObjectsAction{CommitVariable: "Order", WithEvents: true}
	got := e.formatAction(action, nil, nil)
	if got != "commit $Order with events;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_CommitObjects_WithoutEvents(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CommitObjectsAction{CommitVariable: "Order"}
	got := e.formatAction(action, nil, nil)
	if got != "commit $Order;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_CommitObjects_Refresh(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CommitObjectsAction{CommitVariable: "Order", RefreshInClient: true}
	got := e.formatAction(action, nil, nil)
	if got != "commit $Order refresh;" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAction_CommitObjects_WithEventsAndRefresh(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.CommitObjectsAction{CommitVariable: "Order", WithEvents: true, RefreshInClient: true}
	got := e.formatAction(action, nil, nil)
	if got != "commit $Order with events refresh;" {
		t.Errorf("got %q", got)
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
	if got != "$Total = count($Orders);" {
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
	if got != "$TotalAmount = sum($Orders.Amount);" {
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
	if got != "$TotalTax = sum($Orders, $currentObject/Amount * 0.21);" {
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
	want := "show page MyModule.OrderDetail($Order = $Order);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
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
	got := e.formatAction(action, nil, nil)
	if got != "show message 'Line 1\\nLine 2\\tTabbed' type Information;" {
		t.Errorf("got %q", got)
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
	want := "log info node 'App' 'Order {1} for {2}' with ({1} = $OrderNumber, {2} = $CustomerName);"
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
	got := e.formatAction(action, nil, nil)
	want := "log info node 'App' 'Line 1\\nLine 2';"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
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
	want := "log info node @MyModule.SecurityLogNode 'User added';"
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
	want := "retrieve $First from MyModule.Customer\n    limit 1;"
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
	want := "retrieve $Domains from SampleRuntime.Domain\n    where SampleRuntime.Domain_Runtime = $Runtime\n    limit 1;"
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

func TestFormatAction_WebServiceCallResolvesKnownReferences(t *testing.T) {
	moduleID := mkID("soap-module")
	serviceID := mkID("soap-service")
	sendMappingID := mkID("soap-send")
	receiveMappingID := mkID("soap-receive")
	serviceContents, err := bson.Marshal(bson.M{"Name": "OrderService"})
	if err != nil {
		t.Fatal(err)
	}

	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListRawUnitsByTypeFunc: func(typePrefix string) ([]*mdltypes.RawUnit, error) {
			if typePrefix != "WebServices$ImportedWebService" {
				t.Fatalf("unexpected type prefix %q", typePrefix)
			}
			return []*mdltypes.RawUnit{{
				ID:          serviceID,
				ContainerID: moduleID,
				Type:        "WebServices$ImportedWebService",
				Contents:    serviceContents,
			}}, nil
		},
		ListExportMappingsFunc: func() ([]*model.ExportMapping, error) {
			return []*model.ExportMapping{{
				BaseElement: model.BaseElement{ID: sendMappingID},
				ContainerID: moduleID,
				Name:        "OrderRequest",
			}}, nil
		},
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) {
			return []*model.ImportMapping{{
				BaseElement: model.BaseElement{ID: receiveMappingID},
				ContainerID: moduleID,
				Name:        "OrderResponse",
			}}, nil
		},
	}
	h := mkHierarchy(&model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: "SyntheticSOAP"})
	ctx, _ := newMockCtx(t, withBackend(backend), withHierarchy(h))

	action := &microflows.WebServiceCallAction{
		ServiceID:         serviceID,
		OperationName:     "FetchOrders",
		SendMappingID:     sendMappingID,
		ReceiveMappingID:  receiveMappingID,
		OutputVariable:    "Root",
		UseReturnVariable: true,
	}
	got := formatAction(ctx, action, nil, nil)
	want := "$Root = call web service SyntheticSOAP.OrderService\noperation FetchOrders\nsend mapping SyntheticSOAP.OrderRequest\nreceive mapping SyntheticSOAP.OrderResponse;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatAction_WebServiceCallKeepsRawReferencesWhenUnknown(t *testing.T) {
	backend := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListRawUnitsByTypeFunc: func(typePrefix string) ([]*mdltypes.RawUnit, error) {
			return nil, nil
		},
		ListExportMappingsFunc: func() ([]*model.ExportMapping, error) {
			return nil, nil
		},
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) {
			return nil, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(backend), withHierarchy(mkHierarchy()))

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
