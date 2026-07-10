// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bsondebug "github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/bson"
	"go.mongodb.org/mongo-driver/bson"
)

// testReader creates a minimal Reader for roundtrip tests (no database needed).
func testReader() *Reader {
	return &Reader{version: MPRVersionV1}
}

// testWriter creates a minimal Writer for roundtrip tests (no database needed).
func testWriter() *Writer {
	return &Writer{reader: testReader()}
}

// toNDSL unmarshals raw BSON bytes and renders as Normalized DSL text.
func toNDSL(t *testing.T, data []byte) string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		t.Fatalf("failed to unmarshal BSON: %v", err)
	}
	return bsondebug.Render(doc, 0)
}

// roundtripPage: baseline → parse → serialize → parse → serialize → compare two serializations.
// Verifies serialization idempotency. Original baseline is preserved as ground truth.
func roundtripPage(t *testing.T, baselineBytes []byte) {
	t.Helper()
	r := testReader()
	w := testWriter()

	// First pass: baseline → parse → serialize
	page1, err := r.parsePage("test-unit-id", "test-container-id", baselineBytes)
	if err != nil {
		t.Fatalf("parsePage (pass 1) failed: %v", err)
	}
	serialized1, err := w.serializePage(page1)
	if err != nil {
		t.Fatalf("serializePage (pass 1) failed: %v", err)
	}

	// Second pass: serialized → parse → serialize
	page2, err := r.parsePage("test-unit-id", "test-container-id", serialized1)
	if err != nil {
		t.Fatalf("parsePage (pass 2) failed: %v", err)
	}
	serialized2, err := w.serializePage(page2)
	if err != nil {
		t.Fatalf("serializePage (pass 2) failed: %v", err)
	}

	ndsl1 := toNDSL(t, serialized1)
	ndsl2 := toNDSL(t, serialized2)

	if ndsl1 != ndsl2 {
		t.Errorf("serialization not idempotent for page %q\n--- pass 1 ---\n%s\n--- pass 2 ---\n%s\n--- diff ---\n%s",
			page1.Name, ndsl1, ndsl2, ndslDiff(ndsl1, ndsl2))
	}
}

// roundtripMicroflow: baseline → parse → serialize → parse → serialize → compare two serializations.
func roundtripMicroflow(t *testing.T, baselineBytes []byte) {
	t.Helper()
	r := testReader()
	w := testWriter()

	// First pass
	mf1, err := r.parseMicroflow("test-unit-id", "test-container-id", baselineBytes)
	if err != nil {
		t.Fatalf("parseMicroflow (pass 1) failed: %v", err)
	}
	serialized1, err := w.serializeMicroflow(mf1)
	if err != nil {
		t.Fatalf("serializeMicroflow (pass 1) failed: %v", err)
	}

	// Second pass
	mf2, err := r.parseMicroflow("test-unit-id", "test-container-id", serialized1)
	if err != nil {
		t.Fatalf("parseMicroflow (pass 2) failed: %v", err)
	}
	serialized2, err := w.serializeMicroflow(mf2)
	if err != nil {
		t.Fatalf("serializeMicroflow (pass 2) failed: %v", err)
	}

	ndsl1 := toNDSL(t, serialized1)
	ndsl2 := toNDSL(t, serialized2)

	if ndsl1 != ndsl2 {
		t.Errorf("serialization not idempotent for microflow %q\n--- pass 1 ---\n%s\n--- pass 2 ---\n%s\n--- diff ---\n%s",
			mf1.Name, ndsl1, ndsl2, ndslDiff(ndsl1, ndsl2))
	}
}

// roundtripNanoflow: baseline → parse → serialize → parse → serialize → compare two serializations.
func roundtripNanoflow(t *testing.T, baselineBytes []byte) {
	t.Helper()
	r := testReader()
	w := testWriter()

	// First pass
	nf1, err := r.parseNanoflow("test-unit-id", "test-container-id", baselineBytes)
	if err != nil {
		t.Fatalf("parseNanoflow (pass 1) failed: %v", err)
	}
	serialized1, err := w.serializeNanoflow(nf1)
	if err != nil {
		t.Fatalf("serializeNanoflow (pass 1) failed: %v", err)
	}

	// Second pass
	nf2, err := r.parseNanoflow("test-unit-id", "test-container-id", serialized1)
	if err != nil {
		t.Fatalf("parseNanoflow (pass 2) failed: %v", err)
	}
	serialized2, err := w.serializeNanoflow(nf2)
	if err != nil {
		t.Fatalf("serializeNanoflow (pass 2) failed: %v", err)
	}

	ndsl1 := toNDSL(t, serialized1)
	ndsl2 := toNDSL(t, serialized2)

	if ndsl1 != ndsl2 {
		t.Errorf("serialization not idempotent for nanoflow %q\n--- pass 1 ---\n%s\n--- pass 2 ---\n%s\n--- diff ---\n%s",
			nf1.Name, ndsl1, ndsl2, ndslDiff(ndsl1, ndsl2))
	}
}

