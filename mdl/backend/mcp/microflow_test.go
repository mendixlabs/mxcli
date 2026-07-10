// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestPedOpError guards the critical fix: ped_create/ped_update report failures
// in the result TEXT (often with isError=false). A successful op text starts
// with "SUCCESS"; anything else must be treated as an error.
func TestPedOpError(t *testing.T) {
	if err := pedOpError("ped_create_document", "X", &ToolResult{Text: "SUCCESS: Creating documents (1)"}); err != nil {
		t.Errorf("SUCCESS text should not be an error: %v", err)
	}
	if err := pedOpError("ped_create_document", "X", &ToolResult{Text: "Creating documents failed (1 of 1): ERROR: missing $Type"}); err == nil {
		t.Error("a 'failed' text with isError=false must be detected as an error")
	}
	if err := pedOpError("ped_update_document", "X", &ToolResult{IsError: true, Text: "boom"}); err == nil {
		t.Error("isError=true must be an error")
	}
}

func TestMapMicroflowAction_VariableAndMessages(t *testing.T) {
	chg, err := mapMicroflowAction(&microflows.ChangeVariableAction{VariableName: "Result", Value: "$N * 2"})
	if err != nil || chg["changeVariableName"] != "Result" || chg["value"] != "$N * 2" {
		t.Fatalf("change variable: %+v / %v", chg, err)
	}

	show, err := mapMicroflowAction(&microflows.ShowMessageAction{
		Type:     microflows.MessageTypeWarning,
		Template: &model.Text{Translations: map[string]string{"en_US": "Heads up"}},
	})
	if err != nil || show["type"] != "Warning" {
		t.Fatalf("show message: %+v / %v", show, err)
	}
	if tmpl, _ := show["template"].(map[string]any); tmpl["text"] != "Heads up" || tmpl["$Type"] != nil {
		t.Fatalf("show template should be inline (no $Type): %+v", show["template"])
	}

	logged, err := mapMicroflowAction(&microflows.LogMessageAction{
		LogLevel:        microflows.LogLevelInfo,
		LogNodeName:     "MyNode",
		MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "done"}},
	})
	if err != nil || logged["level"] != "Info" || logged["node"] != "MyNode" {
		t.Fatalf("log message: %+v / %v", logged, err)
	}
	if tmpl, _ := logged["messageTemplate"].(map[string]any); tmpl["$Type"] != "Microflows$StringTemplate" || tmpl["text"] != "done" {
		t.Fatalf("log messageTemplate needs $Type: %+v", logged["messageTemplate"])
	}
}

func TestMapMicroflowAction_ObjectActions(t *testing.T) {
	create, err := mapMicroflowAction(&microflows.CreateObjectAction{
		EntityQualifiedName: "Sales.Order",
		OutputVariable:      "Order",
		Commit:              microflows.CommitTypeNo,
		InitialMembers: []*microflows.MemberChange{
			{AttributeQualifiedName: "Sales.Order.Total", Type: "Set", Value: "0"},
		},
	})
	if err != nil || create["$Type"] != "Microflows$CreateObjectAction" ||
		create["entity"] != "Sales.Order" || create["outputVariableName"] != "Order" || create["commit"] != "No" {
		t.Fatalf("create object: %+v / %v", create, err)
	}
	items, _ := create["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 member change, got %+v", create["items"])
	}
	mc, _ := items[0].(map[string]any)
	if mc["$Type"] != "Microflows$MemberChange" || mc["attribute"] != "Sales.Order.Total" || mc["type"] != "Set" || mc["value"] != "0" {
		t.Fatalf("member change shape wrong: %+v", mc)
	}

	commit, err := mapMicroflowAction(&microflows.CommitObjectsAction{CommitVariable: "Order", WithEvents: true})
	if err != nil || commit["$Type"] != "Microflows$CommitAction" || commit["commitVariableName"] != "Order" || commit["withEvents"] != true {
		t.Fatalf("commit: %+v / %v", commit, err)
	}

	del, err := mapMicroflowAction(&microflows.DeleteObjectAction{DeleteVariable: "Order"})
	if err != nil || del["$Type"] != "Microflows$DeleteAction" || del["deleteVariableName"] != "Order" {
		t.Fatalf("delete: %+v / %v", del, err)
	}
}

