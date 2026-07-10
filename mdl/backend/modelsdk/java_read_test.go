// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

// TestReadJavaActionByName_RoundTrip guards the codec-native java-action read:
// create an action, then ReadJavaActionByName must return its name, parameters
// (with types) and return type — the microflow builder relies on this to resolve
// a java-action call's parameter types (else "entity type params will be empty").
func TestReadJavaActionByName_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	ja := &javaactions.JavaAction{
		ContainerID: mod.ID,
		Name:        "ZzJa",
		Parameters: []*javaactions.JavaActionParameter{
			{Name: "Count", IsRequired: true, ParameterType: &javaactions.IntegerType{}},
			{Name: "Cust", IsRequired: true, ParameterType: &javaactions.EntityType{Entity: "MyFirstModule.Thing"}},
		},
		ReturnType: &javaactions.BooleanType{},
	}
	ja.ID = model.ID("")
	if err := b.CreateJavaAction(ja); err != nil {
		t.Fatalf("CreateJavaAction: %v", err)
	}

	got, err := b.ReadJavaActionByName("MyFirstModule.ZzJa")
	if err != nil {
		t.Fatalf("ReadJavaActionByName: %v", err)
	}
	if got.Name != "ZzJa" || len(got.Parameters) != 2 {
		t.Fatalf("got name=%q params=%d, want ZzJa/2", got.Name, len(got.Parameters))
	}
	if _, ok := got.Parameters[0].ParameterType.(*javaactions.IntegerType); !ok {
		t.Errorf("param 0 type = %T, want *IntegerType", got.Parameters[0].ParameterType)
	}
	ent, ok := got.Parameters[1].ParameterType.(*javaactions.EntityType)
	if !ok || ent.Entity != "MyFirstModule.Thing" {
		t.Errorf("param 1 type = %#v, want EntityType{MyFirstModule.Thing}", got.Parameters[1].ParameterType)
	}
	if _, ok := got.ReturnType.(*javaactions.BooleanType); !ok {
		t.Errorf("return type = %T, want *BooleanType", got.ReturnType)
	}
}
