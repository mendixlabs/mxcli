// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2384 §5.4. Both roles of BusinessEvents$BusinessEventService were
// listed identically, so a consumer read as a contradiction — 0 messages and 1
// subscribe — and the channel id, which is the topic the broker derives
// (`businessevents.<channel id>.<space>`) and therefore the one value a
// publisher and a subscriber in another app must agree on, could not be read at
// all: `show contract channels` refused a publisher because it only looked at
// the cached contract, which a publisher does not have.

// twoRoleProject is a publisher and a consumer of the same event, the shape of
// ako/TestApp: the channel id in the publisher's definition is the same string
// as the channel key in the consumer's cached contract.
const sharedChannelID = "f2477dc90f714d69bb12df0df1ee9cce"

const consumerContract = `asyncapi: 2.2.0
info:
  title: "OrdersEventService"
  version: "1.0.0"
channels:
  f2477dc90f714d69bb12df0df1ee9cce:
    subscribe:
      operationId: receiveOrderPlaceEventEvents
      message:
        $ref: '#/components/messages/OrderPlaceEvent'
components:
  messages:
    OrderPlaceEvent:
      name: OrderPlaceEvent
      contentType: application/json
      payload:
        $ref: '#/components/schemas/OrderPlaceEventPayload'
  schemas:
    OrderPlaceEventPayload:
      type: object
      properties:
        OrderId:
          type: integer
          format: int64
        CustomerId:
          type: integer
          format: int64
`

func twoRoleProject(t *testing.T) (*mock.MockBackend, *ContainerHierarchy, *model.BusinessEventService) {
	t.Helper()
	pub := mkModule("CdcPublish")
	con := mkModule("BusinessEventsClient")

	publisher := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: "sp-pub"},
		ContainerID: pub.ID,
		Name:        "OrdersEventService",
		Definition: &model.BusinessEventDefinition{
			ServiceName: "OrdersEventService",
			Channels: []*model.BusinessEventChannel{{
				ChannelName: sharedChannelID,
				Messages: []*model.BusinessEventMessage{{
					MessageName: "OrderPlaceEvent",
					// Studio Pro's "Other apps can: Subscribe" — a published
					// message is one others subscribe to.
					CanSubscribe: true,
					Attributes: []*model.BusinessEventAttribute{
						{AttributeName: "OrderId", AttributeType: "Long"},
						{AttributeName: "CustomerId", AttributeType: "Long"},
					},
				}},
			}},
		},
		OperationImplementations: []*model.ServiceOperation{{
			MessageName: "OrderPlaceEvent", Operation: "publish",
			Entity: "CdcPublish.PBE_OrderPlaceEvent",
		}},
	}
	consumer := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: "sp-con"},
		ContainerID: con.ID,
		Name:        "OrdersEventClient",
		Document:    consumerContract,
		OperationImplementations: []*model.ServiceOperation{{
			MessageName: "OrderPlaceEvent", Operation: "subscribe",
			Entity:    "BusinessEventsClient.CBE_OrderPlaceEvent",
			Microflow: "BusinessEventsClient.HandleBE_OrderPlaceEvent",
		}},
	}

	h := mkHierarchy(pub, con)
	withContainer(h, publisher.ContainerID, pub.ID)
	withContainer(h, consumer.ContainerID, con.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{publisher, consumer}, nil
		},
	}
	return mb, h, publisher
}

func TestListBusinessEventServices_KindColumn(t *testing.T) {
	mb, h, _ := twoRoleProject(t)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listBusinessEventServices(ctx, ""))

	out := buf.String()
	if !strings.Contains(out, "Kind") {
		t.Fatalf("no Kind column:\n%s", out)
	}
	for _, want := range []string{"published", "consumed"} {
		if !strings.Contains(out, want) {
			t.Errorf("Kind %q missing:\n%s", want, out)
		}
	}
	// The kind is what makes the counts legible: a consumer's 0 messages is not
	// a defect, its messages live in the contract.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "OrdersEventClient") && !strings.Contains(line, "consumed") {
			t.Errorf("consumer row not marked consumed: %s", line)
		}
		if strings.Contains(line, "OrdersEventService") && !strings.Contains(line, "published") {
			t.Errorf("publisher row not marked published: %s", line)
		}
	}
}