func TestMapMicroflowAction_Retrieve(t *testing.T) {
	// by database query
	db, err := mapMicroflowAction(&microflows.RetrieveAction{
		OutputVariable: "Locations",
		Source: &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: "ObjListV10.Location",
			XPathConstraint:     "[Name = $Name]",
			Range:               &microflows.Range{RangeType: microflows.RangeTypeFirst},
		},
	})
	if err != nil || db["$Type"] != "Microflows$RetrieveAction" || db["outputVariableName"] != "Locations" {
		t.Fatalf("retrieve db: %+v / %v", db, err)
	}
	q, _ := db["byDatabaseQuery"].(map[string]any)
	if q["entity"] != "ObjListV10.Location" || q["xPathConstraint"] != "[Name = $Name]" || q["takeOnlyFirst"] != true {
		t.Fatalf("byDatabaseQuery wrong: %+v", q)
	}

	// by association
	assoc, err := mapMicroflowAction(&microflows.RetrieveAction{
		OutputVariable: "Orders",
		Source:         &microflows.AssociationRetrieveSource{StartVariable: "Customer", AssociationQualifiedName: "Sales.Order_Customer"},
	})
	if err != nil {
		t.Fatalf("retrieve assoc: %v", err)
	}
	a, _ := assoc["byAssociation"].(map[string]any)
	if a["startVariableName"] != "Customer" || a["association"] != "Sales.Order_Customer" {
		t.Fatalf("byAssociation wrong: %+v", a)
	}
}

func TestMapMicroflowAction_RetrieveRejectsUnsupported(t *testing.T) {
	if _, err := mapMicroflowAction(&microflows.RetrieveAction{Source: &microflows.DatabaseRetrieveSource{
		EntityQualifiedName: "M.E", Range: &microflows.Range{RangeType: microflows.RangeTypeCustom},
	}}); err == nil {
		t.Error("custom range should be rejected")
	}
	if _, err := mapMicroflowAction(&microflows.RetrieveAction{Source: &microflows.DatabaseRetrieveSource{
		EntityQualifiedName: "M.E", Sorting: []*microflows.SortItem{{}},
	}}); err == nil {
		t.Error("sorting should be rejected")
	}
}

func TestMapMicroflowAction_ListAndAggregate(t *testing.T) {
	rb, err := mapMicroflowAction(&microflows.RollbackObjectAction{RollbackVariable: "Order"})
	if err != nil || rb["$Type"] != "Microflows$RollbackAction" || rb["rollbackVariableName"] != "Order" {
		t.Fatalf("rollback: %+v / %v", rb, err)
	}

	cl, err := mapMicroflowAction(&microflows.CreateListAction{EntityQualifiedName: "Sales.Order", OutputVariable: "Orders"})
	if err != nil || cl["$Type"] != "Microflows$CreateListAction" || cl["entity"] != "Sales.Order" || cl["outputVariableName"] != "Orders" {
		t.Fatalf("create list: %+v / %v", cl, err)
	}

	chl, err := mapMicroflowAction(&microflows.ChangeListAction{ChangeVariable: "Orders", Type: microflows.ChangeListTypeAdd, Value: "$Order"})
	if err != nil || chl["$Type"] != "Microflows$ChangeListAction" || chl["changeVariableName"] != "Orders" || chl["type"] != "Add" || chl["value"] != "$Order" {
		t.Fatalf("change list: %+v / %v", chl, err)
	}

	agg, err := mapMicroflowAction(&microflows.AggregateListAction{
		InputVariable: "Orders", OutputVariable: "Total",
		Function: microflows.AggregateFunctionSum, AttributeQualifiedName: "Sales.Order.Amount",
	})
	if err != nil || agg["$Type"] != "Microflows$AggregateListAction" || agg["function"] != "Sum" ||
		agg["inputVariableName"] != "Orders" || agg["attribute"] != "Sales.Order.Amount" {
		t.Fatalf("aggregate: %+v / %v", agg, err)
	}
	// Count needs no attribute
	cnt, _ := mapMicroflowAction(&microflows.AggregateListAction{InputVariable: "Orders", OutputVariable: "N", Function: microflows.AggregateFunctionCount})
	if cnt["attribute"] != nil || cnt["function"] != "Count" {
		t.Fatalf("count should have no attribute: %+v", cnt)
	}
}

