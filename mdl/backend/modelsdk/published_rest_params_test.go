// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// TestCreatePublishedRestService_WritesDerivedParameters: the parameters the executor
// derived from the microflow are stored as Rest$RestOperationParameter elements, each with
// its kind, its microflow parameter and that parameter's type. An operation whose
// microflow could not be read keeps its path placeholders as String path parameters.
func TestCreatePublishedRestService_WritesDerivedParameters(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	svc := &model.PublishedRestService{
		ContainerID: mod.ID,
		Name:        "ZzParams",
		Path:        "rest/zz/v1",
		Version:     "1.0.0",
		Resources: []*model.PublishedRestResource{{
			Name: "orders",
			Operations: []*model.PublishedRestOperation{
				{HTTPMethod: "GET", Path: "items/{id}", Microflow: "MyFirstModule.GetItem",
					OperationParameters: []*model.PublishedRestOperationParameter{
						{Name: "id", ParameterType: "Path", DataType: "Integer"},
						{Name: "orderNumber", ParameterType: "Query", DataType: "String"},
						{Name: "item", ParameterType: "Body", DataType: "Object", QualifiedName: "MyFirstModule.Item"},
					}},
				{HTTPMethod: "GET", Path: "legacy/{code}", Microflow: "MyFirstModule.Unknown"},
			},
		}},
	}
	if err := b.CreatePublishedRestService(svc); err != nil {
		t.Fatalf("CreatePublishedRestService: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	var doc bson.M
	if err := bson.Unmarshal(readUnitBytes(t, proj, string(svc.ID)), &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}
	ops := paramItems(t, paramItems(t, doc, "Resources")[0], "Operations")

	derived := paramItems(t, ops[0], "Parameters")
	want := []struct{ name, kind, typ, ref string }{
		{"id", "Path", "DataTypes$IntegerType", ""},
		{"orderNumber", "Query", "DataTypes$StringType", ""},
		{"item", "Body", "DataTypes$ObjectType", "MyFirstModule.Item"},
	}
	if len(derived) != len(want) {
		t.Fatalf("parameters = %d, want %d", len(derived), len(want))
	}
	for i, w := range want {
		p := derived[i]
		typ := paramDoc(t, p["Type"])
		if p["Name"] != w.name || p["ParameterType"] != w.kind || typ["$Type"] != w.typ ||
			p["MicroflowParameter"] != "MyFirstModule.GetItem."+w.name {
			t.Errorf("parameter %d = %v (Type %v), want %+v", i, p, typ, w)
		}
		if w.ref != "" && typ["Entity"] != w.ref {
			t.Errorf("parameter %s entity = %v, want %s", w.name, typ["Entity"], w.ref)
		}
	}

	fallback := paramItems(t, ops[1], "Parameters")
	if len(fallback) != 1 || fallback[0]["Name"] != "code" || fallback[0]["ParameterType"] != "Path" ||
		paramDoc(t, fallback[0]["Type"])["$Type"] != "DataTypes$StringType" {
		t.Errorf("fallback parameters = %v, want one String path parameter 'code'", fallback)
	}
}

// paramItems returns the elements of a marker-prefixed BSON list property.
func paramItems(t *testing.T, doc map[string]any, key string) []map[string]any {
	t.Helper()
	arr, ok := doc[key].(bson.A)
	if !ok || len(arr) < 1 {
		t.Fatalf("%s = %#v, want a marker-prefixed list", key, doc[key])
	}
	var out []map[string]any
	for _, item := range arr[1:] {
		out = append(out, paramDoc(t, item))
	}
	return out
}

func paramDoc(t *testing.T, v any) map[string]any {
	t.Helper()
	switch m := v.(type) {
	case bson.M:
		return m
	case bson.D:
		out := make(map[string]any, len(m))
		for _, e := range m {
			out[e.Key] = e.Value
		}
		return out
	case map[string]any:
		return m
	}
	t.Fatalf("value has type %T, want a document", v)
	return nil
}
