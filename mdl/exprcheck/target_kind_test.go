// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// TestTargetKindMatrix pins checkTargetKind to what MxBuild 11.16.1 does for an
// expression assigned to a typed variable.
//
// The matrix was measured once and holds for all three positions that give an
// expression a type from outside the slot table — `declare $X T = e`,
// `set $X = e` and `return e` in a microflow that `returns T`: 216 microflows
// (3 positions x 6 types x 12 value shapes), one `mx check`, every rejection
// CE0117 "Error(s) in expression" and every position identical.
//
//	target    accepted
//	String    'a'  empty  $String
//	Integer   1    empty  $Integer $Long
//	Long      1    empty  $Integer $Long
//	Decimal   1  1.5  empty  $Integer $Long $Decimal
//	Boolean   true  $Boolean            (empty is CE0117 too: see below)
//	DateTime  empty  [%CurrentDateTime%]  $DateTime
//
// `empty` for a Boolean target is the one measured rejection this check does
// NOT report, as for a call argument (call_argument.go): `empty` is Mendix's
// null and is accepted for every other type, and refusing it deserves its own
// measurement across versions rather than a corner of this rule.
func TestTargetKindMatrix(t *testing.T) {
	vars := mapScope{
		"pString": KindString, "pInteger": KindInteger, "pLong": KindLong,
		"pDecimal": KindDecimal, "pBoolean": KindBoolean, "pDateTime": KindDateTime,
	}
	shapes := []struct{ name, src string }{
		{"lstr", "'a'"}, {"lint", "1"}, {"ldec", "1.5"}, {"lbool", "true"},
		{"empty", "empty"}, {"tok", "[%CurrentDateTime%]"},
		{"pString", "$pString"}, {"pInteger", "$pInteger"}, {"pLong", "$pLong"},
		{"pDecimal", "$pDecimal"}, {"pBoolean", "$pBoolean"}, {"pDateTime", "$pDateTime"},
	}
	targets := []struct {
		kind     TypeKind
		name     string
		accepted []string
	}{
		{KindString, "String", []string{"lstr", "empty", "pString"}},
		{KindInteger, "Integer", []string{"lint", "empty", "pInteger", "pLong"}},
		{KindLong, "Long", []string{"lint", "empty", "pInteger", "pLong"}},
		{KindDecimal, "Decimal", []string{"lint", "ldec", "empty", "pInteger", "pLong", "pDecimal"}},
		{KindBoolean, "Boolean", []string{"lbool", "pBoolean", "empty"}}, // empty: measured CE0117, deliberately silent
		{KindDateTime, "DateTime", []string{"empty", "tok", "pDateTime"}},
	}
	for _, tg := range targets {
		acc := map[string]bool{}
		for _, a := range tg.accepted {
			acc[a] = true
		}
		for _, sh := range shapes {
			_, hs := (&parserImpl{}).Parse(sh.src, Context{
				SlotPath: "DeclareStmt.InitialValue", Microflow: "Mx.F", Scope: vars, Target: tg.kind,
			})
			var reported bool
			for _, h := range hs {
				if h.Code == "E009" {
					reported = true
				}
			}
			if want := !acc[sh.name]; reported != want {
				t.Errorf("%s target, %s (%s): reported=%v, MxBuild says rejected=%v", tg.name, sh.name, sh.src, reported, want)
			}
		}
	}
}

// A target that is not one of the six primitives, or no target at all, checks
// nothing: an enumeration, an object or a list needs the catalog, and a position
// with no declared type has nothing to compare against.
func TestTargetKindSilentWithoutPrimitiveTarget(t *testing.T) {
	for _, k := range []TypeKind{KindUnknown, KindAny, KindEnumeration, KindObject, KindList, KindBinary} {
		_, hs := (&parserImpl{}).Parse("'a'", Context{SlotPath: "DeclareStmt.InitialValue", Target: k})
		for _, h := range hs {
			if h.Code == "E009" {
				t.Errorf("target %v: unexpected %s %q", k, h.Code, h.Problem)
			}
		}
	}
}

// An expression whose kind is not known is never reported: exec refuses what
// check reports, so a guess would block a script that builds.
func TestTargetKindSilentWhenKindUnknown(t *testing.T) {
	_, hs := (&parserImpl{}).Parse("$nobody + $else", Context{SlotPath: "DeclareStmt.InitialValue", Target: KindInteger})
	for _, h := range hs {
		if h.Code == "E009" {
			t.Errorf("unexpected %s %q", h.Code, h.Problem)
		}
	}
}
