// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2534 (mendixlabs/mxcli#1926): a custom error handler on a Send REST
// request activity was dropped both ways — exec wrote no ErrorHandlingType and
// no error flow, describe never walked the branch — on the strength of a
// CE6035 that was never measured. Measured on 10.24.11, 11.12.6 and 11.16.1,
// one project each: no clause and both custom handlers build; only
// `on error continue` is CE6035.

func TestAuthorOnError_2534_SendRestRequest(t *testing.T) {
	for _, tc := range []struct {
		body string
		want microflows.ErrorHandlingType
	}{
		{"send rest request M.Api.GetStatus on error begin log error node 'R' 'x'; return; end error;",
			microflows.ErrorHandlingTypeCustom},
		{"$r = send rest request M.Api.GetStatus on error without rollback begin return; end error;",
			microflows.ErrorHandlingTypeCustomWithoutRollback},
	} {
		fb := buildFlowFromMDL(t, tc.body)
		if got := actionErrorHandlingTypes(fb); len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s\n  got %v, want first activity %q", tc.body, got, tc.want)
		}
		var errFlows int
		for _, f := range fb.flows {
			if f.IsErrorHandler {
				errFlows++
			}
		}
		if errFlows != 1 {
			t.Errorf("%s\n  %d error-handler flows, want 1 — the handler body was not wired", tc.body, errFlows)
		}
	}
	// Control: no clause leaves the field unset, so the writer keeps storing
	// Rollback exactly as before.
	fb := buildFlowFromMDL(t, "send rest request M.Api.GetStatus;")
	if got := actionErrorHandlingTypes(fb); len(got) != 1 || got[0] != "" {
		t.Errorf("no clause: got %v, want one activity with no ErrorHandlingType", got)
	}
}

func TestDescribe_2534_SendRestRequestErrorHandlerSurvives(t *testing.T) {
	for _, eh := range []microflows.ErrorHandlingType{
		microflows.ErrorHandlingTypeCustom, microflows.ErrorHandlingTypeCustomWithoutRollback,
	} {
		out := describeWithErrorHandler(t, &microflows.RestOperationCallAction{
			ErrorHandlingType: eh,
			Operation:         "M.Api.GetStatus",
		})
		if !strings.Contains(out, "rollback $Handled;") || !strings.Contains(out, "send rest request M.Api.GetStatus") ||
			!strings.Contains(out, "on error") {
			t.Errorf("%s: the error branch was dropped:\n%s", eh, out)
		}
	}
}

func TestMDL076_2534_ContinueRefusedOnSendRestRequest(t *testing.T) {
	if out := checkMicroflowBodyForTest(t, "send rest request M.Api.GetStatus on error continue;"); !strings.Contains(out, "MDL076") {
		t.Errorf("no MDL076, but mxbuild reports CE6035:\n%s", out)
	}
	// Controls: the custom handlers build.
	for _, body := range []string{
		"send rest request M.Api.GetStatus on error begin return; end error;",
		"send rest request M.Api.GetStatus on error without rollback begin return; end error;",
	} {
		if out := checkMicroflowBodyForTest(t, body); strings.Contains(out, "MDL076") {
			t.Errorf("MDL076 fired on %q, which builds:\n%s", body, out)
		}
	}
}