// roundtripSnippet: double roundtrip idempotency test.
func roundtripSnippet(t *testing.T, baselineBytes []byte) {
	t.Helper()
	r := testReader()
	w := testWriter()

	snippet1, err := r.parseSnippet("test-unit-id", "test-container-id", baselineBytes)
	if err != nil {
		t.Fatalf("parseSnippet (pass 1) failed: %v", err)
	}
	serialized1, err := w.serializeSnippet(snippet1)
	if err != nil {
		t.Fatalf("serializeSnippet (pass 1) failed: %v", err)
	}

	snippet2, err := r.parseSnippet("test-unit-id", "test-container-id", serialized1)
	if err != nil {
		t.Fatalf("parseSnippet (pass 2) failed: %v", err)
	}
	serialized2, err := w.serializeSnippet(snippet2)
	if err != nil {
		t.Fatalf("serializeSnippet (pass 2) failed: %v", err)
	}

	ndsl1 := toNDSL(t, serialized1)
	ndsl2 := toNDSL(t, serialized2)

	if ndsl1 != ndsl2 {
		t.Errorf("serialization not idempotent for snippet %q\n--- pass 1 ---\n%s\n--- pass 2 ---\n%s\n--- diff ---\n%s",
			snippet1.Name, ndsl1, ndsl2, ndslDiff(ndsl1, ndsl2))
	}
}

// roundtripEnumeration: double roundtrip idempotency test.
func roundtripEnumeration(t *testing.T, baselineBytes []byte) {
	t.Helper()
	r := testReader()
	w := testWriter()

	enum1, err := r.parseEnumeration("test-unit-id", "test-container-id", baselineBytes)
	if err != nil {
		t.Fatalf("parseEnumeration (pass 1) failed: %v", err)
	}
	serialized1, err := w.serializeEnumeration(enum1)
	if err != nil {
		t.Fatalf("serializeEnumeration (pass 1) failed: %v", err)
	}

	enum2, err := r.parseEnumeration("test-unit-id", "test-container-id", serialized1)
	if err != nil {
		t.Fatalf("parseEnumeration (pass 2) failed: %v", err)
	}
	serialized2, err := w.serializeEnumeration(enum2)
	if err != nil {
		t.Fatalf("serializeEnumeration (pass 2) failed: %v", err)
	}

	ndsl1 := toNDSL(t, serialized1)
	ndsl2 := toNDSL(t, serialized2)

	if ndsl1 != ndsl2 {
		t.Errorf("serialization not idempotent for enumeration %q\n--- pass 1 ---\n%s\n--- pass 2 ---\n%s\n--- diff ---\n%s",
			enum1.Name, ndsl1, ndsl2, ndslDiff(ndsl1, ndsl2))
	}
}

// TestRoundtrip_Pages runs roundtrip tests on all page baselines in testdata/.
func TestRoundtrip_Pages(t *testing.T) {
	runRoundtripDir(t, "testdata/pages", roundtripPage)
}

// TestRoundtrip_Microflows runs roundtrip tests on all microflow baselines.
func TestRoundtrip_Microflows(t *testing.T) {
	runRoundtripDir(t, "testdata/microflows", roundtripMicroflow)
}

// TestRoundtrip_Nanoflows runs roundtrip tests on all nanoflow baselines.
func TestRoundtrip_Nanoflows(t *testing.T) {
	runRoundtripDir(t, "testdata/nanoflows", roundtripNanoflow)
}