func TestBusinessEventServiceKind_Discriminator(t *testing.T) {
	doc := "asyncapi: 2.2.0\n"
	for _, tc := range []struct {
		name string
		svc  *model.BusinessEventService
		want string
	}{
		{"publisher", &model.BusinessEventService{Definition: &model.BusinessEventDefinition{}}, "published"},
		{"consumer", &model.BusinessEventService{Document: doc}, "consumed"},
		// Not a shape Studio Pro writes; naming it is better than picking a side.
		{"both", &model.BusinessEventService{Definition: &model.BusinessEventDefinition{}, Document: doc}, "mixed"},
		{"neither", &model.BusinessEventService{}, "empty"},
		// Whitespace is not a contract.
		{"blank document", &model.BusinessEventService{Definition: &model.BusinessEventDefinition{}, Document: "  \n"}, "published"},
	} {
		if got := businessEventServiceKind(tc.svc); got != tc.want {
			t.Errorf("%s: kind = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The channel id is the join between the two apps, so both rows must show the
// same string — and the consumer's subscription must appear at all, which it
// did not: listBusinessEvents only walked Definition, so everything an app
// consumes was silently missing from the list.
func TestListBusinessEvents_ChannelIDAndConsumedEvents(t *testing.T) {
	mb, h, _ := twoRoleProject(t)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listBusinessEvents(ctx, ""))

	out := buf.String()
	var pubLine, conLine string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "OrdersEventService") && strings.Contains(line, "PUBLISH"):
			pubLine = line
		case strings.Contains(line, "OrdersEventClient") && strings.Contains(line, "SUBSCRIBE"):
			conLine = line
		}
	}
	if pubLine == "" {
		t.Fatalf("publisher's event missing:\n%s", out)
	}
	if conLine == "" {
		t.Fatalf("consumer's subscription missing — an app's consumed events were invisible:\n%s", out)
	}
	for _, line := range []string{pubLine, conLine} {
		if !strings.Contains(line, sharedChannelID) {
			t.Errorf("row does not carry the channel id %s: %s", sharedChannelID, line)
		}
	}
	// The consumer's attribute count comes from the contract's payload schema.
	if !strings.Contains(conLine, "2") {
		t.Errorf("consumer row lost its payload property count: %s", conLine)
	}
}

// `show contract channels from <publisher>` errored with "no cached AsyncAPI
// contract … (it may be a publisher, not a consumer)", although the channel id
// was stored right there in Definition.Channels.
func TestShowContractChannels_WorksOnAPublisher(t *testing.T) {
	mb, h, _ := twoRoleProject(t)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))

	name := ast.QualifiedName{Module: "CdcPublish", Name: "OrdersEventService"}
	if err := listContractChannels(ctx, &name); err != nil {
		t.Fatalf("show contract channels on a publisher: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, sharedChannelID) {
		t.Errorf("publisher's channel id not shown:\n%s", out)
	}
	// AsyncAPI 2's direction is the other app's: a published message is one
	// others subscribe to, which is what Studio Pro's export writes.
	if !strings.Contains(out, "subscribe") {
		t.Errorf("operation direction wrong — a published message is `subscribe` in AsyncAPI 2:\n%s", out)
	}
}

// A service with neither a contract nor a definition still has to refuse, and
// say which of the two is missing. Without this control the projection could
// return an empty document and the reader would print a blank table.
func TestShowContractChannels_RefusesWhenNothingStored(t *testing.T) {
	mod := mkModule("M")
	empty := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: "sp-empty"},
		ContainerID: mod.ID,
		Name:        "Nothing",
	}
	h := mkHierarchy(mod)
	withContainer(h, empty.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{empty}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	name := ast.QualifiedName{Module: "M", Name: "Nothing"}
	err := listContractChannels(ctx, &name)
	if err == nil {
		t.Fatal("a service with neither contract nor definition was accepted")
	}
	if !strings.Contains(err.Error(), "neither") {
		t.Errorf("refusal does not say what is missing: %v", err)
	}
}
