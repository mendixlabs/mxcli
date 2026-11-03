// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1729 (ako/mxcli#1931): `in queue … retry fixed|exponential
// (…)` writes a Queues$QueueFixedRetry / Queues$QueueExponentialRetry under the
// call's QueueSettings, with exactly the properties the type has, and the
// reader decodes it back so describe can print it.
func TestQueueRetryWrittenAndReadBack(t *testing.T) {
	cases := map[string]struct {
		in      *microflows.QueueRetry
		typ     string
		present []string
	}{
		"fixed": {
			&microflows.QueueRetry{Kind: microflows.QueueRetryFixed, Retries: 5, Interval: 10, IntervalType: "Minutes"},
			"Queues$QueueFixedRetry", []string{"Retries", "Interval", "IntervalType"},
		},
		"exponential": {
			&microflows.QueueRetry{Kind: microflows.QueueRetryExponential, Retries: 20, InitialInterval: 1, MaximumInterval: 60, IntervalType: "Seconds"},
			"Queues$QueueExponentialRetry", []string{"Retries", "InitialInterval", "MaximumInterval", "IntervalType"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			el := queueSettingsToGen(&microflows.QueueSettings{Queue: "Ops.SendQueue", RetryPolicy: c.in})
			raw, err := (&codec.Encoder{}).Encode(el)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			retry, ok := bson.Raw(raw).Lookup("Retry").DocumentOK()
			if !ok {
				t.Fatalf("QueueSettings has no Retry document: %s", bson.Raw(raw))
			}
			if got := rawStr(retry, "$Type"); got != c.typ {
				t.Errorf("$Type = %q, want %q", got, c.typ)
			}
			elems, _ := retry.Elements()
			if len(elems) != len(c.present)+2 { // + $ID, $Type
				t.Errorf("Retry has %d properties, want exactly $ID, $Type and %v: %s", len(elems), c.present, retry)
			}
			for _, p := range c.present {
				if _, err := retry.LookupErr(p); err != nil {
					t.Errorf("Retry lacks %s: %s", p, retry)
				}
			}

			wrapped, _ := bson.Marshal(bson.D{{Key: "QueueSettings", Value: bson.Raw(raw)}})
			back := queueSettingsFromRaw(wrapped)
			if back == nil || back.RetryPolicy == nil || *back.RetryPolicy != *c.in {
				t.Errorf("read back %+v, want %+v", back.RetryPolicy, c.in)
			}
		})
	}

	// control: no retry writes Retry: null, as before
	el := queueSettingsToGen(&microflows.QueueSettings{Queue: "Ops.SendQueue"})
	raw, _ := (&codec.Encoder{}).Encode(el)
	if v := bson.Raw(raw).Lookup("Retry"); v.Type != bson.TypeNull {
		t.Errorf("an unretried queued call should store Retry: null, got %s", v.Type)
	}
}
