// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Studio Pro 11.14's constructor, as ped_get_schema returned it live (trimmed to
// the properties, descriptions dropped).
const microflowCtorSchema1114 = `{"kind":"constructor","schema":"constructor type 'Microflows$Microflow' = {\n $Type: 'Microflows$Microflow',\n name: string;\n parameters?: Element<'Microflows$MicroflowParameterObject'>[];\n objects?: ChooseAbstractType<{\n  \"Microflows$ActionActivity\": \"…\",\n  \"Microflows$StartEvent\": \"…\"\n }>[];\n flows?: ChooseAbstractType<{\n  \"Microflows$AnnotationFlow\": \"…\",\n  \"Microflows$SequenceFlow\": \"…\"\n }>[];\n returnType?: 'Void' | 'Boolean' | 'Binary' | 'Decimal' | 'Integer' | 'Float' | 'DateTime' | 'String' | 'Enumeration' | 'Object' | 'List';\n returnTypeEntity?: Reference<'DomainModels$Entity', 'qualified-name'>;\n returnTypeEnumeration?: Reference<'Enumerations$Enumeration', 'qualified-name'>;\n}"}`

// The shape is chosen from the live constructor schema, once per session:
// serverInfo.version is frozen at 1.0.0, so a version gate would be a guess.
func TestMicroflowConstructorShapeProbe(t *testing.T) {
	schemaCalls := 0
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_get_schema" {
			schemaCalls++
			return microflowCtorSchema1114, false
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}
	if !b.microflowConstructorTakesSkeleton() || !b.microflowConstructorTakesSkeleton() {
		t.Fatal("an 11.14 constructor schema must select the skeleton shape")
	}
	if schemaCalls != 1 {
		t.Errorf("ped_get_schema called %d times, want 1 (cached)", schemaCalls)
	}
	if !b.schemaFetched[microflowDocType] {
		t.Error("the probe must count as the schema fetch PED requires before a create")
	}

	old := newFakePED(t, func(string, map[string]any) (string, bool) {
		return `{"schemas":[{"elementType":"Microflows$Microflow","schema":{"properties":{"returnType":{},"flows":{}}}}]}`, false
	})
	if (&Backend{client: old.connectClient(t)}).microflowConstructorTakesSkeleton() {
		t.Error("a constructor without returnTypeEntity must keep the full shape")
	}
}

// probeMicroflow is a parameter, a split, a loop with a logging body, and an end
// returning a list — every property the skeleton constructor moved out of the
// create, plus a parameter to shift the flow references.
func probeMicroflow(containerID model.ID) *microflows.Microflow {
	obj := func(id string, x, y int) microflows.BaseMicroflowObject {
		return microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: model.ID(id)}, Position: model.Point{X: x, Y: y}}
	}
	start := &microflows.StartEvent{BaseMicroflowObject: obj("start", 100, 150)}
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: obj("split", 250, 150),
		Caption:             "Any?",
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$Items != empty"},
	}
	body := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: obj("body", 100, 60)},
		Action: &microflows.LogMessageAction{
			LogLevel: microflows.LogLevelInfo, LogNodeName: "'Probe'",
			MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "hello"}},
		},
	}
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: obj("loop", 450, 150),
		LoopSource:          &microflows.IterableList{ListVariableName: "Items", VariableName: "Item"},
		ObjectCollection:    &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{body}},
	}
	end := &microflows.EndEvent{BaseMicroflowObject: obj("end", 650, 150), ReturnValue: "$Items"}
	return &microflows.Microflow{
		ContainerID: containerID,
		Name:        "ZzProbe",
		Parameters: []*microflows.MicroflowParameter{{
			Name: "Items", Type: &microflows.ListType{EntityQualifiedName: "MyFirstModule.E"},
		}},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{start, split, loop, end},
			Flows: []*microflows.SequenceFlow{
				{OriginID: "start", DestinationID: "split"},
				{OriginID: "split", DestinationID: "loop", CaseValue: &microflows.ExpressionCase{Expression: "true"}},
				{OriginID: "loop", DestinationID: "end"},
			},
		},
		ReturnType:         &microflows.ListType{EntityQualifiedName: "MyFirstModule.E"},
		ReturnVariableName: "Result",
	}
}

