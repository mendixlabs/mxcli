// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestCreateDataTransformer_RoundTrip creates a JSON/JSLT data transformer and
// confirms it round-trips through ListDataTransformers (name, source, step).
func TestCreateDataTransformer_RoundTrip(t *testing.T) {
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
	dt := &model.DataTransformer{
		ContainerID: mod.ID,
		Name:        "ZzXform",
		SourceType:  "JSON",
		SourceJSON:  `{"a":1}`,
		Steps:       []*model.DataTransformerStep{{Technology: "JSLT", Expression: `{"b": .a}`}},
	}
	if err := b.CreateDataTransformer(dt); err != nil {
		t.Fatalf("CreateDataTransformer: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	all, err := b2.ListDataTransformers()
	if err != nil {
		t.Fatalf("ListDataTransformers: %v", err)
	}
	for _, g := range all {
		if g.Name != "ZzXform" {
			continue
		}
		if g.SourceType != "JSON" || g.SourceJSON != `{"a":1}` {
			t.Errorf("source not round-tripped: type=%q json=%q", g.SourceType, g.SourceJSON)
		}
		if len(g.Steps) != 1 || g.Steps[0].Technology != "JSLT" || g.Steps[0].Expression != `{"b": .a}` {
			t.Errorf("step not round-tripped: %+v", g.Steps)
		}
		return
	}
	t.Fatal("ZzXform not found after create")
}