func TestMapListOperation(t *testing.T) {
	head, err := mapListOperation(&microflows.HeadOperation{ListVariable: "Items"})
	if err != nil || head["$Type"] != "Microflows$Head" || head["listVariableName"] != "Items" {
		t.Fatalf("head: %+v / %v", head, err)
	}
	filt, err := mapListOperation(&microflows.FilterOperation{ListVariable: "Items", Expression: "$item/Active"})
	if err != nil || filt["$Type"] != "Microflows$FilterByExpression" || filt["expression"] != "$item/Active" {
		t.Fatalf("filter: %+v / %v", filt, err)
	}
	find, err := mapListOperation(&microflows.FindOperation{ListVariable: "Items", Expression: "$item/Name = 'x'"})
	if err != nil || find["$Type"] != "Microflows$FindByExpression" {
		t.Fatalf("find: %+v / %v", find, err)
	}
	union, err := mapListOperation(&microflows.UnionOperation{ListVariable1: "A", ListVariable2: "B"})
	if err != nil || union["$Type"] != "Microflows$Union" || union["listVariableName"] != "A" || union["secondListOrObjectVariableName"] != "B" {
		t.Fatalf("union: %+v / %v", union, err)
	}
	// attribute-based filter / find
	af, err := mapListOperation(&microflows.FilterByAttributeOperation{ListVariable: "Items", Attribute: "M.E.Name", Expression: "$Target"})
	if err != nil || af["$Type"] != "Microflows$Filter" || af["attribute"] != "M.E.Name" || af["expression"] != "$Target" {
		t.Fatalf("filter-by-attribute: %+v / %v", af, err)
	}
	afa, err := mapListOperation(&microflows.FindByAttributeOperation{ListVariable: "Items", Association: "M.E_Other", Expression: "$Obj"})
	if err != nil || afa["$Type"] != "Microflows$Find" || afa["association"] != "M.E_Other" {
		t.Fatalf("find-by-attribute: %+v / %v", afa, err)
	}

	// sort: nested sortItemList/SortItem/attributeRef structure
	srt, err := mapListOperation(&microflows.SortOperation{
		ListVariable: "Items",
		Sorting: []*microflows.SortItem{
			{AttributeQualifiedName: "M.E.Name", Direction: microflows.SortDirectionDescending},
		},
	})
	if err != nil || srt["$Type"] != "Microflows$Sort" || srt["listVariableName"] != "Items" {
		t.Fatalf("sort: %+v / %v", srt, err)
	}
	sil, _ := srt["sortItemList"].(map[string]any)
	if sil["$Type"] != "Microflows$SortItemList" {
		t.Fatalf("sortItemList: %+v", sil)
	}
	sitems, _ := sil["items"].([]any)
	if len(sitems) != 1 {
		t.Fatalf("expected 1 sort item: %+v", sitems)
	}
	item, _ := sitems[0].(map[string]any)
	if item["sortOrder"] != "Descending" {
		t.Fatalf("sortOrder: %+v", item)
	}
	ar, _ := item["attributeRef"].(map[string]any)
	if ar["$Type"] != "DomainModels$AttributeRef" || ar["attribute"] != "M.E.Name" {
		t.Fatalf("attributeRef: %+v", ar)
	}
}

func TestMapMicroflowAction_ListOperation(t *testing.T) {
	m, err := mapMicroflowAction(&microflows.ListOperationAction{
		OutputVariable: "First",
		Operation:      &microflows.HeadOperation{ListVariable: "Items"},
	})
	if err != nil || m["$Type"] != "Microflows$ListOperationAction" || m["outputVariableName"] != "First" {
		t.Fatalf("list op action: %+v / %v", m, err)
	}
	if op, _ := m["operation"].(map[string]any); op["$Type"] != "Microflows$Head" {
		t.Fatalf("operation: %+v", m["operation"])
	}
}