// TestRoundtrip_Nanoflow_Synthetic tests parse→serialize→parse idempotency
// using programmatically constructed BSON (no .mxunit baseline needed).
func TestRoundtrip_Nanoflow_Synthetic(t *testing.T) {
	r := testReader()
	w := testWriter()

	tests := []struct {
		name string
		doc  bson.D
	}{
		{
			name: "minimal_void",
			doc: bson.D{
				{Key: "$ID", Value: "nf-test-1"},
				{Key: "$Type", Value: "Microflows$Nanoflow"},
				{Key: "AllowedModuleRoles", Value: bson.A{int32(3)}},
				{Key: "Documentation", Value: ""},
				{Key: "Excluded", Value: false},
				{Key: "Flows", Value: bson.A{int32(3)}},
				{Key: "MarkAsUsed", Value: false},
				{Key: "Name", Value: "NF_Minimal"},
			},
		},
		{
			name: "with_return_type",
			doc: bson.D{
				{Key: "$ID", Value: "nf-test-2"},
				{Key: "$Type", Value: "Microflows$Nanoflow"},
				{Key: "AllowedModuleRoles", Value: bson.A{int32(3)}},
				{Key: "Documentation", Value: "A nanoflow that returns a string"},
				{Key: "Excluded", Value: false},
				{Key: "Flows", Value: bson.A{int32(3)}},
				{Key: "MarkAsUsed", Value: true},
				{Key: "MicroflowReturnType", Value: bson.D{
					{Key: "$ID", Value: "rt-1"},
					{Key: "$Type", Value: "Datatypes$StringType"},
				}},
				{Key: "Name", Value: "NF_WithReturn"},
			},
		},
		{
			name: "with_parameters",
			doc: bson.D{
				{Key: "$ID", Value: "nf-test-3"},
				{Key: "$Type", Value: "Microflows$Nanoflow"},
				{Key: "AllowedModuleRoles", Value: bson.A{int32(3), "role-1", "role-2"}},
				{Key: "Documentation", Value: ""},
				{Key: "Excluded", Value: false},
				{Key: "Flows", Value: bson.A{int32(3)}},
				{Key: "MarkAsUsed", Value: false},
				{Key: "Name", Value: "NF_WithParams"},
				{Key: "ObjectCollection", Value: bson.D{
					{Key: "$ID", Value: "oc-1"},
					{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
					{Key: "Objects", Value: bson.A{
						int32(3),
						bson.D{
							{Key: "$ID", Value: "param-1"},
							{Key: "$Type", Value: "Microflows$MicroflowParameter"},
							{Key: "Name", Value: "Input"},
							{Key: "Documentation", Value: ""},
							{Key: "HasWidgetUsages", Value: false},
							{Key: "RelativeMiddlePoint", Value: bson.D{
								{Key: "$ID", Value: "rmp-1"},
								{Key: "$Type", Value: "Microflows$MicroflowObjectRelativeMiddlePoint"},
								{Key: "X", Value: int32(0)},
								{Key: "Y", Value: int32(0)},
							}},
							{Key: "Size", Value: bson.D{
								{Key: "$ID", Value: "sz-1"},
								{Key: "$Type", Value: "Microflows$MicroflowObjectSize"},
								{Key: "Width", Value: int32(30)},
								{Key: "Height", Value: int32(30)},
							}},
							{Key: "VariableType", Value: bson.D{
								{Key: "$ID", Value: "vt-1"},
								{Key: "$Type", Value: "Datatypes$StringType"},
							}},
						},
					}},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseline, err := bson.Marshal(tt.doc)
			if err != nil {
				t.Fatalf("failed to marshal synthetic BSON: %v", err)
			}

			// First pass: parse → serialize
			nf1, err := r.parseNanoflow("test-unit-id", "test-container-id", baseline)
			if err != nil {
				t.Fatalf("parseNanoflow (pass 1) failed: %v", err)
			}
			serialized1, err := w.serializeNanoflow(nf1)
			if err != nil {
				t.Fatalf("serializeNanoflow (pass 1) failed: %v", err)
			}

			// Second pass: serialized → parse → serialize
			nf2, err := r.parseNanoflow("test-unit-id", "test-container-id", serialized1)
			if err != nil {
				t.Fatalf("parseNanoflow (pass 2) failed: %v", err)
			}
			serialized2, err := w.serializeNanoflow(nf2)
			if err != nil {
				t.Fatalf("serializeNanoflow (pass 2) failed: %v", err)
			}

			ndsl1 := toNDSL(t, serialized1)
			ndsl2 := toNDSL(t, serialized2)

			if ndsl1 != ndsl2 {
				t.Errorf("serialization not idempotent:\n--- pass 1 ---\n%s\n--- pass 2 ---\n%s\n--- diff ---\n%s",
					ndsl1, ndsl2, ndslDiff(ndsl1, ndsl2))
			}

			// Verify basic fields survived
			if expectedName, ok := tt.doc.Map()["Name"].(string); ok {
				if nf1.Name != expectedName {
					t.Errorf("Name mismatch: got %q, want %q", nf1.Name, expectedName)
				}
			}
		})
	}
}

// TestRoundtrip_Nanoflow_WithActivities tests parse→serialize→parse idempotency
// for a nanoflow with ObjectCollection containing activities and flows.
func TestRoundtrip_Nanoflow_WithActivities(t *testing.T) {
	r := testReader()
	w := testWriter()

	doc := bson.D{
		{Key: "$ID", Value: "nf-act-1"},
		{Key: "$Type", Value: "Microflows$Nanoflow"},
		{Key: "AllowedModuleRoles", Value: bson.A{int32(3), "role-admin", "role-user"}},
		{Key: "Documentation", Value: "Nanoflow with activities"},
		{Key: "Excluded", Value: false},
		{Key: "Flows", Value: bson.A{
			int32(3),
			bson.D{
				{Key: "$ID", Value: "sf-1"},
				{Key: "$Type", Value: "Microflows$SequenceFlow"},
				{Key: "OriginConnectionIndex", Value: int32(0)},
				{Key: "DestinationConnectionIndex", Value: int32(0)},
				{Key: "OriginBezierVector", Value: bson.D{
					{Key: "$ID", Value: "bv-1"},
					{Key: "$Type", Value: "Microflows$BezierVector"},
					{Key: "X", Value: 0.0},
					{Key: "Y", Value: 0.0},
				}},
				{Key: "DestinationBezierVector", Value: bson.D{
					{Key: "$ID", Value: "bv-2"},
					{Key: "$Type", Value: "Microflows$BezierVector"},
					{Key: "X", Value: 0.0},
					{Key: "Y", Value: 0.0},
				}},
			},
		}},
		{Key: "MarkAsUsed", Value: true},
		{Key: "MicroflowReturnType", Value: bson.D{
			{Key: "$ID", Value: "rt-act"},
			{Key: "$Type", Value: "Datatypes$IntegerType"},
		}},
		{Key: "Name", Value: "NF_WithActivities"},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: "oc-act"},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{
				int32(3),
				bson.D{
					{Key: "$ID", Value: "start-1"},
					{Key: "$Type", Value: "Microflows$StartEvent"},
					{Key: "RelativeMiddlePoint", Value: bson.D{
						{Key: "$ID", Value: "rmp-s"},
						{Key: "$Type", Value: "Microflows$MicroflowObjectRelativeMiddlePoint"},
						{Key: "X", Value: int32(100)},
						{Key: "Y", Value: int32(100)},
					}},
					{Key: "Size", Value: bson.D{
						{Key: "$ID", Value: "sz-s"},
						{Key: "$Type", Value: "Microflows$MicroflowObjectSize"},
						{Key: "Width", Value: int32(20)},
						{Key: "Height", Value: int32(20)},
					}},
				},
				bson.D{
					{Key: "$ID", Value: "end-1"},
					{Key: "$Type", Value: "Microflows$EndEvent"},
					{Key: "RelativeMiddlePoint", Value: bson.D{
						{Key: "$ID", Value: "rmp-e"},
						{Key: "$Type", Value: "Microflows$MicroflowObjectRelativeMiddlePoint"},
						{Key: "X", Value: int32(400)},
						{Key: "Y", Value: int32(100)},
					}},
					{Key: "Size", Value: bson.D{
						{Key: "$ID", Value: "sz-e"},
						{Key: "$Type", Value: "Microflows$MicroflowObjectSize"},
						{Key: "Width", Value: int32(20)},
						{Key: "Height", Value: int32(20)},
					}},
					{Key: "ReturnValue", Value: ""},
				},
			}},
		}},
	}

	baseline, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal synthetic BSON: %v", err)
	}

	// First pass
	nf1, err := r.parseNanoflow("test-unit-id", "test-container-id", baseline)
	if err != nil {
		t.Fatalf("parseNanoflow (pass 1) failed: %v", err)
	}
	serialized1, err := w.serializeNanoflow(nf1)
	if err != nil {
		t.Fatalf("serializeNanoflow (pass 1) failed: %v", err)
	}

	// Second pass
	nf2, err := r.parseNanoflow("test-unit-id", "test-container-id", serialized1)
	if err != nil {
		t.Fatalf("parseNanoflow (pass 2) failed: %v", err)
	}
	serialized2, err := w.serializeNanoflow(nf2)
	if err != nil {
		t.Fatalf("serializeNanoflow (pass 2) failed: %v", err)
	}

	ndsl1 := toNDSL(t, serialized1)
	ndsl2 := toNDSL(t, serialized2)

	if ndsl1 != ndsl2 {
		t.Errorf("serialization not idempotent:\n--- pass 1 ---\n%s\n--- pass 2 ---\n%s\n--- diff ---\n%s",
			ndsl1, ndsl2, ndslDiff(ndsl1, ndsl2))
	}

	// Verify AllowedModuleRoles survived
	if len(nf1.AllowedModuleRoles) != 2 {
		t.Errorf("Expected 2 AllowedModuleRoles, got %d", len(nf1.AllowedModuleRoles))
	}

	// Verify ObjectCollection survived
	if nf1.ObjectCollection == nil {
		t.Error("Expected ObjectCollection to be parsed")
	}

	// Verify name survived
	if nf1.Name != "NF_WithActivities" {
		t.Errorf("Name mismatch: got %q", nf1.Name)
	}
}

