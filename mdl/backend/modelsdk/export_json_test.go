// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

// ExportJSON is fan-out, so the interesting failures are not in the assembly —
// they are a section that is silently empty because a listing returned nothing,
// which looks identical to a project that genuinely has none of that type.
// These pin the keys and pin that the sections carry real content on a fixture
// known to have it.

import (
	"encoding/json"
	"testing"
)

func exportSections(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	data, err := b.ExportJSON()
	if err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("ExportJSON produced invalid JSON: %v", err)
	}
	return out
}

// The key set is the legacy reader's, so a consumer of this dump survives the
// engine swap. A renamed or dropped key is a silent break for anyone parsing it.
func TestExportJSON_KeySet(t *testing.T) {
	got := exportSections(t)

	want := []string{"modules", "domainModels", "microflows", "nanoflows",
		"pages", "layouts", "enumerations", "constants"}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("section %q is missing", k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d sections, want %d: %v", len(got), len(want), got)
	}
}

// THE test that is not vacuous. Every section is nil-on-error, so an export
// where each listing failed is still valid JSON with all eight keys — and would
// pass the key-set test above. The fixture has modules, domain models,
// microflows, nanoflows and pages, so those five must carry content.
func TestExportJSON_SectionsAreNotEmpty(t *testing.T) {
	got := exportSections(t)

	// Layouts, enumerations and constants are deliberately excluded: the
	// fixture's counts are not guaranteed, and asserting on a section that
	// happens to be empty would make this test fail for the wrong reason.
	for _, k := range []string{"modules", "domainModels", "microflows", "nanoflows", "pages"} {
		var items []json.RawMessage
		if err := json.Unmarshal(got[k], &items); err != nil {
			t.Errorf("section %q is not an array (%s) — a failed listing serialises as null", k, got[k])
			continue
		}
		if len(items) == 0 {
			t.Errorf("section %q is empty; the fixture has %s, so the listing failed "+
				"and was swallowed by the nil-on-error path", k, k)
		}
	}
}
