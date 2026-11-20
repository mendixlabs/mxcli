// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"
)

// `Visible: roles (…)` on a tab page is stored. buildTabPageV3 applies it through
// childVisibility (mendixlabs/mxcli#1460), describe prints it back, and MxBuild
// 11.16.1 accepts an own-module role on a tab page (a role of another module is
// CE0149, which is its own check). But tabPageStoredProps did not list
// `visibleroles`, so `check` refused the property as "cannot be stored on a tab
// page" (MDL-WIDGET44) under mdl 1, and a headerless script was warned it was
// not written when it was.
func TestTabPage_VisibleRolesIsWritten(t *testing.T) {
	_, tc := tabContainerOf(t, "", `
      tabpage tpA (Caption: 'A', Visible: roles (M.Viewer)) { dynamictext t1 (Content: 'A') }
      tabpage tpB (Caption: 'B', Visible: roles (M.Viewer, M.Other)) { dynamictext t2 (Content: 'B') }
      tabpage tpC (Caption: 'C', Visible: false, Visible: roles (M.Viewer)) { dynamictext t3 (Content: 'C') }
      tabpage tpD (Caption: 'D') { dynamictext t4 (Content: 'D') }
`)
	got := buildTabs(t, visibleWhenPB("M.Job"), tc)
	want := [][]string{{"M.Viewer"}, {"M.Viewer", "M.Other"}, {"M.Viewer"}, nil}
	for i, w := range want {
		cv := got.TabPages[i].ConditionalVisibility
		if w == nil {
			if cv != nil {
				t.Errorf("tab page %d: no Visible: but got a setting %+v", i, cv)
			}
			continue
		}
		if cv == nil || !reflect.DeepEqual(cv.ModuleRoles, w) {
			t.Errorf("tab page %d: ModuleRoles = %+v, want %v", i, cv, w)
		}
	}
	// The expression beside the roles is kept (tpC).
	if cv := got.TabPages[2].ConditionalVisibility; cv == nil || cv.Expression != "false" {
		t.Errorf("tpC: Expression = %+v, want false beside the roles", cv)
	}
}

// check must not refuse what the builder writes, under mdl 1 or without a header.
func TestCheck_TabPageVisibleRolesNotRefused(t *testing.T) {
	const tabs = `
      tabpage tpA (Caption: 'A', Visible: roles (M.Viewer)) { dynamictext t1 (Content: 'A') }
      tabpage tpB (Caption: 'B', Visible: false, Visible: roles (M.Viewer, M.Other)) { dynamictext t2 (Content: 'B') }
`
	for _, header := range []string{"mdl 1;\n", ""} {
		prog, _ := tabContainerOf(t, header, tabs)
		if vs := tabPageViolations(prog); len(vs) != 0 {
			t.Errorf("header %q: Visible: roles on a tab page was reported: %+v", header, vs)
		}
	}
}

// Controls: what a tab page really cannot store is still refused, with or
// without roles beside it.
func TestCheck_TabPageUnstorablePropsStillRefusedBesideRoles(t *testing.T) {
	prog, _ := tabContainerOf(t, "mdl 1;\n", `
      tabpage tpA (Caption: 'Due {1}', CaptionParams: ({1} = Title), Visible: roles (M.Viewer)) { dynamictext t1 (Content: 'A') }
`)
	vs := tabPageViolations(prog)
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1 for CaptionParams: %+v", len(vs), vs)
	}
	if got := vs[0].Message; !containsAll(got, "CaptionParams") || containsAll(got, "VisibleRoles") {
		t.Errorf("message should name CaptionParams and not VisibleRoles: %q", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
