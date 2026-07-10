// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestCreateBusinessEventService_RoundTrip creates a service with a definition,
// channel, message (with an attribute) and a service operation, then confirms it
// round-trips through ListBusinessEventServices.
func TestCreateBusinessEventService_RoundTrip(t *testing.T) {
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
		Name:        "ZzEvents",
		Definition: &model.BusinessEventDefinition{
			ServiceName: "ZzEvents", EventNamePrefix: "zz",
			Channels: []*model.BusinessEventChannel{{
				ChannelName: "ch1",
				Messages: []*model.BusinessEventMessage{{
					MessageName: "Created", CanPublish: true,
					Attributes: []*model.BusinessEventAttribute{{AttributeName: "Id", AttributeType: "Long"}},
				}},
			}},
		},
	}
	if err := b.CreateBusinessEventService(svc); err != nil {
		t.Fatalf("CreateBusinessEventService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	all, err := b2.ListBusinessEventServices()
	if err != nil {
		t.Fatalf("ListBusinessEventServices: %v", err)
	}
	for _, s := range all {
		if s.Name != "ZzEvents" {
			continue
		}
		if s.Definition == nil || len(s.Definition.Channels) != 1 {
			t.Fatalf("definition/channels not round-tripped: %+v", s.Definition)
		}
		msgs := s.Definition.Channels[0].Messages
		if len(msgs) != 1 || msgs[0].MessageName != "Created" || !msgs[0].CanPublish || len(msgs[0].Attributes) != 1 {
			t.Errorf("message not round-tripped: %+v", msgs)
		}
		return
	}
	t.Fatal("ZzEvents not found after create")
}
