package property

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- Primitive edge cases ---

func TestPrimitiveGetBeforeInit(t *testing.T) {
	p := NewPrimitive[int32]("count", DecodeInt32)
	// No Init, no raw — should return zero
	if got := p.Get(); got != 0 {
		t.Errorf("Get() = %d, want 0", got)
	}
}

func TestPrimitiveSetOverridesLazy(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "name", Value: "lazy"}})
	p := NewPrimitive[string]("name", DecodeString)
	p.Init(raw)

	p.Set("eager")
	if p.Get() != "eager" {
		t.Errorf("Set should override lazy, got %q", p.Get())
	}
}

func TestPrimitiveFloat64(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "val", Value: 3.14}})
	p := NewPrimitive[float64]("val", DecodeFloat64)
	p.Init(raw)
	if got := p.Get(); got != 3.14 {
		t.Errorf("Get() = %f, want 3.14", got)
	}
}

func TestPrimitiveBSONValue(t *testing.T) {
	p := NewPrimitive[string]("name", DecodeString)
	p.Set("test")
	if p.BSONValue() != "test" {
		t.Errorf("BSONValue() = %v", p.BSONValue())
	}
}

// --- Part edge cases ---

func TestPartSetNil(t *testing.T) {
	p := NewPart[element.Element]("gen")
	p.Set(nil)
	if p.Get() != nil {
		t.Error("Set(nil) then Get should return nil")
	}
	if !p.Dirty() {
		t.Error("Set(nil) should still mark dirty")
	}
}

func TestPartChildElement(t *testing.T) {
	p := NewPart[element.Element]("gen")
	if p.ChildElement() != nil {
		t.Error("unset Part.ChildElement should be nil")
	}
	child := &element.Base{}
	p.Set(child)
	if p.ChildElement() != child {
		t.Error("ChildElement should return the set child")
	}
}

// --- PartList edge cases ---

func TestPartListRemoveOutOfBoundsEdge(t *testing.T) {
	pl := NewPartList[element.Element]("items")
	pl.Append(&element.Base{})

	pl.Remove(-1)
	pl.Remove(999)
	if pl.Len() != 1 {
		t.Errorf("out-of-bounds Remove should not change Len, got %d", pl.Len())
	}
}

func TestPartListChildElements(t *testing.T) {
	pl := NewPartList[element.Element]("items")
	a := &element.Base{}
	b := &element.Base{}
	pl.AppendFromDecode(a)
	pl.AppendFromDecode(b)

	children := pl.ChildElements()
	if len(children) != 2 {
		t.Errorf("ChildElements len = %d, want 2", len(children))
	}
	if children[0] != a || children[1] != b {
		t.Error("ChildElements order wrong")
	}
}

func TestPartListAppendFromDecodeNotDirty(t *testing.T) {
	pl := NewPartList[element.Element]("items")
	pl.AppendFromDecode(&element.Base{})
	if pl.Dirty() {
		t.Error("AppendFromDecode should not mark dirty")
	}
}

// --- ByNameRef edge cases ---

func TestByNameRefSetFromDecodeNotDirty(t *testing.T) {
	r := NewByNameRef[element.Element]("img", "Images$Image")
	r.SetFromDecode("Mod.Img")
	if r.Dirty() {
		t.Error("SetFromDecode should not mark dirty")
	}
	if r.QualifiedName() != "Mod.Img" {
		t.Errorf("QualifiedName = %q", r.QualifiedName())
	}
}

func TestByNameRefBSONValue(t *testing.T) {
	r := NewByNameRef[element.Element]("img", "Images$Image")
	r.SetQualifiedName("Mod.Img")
	if r.BSONValue() != "Mod.Img" {
		t.Errorf("BSONValue = %v", r.BSONValue())
	}
}

// --- ByNameRefList edge cases ---

func TestByNameRefListAppend(t *testing.T) {
	r := NewByNameRefList[element.Element]("roles", "Security$ModuleRole")
	r.Append("Admin")
	r.Append("User")
	if len(r.QualifiedNames()) != 2 {
		t.Errorf("len = %d", len(r.QualifiedNames()))
	}
	if !r.Dirty() {
		t.Error("should be dirty after Append")
	}
}

