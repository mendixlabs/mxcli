// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// TestListMicroflows_ActivitiesHaveUniqueIDs guards the activities_data.Id
// collision: every flow object read back must carry its real, distinct ID. The
// catalog (full mode / REFRESH CATALOG FULL) keys activities_data on the activity
// Id, so empty IDs collide on the second activity of any microflow.
func TestListMicroflows_ActivitiesHaveUniqueIDs(t *testing.T) {
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
	mf := &microflows.Microflow{ContainerID: mod.ID, Name: "ZzActIDs"}
	mf.ObjectCollection = &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.ActionActivity{Action: &microflows.LogMessageAction{LogLevel: "Info", MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "a"}}}},
			&microflows.ActionActivity{Action: &microflows.LogMessageAction{LogLevel: "Info", MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "b"}}}},
		},
	}
	if err := b.CreateMicroflow(mf); err != nil {
		t.Fatalf("CreateMicroflow: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	mfs, err := b2.ListMicroflows()
	if err != nil {
		t.Fatalf("ListMicroflows: %v", err)
	}
	for _, m := range mfs {
		if m.Name != "ZzActIDs" || m.ObjectCollection == nil {
			continue
		}
		seen := map[model.ID]bool{}
		for _, o := range m.ObjectCollection.Objects {
			id := o.GetID()
			if id == "" {
				t.Errorf("flow object %T has empty ID (activities_data.Id collision risk)", o)
			}
			if seen[id] {
				t.Errorf("duplicate flow object ID %q", id)
			}
			seen[id] = true
		}
		return
	}
	t.Fatal("ZzActIDs not found after create")
}
