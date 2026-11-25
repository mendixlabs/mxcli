// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// ako/mxcli#2578: describe prints Studio Pro's "Show home page" on-click as
// `call microflow System.ShowHomePage`, and check accepts it (#2247), but exec
// refused it as a widget action — "microflow not found: System.ShowHomePage" —
// so the description did not round-trip. System.ShowHomePage ships with the
// platform and is not stored in the project.
func TestShowHomePageWidgetAction2578(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const src = `mdl 1;
create snippet MyFirstModule.SNIP_ReproHome {
  actionbutton btnHome (Caption: 'Home', Action: call microflow System.ShowHomePage)
};`
	assertAgree(t, exec, dir, src, "")
	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	err := afRun(t, exec, "describe snippet MyFirstModule.SNIP_ReproHome;")
	exec.output = saved
	if err != nil {
		t.Fatalf("describe: %v\n%s", err, out.String())
	}
	if !strings.Contains(buf.String(), "call microflow System.ShowHomePage") {
		t.Errorf("the snippet does not describe the action back:\n%s", buf.String())
	}
}
