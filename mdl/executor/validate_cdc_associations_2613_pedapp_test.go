// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2613: mxbuild refuses a change data capture service that tracks an
// association without the entity at its other end (CE1583, "the associated
// entity … must also be included"), and one that tracks an association both
// sides own (owner Both) from one side only — the same CE1583, at the other
// entity's change source. check and exec passed both; mx check failed.
// Measured on 11.17.0: tracked from both sides, mx check is clean.
func TestCdcAssociationsCheck_2613(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = `mdl 1;
create or modify persistent entity MyFirstModule.Tag ( Label: String(30) );
create or modify persistent entity MyFirstModule.Customer ( Name: String(30) );
create or modify persistent entity MyFirstModule.Purchase ( Number: String(20) );
create or modify association MyFirstModule.Purchase_Primary from MyFirstModule.Purchase to MyFirstModule.Tag owner Both;
create or modify association MyFirstModule.Purchase_Customer from MyFirstModule.Purchase to MyFirstModule.Customer;`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	check := func(src string) []string {
		prog, errs := visitor.Build("mdl 1;\n" + src)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		var got []string
		for _, v := range exec.CheckCdcAssociations(prog) {
			got = append(got, v.RuleID+" "+v.Message)
		}
		return got
	}
	svc := func(body string) string {
		return "create or modify change data capture service MyFirstModule.Cdc ( ServiceName: 'Cdc' ) {\n" + body + "\n};"
	}

	// owner Both, tracked from the FROM side only: refused, naming the other side.
	got := check(svc(`entity MyFirstModule.Purchase ( Number as 'Number', Purchase_Primary as 'Primary' );
entity MyFirstModule.Tag ( Label as 'Label' );`))
	if len(got) != 1 || !strings.HasPrefix(got[0], cdcAssociationRule) || !strings.Contains(got[0], "MyFirstModule.Tag") ||
		!strings.Contains(got[0], "owner Both") {
		t.Errorf("one-sided owner Both: %v", got)
	}

	// The control: tracked from both sides, nothing to report.
	if got := check(svc(`entity MyFirstModule.Purchase ( Number as 'Number', Purchase_Primary as 'Primary' );
entity MyFirstModule.Tag ( Label as 'Label', Purchase_Primary as 'Purchase' );`)); len(got) != 0 {
		t.Errorf("both sides tracked: %v", got)
	}

	// An ordinary association whose TO entity is not tracked.
	got = check(svc(`entity MyFirstModule.Purchase ( Number as 'Number', Purchase_Customer as 'Customer' );`))
	if len(got) != 1 || !strings.Contains(got[0], "MyFirstModule.Customer") || strings.Contains(got[0], "owner Both") {
		t.Errorf("untracked TO entity: %v", got)
	}

	// An association the same script creates is judged by the script.
	got = check(`create or modify association MyFirstModule.Purchase_Tag2 from MyFirstModule.Purchase to MyFirstModule.Tag owner Both;
` + svc(`entity MyFirstModule.Purchase ( Number as 'Number', Purchase_Tag2 as 'Tag2' );
entity MyFirstModule.Tag ( Label as 'Label' );`))
	if len(got) != 1 || !strings.Contains(got[0], "Purchase_Tag2") {
		t.Errorf("script-created owner Both: %v", got)
	}
}
