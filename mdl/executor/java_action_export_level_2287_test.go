// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2287 (mendixlabs/mxcli#1865): describe printed a stored
// ExportLevel "Public" as `-- export level: Public`. It is not a Mendix value —
// JavaActionsExportLevel is Hidden | API, and older mxcli wrote "Public" (#705)
// — and Mendix reads it as Hidden, so `fix widgets` (mx update-widgets)
// persisted Hidden and the level looked lost.
func TestJavaActionExportLevelComment_Issue2287(t *testing.T) {
	if got := javaActionExportLevelComment("Hidden"); got != "" {
		t.Errorf("Hidden: %q, want nothing (the default)", got)
	}
	if got := javaActionExportLevelComment(""); got != "" {
		t.Errorf("empty: %q, want nothing", got)
	}
	if got := javaActionExportLevelComment("API"); got != "-- export level: API" {
		t.Errorf("API: %q", got)
	}
	got := javaActionExportLevelComment("Public")
	for _, want := range []string{"not a Mendix export level", "reads it as Hidden", "API"} {
		if !strings.Contains(got, want) {
			t.Errorf("Public: %q does not say %q", got, want)
		}
	}
	if !strings.HasPrefix(got, "-- ") || strings.Contains(got, "\n") {
		t.Errorf("Public: %q must stay one comment line, so the output still parses", got)
	}
}
