// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// =============================================================================
// formatRestCallAction
// =============================================================================

func TestFormatRestCallAction_GET(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodGet,
			LocationTemplate: "https://api.example.com/orders",
		},
		ResultHandling: &microflows.ResultHandlingString{VariableName: "Response"},
	}
	got := e.formatRestCallAction(action)
	if got == "" {
		t.Fatal("expected non-empty output")
	}
	assertContains(t, got, "rest call get")
	assertContains(t, got, "'https://api.example.com/orders'")
	assertContains(t, got, "$Response = ")
	assertContains(t, got, "returns String")
}

func TestFormatRestCallAction_HttpResponse(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodGet,
			LocationTemplate: "https://api.example.com/orders",
		},
		ResultHandling: &microflows.ResultHandlingHttpResponse{VariableName: "Response"},
	}
	got := e.formatRestCallAction(action)
	assertContains(t, got, "$Response = ")
	assertContains(t, got, "returns response")
}

func TestFormatRestCallAction_POST_CustomBody(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodPost,
			LocationTemplate: "https://api.example.com/orders",
		},
		RequestHandling: &microflows.CustomRequestHandling{
			Template: `{"name": "test"}`,
		},
		ResultHandling: &microflows.ResultHandlingNone{},
	}
	got := e.formatRestCallAction(action)
	assertContains(t, got, "rest call post")
	assertContains(t, got, "body '{\"name\": \"test\"}'")
	assertContains(t, got, "returns Nothing")
}

func TestFormatRestCallAction_WithHeaders(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodGet,
			LocationTemplate: "https://api.example.com",
			CustomHeaders: []*microflows.HttpHeader{
				{Name: "Authorization", Value: "'Bearer ' + $Token"},
			},
		},
		ResultHandling: &microflows.ResultHandlingString{VariableName: "Resp"},
	}
	got := e.formatRestCallAction(action)
	assertContains(t, got, "header 'Authorization' = 'Bearer ' + $Token")
}

func TestFormatRestCallAction_WithAuth(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:        microflows.HttpMethodGet,
			LocationTemplate:  "https://api.example.com",
			UseAuthentication: true,
			Username:          "'admin'",
			Password:          "'secret'",
		},
		ResultHandling: &microflows.ResultHandlingString{},
	}
	got := e.formatRestCallAction(action)
	assertContains(t, got, "auth basic 'admin' password 'secret'")
}

func TestFormatRestCallAction_WithTimeout(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodGet,
			LocationTemplate: "https://api.example.com",
		},
		TimeoutExpression: "30",
		ResultHandling:    &microflows.ResultHandlingString{},
	}
	got := e.formatRestCallAction(action)
	assertContains(t, got, "timeout 30")
}

// `returns mapping ... as Module.Entity` (no LIST_OF) describes a single
// object result. SingleObject=true must produce the bare `as` form so the
// roundtrip preserves the call site's cardinality. PrivateCloudData's
// REST_GetEnvironmentByUUID (and any REST call binding the first item of a
// list-typed mapping) depends on this form: emitting `as list of` would
// make the builder produce a ListType return value and trip CE0117 at the
// microflow's End event.
func TestFormatRestCallAction_MappingSingleObject(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodGet,
			LocationTemplate: "https://example.com",
		},
		ResultHandling: &microflows.ResultHandlingMapping{
			MappingID:      "Synthetic.IMM_OneItem",
			ResultEntityID: "Synthetic.Item",
			ResultVariable: "Item",
			SingleObject:   true,
		},
	}
	got := e.formatRestCallAction(action)
	assertContains(t, got, "returns mapping Synthetic.IMM_OneItem as Synthetic.Item")
	if strings.Contains(got, "as list of") {
		t.Fatalf("expected single-object form, got list-of form:\n%s", got)
	}
}

// `returns mapping ... as list of Module.Entity` describes a list result.
// SingleObject=false must produce the `as list of` form so the builder
// reconstructs a ListType-bound result handling on re-execution.
func TestFormatRestCallAction_MappingListOf(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodGet,
			LocationTemplate: "https://example.com",
		},
		ResultHandling: &microflows.ResultHandlingMapping{
			MappingID:      "Synthetic.IMM_ManyItems",
			ResultEntityID: "Synthetic.Item",
			ResultVariable: "Items",
			SingleObject:   false,
		},
	}
	got := e.formatRestCallAction(action)
	assertContains(t, got, "returns mapping Synthetic.IMM_ManyItems as list of Synthetic.Item")
}
