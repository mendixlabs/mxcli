// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// TestCreateConsumedRestService_RoundTrip creates a consumed REST service with
// basic auth and two operations (a GET with a query param and a POST with a JSON
// body), then confirms it round-trips through ListConsumedRestServices.
func TestCreateConsumedRestService_RoundTrip(t *testing.T) {
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

	svc := &model.ConsumedRestService{
		ContainerID:    mod.ID,
		Name:           "ZzClient",
		BaseUrl:        "https://api.example.com",
		Authentication: &model.RestAuthentication{Scheme: "Basic", Username: "u", Password: "$MyMod.PwConst"},
		Operations: []*model.RestClientOperation{
			{Name: "GetItems", HttpMethod: "GET", Path: "/items", ResponseType: "JSON",
				QueryParameters: []*model.RestClientParameter{{Name: "q", DataType: "String"}}},
			{Name: "CreateItem", HttpMethod: "POST", Path: "/items", BodyType: "JSON", BodyVariable: "$Payload", ResponseType: "JSON"},
		},
	}
	if err := b.CreateConsumedRestService(svc); err != nil {
		t.Fatalf("CreateConsumedRestService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListConsumedRestServices()
	if err != nil {
		t.Fatalf("ListConsumedRestServices: %v", err)
	}
	var got *model.ConsumedRestService
	for _, s := range all {
		if s.Name == "ZzClient" {
			got = s
			break
		}
	}
	if got == nil {
		t.Fatalf("ZzClient not found after create")
	}
	if got.BaseUrl != "https://api.example.com" {
		t.Errorf("BaseUrl not round-tripped: %q", got.BaseUrl)
	}
	if len(got.Operations) != 2 {
		t.Errorf("operations = %d, want 2", len(got.Operations))
	}
}
