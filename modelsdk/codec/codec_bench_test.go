package codec

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkEncodeCleanPassthrough(b *testing.B) {
	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Test$Bench"},
		{Key: "name", Value: "entity"},
		{Key: "doc", Value: "some documentation"},
		{Key: "count", Value: int32(100)},
	})

	elem := &element.Base{}
	elem.SetRaw(raw)

	enc := &Encoder{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.Encode(elem)
	}
}

func BenchmarkEncodeDirtyRebuild(b *testing.B) {
	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Test$Bench"},
		{Key: "name", Value: "original"},
		{Key: "doc", Value: "documentation"},
	})

	enc := &Encoder{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		elem := &element.Base{}
		elem.SetID("test-id")
		elem.SetTypeName("Test$Bench")
		elem.SetRaw(raw)
		p := property.NewPrimitive[string]("name", property.DecodeString)
		p.Init(raw)
		p.Bind(elem, 0)
		p.Set("modified")
		elem.SetProperties([]element.Property{p})
		enc.Encode(elem)
	}
}

func BenchmarkDecodeUnknownType(b *testing.B) {
	r := NewRegistry()
	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: "id-bench"},
		{Key: "$Type", Value: "Unknown$Type"},
	})
	dec := NewDecoder(r)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec.Decode(raw)
	}
}

func BenchmarkDecodeRegisteredType(b *testing.B) {
	r := NewRegistry()
	r.Register("Test$Bench", func() element.Element { return &element.Base{} })
	raw := mustMarshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Test$Bench"},
		{Key: "name", Value: "bench"},
	})
	dec := NewDecoder(r)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec.Decode(raw)
	}
}
