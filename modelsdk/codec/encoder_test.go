package codec

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestEncoderNewElementHasBinaryID(t *testing.T) {
	elem := &element.Base{}
	elem.SetTypeName("Test$New")
	elem.SetID("aaaabbbb-cccc-dddd-eeee-ffffffffffff")
	elem.MarkDirty(63) // new element

	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	raw := bson.Raw(out)
	idVal, _ := raw.LookupErr("$ID")
	if idVal.Type.String() != "binary" {
		t.Errorf("$ID type = %s, want binary", idVal.Type)
	}
}

func TestEncoderNewElementWithNoIDHasBinaryID(t *testing.T) {
	// New element with no ID assigned (the bug case: NewAccessRule() leaves id="").
	elem := &element.Base{}
	elem.SetTypeName("Test$NoID")
	elem.MarkDirty(63) // new element flag

	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	raw := bson.Raw(out)
	idVal, err := raw.LookupErr("$ID")
	if err != nil {
		t.Fatalf("$ID field missing from encoded output")
	}
	if idVal.Type.String() != "binary" {
		t.Errorf("$ID type = %q, want \"binary\" — new element with no pre-set ID wrote a non-binary $ID", idVal.Type)
	}
	// Also confirm the binary is 16 bytes (a real UUID, not zero-length).
	_, data := idVal.Binary()
	if len(data) != 16 {
		t.Errorf("$ID binary length = %d, want 16", len(data))
	}
}

func TestEncoderPreservesUnknownFields(t *testing.T) {
	original := mustMarshal(bson.D{
		{Key: "$ID", Value: "id-1"},
		{Key: "$Type", Value: "Test$X"},
		{Key: "known", Value: "v1"},
		{Key: "unknown_field", Value: int32(42)},
		{Key: "another_unknown", Value: true},
	})

	knownProp := property.NewPrimitive[string]("known", property.DecodeString)
	knownProp.Init(original)

	elem := &element.Base{}
	elem.SetID("id-1")
	elem.SetTypeName("Test$X")
	elem.SetRaw(original)
	elem.SetProperties([]element.Property{knownProp})

	// Modify the known property
	knownProp.Bind(elem, 0)
	knownProp.Set("v2")

	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var doc bson.D
	bson.Unmarshal(out, &doc)
	fields := map[string]any{}
	for _, e := range doc {
		fields[e.Key] = e.Value
	}

	if fields["known"] != "v2" {
		t.Errorf("known = %v, want v2", fields["known"])
	}
	if fields["unknown_field"] != int32(42) {
		t.Errorf("unknown_field = %v, want 42", fields["unknown_field"])
	}
	if fields["another_unknown"] != true {
		t.Errorf("another_unknown = %v, want true", fields["another_unknown"])
	}
}

