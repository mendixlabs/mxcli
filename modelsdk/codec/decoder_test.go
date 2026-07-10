package codec

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- DecodeChild / DecodeChildren ---

func TestDecodeChildMissingKey(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "$Type", Value: "X"}})
	_, err := DecodeChild(raw, "noSuchKey")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestDecodeChildNotDocument(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "child", Value: "stringNotDoc"}})
	_, err := DecodeChild(raw, "child")
	if err == nil {
		t.Error("expected error for non-document value")
	}
}

func TestDecodeChildValid(t *testing.T) {
	raw := mustMarshal(bson.D{
		{Key: "child", Value: bson.D{
			{Key: "$ID", Value: "ch-1"},
			{Key: "$Type", Value: "Test$Child"},
		}},
	})
	elem, err := DecodeChild(raw, "child")
	if err != nil {
		t.Fatalf("DecodeChild: %v", err)
	}
	if elem.TypeName() != "Test$Child" {
		t.Errorf("TypeName = %q", elem.TypeName())
	}
}

// TestDecodeChildAliasNewCaseValue guards the storage-name alias for a
// SequenceFlow's branch case: Mendix stores it under "NewCaseValue" while the
// gen property looks it up as "CaseValue". Without the fieldAliases entry the
// case is never decoded, so enumeration/boolean splits read as case-less and
// DESCRIBE MICROFLOW drops the then/when body ("if cond then end if;").
func TestDecodeChildAliasNewCaseValue(t *testing.T) {
	raw := mustMarshal(bson.D{
		{Key: "NewCaseValue", Value: bson.D{
			{Key: "$ID", Value: "case-1"},
			{Key: "$Type", Value: "Test$Child"},
		}},
	})
	elem, err := DecodeChild(raw, "CaseValue")
	if err != nil {
		t.Fatalf("DecodeChild(CaseValue) should fall back to NewCaseValue: %v", err)
	}
	if elem.TypeName() != "Test$Child" {
		t.Errorf("TypeName = %q", elem.TypeName())
	}
}

func TestDecodeChildrenMissingKey(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "$Type", Value: "X"}})
	_, err := DecodeChildren(raw, "noSuchKey")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestDecodeChildrenSkipsNonDocument(t *testing.T) {
	raw := mustMarshal(bson.D{
		{Key: "items", Value: bson.A{
			int32(3), // Mendix marker — non-document, should be skipped
			bson.D{{Key: "$ID", Value: "c1"}, {Key: "$Type", Value: "Test$C"}},
		}},
	})
	elems, err := DecodeChildren(raw, "items")
	if err != nil {
		t.Fatalf("DecodeChildren: %v", err)
	}
	if len(elems) != 1 {
		t.Errorf("got %d elements, want 1 (marker skipped)", len(elems))
	}
}

// --- decodeID ---

func TestDecodeIDBinary(t *testing.T) {
	data := make([]byte, 16)
	for i := range data {
		data[i] = byte(i + 0xa0)
	}
	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: data}},
		{Key: "$Type", Value: "X"},
	})
	id := decodeID(raw)
	if id == "" {
		t.Error("decodeID returned empty for binary $ID")
	}
	t.Logf("decoded ID: %s", id)
}

func TestDecodeIDString(t *testing.T) {
	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: "some-string-id"},
		{Key: "$Type", Value: "X"},
	})
	id := decodeID(raw)
	if id != "some-string-id" {
		t.Errorf("decodeID = %q, want some-string-id", id)
	}
}

func TestDecodeIDMissing(t *testing.T) {
	raw := mustMarshal(bson.D{{Key: "$Type", Value: "X"}})
	id := decodeID(raw)
	if id != "" {
		t.Errorf("decodeID should be empty for missing $ID, got %q", id)
	}
}

// --- Decoder with RawInitializer ---

type initTracker struct {
	element.Base
	initialized bool
}

func (it *initTracker) InitFromRaw(raw bson.Raw) {
	it.initialized = true
}

func TestDecoderCallsRawInitializer(t *testing.T) {
	r := NewRegistry()
	r.Register("Test$Init", func() element.Element { return &initTracker{} })

	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: "id-1"},
		{Key: "$Type", Value: "Test$Init"},
	})

	dec := NewDecoder(r)
	elem, err := dec.Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	it := elem.(*initTracker)
	if !it.initialized {
		t.Error("InitFromRaw was not called")
	}
}
