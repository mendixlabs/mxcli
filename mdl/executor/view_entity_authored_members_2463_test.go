// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"
)

// mendixlabs/mxcli#1908 (ako/mxcli#2463): a script that adds a column with
// `create or modify view entity` and then aggregates over it was refused by
// check and exec — "ReproSummary has no member OpenCount" — because a member a
// flow names was read from the script for an entity it declares and from the
// stored model for a view entity it declares, which does not have the column
// yet. The split script built clean.
const viewEntity2463Setup = `mdl 1;
create module V2463;
create persistent entity V2463.Item ( Dept: String(50), Amount: Integer );
create or modify view entity V2463.Summary ( Dept: String(50), ItemCount: Integer ) as (
  select i.Dept as Dept, count(i.ID) as ItemCount from V2463.Item as i group by i.Dept );
`

const viewEntity2463Script = `mdl 1;
create or modify view entity V2463.Summary (
  Dept: String(50), ItemCount: Integer, OpenCount: Integer
) as (
  select i.Dept as Dept, count(i.ID) as ItemCount, sum(i.Amount) as OpenCount
  from V2463.Item as i group by i.Dept );
create microflow V2463.Sum () returns Integer as $Total
begin
  retrieve $Rows from V2463.Summary;
  $Total = sum $Rows by %s;
  return $Total;
end;
`

func TestViewEntityColumnAddedInTheScript_2463(t *testing.T) {
	for _, tc := range []struct{ member, want string }{
		{"OpenCount", ""}, // the report: added by the same script
		// Control: a column neither stored nor declared is still refused.
		{"NoSuchCount", `V2463.Summary has no member "NoSuchCount"`},
	} {
		t.Run(tc.member, func(t *testing.T) {
			exec, out, dir := openPedAppCopy(t)
			if err := agreeExec(t, exec, viewEntity2463Setup); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			assertAgree(t, exec, dir, fmt.Sprintf(viewEntity2463Script, tc.member), tc.want)
		})
	}
}
