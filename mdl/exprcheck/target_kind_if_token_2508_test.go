// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// ako/mxcli#2508: `change $P (CreatedAt = if $P/CreatedAt = empty then
// [%CurrentDateTime%] else $P/CreatedAt)` was refused as E009 "the value
// assigned to it is a String", and mxbuild builds it (0 errors, in a running
// app for a month). A bare token was typed by argKind; inside an `if` its
// branch went to inferKind, which calls every token a String.
func TestIfWithDateTokenBranchIsADateTime_2508(t *testing.T) {
	vars := mapScope{"pDateTime": KindDateTime, "pString": KindString, "pBoolean": KindBoolean}
	hit := func(src string, target TypeKind) bool {
		_, hs := (&parserImpl{}).Parse(src, Context{SlotPath: "DeclareStmt.InitialValue", Microflow: "Mx.F", Scope: vars, Target: target})
		for _, h := range hs {
			if h.Code == "E009" {
				return true
			}
		}
		return false
	}
	for _, src := range []string{
		"if $pBoolean then [%CurrentDateTime%] else $pDateTime",
		"if $pBoolean then $pDateTime else [%CurrentDateTime%]",
		"if $pBoolean then ([%BeginOfCurrentDay%]) else empty",
		"if $pBoolean then empty else [%EndOfCurrentMonthUTC%]",
		"if $pBoolean then (if $pBoolean then [%CurrentDateTime%] else $pDateTime) else $pDateTime",
	} {
		if hit(src, KindDateTime) {
			t.Errorf("%s: refused for a DateTime target, which mxbuild accepts", src)
		}
	}
	// Controls: what is provably not a DateTime is still refused.
	for _, src := range []string{
		"if $pBoolean then 'a' else $pString",
		"if $pBoolean then 'a' else 'b'",
	} {
		if !hit(src, KindDateTime) {
			t.Errorf("%s: a String if was not refused for a DateTime target", src)
		}
	}
	// A date-token if is still a mismatch for a String target.
	if !hit("if $pBoolean then [%CurrentDateTime%] else $pDateTime", KindString) {
		t.Error("a DateTime if was not refused for a String target")
	}
}
