// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2490: a stated String length in a business event message was not
// stored, so the build applied its 200 and mx check reported the length
// differing from the entity's. A stated length (0 for unlimited) is stored and
// read back; an unstated one stays absent, as before, so the build's 200 still
// applies to scripts that relied on it.
func TestBusinessEventMessageStringLengthRoundTrip_2490(t *testing.T) {
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
	svc := &model.BusinessEventService{
		ContainerID: mod.ID,
		Name:        "Lengths2490",
		Definition: &model.BusinessEventDefinition{
			ServiceName: "Lengths2490",
			Channels: []*model.BusinessEventChannel{{
				ChannelName: "ch1",
				Messages: []*model.BusinessEventMessage{{
					MessageName: "M", CanSubscribe: true,
					Attributes: []*model.BusinessEventAttribute{
						{AttributeName: "TaskId", AttributeType: "String", Length: 36, LengthSet: true},
						{AttributeName: "Text", AttributeType: "String", Length: 0, LengthSet: true},
						{AttributeName: "Note", AttributeType: "String"},
					},
				}},
			}},
		},
	}
	if err := b.CreateBusinessEventService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}
	all, err := b.ListBusinessEventServices()
	if err != nil {
		t.Fatal(err)
	}
	var got *model.BusinessEventService
	for _, s := range all {
		if s.Name == "Lengths2490" {
			got = s
		}
	}
	if got == nil {
		t.Fatal("service not read back")
	}
	attrs := map[string]*model.BusinessEventAttribute{}
	for _, a := range got.Definition.Channels[0].Messages[0].Attributes {
		attrs[a.AttributeName] = a
	}
	if a := attrs["TaskId"]; a == nil || !a.LengthSet || a.Length != 36 {
		t.Errorf("TaskId = %+v, want Length 36 stored", a)
	}
	if a := attrs["Text"]; a == nil || !a.LengthSet || a.Length != 0 {
		t.Errorf("Text = %+v, want Length 0 (unlimited) stored", a)
	}
	if a := attrs["Note"]; a == nil || a.LengthSet {
		t.Errorf("Note = %+v, want no Length stored (the build's default)", a)
	}
}
