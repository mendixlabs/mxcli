// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#2356: SEC012 reported data a user role may not read on a page it
// reaches, saying "mxbuild reports CE2729" — 19 times on a real 11.15 project
// where mx check reported 0. Every one was inside a container visible only to
// module roles that may read it (`Visible: roles (…)`): mxbuild does not check
// a widget hidden from the role, and neither does the rule now.

// rawPageReader serves one page's stored widget tree for any id.
type rawPageReader struct {
	navSecurityReader
	raw map[string]any
}

func (r rawPageReader) GetRawUnit(model.ID) (map[string]any, error) { return r.raw, nil }

// pageWithGridIn is a page whose data grid over Probe.Ticket shows Title, inside
// a container visible only to the given module roles (none: no restriction).
func pageWithGridIn(visibleTo ...string) map[string]any {
	grid := map[string]any{
		"$Type":      "Forms$DataGrid",
		"DataSource": map[string]any{"$Type": "Forms$DataViewSource", "EntityRef": map[string]any{"Entity": "Probe.Ticket"}},
		"Columns": []any{int32(3), map[string]any{"$Type": "Forms$DataGridColumn",
			"AttributeRef": map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": "Probe.Ticket.Title"}}},
	}
	container := map[string]any{"$Type": "Forms$DivContainer", "Widgets": []any{int32(3), grid}}
	if len(visibleTo) > 0 {
		roles := []any{int32(1)}
		for _, r := range visibleTo {
			roles = append(roles, r)
		}
		container["ConditionalVisibilitySettings"] = map[string]any{
			"$Type": "Forms$ConditionalVisibilitySettings", "ModuleRoles": roles}
	}
	return map[string]any{"$Type": "Forms$Page", "Widgets": []any{int32(3), container}}
}

func (f *readAccessFixture) checkReachWithRaw(raw map[string]any) int {
	f.t.Helper()
	ctx := linter.NewLintContext(f.cat, rawPageReader{navSecurityReader{sec: f.sec}, raw})
	return len(NewReachablePageReadAccessRule().Check(ctx))
}

func roleVisibilityFixture(t *testing.T) *readAccessFixture {
	f := reachFixture(t, security.SecurityLevelProduction)
	f.grid("Probe.P_Menu", "Probe.Ticket", "Probe.Ticket.Title", "Probe.User")
	f.menuItem("Probe.P_Menu")
	return f
}

func TestReachablePageRead_HiddenFromTheRoleIsSilent_Issue2356(t *testing.T) {
	f := roleVisibilityFixture(t)
	if n := f.checkReachWithRaw(pageWithGridIn("Probe.Admin")); n != 0 {
		t.Errorf("violations = %d, want none: the grid is visible only to Probe.Admin, which ProbeUser does not hold", n)
	}
}

// CONTROLS: visible to a role the user role holds, or not restricted, is
// reported as before; so is a page whose stored tree cannot be read.
func TestReachablePageRead_VisibleToTheRoleIsReported_Issue2356(t *testing.T) {
	f := roleVisibilityFixture(t)
	if n := f.checkReachWithRaw(pageWithGridIn("Probe.User", "Probe.Admin")); n != 1 {
		t.Errorf("visible to Probe.User: violations = %d, want 1", n)
	}
	if n := f.checkReachWithRaw(pageWithGridIn()); n != 1 {
		t.Errorf("no restriction: violations = %d, want 1", n)
	}
	if n := f.checkReachWithRaw(nil); n != 1 {
		t.Errorf("unreadable page: violations = %d, want 1", n)
	}
}