func TestMapMicroflowAction_MicroflowCall(t *testing.T) {
	m, err := mapMicroflowAction(&microflows.MicroflowCallAction{
		ResultVariableName: "Tripled",
		UseReturnVariable:  true,
		MicroflowCall: &microflows.MicroflowCall{
			Microflow: "MyFirstModule.ACT_Callee",
			ParameterMappings: []*microflows.MicroflowCallParameterMapping{
				{Parameter: "MyFirstModule.ACT_Callee.N", Argument: "$X"},
			},
		},
	})
	if err != nil || m["$Type"] != "Microflows$MicroflowCallAction" || m["outputVariableName"] != "Tripled" || m["useReturnVariable"] != true {
		t.Fatalf("call action: %+v / %v", m, err)
	}
	mc, _ := m["microflowCall"].(map[string]any)
	if mc["$Type"] != "Microflows$MicroflowCall" || mc["microflow"] != "MyFirstModule.ACT_Callee" {
		t.Fatalf("microflowCall wrong: %+v", mc)
	}
	pms, _ := mc["parameterMappings"].([]any)
	if len(pms) != 1 {
		t.Fatalf("expected 1 mapping: %+v", mc["parameterMappings"])
	}
	pm, _ := pms[0].(map[string]any)
	if pm["$Type"] != "Microflows$MicroflowCallParameterMapping" || pm["parameter"] != "MyFirstModule.ACT_Callee.N" || pm["argument"] != "$X" {
		t.Fatalf("parameter mapping wrong: %+v", pm)
	}
}

func TestMapMicroflowAction_UIActions(t *testing.T) {
	dl, err := mapMicroflowAction(&microflows.DownloadFileAction{FileDocument: "File", ShowInBrowser: true})
	if err != nil || dl["$Type"] != "Microflows$DownloadFileAction" || dl["fileDocumentVariableName"] != "File" || dl["showFileInBrowser"] != true {
		t.Fatalf("download: %+v / %v", dl, err)
	}

	cp, err := mapMicroflowAction(&microflows.ClosePageAction{NumberOfPages: 2})
	if err != nil || cp["$Type"] != "Microflows$CloseFormAction" || cp["numberOfPagesToClose"] != "2" {
		t.Fatalf("close page: %+v / %v", cp, err)
	}
	// default to 1 page when unset
	cp0, _ := mapMicroflowAction(&microflows.ClosePageAction{})
	if cp0["numberOfPagesToClose"] != "1" {
		t.Fatalf("close page default: %+v", cp0)
	}

	vf, err := mapMicroflowAction(&microflows.ValidationFeedbackAction{
		ObjectVariable: "Loc", AttributeName: "M.E.Name",
		Template: &model.Text{Translations: map[string]string{"en_US": "required"}},
	})
	if err != nil || vf["$Type"] != "Microflows$ValidationFeedbackAction" || vf["objectVariableName"] != "Loc" || vf["attribute"] != "M.E.Name" {
		t.Fatalf("validation feedback: %+v / %v", vf, err)
	}
	if ft, _ := vf["feedbackTemplate"].(map[string]any); ft["$Type"] != "Microflows$TextTemplate" || ft["text"] != "required" {
		t.Fatalf("feedbackTemplate: %+v", vf["feedbackTemplate"])
	}

	if _, err := mapMicroflowAction(&microflows.ShowPageAction{PageName: "M.Home"}); err == nil {
		t.Error("show page should be rejected (PED can't reference the page)")
	}
}

func TestMapMicroflowAction_NanoflowCall(t *testing.T) {
	m, err := mapMicroflowAction(&microflows.NanoflowCallAction{
		OutputVariableName: "Valid",
		UseReturnVariable:  true,
		NanoflowCall: &microflows.NanoflowCall{
			Nanoflow:          "M.VAL_X",
			ParameterMappings: []*microflows.NanoflowCallParameterMapping{{Parameter: "M.VAL_X.P", Argument: "$P"}},
		},
	})
	if err != nil || m["$Type"] != "Microflows$NanoflowCallAction" || m["outputVariableName"] != "Valid" {
		t.Fatalf("nanoflow call: %+v / %v", m, err)
	}
	nc, _ := m["nanoflowCall"].(map[string]any)
	if nc["$Type"] != "Microflows$NanoflowCall" || nc["nanoflow"] != "M.VAL_X" {
		t.Fatalf("nanoflowCall: %+v", nc)
	}
}

