// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestCreatePublishedRestService_RoundTrip creates a published REST service with
// one resource and two operations (one with a path parameter), then confirms it
// round-trips through ListPublishedRestServices.
func TestCreatePublishedRestService_RoundTrip(t *testing.T) {
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

	svc := &model.PublishedRestService{
		ContainerID: mod.ID,
		Name:        "ZzApi",
		Path:        "rest/zz/v1",
		Version:     "1.0.0",
		ServiceName: "Zz API",
		Resources: []*model.PublishedRestResource{{
			Name: "items",
			Operations: []*model.PublishedRestOperation{
				{Path: "", HTTPMethod: "GET", Microflow: "MyFirstModule.ACT_List"},
				{Path: "{id}", HTTPMethod: "GET", Microflow: "MyFirstModule.ACT_Get"},
			},
		}},
	}
	if err := b.CreatePublishedRestService(svc); err != nil {
		t.Fatalf("CreatePublishedRestService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListPublishedRestServices()
	if err != nil {
		t.Fatalf("ListPublishedRestServices: %v", err)
	}
	var got *model.PublishedRestService
	for _, s := range all {
		if s.Name == "ZzApi" {
			got = s
			break
		}
	}
	if got == nil {
		t.Fatalf("ZzApi not found after create")
	}
	if got.Path != "rest/zz/v1" || got.Version != "1.0.0" {
		t.Errorf("service header not round-tripped: %+v", got)
	}
	if len(got.Resources) != 1 || got.Resources[0].Name != "items" {
		t.Fatalf("resource not round-tripped: %+v", got.Resources)
	}
	if len(got.Resources[0].Operations) != 2 {
		t.Errorf("operations = %d, want 2", len(got.Resources[0].Operations))
	}
}
