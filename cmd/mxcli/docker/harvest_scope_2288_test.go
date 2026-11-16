// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"strings"
	"testing"
)

// unitOfType returns a unit of the given $Type in the project.
func unitOfType(t *testing.T, mprPath, unitType string) string {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatalf("open %s: %v", mprPath, err)
	}
	defer reader.Disconnect()
	units, err := reader.ListUnits()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range units {
		if u.Type == unitType {
			return string(u.ID)
		}
	}
	t.Fatalf("fixture has no %s", unitType)
	return ""
}

// ako/mxcli#2288 (mendixlabs/mxcli#1866): on a project with no stale widget
// definition, `fix widgets` wrote 13 units that hold no widget — a Java action,
// microflows, a domain model, a REST service — whose describe output was
// identical before and after: `mx update-widgets` re-saves units it loads, and
// the harvest wrote every one that was not semantically equal. It also turned a
// Java action's stored ExportLevel into Hidden (#2287). Only a document that can
// hold a widget is the tool's business; any other unit it re-saved is left as
// stored, and named.
func TestFixWidgetsHarvest_WritesOnlyWidgetDocuments_Issue2288(t *testing.T) {
	mprPath := v2Fixture(t)
	page := unitOfType(t, mprPath, "Forms$Page")
	snippet := unitOfType(t, mprPath, "Forms$Snippet")
	javaAction := unitOfType(t, mprPath, "JavaActions$JavaAction")
	microflow := unitOfType(t, mprPath, "Microflows$Microflow")

	stubMxTool(t, func(_ string, _ []string, _, _ io.Writer) error {
		for _, id := range []string{page, snippet, javaAction, microflow} {
			markUnit(t, mprPath, id, "resaved")
		}
		return nil
	})
	res, err := RunToolPreservingFormat("mx", mprPath, "update-widgets", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("RunToolPreservingFormat: %v", err)
	}
	for _, id := range []string{page, snippet} {
		if unitMarker(t, mprPath, id) != "resaved" {
			t.Errorf("widget document %s: the tool's change was not carried back", id)
		}
	}
	for _, id := range []string{javaAction, microflow} {
		if got := unitMarker(t, mprPath, id); got != "" {
			t.Errorf("unit %s holds no widget but was rewritten (marker %q)", id, got)
		}
	}
	if res.UnitsWritten != 2 {
		t.Errorf("UnitsWritten = %d, want 2 (the page and the snippet)", res.UnitsWritten)
	}
	left := strings.Join(res.LeftAsStored, "\n")
	if len(res.LeftAsStored) != 2 || !strings.Contains(left, "JavaActions$JavaAction") || !strings.Contains(left, "Microflows$Microflow") {
		t.Errorf("LeftAsStored = %v, want the Java action and the microflow", res.LeftAsStored)
	}
}

// CONTROL: the design-property rename is not scoped this way — a unit it
// changes is carried back whatever its type, as before.
func TestRenameDesignPropertiesHarvest_StillWritesAnyUnit_Issue2288(t *testing.T) {
	mprPath := v2Fixture(t)
	microflow := unitOfType(t, mprPath, "Microflows$Microflow")
	stubMxTool(t, func(_ string, _ []string, _, _ io.Writer) error {
		markUnit(t, mprPath, microflow, "renamed")
		return nil
	})
	res, err := RunToolPreservingFormat("mx", mprPath, "rename-design-properties", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("RunToolPreservingFormat: %v", err)
	}
	if unitMarker(t, mprPath, microflow) != "renamed" || res.UnitsWritten != 1 || len(res.LeftAsStored) != 0 {
		t.Errorf("rename-design-properties: UnitsWritten=%d LeftAsStored=%v, want the microflow written", res.UnitsWritten, res.LeftAsStored)
	}
}