func TestMapMicroflowAction_JavaActionCall(t *testing.T) {
	m, err := mapMicroflowAction(&microflows.JavaActionCallAction{
		JavaAction:         "M.ValidateEmail",
		ResultVariableName: "Valid",
		ParameterMappings: []*microflows.JavaActionParameterMapping{
			{Parameter: "M.ValidateEmail.Email", Value: &microflows.BasicCodeActionParameterValue{Argument: "$Email"}},
		},
	})
	if err != nil || m["$Type"] != "Microflows$JavaActionCallAction" || m["javaAction"] != "M.ValidateEmail" || m["outputVariableName"] != "Valid" {
		t.Fatalf("java call: %+v / %v", m, err)
	}
	pms, _ := m["parameterMappings"].([]any)
	pm, _ := pms[0].(map[string]any)
	if pm["$Type"] != "Microflows$JavaActionParameterMapping" || pm["parameter"] != "M.ValidateEmail.Email" {
		t.Fatalf("java param mapping: %+v", pm)
	}
	pv, _ := pm["parameterValue"].(map[string]any)
	if pv["$Type"] != "Microflows$BasicCodeActionParameterValue" || pv["argument"] != "$Email" {
		t.Fatalf("java param value: %+v", pv)
	}
	// entity-type value
	ev, err := mapCodeActionParameterValue(&microflows.EntityTypeCodeActionParameterValue{Entity: "M.E"})
	if err != nil || ev["$Type"] != "Microflows$EntityTypeCodeActionParameterValue" || ev["entity"] != "M.E" {
		t.Fatalf("entity-type value: %+v / %v", ev, err)
	}
}

func TestMapMicroflowAction_MicroflowCallMissingTarget(t *testing.T) {
	if _, err := mapMicroflowAction(&microflows.MicroflowCallAction{MicroflowCall: &microflows.MicroflowCall{}}); err == nil {
		t.Error("a call with no target microflow should error")
	}
}

func TestMfDataType_Void(t *testing.T) {
	// nil and an explicit VoidType both map to "Void".
	if got, _, _, err := mfDataType(nil); err != nil || got != "Void" {
		t.Errorf("nil -> %q/%v", got, err)
	}
	if got, _, _, err := mfDataType(&microflows.VoidType{}); err != nil || got != "Void" {
		t.Errorf("VoidType -> %q/%v", got, err)
	}
}

func TestMapCaseValue(t *testing.T) {
	cases := []struct {
		cv      microflows.CaseValue
		wantKey string
		wantVal string
	}{
		{&microflows.ExpressionCase{Expression: "true"}, "enumerationCase", "true"},
		{&microflows.ExpressionCase{Expression: "false"}, "enumerationCase", "false"},
		{microflows.EnumerationCase{Value: "Active"}, "enumerationCase", "Active"},
		{&microflows.BooleanCase{Value: true}, "enumerationCase", "true"},
		{&microflows.InheritanceCase{EntityQualifiedName: "M.Employee"}, "inheritanceCase", "M.Employee"},
	}
	for _, c := range cases {
		m, err := mapCaseValue(c.cv)
		if err != nil || m[c.wantKey] != c.wantVal {
			t.Errorf("mapCaseValue(%T) = %+v / %v; want %s=%s", c.cv, m, err, c.wantKey, c.wantVal)
		}
	}
	if m, err := mapCaseValue(nil); err != nil || m != nil {
		t.Errorf("nil case value should map to nil: %+v / %v", m, err)
	}
}

func TestMapObjectTree_Split(t *testing.T) {
	b := &Backend{}
	idPath := map[model.ID]string{}
	split, err := b.mapObjectTree(&microflows.ExclusiveSplit{
		Caption:        "Is big?",
		SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$N > 10"},
	}, "/objects/3", idPath)
	if err != nil || split["$Type"] != "Microflows$ExclusiveSplit" ||
		split["expressionSplitCondition"] != "$N > 10" || split["caption"] != "Is big?" {
		t.Fatalf("exclusive split: %+v / %v", split, err)
	}

	merge, err := b.mapObjectTree(&microflows.ExclusiveMerge{}, "/objects/4", idPath)
	if err != nil || merge["$Type"] != "Microflows$ExclusiveMerge" {
		t.Fatalf("exclusive merge: %+v / %v", merge, err)
	}
}