// TestEncoder_PropagatesPartChildDirty mirrors TestEncoderChildDirty
// (which covers PartList) for the Part property: the child is dirty
// because one of its scalar properties changed, but the Part property
// itself was never re-Set (the typical decode-and-mutate scenario —
// SetFromDecode wires the child without dirtying the Part). The
// encoder MUST still re-encode the Part subtree so the child's change
// reaches the output bytes; otherwise the parent passthroughs raw.
//
// This is the symmetric counterpart of PartList's anyChildDirty branch
// in Encoder.buildDoc and was uncovered by Stage 2.5 PageMutator
// round-trip tests where deep widget-tree edits did not persist.
func TestEncoder_PropagatesPartChildDirty(t *testing.T) {
	parentRaw := mustMarshal(bson.D{
		{Key: "$ID", Value: "p-1"},
		{Key: "$Type", Value: "Test$P"},
		{Key: "child", Value: bson.D{
			{Key: "$ID", Value: "c-1"},
			{Key: "$Type", Value: "Test$C"},
			{Key: "val", Value: "old"},
		}},
	})

	child := &element.Base{}
	child.SetID("c-1")
	child.SetTypeName("Test$C")
	child.SetRaw(mustMarshal(bson.D{
		{Key: "$ID", Value: "c-1"},
		{Key: "$Type", Value: "Test$C"},
		{Key: "val", Value: "old"},
	}))
	valProp := property.NewPrimitive[string]("val", property.DecodeString)
	valProp.Init(child.Raw())
	valProp.Bind(child, 0)
	child.SetProperties([]element.Property{valProp})

	parent := &element.Base{}
	parent.SetID("p-1")
	parent.SetTypeName("Test$P")
	parent.SetRaw(parentRaw)

	childPart := property.NewPart[element.Element]("child")
	childPart.Bind(parent, 0)
	parent.SetProperties([]element.Property{childPart})

	// Install the child via SetFromDecode (the decode path), NOT Set —
	// so the Part property itself stays clean. This mimics the
	// reopened-page-then-mutate-deep-child workflow. SetFromDecode also
	// wires child.container = parent so dirty propagation works.
	childPart.SetFromDecode(child)

	// Now mutate the child's scalar. With the container chain in place,
	// this dirties child + propagates MarkChildDirty up to parent.
	valProp.Set("new")
	if childPart.Dirty() {
		t.Fatal("setup: childPart should NOT be dirty after SetFromDecode")
	}
	if !child.IsDirty() {
		t.Fatal("setup: child must be dirty after valProp.Set")
	}
	if !parent.IsChildDirty() {
		t.Fatal("setup: parent must observe IsChildDirty after the child mutation")
	}

	enc := &Encoder{}
	out, err := enc.Encode(parent)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Parent encodes a 'child' field whose val == "new".
	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	found := false
	for _, e := range doc {
		if e.Key != "child" {
			continue
		}
		found = true
		childDoc, ok := e.Value.(bson.D)
		if !ok {
			t.Fatalf("child field is %T, want bson.D", e.Value)
		}
		gotVal := ""
		for _, f := range childDoc {
			if f.Key == "val" {
				if s, ok := f.Value.(string); ok {
					gotVal = s
				}
			}
		}
		if gotVal != "new" {
			t.Errorf("encoded child.val = %q, want %q (Part propagation broken — encoder passed through stale raw)", gotVal, "new")
		}
	}
	if !found {
		t.Error("encoded output missing 'child' field")
	}
}

func TestEncoderPartDirty(t *testing.T) {
	parentRaw := mustMarshal(bson.D{
		{Key: "$ID", Value: "p-1"},
		{Key: "$Type", Value: "Test$P"},
		{Key: "child", Value: bson.D{
			{Key: "$ID", Value: "c-1"},
			{Key: "$Type", Value: "Test$C"},
			{Key: "val", Value: "old"},
		}},
	})

	child := &element.Base{}
	child.SetID("c-1")
	child.SetTypeName("Test$C")
	child.SetRaw(mustMarshal(bson.D{
		{Key: "$ID", Value: "c-1"},
		{Key: "$Type", Value: "Test$C"},
		{Key: "val", Value: "old"},
	}))
	valProp := property.NewPrimitive[string]("val", property.DecodeString)
	valProp.Init(child.Raw())
	valProp.Bind(child, 0)
	valProp.Set("new")
	child.SetProperties([]element.Property{valProp})

	parent := &element.Base{}
	parent.SetID("p-1")
	parent.SetTypeName("Test$P")
	parent.SetRaw(parentRaw)

	childPart := property.NewPart[element.Element]("child")
	childPart.Bind(parent, 0)
	parent.SetProperties([]element.Property{childPart})

	// Mark the Part as dirty by setting the child
	childPart.Set(child)

	enc := &Encoder{}
	out, err := enc.Encode(parent)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var doc bson.D
	bson.Unmarshal(out, &doc)
	for _, e := range doc {
		if e.Key == "child" {
			childDoc := e.Value.(bson.D)
			for _, f := range childDoc {
				if f.Key == "val" && f.Value != "new" {
					t.Errorf("child.val = %v, want new", f.Value)
				}
			}
		}
	}
}
