// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func newAction(id string, action microflows.MicroflowAction) *microflows.ActionActivity {
	return &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(id)},
			},
		},
		Action: action,
	}
}

func newLoop(id string, children ...microflows.MicroflowObject) *microflows.LoopedActivity {
	return &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(id)},
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: children},
	}
}

func TestCollectActionActivities_TopLevelOnly(t *testing.T) {
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			newAction("a1", &microflows.MicroflowCallAction{}),
			newAction("a2", &microflows.CreateObjectAction{}),
		},
	}
	result := collectActionActivities(oc)
	if len(result) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(result))
	}
}

func TestCollectActionActivities_InsideLoop(t *testing.T) {
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			newLoop("loop1",
				newAction("inner1", &microflows.MicroflowCallAction{}),
				newAction("inner2", &microflows.ShowPageAction{}),
			),
			newAction("outer1", &microflows.RetrieveAction{}),
		},
	}
	result := collectActionActivities(oc)
	if len(result) != 3 {
		t.Fatalf("expected 3 activities (2 inside loop + 1 outside), got %d", len(result))
	}
}

func TestCollectActionActivities_NestedLoops(t *testing.T) {
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			newLoop("outer-loop",
				newLoop("inner-loop",
					newAction("deep", &microflows.MicroflowCallAction{}),
				),
			),
		},
	}
	result := collectActionActivities(oc)
	if len(result) != 1 {
		t.Fatalf("expected 1 deeply nested activity, got %d", len(result))
	}
	if result[0].ID != "deep" {
		t.Errorf("expected activity ID 'deep', got %q", result[0].ID)
	}
}

func TestCollectActionActivities_NilCollection(t *testing.T) {
	result := collectActionActivities(nil)
	if result != nil {
		t.Fatalf("expected nil for nil collection, got %v", result)
	}
}

func TestCollectActionActivities_SkipsNilActions(t *testing.T) {
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			newAction("no-action", nil),
			newAction("has-action", &microflows.MicroflowCallAction{}),
		},
	}
	result := collectActionActivities(oc)
	if len(result) != 1 {
		t.Fatalf("expected 1 activity (skipping nil action), got %d", len(result))
	}
}

