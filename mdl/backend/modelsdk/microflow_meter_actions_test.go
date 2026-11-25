// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The metrics activities as Studio Pro 11.16.1 stores them (ako/TestApp,
// BusinessEventsClient.MetricsMf): every key below on every activity, Value only
// on Counter and Gauge, Tags a [2, …] list of Microflows$MeterTagMapping whose
// Key and Value are expressions ("'tag-abc'", quoted).
var studioProMeterKeys = map[string][]string{
	"Microflows$CounterMeterAction":          {"$ID", "$Type", "Description", "ErrorHandlingType", "Name", "Tags", "Value"},
	"Microflows$IncrementCounterMeterAction": {"$ID", "$Type", "Description", "ErrorHandlingType", "Name", "Tags"},
	"Microflows$GaugeMeterAction":            {"$ID", "$Type", "Description", "ErrorHandlingType", "Name", "Tags", "Value"},
}

func encodeMeterAction(t *testing.T, a microflows.MicroflowAction) bson.Raw {
	t.Helper()
	g := microflowActionToGen(a)
	if g == nil {
		t.Fatalf("%T: microflowActionToGen returned nil (an actionless activity, CE0008)", a)
	}
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode %T: %v", a, err)
	}
	return raw
}

func meterTag(k, v string) *microflows.MeterTag {
	t := &microflows.MeterTag{Key: k, Value: v}
	t.ID = model.ID("aaaaaaaa-0000-0000-0000-000000000001")
	return t
}

func TestMeterActionsWriteStudioProShape(t *testing.T) {
	cases := []struct {
		typ    string
		action microflows.MicroflowAction
		value  string
	}{
		{"Microflows$CounterMeterAction", &microflows.CounterMeterAction{
			Name: "@Metrics.NAME_COUNTER", Value: "4", Tags: []*microflows.MeterTag{meterTag("'tag-abc'", "'123'")},
		}, "4"},
		{"Microflows$IncrementCounterMeterAction", &microflows.IncrementCounterMeterAction{
			Name: "@Metrics.NAME_COUNTER", Tags: []*microflows.MeterTag{meterTag("'klm'", "'nop'")},
		}, ""},
		{"Microflows$GaugeMeterAction", &microflows.GaugeMeterAction{
			Name: "@Metrics.NAME_COUNTER", Value: "3.14", Tags: []*microflows.MeterTag{meterTag("'def'", "'xyz'")},
		}, "3.14"},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			raw := encodeMeterAction(t, c.action)
			elems, _ := raw.Elements()
			var keys []string
			for _, e := range elems {
				keys = append(keys, e.Key())
			}
			sort.Strings(keys)
			if got, want := strings.Join(keys, ","), strings.Join(studioProMeterKeys[c.typ], ","); got != want {
				t.Errorf("keys = %s, want Studio Pro's %s", got, want)
			}
			if got := raw.Lookup("$Type").StringValue(); got != c.typ {
				t.Errorf("$Type = %q, want %q", got, c.typ)
			}
			if got := raw.Lookup("ErrorHandlingType").StringValue(); got != "Rollback" {
				t.Errorf("ErrorHandlingType = %q, want Rollback (Studio Pro writes it explicitly)", got)
			}
			if got := raw.Lookup("Name").StringValue(); got != "@Metrics.NAME_COUNTER" {
				t.Errorf("Name = %q", got)
			}
			if c.value != "" {
				if got := raw.Lookup("Value").StringValue(); got != c.value {
					t.Errorf("Value = %q, want %q", got, c.value)
				}
			}
			tags := raw.Lookup("Tags").Array()
			vals, _ := tags.Values()
			if len(vals) != 2 {
				t.Fatalf("Tags has %d entries, want marker + one tag", len(vals))
			}
			if m, ok := vals[0].Int32OK(); !ok || m != 2 {
				t.Errorf("Tags marker = %v, want 2 (Studio Pro 11.16.1, all three activities)", vals[0])
			}
			tag := vals[1].Document()
			if got := tag.Lookup("$Type").StringValue(); got != "Microflows$MeterTagMapping" {
				t.Errorf("tag $Type = %q", got)
			}
			if k := tag.Lookup("Key").StringValue(); !strings.HasPrefix(k, "'") {
				t.Errorf("tag Key = %q, want the expression as written ('…', quoted)", k)
			}
		})
	}
}

// A new activity with no tags still writes Tags, as the bare marker: Studio Pro's
// property set has the key on every activity. Without the mandatory-list
// registration the encoder omits it.
func TestMeterActionWithoutTagsWritesEmptyMarker(t *testing.T) {
	raw := encodeMeterAction(t, &microflows.IncrementCounterMeterAction{Name: "'acme.orders'"})
	v, err := raw.LookupErr("Tags")
	if err != nil {
		t.Fatal("Tags is missing; Studio Pro writes it on every metrics activity")
	}
	vals, _ := v.Array().Values()
	if len(vals) != 1 || vals[0].Int32() != 2 {
		t.Errorf("empty Tags = %v, want [2]", vals)
	}
}

// Reading Studio Pro's document: each $Type maps to its own SDK action with the
// expressions verbatim. Before, all three read as UnsupportedAction.
func TestMeterActionsReadFromStudioProBSON(t *testing.T) {
	doc := func(typ string, withValue bool) bson.Raw {
		d := bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: make([]byte, 16)}},
			{Key: "$Type", Value: typ},
			{Key: "Description", Value: ""},
			{Key: "ErrorHandlingType", Value: "Rollback"},
			{Key: "Name", Value: "@Metrics.NAME_COUNTER"},
			{Key: "Tags", Value: bson.A{int32(2), bson.D{
				{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: append(make([]byte, 15), 1)}},
				{Key: "$Type", Value: "Microflows$MeterTagMapping"},
				{Key: "Key", Value: "'tag-abc'"},
				{Key: "Value", Value: "'123'"},
			}}},
		}
		if withValue {
			d = append(d, bson.E{Key: "Value", Value: "4"})
		}
		raw, err := bson.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	read := func(raw bson.Raw) microflows.MicroflowAction {
		el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		return actionFromGen(el)
	}
	wantTags := func(t *testing.T, tags []*microflows.MeterTag) {
		t.Helper()
		if len(tags) != 1 || tags[0].Key != "'tag-abc'" || tags[0].Value != "'123'" {
			t.Errorf("tags = %+v", tags)
		}
	}

	switch a := read(doc("Microflows$CounterMeterAction", true)).(type) {
	case *microflows.CounterMeterAction:
		if a.Name != "@Metrics.NAME_COUNTER" || a.Value != "4" || a.ErrorHandlingType != microflows.ErrorHandlingTypeRollback {
			t.Errorf("counter = %+v", a)
		}
		wantTags(t, a.Tags)
	default:
		t.Errorf("CounterMeterAction read as %T", a)
	}
	switch a := read(doc("Microflows$IncrementCounterMeterAction", false)).(type) {
	case *microflows.IncrementCounterMeterAction:
		wantTags(t, a.Tags)
	default:
		t.Errorf("IncrementCounterMeterAction read as %T", a)
	}
	switch a := read(doc("Microflows$GaugeMeterAction", true)).(type) {
	case *microflows.GaugeMeterAction:
		if a.Value != "4" {
			t.Errorf("gauge value = %q", a.Value)
		}
		wantTags(t, a.Tags)
	default:
		t.Errorf("GaugeMeterAction read as %T", a)
	}
}
