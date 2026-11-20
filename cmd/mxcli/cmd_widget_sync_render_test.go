// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
)

// ako/mxcli#1997: an applying run printed "Read-only: nothing was written."
// right before "Applied: N property change(s)". Only a dry run says so.
func TestRenderSyncPlan_ReadOnlyOnlyOnDryRun(t *testing.T) {
	plan := &executor.SyncPlan{Widgets: []executor.SyncWidgetPlan{{
		Container: "PageD", Widget: "fs", WidgetID: "com.mendix.widget.web.fieldset.Fieldset", PackageVer: "3.2.1",
		Changes: []executor.SyncPropertyChange{{Kind: executor.SyncRemove, Key: "legend", Detail: "not declared by Fieldset 3.2.1"}},
	}}}
	for _, dry := range []bool{true, false} {
		var out bytes.Buffer
		renderSyncPlan(&out, plan, dry)
		if !strings.Contains(out.String(), "legend") {
			t.Fatalf("dry=%v: the plan was not rendered:\n%s", dry, out.String())
		}
		if got := strings.Contains(out.String(), "nothing was written"); got != dry {
			t.Errorf("dry=%v: says nothing was written = %v:\n%s", dry, got, out.String())
		}
	}
}