func TestEntityOfDataType(t *testing.T) {
	cases := []struct {
		name string
		dt   microflows.DataType
		want string
	}{
		{"object", &microflows.ObjectType{EntityQualifiedName: "M.Customer"}, "M.Customer"},
		{"list", &microflows.ListType{EntityQualifiedName: "M.Order"}, "M.Order"},
		{"string", &microflows.StringType{}, ""},
		{"nil", nil, ""},
	}
	for _, c := range cases {
		if got := entityOfDataType(c.dt); got != c.want {
			t.Errorf("%s: entityOfDataType = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBuildVarEntityMap_And_VarActionRef(t *testing.T) {
	// A flow: param $In : M.Customer; create $New = M.Order; retrieve $List from
	// M.Product (db); then change $New, delete $In, change $Unknown (untracked).
	params := []*microflows.MicroflowParameter{
		{Name: "In", Type: &microflows.ObjectType{EntityQualifiedName: "M.Customer"}},
		{Name: "Count", Type: &microflows.IntegerType{}}, // primitive → not mapped
	}
	acts := []*microflows.ActionActivity{
		newAction("c", &microflows.CreateObjectAction{EntityQualifiedName: "M.Order", OutputVariable: "New"}),
		newAction("r", &microflows.RetrieveAction{
			OutputVariable: "List",
			Source:         &microflows.DatabaseRetrieveSource{EntityQualifiedName: "M.Product"},
		}),
	}
	varEntity := buildVarEntityMap(params, acts)
	wantMap := map[string]string{"In": "M.Customer", "New": "M.Order", "List": "M.Product"}
	for k, v := range wantMap {
		if varEntity[k] != v {
			t.Errorf("varEntity[%q] = %q, want %q", k, varEntity[k], v)
		}
	}
	if _, ok := varEntity["Count"]; ok {
		t.Error("primitive param should not be mapped")
	}

	// change/delete resolve via the map (with and without the $ prefix).
	checks := []struct {
		name       string
		action     microflows.MicroflowAction
		wantOK     bool
		targetName string
		refKind    string
	}{
		{"change tracked", &microflows.ChangeObjectAction{ChangeVariable: "$New"}, true, "M.Order", RefKindChange},
		{"delete tracked param", &microflows.DeleteObjectAction{DeleteVariable: "In"}, true, "M.Customer", RefKindDelete},
		{"change untracked var", &microflows.ChangeObjectAction{ChangeVariable: "$Mystery"}, false, "", ""},
		{"non var action", &microflows.MicroflowCallAction{}, false, "", ""},
	}
	for _, c := range checks {
		tt, tn, rk, ok := microflowVarActionRef(c.action, varEntity)
		if ok != c.wantOK {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.wantOK)
			continue
		}
		if ok && (tt != "ENTITY" || tn != c.targetName || rk != c.refKind) {
			t.Errorf("%s: got (%q,%q,%q), want (ENTITY,%q,%q)", c.name, tt, tn, rk, c.targetName, c.refKind)
		}
	}
}

func TestMicroflowActionRef(t *testing.T) {
	tests := []struct {
		name       string
		action     microflows.MicroflowAction
		wantOK     bool
		targetType string
		targetName string
		refKind    string
	}{
		{
			name:       "MicroflowCallAction",
			action:     &microflows.MicroflowCallAction{MicroflowCall: &microflows.MicroflowCall{Microflow: "M.Sub"}},
			wantOK:     true,
			targetType: "MICROFLOW", targetName: "M.Sub", refKind: RefKindCall,
		},
		{
			name:       "NanoflowCallAction (previously dropped)",
			action:     &microflows.NanoflowCallAction{NanoflowCall: &microflows.NanoflowCall{Nanoflow: "M.NF"}},
			wantOK:     true,
			targetType: "NANOFLOW", targetName: "M.NF", refKind: RefKindCall,
		},
		{
			name:       "RestOperationCallAction (previously dropped)",
			action:     &microflows.RestOperationCallAction{Operation: "M.Svc.GetThing"},
			wantOK:     true,
			targetType: "REST_OPERATION", targetName: "M.Svc.GetThing", refKind: RefKindCall,
		},
		{
			name:       "JavaActionCallAction",
			action:     &microflows.JavaActionCallAction{JavaAction: "M.DoJava"},
			wantOK:     true,
			targetType: "JAVA_ACTION", targetName: "M.DoJava", refKind: RefKindCall,
		},
		{
			name:       "CreateObjectAction",
			action:     &microflows.CreateObjectAction{EntityQualifiedName: "M.Customer"},
			wantOK:     true,
			targetType: "ENTITY", targetName: "M.Customer", refKind: RefKindCreate,
		},
		{
			name:       "ShowPageAction",
			action:     &microflows.ShowPageAction{PageName: "M.Customer_Edit"},
			wantOK:     true,
			targetType: "PAGE", targetName: "M.Customer_Edit", refKind: RefKindShowPage,
		},
		{
			name:       "RetrieveAction via database source",
			action:     &microflows.RetrieveAction{Source: &microflows.DatabaseRetrieveSource{EntityQualifiedName: "M.Order"}},
			wantOK:     true,
			targetType: "ENTITY", targetName: "M.Order", refKind: RefKindRetrieve,
		},
		{
			name:       "RetrieveAction via association source (previously dropped)",
			action:     &microflows.RetrieveAction{Source: &microflows.AssociationRetrieveSource{AssociationQualifiedName: "M.Order_Customer"}},
			wantOK:     true,
			targetType: "ASSOCIATION", targetName: "M.Order_Customer", refKind: RefKindRetrieve,
		},
		// Actions whose target is a local variable (no resolvable document QN) must
		// not emit a ref.
		{name: "ChangeObjectAction has no document ref", action: &microflows.ChangeObjectAction{ChangeVariable: "$Order"}, wantOK: false},
		{name: "DeleteObjectAction has no document ref", action: &microflows.DeleteObjectAction{}, wantOK: false},
		{name: "CastAction has no document ref", action: &microflows.CastAction{ObjectVariable: "$x"}, wantOK: false},
		{name: "empty MicroflowCallAction", action: &microflows.MicroflowCallAction{}, wantOK: false},
		{name: "nil action", action: nil, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tType, tName, rKind, ok := microflowActionRef(tt.action)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if tType != tt.targetType || tName != tt.targetName || rKind != tt.refKind {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					tType, tName, rKind, tt.targetType, tt.targetName, tt.refKind)
			}
		})
	}
}
