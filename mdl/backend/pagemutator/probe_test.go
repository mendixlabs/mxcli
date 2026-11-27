// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// recordingDeps counts the one thing a dry run must never do.
type recordingDeps struct{ saves int }

func (d *recordingDeps) SerializeWidget(pages.Widget) bson.D                 { return nil }
func (d *recordingDeps) SerializeClientAction(pages.ClientAction) bson.D     { return nil }
func (*recordingDeps) SerializeFormDataSource(pages.DataSource, bool) bson.D { return nil }
func (d *recordingDeps) SerializeClientTemplate() bson.D                     { return nil }

func (d *recordingDeps) SerializeCustomWidgetDataSource(pages.DataSource) bson.D {
	return nil
}
func (d *recordingDeps) BuildDataGrid2Column(*backend.DataGridColumnSpec, string, map[string]pages.PropertyTypeIDEntry) (bson.D, error) {
	return nil, nil
}
func (d *recordingDeps) SerializeParameter(backend.ContainerKind, backend.PageParameterSpec) (bson.D, error) {
	return nil, nil
}
func (d *recordingDeps) SaveUnit(string, []byte) error { d.saves++; return nil }

// TestProbeWritesReachNothing is the property the whole check-time dry run rests
// on: a probe is written to, and neither the mutator it came from nor storage
// sees it.
//
// The second half is the control. Without it the test passes against a Probe
// that returns a mutator over a document nothing can change — which would also
// leave the original alone, and would report every property as fine.
func TestProbeWritesReachNothing(t *testing.T) {
	deps := &recordingDeps{}
	raw := makeRawPage(makePluggableWidget("dgProducts", "pageSize", "20"))
	m := New(raw, "unit-1", deps)

	probe, err := m.Probe()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if err := probe.SetWidgetProperty("dgProducts", "pageSize", 10); err != nil {
		t.Fatalf("set on probe: %v", err)
	}
	if got := pluggablePrimitive(t, raw, "dgProducts"); got != "20" {
		t.Errorf("original pageSize = %q after a probe write, want it untouched at %q", got, "20")
	}
	if deps.saves != 0 {
		t.Errorf("SaveUnit called %d times during a dry run, want 0", deps.saves)
	}

	// Control: the same write, on the mutator itself, does land. Without this
	// the assertion above is satisfied by a probe that writes nowhere at all.
	if err := m.SetWidgetProperty("dgProducts", "pageSize", 10); err != nil {
		t.Fatalf("set on mutator: %v", err)
	}
	if got := pluggablePrimitive(t, raw, "dgProducts"); got != "10" {
		t.Errorf("pageSize = %q after a real write, want %q", got, "10")
	}
}

// TestProbeRefusesToSave guards the direction that would turn `mxcli check` into
// a write: a caller that dry-runs an operation and then, by mistake, persists
// the copy.
func TestProbeRefusesToSave(t *testing.T) {
	deps := &recordingDeps{}
	m := New(makeRawPage(makePluggableWidget("dgProducts", "pageSize", "20")), "unit-1", deps)

	probe, err := m.Probe()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if err := probe.Save(); err == nil {
		t.Fatal("Save on a probe returned nil, want a refusal")
	}
	if deps.saves != 0 {
		t.Errorf("SaveUnit called %d times, want 0", deps.saves)
	}
	// Control: the mutator it came from still saves.
	if err := m.Save(); err != nil {
		t.Fatalf("Save on the real mutator: %v", err)
	}
	if deps.saves != 1 {
		t.Errorf("SaveUnit called %d times on a real save, want 1", deps.saves)
	}
}

// TestProbeReportsWhatTheSetterWouldReport pins the point of the dry run: the
// error a probe produces is the setter's own, so an author reads the same
// sentence from `check` that `exec` would have given them.
func TestProbeReportsWhatTheSetterWouldReport(t *testing.T) {
	m := New(makeRawPage(makePluggableWidget("dgProducts", "pageSize", "20")), "unit-1", &recordingDeps{})
	probe, err := m.Probe()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	probeErr := probe.SetWidgetProperty("dgProducts", "NoSuchProperty", 10)
	if probeErr == nil {
		t.Fatal("probe accepted an unknown pluggable property")
	}
	realErr := m.SetWidgetProperty("dgProducts", "NoSuchProperty", 10)
	if realErr == nil {
		t.Fatal("the setter accepted an unknown pluggable property")
	}
	if probeErr.Error() != realErr.Error() {
		t.Errorf("probe error %q != setter error %q", probeErr, realErr)
	}
}

// TestWidgetPropertyKeysNamesTheStoredTemplateKeys — the hint the check appends
// comes from the document, not from a widget definition on disk, so it is right
// for whatever widget package this project happens to have installed.
func TestWidgetPropertyKeysNamesTheStoredTemplateKeys(t *testing.T) {
	m := New(makeRawPage(makePluggableWidget("dgProducts", "pageSize", "20")), "unit-1", &recordingDeps{})

	keys := m.WidgetPropertyKeys("dgProducts", "")
	if len(keys) != 1 || keys[0] != "pageSize" {
		t.Errorf("WidgetPropertyKeys = %v, want [pageSize]", keys)
	}
	if got := m.WidgetPropertyKeys("noSuchWidget", ""); got != nil {
		t.Errorf("WidgetPropertyKeys for a missing widget = %v, want nil", got)
	}
	// A built-in widget declares nothing in the document; its vocabulary is the
	// setter's switch. Reporting an empty list as the widget's properties would
	// be worse than saying nothing, so the caller must get nothing.
	m2 := New(makeRawPage(makeWidget("topBar", "Forms$DivContainer")), "unit-2", &recordingDeps{})
	if got := m2.WidgetPropertyKeys("topBar", ""); len(got) != 0 {
		t.Errorf("WidgetPropertyKeys for a built-in widget = %v, want none", got)
	}
}

// TestResolvesTargetSeparatesMissingFromWrong lets a caller tell a target that
// is not there from one whose property is wrong, without parsing an error
// message to find out which.
func TestResolvesTargetSeparatesMissingFromWrong(t *testing.T) {
	m := New(makeRawPage(makePluggableWidget("dgProducts", "pageSize", "20")), "unit-1", &recordingDeps{})

	if !m.ResolvesTarget("dgProducts", "") {
		t.Error("stored widget did not resolve")
	}
	if m.ResolvesTarget("noSuchWidget", "") {
		t.Error("missing widget resolved")
	}
	if !m.ResolvesTarget("", "") {
		t.Error("page-level target did not resolve")
	}
	if m.ResolvesTarget("dgProducts", "NoSuchColumn") {
		t.Error("missing column resolved")
	}
}

func TestProbeErrorMentionsTheContainerKind(t *testing.T) {
	m := New(makeRawPage(), "unit-1", &recordingDeps{})
	probe, err := m.Probe()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if err := probe.Save(); err == nil || !strings.Contains(err.Error(), "page") {
		t.Errorf("Save refusal = %v, want it to name the container kind", err)
	}
}