// TestRoundtrip_Nanoflow_EmptyObjectCollection tests a nanoflow with an empty ObjectCollection.
func TestRoundtrip_Nanoflow_EmptyObjectCollection(t *testing.T) {
	r := testReader()
	w := testWriter()

	doc := bson.D{
		{Key: "$ID", Value: "nf-empty-oc"},
		{Key: "$Type", Value: "Microflows$Nanoflow"},
		{Key: "AllowedModuleRoles", Value: bson.A{int32(3)}},
		{Key: "Documentation", Value: ""},
		{Key: "Excluded", Value: false},
		{Key: "Flows", Value: bson.A{int32(3)}},
		{Key: "MarkAsUsed", Value: false},
		{Key: "Name", Value: "NF_EmptyOC"},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: "oc-empty"},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3)}},
		}},
	}

	baseline, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	nf1, err := r.parseNanoflow("test-unit-id", "test-container-id", baseline)
	if err != nil {
		t.Fatalf("parseNanoflow failed: %v", err)
	}
	serialized1, err := w.serializeNanoflow(nf1)
	if err != nil {
		t.Fatalf("serializeNanoflow failed: %v", err)
	}

	nf2, err := r.parseNanoflow("test-unit-id", "test-container-id", serialized1)
	if err != nil {
		t.Fatalf("parseNanoflow (pass 2) failed: %v", err)
	}
	serialized2, err := w.serializeNanoflow(nf2)
	if err != nil {
		t.Fatalf("serializeNanoflow (pass 2) failed: %v", err)
	}

	ndsl1 := toNDSL(t, serialized1)
	ndsl2 := toNDSL(t, serialized2)
	if ndsl1 != ndsl2 {
		t.Errorf("serialization not idempotent:\n--- pass 1 ---\n%s\n--- pass 2 ---\n%s", ndsl1, ndsl2)
	}
}

