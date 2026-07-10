// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestRestRequestHandling_MappingVariableName guards the REST `body mapping X
// from $var` clause. The export-mapping source variable is stored under
// "MappingVariableName"; reading the wrong "ParameterVariable" key left it empty,
// so the renderer emitted `body mapping X` without the grammar-mandatory
// `from $var` — invalid MDL that broke the DESCRIBE roundtrip.
func TestRestRequestHandling_MappingVariableName(t *testing.T) {
	doc := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "rq-1"},
		{Key: "$Type", Value: "Microflows$MappingRequestHandling"},
		{Key: "ContentType", Value: "Json"},
		{Key: "MappingId", Value: "DatahubAPI.EM_EventRequest"},
		{Key: "MappingVariableName", Value: "eventRequest"},
	})
	h, ok := restRequestHandlingFromRaw(doc).(*microflows.MappingRequestHandling)
	if !ok {
		t.Fatalf("restRequestHandlingFromRaw → not a MappingRequestHandling")
	}
	if string(h.MappingID) != "DatahubAPI.EM_EventRequest" {
		t.Errorf("MappingID = %q", h.MappingID)
	}
	if h.ParameterVariable != "eventRequest" {
		t.Errorf("ParameterVariable = %q, want eventRequest (from MappingVariableName; empty → invalid 'body mapping' with no 'from')", h.ParameterVariable)
	}
}
