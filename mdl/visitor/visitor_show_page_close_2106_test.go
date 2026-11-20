// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// ako/mxcli#2106: a show page action's "number of pages to close"
// (Forms$FormAction.NumberOfPagesToClose2) had no spelling, so describe → exec
// reset SnowflakeRESTSQL.ConnectionDetails_NewEdit_Step2's buttons from closing
// one page to closing none.
func TestActionSettings_ShowPageNumberOfPagesToClose(t *testing.T) {
	a, errs := buttonAction(t, "show page M.Next(Order = $currentObject) with (NumberOfPagesToClose: 1)")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if a.Settings == nil || a.Settings.NumberOfPagesToClose != "1" {
		t.Fatalf("settings = %+v, want NumberOfPagesToClose 1", a.Settings)
	}
}

// It is the show page action's setting only, and a whole number of at least one.
func TestActionSettings_NumberOfPagesToCloseRefusals(t *testing.T) {
	for _, action := range []string{
		"call microflow M.Go() with (NumberOfPagesToClose: 1)",
		"show page M.Next with (NumberOfPagesToClose: 0)",
		"show page M.Next with (NumberOfPagesToClose: 'x')",
	} {
		_, errs := buttonAction(t, action)
		if len(errs) == 0 {
			t.Errorf("%s: accepted, want a refusal", action)
			continue
		}
		if !strings.Contains(errs[0].Error(), "NumberOfPagesToClose") {
			t.Errorf("%s: refusal %q does not name the setting", action, errs[0])
		}
	}
}