// TestRoundtrip_Snippets runs roundtrip tests on all snippet baselines.
func TestRoundtrip_Snippets(t *testing.T) {
	runRoundtripDir(t, "testdata/snippets", roundtripSnippet)
}

// TestRoundtrip_Enumerations runs roundtrip tests on all enumeration baselines.
func TestRoundtrip_Enumerations(t *testing.T) {
	runRoundtripDir(t, "testdata/enumerations", roundtripEnumeration)
}

// runRoundtripDir loads all .mxunit files from a directory and runs the given roundtrip function.
func runRoundtripDir(t *testing.T, dir string, fn func(*testing.T, []byte)) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no baseline directory: %s", dir)
			return
		}
		t.Fatalf("failed to read directory %s: %v", dir, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".mxunit") {
			continue
		}
		count++
		name := strings.TrimSuffix(entry.Name(), ".mxunit")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatalf("failed to read baseline: %v", err)
			}
			fn(t, data)
		})
	}
	if count == 0 {
		t.Skipf("no .mxunit baselines in %s", dir)
	}
}

// ndslDiff returns a simple line-by-line diff of two NDSL strings.
func ndslDiff(a, b string) string {
	linesA := strings.Split(a, "\n")
	linesB := strings.Split(b, "\n")

	var diffs []string
	maxLen := len(linesA)
	if len(linesB) > maxLen {
		maxLen = len(linesB)
	}

	for i := 0; i < maxLen; i++ {
		la, lb := "", ""
		if i < len(linesA) {
			la = linesA[i]
		}
		if i < len(linesB) {
			lb = linesB[i]
		}
		if la != lb {
			diffs = append(diffs, "- "+la)
			diffs = append(diffs, "+ "+lb)
		}
	}
	return strings.Join(diffs, "\n")
}
