// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// roundTripMicroflow encodes a semantic microflow to gen, through the codec, and
// back to the semantic model — the exact write→read path any UpdateMicroflow
// caller takes. It is the precise reproduction for issue #723 §A: fields set on
// the way out but never read back are silently reset on the return trip.
func roundTripMicroflow(t *testing.T, mf *microflows.Microflow) *microflows.Microflow {
	t.Helper()
	gm := microflowToGen(mf, 11)
	raw, err := (&codec.Encoder{}).Encode(gm)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	dec, ok := el.(*genMf.Microflow)
	if !ok {
		t.Fatalf("decoded %T, want *genMf.Microflow", el)
	}
	return microflowFromGen(dec, "mod-1")
}

// TestMicroflowRoundTrip_ConcurrentExecutionFlags guards issue #723 §A (CE4899).
// AllowConcurrentExecution / MarkAsUsed were written by microflowToGen but never
// read back by microflowFromGen, so an UpdateMicroflow round-trip reset them to
// the Go zero value (false). A microflow that allowed concurrent execution then
// came back as "disallow concurrent execution" with no error message/microflow
// configured, and mx check reported CE4899 "concurrent execution: error message
// or microflow required".
func TestMicroflowRoundTrip_ConcurrentExecutionFlags(t *testing.T) {
	mf := &microflows.Microflow{
		Name:                     "ACT_Test",
		AllowConcurrentExecution: true,
		MarkAsUsed:               true,
	}
	mf.ID = model.ID("mf-1")

	got := roundTripMicroflow(t, mf)
	if !got.AllowConcurrentExecution {
		t.Error("AllowConcurrentExecution lost on round-trip → CE4899 (want true)")
	}
	if !got.MarkAsUsed {
		t.Error("MarkAsUsed lost on round-trip (want true)")
	}
}
