package codec

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type stubElement struct {
	element.Base
}

// dirtyElement has a property that was modified.
type dirtyElement struct {
	element.Base
	name *property.Primitive[string]
}

func newDirtyElement(raw bson.Raw) *dirtyElement {
	e := &dirtyElement{
		name: property.NewPrimitive[string]("name", property.DecodeString),
	}
	e.SetRaw(raw)
	e.name.Init(raw)
	e.SetProperties([]element.Property{e.name})
	return e
}

func TestRegistryLookup(t *testing.T) {
	r := NewRegistry()
	r.Register("Test$Foo", func() element.Element { return &stubElement{} })

	factory, ok := r.Lookup("Test$Foo")
	if !ok {
		t.Fatal("should find registered type")
	}
	elem := factory()
	if elem == nil {
		t.Fatal("factory should return non-nil")
	}

	_, ok = r.Lookup("Test$Unknown")
	if ok {
		t.Error("should not find unregistered type")
	}
}

func TestDecoderBasic(t *testing.T) {
	r := NewRegistry()
	r.Register("Test$Foo", func() element.Element { return &stubElement{} })

	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 4, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Test$Foo"},
		{Key: "name", Value: "hello"},
	})

	dec := NewDecoder(r)
	elem, err := dec.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if elem.TypeName() != "Test$Foo" {
		t.Errorf("TypeName = %q", elem.TypeName())
	}
	if elem.Raw() == nil {
		t.Error("should retain raw bytes")
	}
}

func TestDecoderUnknownType(t *testing.T) {
	r := NewRegistry()
	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: "some-id"},
		{Key: "$Type", Value: "Unknown$Type"},
	})
	dec := NewDecoder(r)
	elem, err := dec.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if elem.TypeName() != "Unknown$Type" {
		t.Errorf("TypeName = %q", elem.TypeName())
	}
}

func TestDecoderMissingType(t *testing.T) {
	r := NewRegistry()
	raw := mustMarshal(bson.D{{Key: "$ID", Value: "some-id"}})
	dec := NewDecoder(r)
	_, err := dec.Decode(raw)
	if err == nil {
		t.Error("should error on missing $Type")
	}
}

func mustMarshal(d bson.D) bson.Raw {
	b, err := bson.Marshal(d)
	if err != nil {
		panic(err)
	}
	return bson.Raw(b)
}

func TestEncoderPassthrough(t *testing.T) {
	original := mustMarshal(bson.D{
		{Key: "$ID", Value: "test-id"},
		{Key: "$Type", Value: "Test$Foo"},
		{Key: "name", Value: "hello"},
	})

	elem := &stubElement{}
	elem.SetRaw(original)
	elem.SetTypeName("Test$Foo")
	elem.SetID("test-id")

	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(out) != len(original) {
		t.Errorf("passthrough size mismatch: %d vs %d", len(out), len(original))
	}
}

func TestEncoderNewElement(t *testing.T) {
	elem := &stubElement{}
	elem.SetTypeName("Test$New")
	elem.SetID("new-id")

	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Verify it contains our fields
	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	found := map[string]bool{}
	for _, e := range doc {
		found[e.Key] = true
	}
	if !found["$ID"] {
		t.Error("missing $ID")
	}
	if !found["$Type"] {
		t.Error("missing $Type")
	}
}

func TestEncoderChildDirty(t *testing.T) {
	// child0 — clean, no property overrides; raw bytes should pass through.
	child0 := &element.Base{}
	child0.SetID("child-0")
	child0.SetTypeName("Test$Child")
	child0.SetRaw(mustMarshal(bson.D{
		{Key: "$ID", Value: "child-0"}, {Key: "$Type", Value: "Test$Child"}, {Key: "val", Value: "original-0"},
	}))

	// child1 — dirty: property was modified.
	child1 := &element.Base{}
	child1.SetID("child-1")
	child1.SetTypeName("Test$Child")
	child1.SetRaw(mustMarshal(bson.D{
		{Key: "$ID", Value: "child-1"}, {Key: "$Type", Value: "Test$Child"}, {Key: "val", Value: "original-1"},
	}))
	nameProp := property.NewPrimitive[string]("val", property.DecodeString)
	nameProp.Set("modified-1")
	child1.SetProperties([]element.Property{nameProp})
	child1.MarkDirty(0) // simulate modification

	// Build parent with a PartList containing both children.
	items := property.NewPartList[element.Element]("items")
	items.AppendFromDecode(child0)
	items.AppendFromDecode(child1)

	parent := &element.Base{}
	parent.SetID("parent-id")
	parent.SetTypeName("Test$Parent")
	parent.SetRaw(mustMarshal(bson.D{
		{Key: "$ID", Value: "parent-id"},
		{Key: "$Type", Value: "Test$Parent"},
		{Key: "items", Value: bson.A{int32(3),
			bson.D{{Key: "$ID", Value: "child-0"}, {Key: "$Type", Value: "Test$Child"}, {Key: "val", Value: "original-0"}},
			bson.D{{Key: "$ID", Value: "child-1"}, {Key: "$Type", Value: "Test$Child"}, {Key: "val", Value: "original-1"}},
		}},
	}))
	parent.SetProperties([]element.Property{items})
	parent.MarkChildDirty() // parent is dirty because a child changed

	enc := &Encoder{}
	out, err := enc.Encode(parent)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	foundItems := false
	for _, e := range doc {
		if e.Key != "items" {
			continue
		}
		foundItems = true
		arr := e.Value.(bson.A)
		if len(arr) < 3 {
			t.Fatalf("items has %d elements, want >= 3 (marker + 2 children)", len(arr))
		}

		// child0 (arr[1]) should retain "original-0".
		child0Doc := arr[1].(bson.D)
		for _, f := range child0Doc {
			if f.Key == "val" && f.Value != "original-0" {
				t.Errorf("child0.val = %v, want original-0", f.Value)
			}
		}

		// child1 (arr[2]) should have "modified-1".
		child1Doc := arr[2].(bson.D)
		for _, f := range child1Doc {
			if f.Key == "val" && f.Value != "modified-1" {
				t.Errorf("child1.val = %v, want modified-1", f.Value)
			}
		}
		t.Log("child-dirty encoding verified")
	}
	if !foundItems {
		t.Error("items field not found in encoded output")
	}
}

func TestEncoderDirtyRebuild(t *testing.T) {
	// Create element with raw data, then modify a property.
	original := mustMarshal(bson.D{
		{Key: "$ID", Value: "test-id"},
		{Key: "$Type", Value: "Test$Foo"},
		{Key: "name", Value: "original"},
		{Key: "unknownField", Value: "preserved"},
	})

	elem := newDirtyElement(original)
	elem.SetTypeName("Test$Foo")
	elem.SetID("test-id")

	// Verify original value
	if elem.name.Get() != "original" {
		t.Fatalf("initial name = %q", elem.name.Get())
	}

	// Modify the property
	elem.name.Set("modified")
	elem.MarkDirty(0)

	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Decode and verify the output has the modified value
	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	fields := map[string]any{}
	for _, e := range doc {
		fields[e.Key] = e.Value
	}

	// Modified field should have new value
	if fields["name"] != "modified" {
		t.Errorf("name = %v, want 'modified'", fields["name"])
	}

	// Unknown field should be preserved
	if fields["unknownField"] != "preserved" {
		t.Errorf("unknownField = %v, want 'preserved'", fields["unknownField"])
	}

	// Identity fields should be intact
	if fields["$Type"] != "Test$Foo" {
		t.Errorf("$Type = %v", fields["$Type"])
	}
}
