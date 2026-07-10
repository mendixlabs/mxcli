package codec

import (
	"bytes"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestFullRoundtripClean verifies decode → encode passthrough is byte-identical.
func TestFullRoundtripClean(t *testing.T) {
	r := NewRegistry()
	r.Register("Test$RT", func() element.Element {
		o := &element.Base{}
		p := property.NewPrimitive[string]("Name", property.DecodeString)
		p.Bind(o, 0)
		o.SetProperties([]element.Property{p})
		return o
	})

	original := mustMarshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Test$RT"},
		{Key: "Name", Value: "hello"},
		{Key: "Extra", Value: int32(99)},
	})

	dec := NewDecoder(r)
	elem, err := dec.Decode(original)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Element should be clean (not dirty)
	if elem.IsDirty() {
		t.Error("decoded element should not be dirty")
	}

	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if !bytes.Equal(out, []byte(original)) {
		t.Errorf("roundtrip mismatch: %d bytes in, %d bytes out", len(original), len(out))
	}
}

// TestFullRoundtripDirty verifies decode → modify → encode preserves unmodified fields.
func TestFullRoundtripDirty(t *testing.T) {
	r := NewRegistry()
	r.Register("Test$RTD", func() element.Element {
		o := &element.Base{}
		p := property.NewPrimitive[string]("Name", property.DecodeString)
		p.Bind(o, 0)
		o.SetProperties([]element.Property{p})
		return o
	})

	original := mustMarshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Test$RTD"},
		{Key: "Name", Value: "original"},
		{Key: "Unknown", Value: "preserved"},
	})

	dec := NewDecoder(r)
	elem, err := dec.Decode(original)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Modify the Name property
	props := elem.Properties()
	if len(props) == 0 {
		t.Fatal("no properties")
	}
	prim := props[0].(*property.Primitive[string])
	prim.Init(elem.Raw())
	if prim.Get() != "original" {
		t.Fatalf("Name = %q, want original", prim.Get())
	}
	prim.Set("modified")

	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Verify output
	var doc bson.D
	bson.Unmarshal(out, &doc)
	fields := map[string]any{}
	for _, e := range doc {
		fields[e.Key] = e.Value
	}

	if fields["Name"] != "modified" {
		t.Errorf("Name = %v, want modified", fields["Name"])
	}
	if fields["Unknown"] != "preserved" {
		t.Errorf("Unknown = %v, want preserved", fields["Unknown"])
	}
	if fields["$Type"] != "Test$RTD" {
		t.Errorf("$Type = %v", fields["$Type"])
	}
}

// TestDecodeEncodeWithChildren tests parent+children roundtrip.
func TestDecodeEncodeWithChildren(t *testing.T) {
	original := mustMarshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Test$Parent"},
		{Key: "Items", Value: bson.A{int32(3),
			bson.D{{Key: "$ID", Value: "c1"}, {Key: "$Type", Value: "Test$C1"}, {Key: "V", Value: "v1"}},
			bson.D{{Key: "$ID", Value: "c2"}, {Key: "$Type", Value: "Test$C2"}, {Key: "V", Value: "v2"}},
		}},
	})

	// No registry — children decode as *element.Base
	dec := NewDecoder(NewRegistry())
	elem, err := dec.Decode(original)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Clean passthrough
	enc := &Encoder{}
	out, err := enc.Encode(elem)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Equal(out, []byte(original)) {
		t.Error("clean roundtrip should be byte-identical")
	}
}
