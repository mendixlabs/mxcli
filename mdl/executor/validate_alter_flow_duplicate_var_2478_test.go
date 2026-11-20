// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// mendixlabs/mxcli#1914 (ako/mxcli#2478). A loop body replaced by
// `alter microflow … replace body of` that declares $FacilityTrancheCount and
// then makes it a COUNT's output passed check and exec ("Altered microflow"),
// and mx check reported CE0111 "Duplicate variable name" (measured 11.12.6).
// The same body in `create microflow` is MDL063. A microflow's namespace is
// flat, so two definitions inside one fragment are CE0111 whatever the stored
// flow holds: MDL063 runs over each fragment.
func TestValidateAlterFlow_DuplicateVariableInsideAFragment_2478(t *testing.T) {
	const dup = `alter microflow M.MF_Loop {
  replace body of loop $t in $FacilityTrancheList with begin
    declare $FacilityTrancheCount Integer = 0;
    $FacilityTrancheCount = count($FacilityTrancheList);
  end;
};`
	prog, _ := parseAlterFlow(t, dup)
	if got := alterFlowRuleCounts(t, prog)["MDL063"]; got != 1 {
		t.Errorf("MDL063 = %d, want 1 for a fragment that defines $FacilityTrancheCount twice", got)
	}

	// Controls. A reassignment defines nothing (`set` is a Change Variable);
	// an insert fragment is judged the same way as a loop body.
	for name, src := range map[string]string{
		"reassignment": `alter microflow M.MF_Loop {
  replace body of loop $t in $FacilityTrancheList with begin
    declare $FacilityTrancheCount Integer = 0;
    set $FacilityTrancheCount = $FacilityTrancheCount + 1;
  end;
};`,
		"distinct names": `alter microflow M.MF_Loop {
  insert after $X begin
    declare $A Integer = 0;
    $B = count($FacilityTrancheList);
  end;
};`,
	} {
		prog, _ := parseAlterFlow(t, src)
		if got := alterFlowRuleCounts(t, prog)["MDL063"]; got != 0 {
			t.Errorf("%s: MDL063 = %d, want 0", name, got)
		}
	}

	// An insert fragment with the duplicate is the same defect.
	prog, _ = parseAlterFlow(t, `alter microflow M.MF_Loop {
  insert after $X begin
    declare $N Integer = 0;
    $N = count($FacilityTrancheList);
  end;
};`)
	if got := alterFlowRuleCounts(t, prog)["MDL063"]; got != 1 {
		t.Errorf("insert fragment: MDL063 = %d, want 1", got)
	}
}
