package property

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func mustMarshal(d bson.D) bson.Raw {
	b, err := bson.Marshal(d)
	if err != nil {
		panic(err)
	}
	return bson.Raw(b)
}

func TestPrimitiveString(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "name", Value: "Customer"}})

	p := NewPrimitive[string]("name", DecodeString)
	p.Init(raw)

	if got := p.Get(); got != "Customer" {
		t.Errorf("expected 'Customer', got %q", got)
	}
	if p.Dirty() {
		t.Error("expected Dirty() == false before Set()")
	}

	p.Set("Order")

	if got := p.Get(); got != "Order" {
		t.Errorf("expected 'Order' after Set(), got %q", got)
	}
	if !p.Dirty() {
		t.Error("expected Dirty() == true after Set()")
	}
}

func TestPrimitiveBool(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "excluded", Value: true}})

	p := NewPrimitive[bool]("excluded", DecodeBool)
	p.Init(raw)

	if got := p.Get(); !got {
		t.Errorf("expected true, got %v", got)
	}
}

func TestPrimitiveInt32(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "length", Value: int32(200)}})

	p := NewPrimitive[int32]("length", DecodeInt32)
	p.Init(raw)

	if got := p.Get(); got != 200 {
		t.Errorf("expected 200, got %d", got)
	}
}

func TestPrimitiveDefault(t *testing.T) {
	p := NewPrimitive[string]("name", DecodeString)
	// no Init call — raw remains nil

	if got := p.Get(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if p.Dirty() {
		t.Error("expected Dirty() == false when no raw and no Set()")
	}
}

// Mendix's reflection type "Integer" is bound in modelsdk/gen as
// Primitive[int32], but Studio Pro stores every such property as BSON int64 —
// measured on 15,354 units from nine Studio Pro-authored projects (Mendix
// 10.6.4 to 11.13.0): 208,038 integer property values, 0 of them int32, and every
// one of the 137 gen Integer properties that occurs there is int64-only. Only
// the leading typed-array marker of a list is int32, and that is not a
// Primitive. So the encoder must widen, or a codec-created page carries
// CanvasWidth/PopupWidth at a width Studio Pro never writes (ako/mxcli#1264,
// mendixlabs/mxcli#675).
func TestPrimitiveInt32EncodesAsBSONInt64(t *testing.T) {
	p := NewPrimitive[int32]("CanvasWidth", DecodeInt32)
	p.Set(1200)
	got, ok := p.BSONValue().(int64)
	if !ok {
		t.Fatalf("BSONValue() = %T(%v), want int64 — Studio Pro stores Integer properties as BSON int64", p.BSONValue(), p.BSONValue())
	}
	if got != 1200 {
		t.Errorf("BSONValue() = %d, want 1200", got)
	}

	// A value decoded from a stored int64 (what Studio Pro writes) re-encodes at
	// the same width, so a rebuild never narrows a Studio Pro document.
	q := NewPrimitive[int32]("PopupWidth", DecodeInt32)
	q.Init(mustMarshal(bson.D{{Key: "PopupWidth", Value: int64(640)}}))
	if v, ok := q.BSONValue().(int64); !ok || v != 640 {
		t.Errorf("decoded int64 re-encodes as %T(%v), want int64(640)", q.BSONValue(), q.BSONValue())
	}

	// Control: widening is for Integer only. Strings, booleans and doubles keep
	// their own Go type, so this is not "BSONValue converts everything".
	if v, ok := setPrimitive("s", "x").BSONValue().(string); !ok || v != "x" {
		t.Errorf("string primitive BSONValue = %T, want string", setPrimitive("s", "x").BSONValue())
	}
	if _, ok := setPrimitive("b", true).BSONValue().(bool); !ok {
		t.Error("bool primitive must stay bool")
	}
	if _, ok := setPrimitive("f", 1.5).BSONValue().(float64); !ok {
		t.Error("float64 primitive must stay float64")
	}
}

// setPrimitive builds a primitive of any T holding v, for the control above.
func setPrimitive[T any](name string, v T) *Primitive[T] {
	p := NewPrimitive[T](name, func(bson.Raw, string) T { var z T; return z })
	p.Set(v)
	return p
}
