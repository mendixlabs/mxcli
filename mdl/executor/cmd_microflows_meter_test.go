// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

const meterHeader = "create microflow M.ACT_Metrics ($N: Integer, $D: Decimal, $R: String)\nbegin\n"

// The three statements as describe prints them, so the block is also the
// expected description.
const meterStatements = `increment counter 'acme.orders' (Tags: ('region': $R, 'channel': 'web'));
increment counter @Metrics.NAME_COUNTER by $N;
set gauge 'acme.depth' to $D (Tags: ('queue': 'mail'));`

// Each statement builds its own action with the expressions verbatim, and
// describe gives the statements back — a fixed point, so describe → exec keeps
// the activities (they were UnsupportedAction before, which a rebuild refuses).
func TestMeterActivities_BuildAndDescribeRoundTrip(t *testing.T) {
	src := meterHeader + "  " + strings.ReplaceAll(meterStatements, "\n", "\n  ") + "\nend;"
	described, oc := describeFold(t, src)

	var got []microflows.MicroflowAction
	for _, obj := range oc.Objects {
		if aa, ok := obj.(*microflows.ActionActivity); ok {
			got = append(got, aa.Action)
		}
	}
	if len(got) != 3 {
		t.Fatalf("got %d actions, want 3", len(got))
	}
	inc, ok := got[0].(*microflows.IncrementCounterMeterAction)
	if !ok || inc.Name != "'acme.orders'" || len(inc.Tags) != 2 || inc.Tags[0].Key != "'region'" || inc.Tags[0].Value != "$R" {
		t.Errorf("increment counter built %#v", got[0])
	}
	if inc != nil && inc.ErrorHandlingType != microflows.ErrorHandlingTypeRollback {
		t.Errorf("ErrorHandlingType = %q, want Rollback (Studio Pro's explicit default)", inc.ErrorHandlingType)
	}
	if c, ok := got[1].(*microflows.CounterMeterAction); !ok || c.Name != "@Metrics.NAME_COUNTER" || c.Value != "$N" {
		t.Errorf("increment counter … by built %#v", got[1])
	}
	if g, ok := got[2].(*microflows.GaugeMeterAction); !ok || g.Value != "$D" || len(g.Tags) != 1 {
		t.Errorf("set gauge built %#v", got[2])
	}

	for _, line := range strings.Split(meterStatements, "\n") {
		if !strings.Contains(described, line) {
			t.Errorf("describe lost %q\n got:\n%s", line, described)
		}
	}
	again, _ := describeFold(t, meterHeader+described+"\nend;")
	if again != described {
		t.Errorf("describe is not a fixed point\nfirst:\n%s\n\nsecond:\n%s", described, again)
	}
}

// The check rules, each against what mxbuild 11.16.1 reports (ako/TestApp):
// MDL-METRIC01 for CE7040/CE7048/CE7049, MDL-METRIC02 for a negative counter
// mxbuild accepts, MDL076 for CE6035.
func TestMeterActivities_CheckRules(t *testing.T) {
	for _, tc := range []struct{ body, rule string }{
		{"increment counter 'mx.reserved';", "MDL-METRIC01"},
		{"increment counter 'MX_upper';", "MDL-METRIC01"},
		{"increment counter '9abc';", "MDL-METRIC01"},
		{"increment counter 'a-b';", "MDL-METRIC01"},
		{"set gauge '' to 1;", "MDL-METRIC01"},
		{"increment counter 'acme.n' by -1;", "MDL-METRIC02"},
		{"increment counter 'acme.c' on error continue;", "MDL076"},
		{"increment counter 'acme.c' by 1 on error continue;", "MDL076"},
		{"set gauge 'acme.g' to 1 on error continue;", "MDL076"},
	} {
		if out := checkMicroflowBodyForTest(t, tc.body); !strings.Contains(out, tc.rule) {
			t.Errorf("%s: want %s, got:\n%s", tc.body, tc.rule, out)
		}
	}
	// CONTROLS, each measured clean on mxbuild 11.16.1: a constant name (not
	// judged), a negative gauge, a custom handler.
	for _, body := range []string{
		"increment counter 'acme.orders.placed';",
		"increment counter @Metrics.NAME_COUNTER by 4;",
		"set gauge 'acme.gneg' to -2.5;",
		"increment counter 'acme.h' on error without rollback begin\n  log error node 'M' 'x';\nend error;",
	} {
		if out := checkMicroflowBodyForTest(t, body); strings.Contains(out, "MDL-METRIC") || strings.Contains(out, "MDL076") {
			t.Errorf("%s: reported a statement mxbuild accepts:\n%s", body, out)
		}
	}
}

// Studio Pro: "This activity can only be used in microflows."
func TestMeterActivities_RefusedInNanoflow(t *testing.T) {
	for _, body := range []string{"increment counter 'a';", "set gauge 'a' to 1;"} {
		if vs := nanoflowViolations(t, body); !hasErrorRule(vs, nanoflowActivityRule) {
			t.Errorf("%s in a nanoflow: want %s, got %+v", body, nanoflowActivityRule, vs)
		}
	}
}
