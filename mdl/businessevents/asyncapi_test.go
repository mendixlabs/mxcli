// SPDX-License-Identifier: Apache-2.0

package businessevents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// fixturePublisher is the model behind testdata/asyncapi-OrdersEventService.yaml:
// CdcPublish.OrdersEventService of ako/TestApp at 619864b, read out of the
// stored unit. The channel id is the one in the export, which is the point —
// it is what a subscriber in another app binds to.
func fixturePublisher() *model.BusinessEventService {
	return &model.BusinessEventService{
		Name: "OrdersEventService",
		Definition: &model.BusinessEventDefinition{
			ServiceName: "OrdersEventService",
			Channels: []*model.BusinessEventChannel{{
				ChannelName: "f2477dc90f714d69bb12df0df1ee9cce",
				Messages: []*model.BusinessEventMessage{{
					MessageName: "OrderPlaceEvent",
					// Studio Pro's "Other apps can: Subscribe".
					CanSubscribe: true,
					Attributes: []*model.BusinessEventAttribute{
						{AttributeName: "OrderId", AttributeType: "Long"},
						{AttributeName: "CustomerId", AttributeType: "Long"},
					},
				}},
			}},
		},
	}
}

// The golden test. Studio Pro exported this file from the model above, so a
// byte-identical render is the only evidence that the document a consuming app
// imports is the one Mendix would have produced. Anything weaker — "it is valid
// YAML", "it parses as AsyncAPI" — passes against a dozen documents that would
// bind a subscriber to the wrong channel or the wrong payload.
func TestRenderAsyncAPI_MatchesStudioProExport(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "asyncapi-OrdersEventService.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := RenderAsyncAPI(fixturePublisher())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != string(want) {
		t.Errorf("render does not match Studio Pro's export\n%s", firstDifference(got, string(want)))
	}
}

// The renderer's output is what gets stored as a consumer's Document, so it has
// to survive the round trip into storage unchanged — including the trailing
// newline, whose loss made describe → exec report Modified on an untouched
// service in part B.
func TestRenderAsyncAPI_EndsWithNewline(t *testing.T) {
	got, err := RenderAsyncAPI(fixturePublisher())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("render does not end with a newline; it ends %q", got[len(got)-20:])
	}
	if strings.Contains(got, "\r") {
		t.Error("render contains a raw carriage return; the fixture's \\r are YAML escapes, not CR bytes")
	}
}

// §9.1: only Long is measured. A contract that is subtly wrong is worse than
// one that was refused, because the consuming app imports it and finds out at
// deploy time — so an unmeasured type is an error naming the attribute and the
// way round it, not a guess.
func TestRenderAsyncAPI_RefusesUnmeasuredAttributeTypes(t *testing.T) {
	for _, typ := range []string{"String", "Integer", "Decimal", "Boolean", "DateTime", "Enumeration"} {
		t.Run(typ, func(t *testing.T) {
			svc := fixturePublisher()
			svc.Definition.Channels[0].Messages[0].Attributes[0].AttributeType = typ
			_, err := RenderAsyncAPI(svc)
			if err == nil {
				t.Fatalf("rendered %s without a measurement to back it", typ)
			}
			var unsupported *UnsupportedAttributeTypeError
			if !asUnsupported(err, &unsupported) {
				t.Fatalf("error is not an UnsupportedAttributeTypeError: %v", err)
			}
			if unsupported.Attribute != "OrderId" || unsupported.Type != typ {
				t.Errorf("error does not name the attribute and type: %+v", unsupported)
			}
			if !strings.Contains(err.Error(), "ContractFile") {
				t.Errorf("error does not say how to proceed: %v", err)
			}
		})
	}
}

// A consumed service has no definition: it already stores a contract, and a
// second one rendered from nothing would carry channel ids no producer has.
func TestRenderAsyncAPI_RefusesAServiceWithoutADefinition(t *testing.T) {
	_, err := RenderAsyncAPI(&model.BusinessEventService{Name: "OrdersEventClient", Document: "asyncapi: 2.2.0\n"})
	if err == nil {
		t.Fatal("rendered a contract for a service that has no definition")
	}
	if !strings.Contains(err.Error(), "consumed") {
		t.Errorf("error does not explain which shape this is: %v", err)
	}
}

func asUnsupported(err error, target **UnsupportedAttributeTypeError) bool {
	u, ok := err.(*UnsupportedAttributeTypeError)
	if ok {
		*target = u
	}
	return ok
}

// firstDifference reports where two renders diverge, with context. A 2.7 KB
// diff of YAML is unreadable in a test failure; the first differing line is
// what tells you which field drifted.
func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) && i < len(w); i++ {
		if g[i] != w[i] {
			return "line " + itoa(i+1) + ":\n  got  " + g[i] + "\n  want " + w[i]
		}
	}
	if len(g) != len(w) {
		return "line counts differ: got " + itoa(len(g)) + ", want " + itoa(len(w))
	}
	return "(no line differs; check trailing bytes)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