func TestMapObjectTree_SplitRejectsRuleCondition(t *testing.T) {
	b := &Backend{}
	if _, err := b.mapObjectTree(&microflows.ExclusiveSplit{
		SplitCondition: &microflows.RuleSplitCondition{RuleQualifiedName: "M.SomeRule"},
	}, "/objects/0", map[model.ID]string{}); err == nil {
		t.Error("rule-based split condition should be rejected (only expression supported)")
	}
}

func TestMapObjectTree_Loop(t *testing.T) {
	b := &Backend{}
	idPath := map[model.ID]string{}
	body := &microflows.ActionActivity{Action: &microflows.LogMessageAction{LogLevel: microflows.LogLevelInfo}}
	body.ID = "body-1"
	loop := &microflows.LoopedActivity{
		LoopSource: &microflows.IterableList{ListVariableName: "$Items", VariableName: "Item"},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{body},
		},
	}
	loop.ID = "loop-1"

	m, err := b.mapObjectTree(loop, "/objects/2", idPath)
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	if m["$Type"] != "Microflows$LoopedActivity" {
		t.Fatalf("loop $Type: %+v", m)
	}
	src, _ := m["iterableListSource"].(map[string]any)
	if src["listVariableName"] != "$Items" || src["iteratorVariableName"] != "Item" {
		t.Fatalf("loop source: %+v", src)
	}
	objs, _ := m["objects"].([]any)
	if len(objs) != 1 {
		t.Fatalf("loop body should have 1 object: %+v", objs)
	}
	// the body object's path nests under the loop
	if idPath["body-1"] != "/objects/2/objects/0" {
		t.Fatalf("body path = %q, want /objects/2/objects/0", idPath["body-1"])
	}
}