// TestByNameRefList_BSONValue_PrependsVersionPrefix verifies that BSONValue()
// returns a []any with int32(1) version prefix followed by the qualified names.
// Mendix versioned string arrays (AllowedModuleRoles, ModuleRoles, etc.) require
// this prefix — omitting it causes CE0003 in Studio Pro.
func TestByNameRefList_BSONValue_PrependsVersionPrefix(t *testing.T) {
	r := NewByNameRefList[element.Element]("roles", "Security$ModuleRole")
	r.SetQualifiedNames([]string{"MyModule.User", "MyModule.Editor"})
	bv := r.BSONValue()
	arr, ok := bv.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", bv)
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 entries (1 version + 2 names), got %d: %v", len(arr), arr)
	}
	v, ok := arr[0].(int32)
	if !ok || v != 1 {
		t.Errorf("expected arr[0] = int32(1), got %v (%T)", arr[0], arr[0])
	}
	if arr[1] != "MyModule.User" || arr[2] != "MyModule.Editor" {
		t.Errorf("expected names at indices 1+2, got %v %v", arr[1], arr[2])
	}
}

// TestByNameRefList_BSONValue_EmptyNoPanic verifies that BSONValue() on an
// empty ByNameRefList returns a []any with only the version prefix (no panic).
func TestByNameRefList_BSONValue_EmptyNoPanic(t *testing.T) {
	r := NewByNameRefList[element.Element]("roles", "Security$ModuleRole")
	bv := r.BSONValue()
	arr, ok := bv.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", bv)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 entry (version only), got %d: %v", len(arr), arr)
	}
	v, ok := arr[0].(int32)
	if !ok || v != 1 {
		t.Errorf("expected arr[0] = int32(1), got %v (%T)", arr[0], arr[0])
	}
}

// TestByNameRefList_SetFromDecode_StripsVersionPrefix verifies that
// SetFromDecode does NOT store the version prefix — callers (gen-typed
// InitFromRaw) already strip it via StringValueOK(), so SetFromDecode must
// only receive plain string values.
func TestByNameRefList_SetFromDecode_RoundTrip(t *testing.T) {
	r := NewByNameRefList[element.Element]("roles", "Security$ModuleRole")
	// Simulate what InitFromRaw does: passes only the string values (no int32 prefix).
	r.SetFromDecode([]string{"MyModule.RoleA", "MyModule.RoleB"})
	if r.Dirty() {
		t.Error("SetFromDecode should not mark dirty")
	}
	bv := r.BSONValue()
	// BSONValue must NOT be called on a clean property in normal encoder flow
	// (encoder skips non-dirty), but if called, must still return versioned array.
	// After SetFromDecode we mark dirty to trigger BSONValue in this test.
	r.SetQualifiedNames(r.QualifiedNames()) // re-set same values to mark dirty
	bv = r.BSONValue()
	arr, ok := bv.([]any)
	if !ok {
		t.Fatalf("expected []any after roundtrip, got %T", bv)
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 entries after roundtrip, got %d: %v", len(arr), arr)
	}
	if v, ok := arr[0].(int32); !ok || v != 1 {
		t.Errorf("version prefix wrong after roundtrip: %v (%T)", arr[0], arr[0])
	}
}

// --- ByIdRef edge cases ---

func TestByIdRefSetFromDecodeNotDirty(t *testing.T) {
	r := NewByIdRef[element.Element]("child")
	r.SetFromDecode("id-123")
	if r.Dirty() {
		t.Error("SetFromDecode should not mark dirty")
	}
}

// --- Enum edge cases ---

func TestEnumSetFromDecodeNotDirty(t *testing.T) {
	e := NewEnum[string]("level")
	e.SetFromDecode("Hidden")
	if e.Dirty() {
		t.Error("SetFromDecode should not mark dirty")
	}
	if e.Get() != "Hidden" {
		t.Errorf("Get = %q", e.Get())
	}
}

func TestEnumBSONValue(t *testing.T) {
	e := NewEnum[string]("level")
	e.Set("API")
	if e.BSONValue() != "API" {
		t.Errorf("BSONValue = %v", e.BSONValue())
	}
}

// --- EnumList ---

func TestEnumListAppend(t *testing.T) {
	el := NewEnumList[string]("tags")
	el.Append("A")
	el.Append("B")
	if len(el.Items()) != 2 {
		t.Errorf("Items len = %d", len(el.Items()))
	}
	if !el.Dirty() {
		t.Error("should be dirty")
	}
}

func TestEnumListBSONValue(t *testing.T) {
	el := NewEnumList[string]("tags")
	el.Append("X")
	bv := el.BSONValue().([]string)
	if len(bv) != 1 || bv[0] != "X" {
		t.Errorf("BSONValue = %v", bv)
	}
}
