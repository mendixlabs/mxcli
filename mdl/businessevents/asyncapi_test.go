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

// The six types a business event attribute may have, each with the encoding
// Mendix writes (ako/mxcli#2384 M1). Asserted per type on the rendered YAML
// rather than through the golden file, because the golden fixture is all Long:
// it would keep passing whatever the other five rendered as, which is how a
// widening can ship with no evidence behind it.
//
// The three sources behind the table are in asyncAPIType's comment. Long is the
// one measured byte-for-byte, and TestRenderAsyncAPI_MatchesStudioProExport
// pins it.
func TestRenderAsyncAPI_EncodesEverySupportedType(t *testing.T) {
	for _, c := range []struct {
		mendix string
		want   string
	}{
		{"Boolean", "          type: boolean\n"},
		{"Decimal", "          type: number\n"},
		{"Integer", "          type: integer\n          format: int32\n"},
		{"Long", "          type: integer\n          format: int64\n"},
		{"String", "          type: string\n"},
		{"DateTime", "          type: string\n          format: date-time\n"},
		// mxcli's own spelling: Mendix has no date-only type, so it is a DateTime.
		{"Date", "          type: string\n          format: date-time\n"},
	} {
		t.Run(c.mendix, func(t *testing.T) {
			svc := fixturePublisher()
			svc.Definition.Channels[0].Messages[0].Attributes[0].AttributeType = c.mendix
			got, err := RenderAsyncAPI(svc)
			if err != nil {
				t.Fatalf("refused %s: %v", c.mendix, err)
			}
			want := "        OrderId:\n" + c.want
			if !strings.Contains(got, want) {
				t.Errorf("OrderId of type %s rendered as:\n%s\nwant it to contain:\n%s",
					c.mendix, payloadPropertiesOf(got), want)
			}
		})
	}
}

// payloadPropertiesOf is the schemas section of a render, for a readable
// failure message.
func payloadPropertiesOf(contract string) string {
	start := strings.Index(contract, "  schemas:\n")
	if start < 0 {
		return contract
	}
	rest := contract[start:]
	if end := strings.Index(rest, "  messageTraits:"); end >= 0 {
		return rest[:end]
	}
	return rest
}

// A type Mendix does not support for a business event attribute is refused, and
// the refusal says so rather than blaming a missing measurement. The list is
// Mendix's own ("Attribute Types", Business Events configuration).
func TestRenderAsyncAPI_RefusesTypesMendixDoesNotSupport(t *testing.T) {
	for _, typ := range []string{"AutoNumber", "Binary", "HashedString"} {
		t.Run(typ, func(t *testing.T) {
			svc := fixturePublisher()
			svc.Definition.Channels[0].Messages[0].Attributes[0].AttributeType = typ
			_, err := RenderAsyncAPI(svc)
			if err == nil {
				t.Fatalf("rendered %s, which Mendix does not support for a business event", typ)
			}
			var unsupported *UnsupportedAttributeTypeError
			if !asUnsupported(err, &unsupported) {
				t.Fatalf("error is not an UnsupportedAttributeTypeError: %v", err)
			}
			if unsupported.Attribute != "OrderId" || unsupported.Type != typ {
				t.Errorf("error does not name the attribute and type: %+v", unsupported)
			}
			if !strings.Contains(err.Error(), "does not support") {
				t.Errorf("refusal should say Mendix does not support the type: %v", err)
			}
			// It must name a type the author can switch to.
			if !strings.Contains(err.Error(), "String") {
				t.Errorf("refusal should name the supported types: %v", err)
			}
		})
	}
}

// An enumeration is supported by Mendix from Studio Pro 10.0, and its ITEMS go
// into the exported contract. A message attribute carries only a name, a type
// and a length, so the renderer has no items to write — and a bare `string`
// would import cleanly and be wrong, which is the one failure refusing exists
// to prevent. So it is refused, and the refusal explains that rather than
// claiming Mendix cannot do it.
func TestRenderAsyncAPI_RefusesEnumerationBecauseItsItemsAreMissing(t *testing.T) {
	svc := fixturePublisher()
	svc.Definition.Channels[0].Messages[0].Attributes[0].AttributeType = "Enumeration"
	_, err := RenderAsyncAPI(svc)
	if err == nil {
		t.Fatal("rendered an enumeration without its items")
	}
	for _, want := range []string{"items", "ContractFile"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal should mention %q: %v", want, err)
		}
	}
	// NOT the wording for a type Mendix rejects: an enumeration is supported.
	if strings.Contains(err.Error(), "does not support Enumeration") {
		t.Errorf("an enumeration is supported by Mendix; the refusal is about its items: %v", err)
	}
}

// A name that is not a Mendix attribute type at all still reaches the error,
// with the generic reason.
func TestRenderAsyncAPI_RefusesANonType(t *testing.T) {
	svc := fixturePublisher()
	svc.Definition.Channels[0].Messages[0].Attributes[0].AttributeType = "Sparkle"
	_, err := RenderAsyncAPI(svc)
	if err == nil || !strings.Contains(err.Error(), "not a Mendix attribute type") {
		t.Fatalf("want the generic refusal for an unknown type, got %v", err)
	}
}

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