func TestMfCommitType(t *testing.T) {
	cases := map[microflows.CommitType]string{
		microflows.CommitTypeYes:           "Yes",
		microflows.CommitTypeYesWithEvents: "Yes",
		microflows.CommitTypeNoEvent:       "YesWithoutEvents",
		microflows.CommitTypeNo:            "No",
		microflows.CommitType(""):          "No",
	}
	for in, want := range cases {
		if got := mfCommitType(in); got != want {
			t.Errorf("mfCommitType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMfDataType_Primitives(t *testing.T) {
	cases := []struct {
		dt   microflows.DataType
		want string
	}{
		{&microflows.BooleanType{}, "Boolean"},
		{&microflows.IntegerType{}, "Integer"},
		{&microflows.DecimalType{}, "Decimal"},
		{&microflows.StringType{}, "String"},
		{&microflows.DateTimeType{}, "DateTime"},
		{&microflows.DateType{}, "DateTime"}, // Date maps to DateTime
	}
	for _, c := range cases {
		got, ent, enum, err := mfDataType(c.dt)
		if err != nil || got != c.want || ent != "" || enum != "" {
			t.Errorf("mfDataType(%s) = %q/%q/%q/%v; want %q", c.dt.GetTypeName(), got, ent, enum, err, c.want)
		}
	}
}

func TestMfDataType_ObjectAndEnumeration(t *testing.T) {
	got, ent, _, err := mfDataType(&microflows.ObjectType{EntityQualifiedName: "Sales.Order"})
	if err != nil || got != "Object" || ent != "Sales.Order" {
		t.Errorf("object type wrong: %q/%q/%v", got, ent, err)
	}
	got, ent, _, err = mfDataType(&microflows.ListType{EntityQualifiedName: "Sales.Order"})
	if err != nil || got != "List" || ent != "Sales.Order" {
		t.Errorf("list type wrong: %q/%q/%v", got, ent, err)
	}
	got, _, enum, err := mfDataType(&microflows.EnumerationType{EnumerationQualifiedName: "M.Status"})
	if err != nil || got != "Enumeration" || enum != "M.Status" {
		t.Errorf("enumeration type wrong: %q/%q/%v", got, enum, err)
	}
}

func TestMfDataType_UnsupportedErrors(t *testing.T) {
	if _, _, _, err := mfDataType(&microflows.LongType{}); err == nil {
		t.Error("Long is not in the PED param/return enum and should error")
	}
}

func TestMapMicroflowAction_CreateVariable(t *testing.T) {
	m, err := mapMicroflowAction(&microflows.CreateVariableAction{
		VariableName: "Result",
		DataType:     &microflows.IntegerType{},
		InitialValue: "$N * 5",
	})
	if err != nil {
		t.Fatalf("mapMicroflowAction: %v", err)
	}
	if m["$Type"] != "Microflows$CreateVariableAction" || m["variableName"] != "Result" ||
		m["variableType"] != "Integer" || m["initialValue"] != "$N * 5" {
		t.Fatalf("unexpected mapping: %+v", m)
	}
}

func TestMapMicroflowAction_Unsupported(t *testing.T) {
	if _, err := mapMicroflowAction(&microflows.RetrieveAction{}); err == nil {
		t.Error("an unmapped action type should error")
	}
}

func TestMfVariableType(t *testing.T) {
	if got, err := mfVariableType(&microflows.StringType{}); err != nil || got != "String" {
		t.Errorf("String: %q/%v", got, err)
	}
	if got, err := mfVariableType(&microflows.DateType{}); err != nil || got != "DateTime" {
		t.Errorf("Date should map to DateTime: %q/%v", got, err)
	}
	if _, err := mfVariableType(&microflows.ObjectType{}); err == nil {
		t.Error("object variable type should be rejected (primitives only)")
	}
}

func TestMapMicroflowAction_ShowHomePageAndJavaScript(t *testing.T) {
	home, err := mapMicroflowAction(&microflows.ShowHomePageAction{})
	if err != nil || home["$Type"] != "Microflows$ShowHomePageAction" {
		t.Fatalf("show home page: %+v / %v", home, err)
	}

	js, err := mapMicroflowAction(&microflows.JavaScriptActionCallAction{
		JavaScriptAction:  "MyMod.JS_DoThing",
		UseReturnVariable: true,
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{Parameter: "MyMod.JS_DoThing.Input", Value: &microflows.BasicCodeActionParameterValue{Argument: "$N"}},
		},
		OutputVariableName: "Result",
	})
	if err != nil {
		t.Fatalf("js action: %v", err)
	}
	if js["$Type"] != "Microflows$JavaScriptActionCallAction" || js["javaScriptAction"] != "MyMod.JS_DoThing" {
		t.Fatalf("js action shell: %+v", js)
	}
	if js["outputVariableName"] != "Result" || js["useReturnVariable"] != true {
		t.Fatalf("js action output: %+v", js)
	}
	pms, _ := js["parameterMappings"].([]any)
	if len(pms) != 1 || pms[0].(map[string]any)["$Type"] != "Microflows$JavaScriptActionParameterMapping" {
		t.Fatalf("js param mappings: %+v", js["parameterMappings"])
	}

	// Missing target is rejected.
	if _, err := mapMicroflowAction(&microflows.JavaScriptActionCallAction{}); err == nil {
		t.Error("js action with no target should be rejected")
	}
}

func TestMapListOperation_ContainsAndEquals(t *testing.T) {
	c, err := mapListOperation(&microflows.ContainsOperation{ListVariable: "Orders", ObjectVariable: "Order"})
	if err != nil || c["$Type"] != "Microflows$Contains" ||
		c["listVariableName"] != "Orders" || c["secondListOrObjectVariableName"] != "Order" {
		t.Fatalf("contains: %+v / %v", c, err)
	}

	e, err := mapListOperation(&microflows.EqualsOperation{ListVariable1: "A", ListVariable2: "B"})
	if err != nil || e["$Type"] != "Microflows$ListEquals" ||
		e["listVariableName"] != "A" || e["secondListOrObjectVariableName"] != "B" {
		t.Fatalf("equals: %+v / %v", e, err)
	}
}
