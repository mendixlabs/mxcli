// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// Bug 3 — a DynamicText contentparam that navigates an association must serialize
// its AttributeRef with an EntityRef (IndirectEntityRef of association steps),
// matching a Studio-Pro-authored page. Previously the path was dropped (AttributeRef
// null → CE0402 "No value specified").
func TestAttributeRefWithSteps_Serialized(t *testing.T) {
	el := attributeRefWithStepsToGen(
		"MyFirstModule.Employee.Name",
		[]pages.AttributeRefStep{
			{Association: "MyFirstModule.Expense_Employee", DestinationEntity: "MyFirstModule.Employee"},
		},
	)
	out, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var doc bsonv1.D
	if err := bsonv1.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := docGet(doc, "$Type"); got != "DomainModels$AttributeRef" {
		t.Errorf("$Type = %v, want DomainModels$AttributeRef", got)
	}
	if got := docGet(doc, "Attribute"); got != "MyFirstModule.Employee.Name" {
		t.Errorf("Attribute = %v, want MyFirstModule.Employee.Name", got)
	}

	entityRef, ok := docGet(doc, "EntityRef").(bsonv1.D)
	if !ok {
		t.Fatalf("EntityRef not a document (got %T)", docGet(doc, "EntityRef"))
	}
	if got := docGet(entityRef, "$Type"); got != "DomainModels$IndirectEntityRef" {
		t.Errorf("EntityRef.$Type = %v, want DomainModels$IndirectEntityRef", got)
	}

	step := firstListItem(t, docGet(entityRef, "Steps"))
	if got := docGet(step, "$Type"); got != "DomainModels$EntityRefStep" {
		t.Errorf("step.$Type = %v, want DomainModels$EntityRefStep", got)
	}
	if got := docGet(step, "Association"); got != "MyFirstModule.Expense_Employee" {
		t.Errorf("step.Association = %v, want MyFirstModule.Expense_Employee", got)
	}
	if got := docGet(step, "DestinationEntity"); got != "MyFirstModule.Employee" {
		t.Errorf("step.DestinationEntity = %v, want MyFirstModule.Employee", got)
	}
}

// firstListItem unwraps the codec's marker-prefixed list encoding
// ([marker, [item, ...]]) and returns the first item as a document.
func firstListItem(t *testing.T, v any) bsonv1.D {
	t.Helper()
	arr, ok := v.(bsonv1.A)
	if !ok {
		t.Fatalf("Steps not a list (got %T)", v)
	}
	// Find the inner list of items (the element that is itself an array).
	for _, e := range arr {
		if inner, ok := e.(bsonv1.A); ok {
			if len(inner) == 0 {
				t.Fatal("Steps list is empty")
			}
			if d, ok := inner[0].(bsonv1.D); ok {
				return d
			}
		}
		if d, ok := e.(bsonv1.D); ok {
			return d
		}
	}
	t.Fatalf("no step document found in %#v", arr)
	return nil
}
