// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"
)

// ako/mxcli#2529: `search "%UserRole_Administrator%"` reported no matches
// while `search "UserRole_Administrator"` found the microflow. `%` is not a
// character an FTS5 query may hold outside a phrase, so the query was a syntax
// error — and search reports a failed query as no rows. The same held for
// `[`, `]` and `$`, so a token or a `$Variable/Attr` pasted from a model never
// matched. Each is now a separator, as `.` and `/` already were.
func TestSearchQueryToleratesModelPunctuation_2529(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = "mdl 1;\ncreate user role Spare ( ModuleRoles: (MyFirstModule.User) );\n" +
		"create microflow MyFirstModule.ACT_Spares ()\nbegin\n" +
		"  retrieve $L from System.User where System.UserRoles = '[%UserRole_Spare%]';\nend;\n"
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	ctx := exec.newExecContext(context.Background())
	for _, q := range []string{"%UserRole_Spare%", "[%UserRole_Spare%]", "UserRole_Spare"} {
		out.Reset()
		got, err := searchReport(ctx, q, "names", SearchOptions{Strict: true})
		if err != nil {
			t.Errorf("search %q: %v", q, err)
			continue
		}
		if got.Hits == 0 || !strings.Contains(out.String(), "ACT_Spares") {
			t.Errorf("search %q found %d, want MyFirstModule.ACT_Spares:\n%s", q, got.Hits, out.String())
		}
	}
}