// Against Studio Pro 11.14 a microflow is created as a canvas skeleton and its
// behaviour set by one update. Measured live: the full shape's flows and
// returnType are rejected, and its action/returnValue/relativeMiddlePoint are
// accepted and silently dropped — so both halves are asserted.
func TestCreateMicroflow_SkeletonConstructor(t *testing.T) {
	ped := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		switch name {
		case "ped_get_schema":
			return microflowCtorSchema1114, false
		case "ped_create_document":
			return "SUCCESS: Creating documents (1)", false
		case "ped_update_document":
			return "SUCCESS: All operations have been performed successfully.", false
		case "ped_check_errors":
			return "No errors found.", false
		}
		return "{}", false
	})
	b := backendFor(ped)
	if err := b.Connect(localProject(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	if err := b.CreateMicroflow(probeMicroflow(mod.ID)); err != nil {
		t.Fatalf("CreateMicroflow: %v", err)
	}

	order := map[string]int{}
	for i, c := range ped.calls {
		if _, seen := order[c.Name]; !seen {
			order[c.Name] = i
		}
	}
	if !(order["ped_create_document"] < order["ped_update_document"] && order["ped_update_document"] < order["ped_check_errors"]) {
		t.Errorf("want create, then update, then check; calls = %v", ped.calls)
	}

	create, _ := ped.callByName("ped_create_document")
	docs, _ := create.Args["documents"].([]any)
	doc, _ := docs[0].(map[string]any)
	content, _ := doc["documentContent"].(map[string]any)

	if content["returnType"] != "List" || content["returnTypeEntity"] != "MyFirstModule.E" {
		t.Errorf("return type = %v / %v, want the bare enum plus returnTypeEntity", content["returnType"], content["returnTypeEntity"])
	}
	if _, ok := content["returnVariableName"]; ok {
		t.Error("the skeleton constructor does not take returnVariableName")
	}
	params, _ := content["parameters"].([]any)
	if len(params) != 1 {
		t.Fatalf("parameters = %v, want the one parameter moved out of objects", content["parameters"])
	}
	if p, _ := params[0].(map[string]any); p["x"] == nil || p["entity"] != "MyFirstModule.E" {
		t.Errorf("parameter = %v", p)
	}

	dropped := []string{"relativeMiddlePoint", "action", "returnValue", "expressionSplitCondition", "iterableListSource", "whileLoopSource"}
	var walk func(objs []any, where string)
	walk = func(objs []any, where string) {
		for i, o := range objs {
			m, _ := o.(map[string]any)
			for _, k := range dropped {
				if _, ok := m[k]; ok {
					t.Errorf("%s[%d] %s still carries %q, which 11.14 drops silently", where, i, m["$Type"], k)
				}
			}
			if _, ok := m["x"]; !ok {
				t.Errorf("%s[%d] %s has no x/y position", where, i, m["$Type"])
			}
			if body, ok := m["objects"].([]any); ok {
				walk(body, where+"/objects")
			}
		}
	}
	objects, _ := content["objects"].([]any)
	if len(objects) != 4 {
		t.Fatalf("objects = %d, want 4 (the parameter is not one of them)", len(objects))
	}
	walk(objects, "objects")
	if loop, _ := objects[2].(map[string]any); loop["loopType"] != "ForEach" {
		t.Errorf("loop = %v, want loopType ForEach", loop)
	}

	flows, _ := content["flows"].([]any)
	wantRefs := [][2]string{{"$id(/objects/0)", "$id(/objects/1)"}, {"$id(/objects/1)", "$id(/objects/2)"}, {"$id(/objects/2)", "$id(/objects/3)"}}
	for i, f := range flows {
		fm, _ := f.(map[string]any)
		if fm["$Type"] != "Microflows$SequenceFlow" {
			t.Errorf("flows[%d] $Type = %v", i, fm["$Type"])
		}
		if fm["originId"] != wantRefs[i][0] || fm["destinationId"] != wantRefs[i][1] {
			t.Errorf("flows[%d] = %v -> %v, want %v (shifted past the parameter)", i, fm["originId"], fm["destinationId"], wantRefs[i])
		}
	}

	update, ok := ped.callByName("ped_update_document")
	if !ok {
		t.Fatal("no ped_update_document: the activities were never set")
	}
	if update.Args["documentName"] != "MyFirstModule.ZzProbe" {
		t.Errorf("update targets %v", update.Args["documentName"])
	}
	set := map[string]any{}
	ops, _ := update.Args["operations"].([]any)
	for _, o := range ops {
		om, _ := o.(map[string]any)
		op, _ := om["operation"].(map[string]any)
		if op["type"] != "set" {
			t.Errorf("operation %v is not a set", om)
		}
		set[om["path"].(string)] = op["value"]
	}
	// Stored paths keep the parameter in slot 0.
	if v, _ := set["/objectCollection/objects/2/splitCondition"].(map[string]any); v["$Type"] != "Microflows$ExpressionSplitCondition" || v["expression"] != "$Items != empty" {
		t.Errorf("split condition = %v", set["/objectCollection/objects/2/splitCondition"])
	}
	if v, _ := set["/objectCollection/objects/3/loopSource"].(map[string]any); v["$Type"] != "Microflows$IterableList" || v["listVariableName"] != "Items" || v["variableName"] != "Item" {
		t.Errorf("loop source = %v", set["/objectCollection/objects/3/loopSource"])
	}
	if v, _ := set["/objectCollection/objects/3/objectCollection/objects/0/action"].(map[string]any); v["$Type"] != "Microflows$LogMessageAction" {
		t.Errorf("loop body action = %v", set["/objectCollection/objects/3/objectCollection/objects/0/action"])
	}
	if set["/objectCollection/objects/4/returnValue"] != "$Items" {
		t.Errorf("return value = %v", set["/objectCollection/objects/4/returnValue"])
	}
	if set["/returnVariableName"] != "Result" {
		t.Errorf("returnVariableName = %v", set["/returnVariableName"])
	}
	if len(set) != 5 {
		t.Errorf("got %d operations, want 5: %v", len(set), set)
	}
}

// An older constructor still gets the full shape in one create, with no update.
func TestCreateMicroflow_FullConstructorUnchanged(t *testing.T) {
	ped := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		switch name {
		case "ped_create_document":
			return "SUCCESS: Creating documents (1)", false
		case "ped_check_errors":
			return "No errors found.", false
		}
		return "{}", false
	})
	b := backendFor(ped)
	if err := b.Connect(localProject(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	if err := b.CreateMicroflow(probeMicroflow(mod.ID)); err != nil {
		t.Fatalf("CreateMicroflow: %v", err)
	}
	if _, ok := ped.callByName("ped_update_document"); ok {
		t.Error("the full constructor takes everything in the create; no update expected")
	}
	create, _ := ped.callByName("ped_create_document")
	content := create.Args["documents"].([]any)[0].(map[string]any)["documentContent"].(map[string]any)
	if rt, _ := content["returnType"].(map[string]any); rt["type"] != "List" {
		t.Errorf("returnType = %v, want the {type: …} object", content["returnType"])
	}
}

func TestShiftObjectRef(t *testing.T) {
	cases := []struct {
		ref   string
		shift int
		want  string
	}{
		{"$id(/objects/3)", 1, "$id(/objects/2)"},
		{"$id(/objects/3/objects/0)", 2, "$id(/objects/1/objects/0)"},
		{"$id(/objects/12)", 0, "$id(/objects/12)"},
		{"$id(/entities/0)", 1, "$id(/entities/0)"},
	}
	for _, c := range cases {
		if got := shiftObjectRef(c.ref, c.shift); got != c.want {
			t.Errorf("shiftObjectRef(%q, %d) = %q, want %q", c.ref, c.shift, got, c.want)
		}
	}
}
