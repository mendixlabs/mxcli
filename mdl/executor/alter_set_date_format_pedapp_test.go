// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#1965 (mendixlabs/mxcli#1743): `set (DateFormat: Custom,
// CustomDateFormat: …)` on a date picker whose stored format is not Custom was
// refused by check — and so by exec's pre-check — in either order and as two
// SETs, while --no-check wrote it correctly and mx check passed. check dry-ran
// each property alone on the stored widget, where Custom has no pattern. The
// widget is now judged as the whole statement leaves it, as exec does.
func TestCheckExecAgree_CustomDateFormatSetWithItsPattern(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const setup = `mdl 1;
create persistent entity MyFirstModule.Booking1965 (Moment: DateTime);
create page MyFirstModule.Booking1965_Edit ( Title: 'Booking', Layout: Atlas_Core.PopupLayout, Params: ( $Booking: MyFirstModule.Booking1965 ) ) {
  layoutgrid lg { row { column (DesktopWidth: autofill) {
    dataview dv (DataSource: $Booking) {
      datepicker dpA (Label: 'A', Attribute: Moment)
      datepicker dpB (Label: 'B', Attribute: Moment)
      datepicker dpC (Label: 'C', Attribute: Moment)
      datepicker dpD (Label: 'D', Attribute: Moment)
    }
  } } }
};`
	if err := agreeExec(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	for _, tc := range []struct{ name, set, want string }{
		{"one SET, DateFormat first", "set (DateFormat: Custom, CustomDateFormat: 'dd/MM/yyyy') on dpA", ""},
		{"one SET, the pattern first", "set (CustomDateFormat: 'dd/MM/yyyy', DateFormat: Custom) on dpB", ""},
		{"two SETs, the pattern first", "set (CustomDateFormat: 'HH:mm') on dpC; set (DateFormat: Custom) on dpC", ""},
		// Control: Custom with no pattern anywhere is still CE0493, in both.
		{"Custom with no pattern", "set (DateFormat: Custom) on dpD", "CE0493"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertAgree(t, exec, dir, "mdl 1;\nalter page MyFirstModule.Booking1965_Edit { "+tc.set+" };", tc.want)
		})
	}
}
