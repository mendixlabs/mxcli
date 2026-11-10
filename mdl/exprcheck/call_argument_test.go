// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"strings"
	"testing"
)

// A call argument was checked for syntax and functions only: the slot table
// declares CallArgument.Value as ResolveBy "TargetParameter", checkSlotKind
// skips every such slot, so `call microflow Lab.Target (I = '5')` checked clean
// and failed the build with CE0117 "Error(s) in expression".
//
// Every expectation below was measured on Mendix 11.14.0 with mx check: one
// caller microflow per case calling Lab.Target, whose parameters are I
// (Integer), L (Long), D (Decimal), S (String), B (Boolean), T (DateTime), C
// (Enumeration Lab.Color), O (Lab.Base) and Lst (List of Lab.Base).

// paramCatalog answers the parameter and enumeration lookups the rule needs.
type paramCatalog struct {
	params    map[string]TypeKind // "Flow.Param"
	paramEnum map[string]string   // "Flow.Param"
	enums     map[string][]string
	attrs     map[string]TypeKind // "Entity.Attr"
	attrEnum  map[string]string
}

func (c paramCatalog) AttributeKind(e, a string) (TypeKind, bool) {
	k, ok := c.attrs[e+"."+a]
	return k, ok
}
func (c paramCatalog) AttributeEnumQN(e, a string) (string, bool) {
	q, ok := c.attrEnum[e+"."+a]
	return q, ok
}
func (c paramCatalog) EnumCases(q string) ([]string, bool) {
	v, ok := c.enums[q]
	return v, ok
}
func (paramCatalog) MicroflowReturn(string) (TypeKind, bool) { return KindUnknown, false }
func (c paramCatalog) MicroflowParam(qn, p string) (TypeKind, bool) {
	k, ok := c.params[qn+"."+strings.TrimPrefix(p, "$")]
	return k, ok
}
func (c paramCatalog) MicroflowParamEnumQN(qn, p string) (string, bool) {
	q, ok := c.paramEnum[qn+"."+strings.TrimPrefix(p, "$")]
	return q, ok
}

func labCatalog() paramCatalog {
	return paramCatalog{
		params: map[string]TypeKind{
			"Lab.Target.I": KindInteger, "Lab.Target.L": KindLong, "Lab.Target.D": KindDecimal,
			"Lab.Target.S": KindString, "Lab.Target.B": KindBoolean, "Lab.Target.T": KindDateTime,
			"Lab.Target.C": KindEnumeration, "Lab.Target.O": KindObject, "Lab.Target.Lst": KindList,
		},
		paramEnum: map[string]string{"Lab.Target.C": "Lab.Color"},
		enums:     map[string][]string{"Lab.Color": {"Red", "Blue"}, "Lab.Size": {"Small", "Large"}},
		attrs:     map[string]TypeKind{"Lab.Base.Num": KindInteger, "Lab.Base.Col": KindEnumeration},
		attrEnum:  map[string]string{"Lab.Base.Col": "Lab.Color"},
	}
}

// labCtx is the caller's scope: $Base (an object), $Lst (a list, which the
// flow scope records as an entity variable too), and declared primitives.
func labCtx(c *Context) {
	c.Catalog = labCatalog()
	c.Entities = fakeEntities{"Base": "Lab.Base", "Lst": "Lab.Base"}
	c.Scope = mapScope{"Int": KindInteger, "Lng": KindLong, "Dec": KindDecimal, "Str": KindString, "Bool": KindBoolean}
}

func argHints(t *testing.T, param, src string) []Hint {
	t.Helper()
	return parseIn(t, src, "CallArgument.Value:Lab.Target."+param, labCtx)
}

func TestCallArgument_MxbuildRejects(t *testing.T) {
	for _, tc := range []struct{ param, src string }{
		{"I", "'5'"}, {"I", "$Str"}, {"I", "5.5"}, {"I", "$Dec"}, {"I", "4 div 2"},
		{"I", "'a' + 'b'"}, {"I", "true"}, {"I", "[%CurrentDateTime%]"}, {"I", "Lab.Color.Red"},
		{"I", "$Base"}, {"L", "$Dec"}, {"L", "5.5"},
		{"D", "'1.5'"}, {"D", "false"}, {"D", "[%CurrentDateTime%]"},
		{"S", "5"}, {"S", "$Int"}, {"S", "$Lng"}, {"S", "1.5"}, {"S", "true"}, {"S", "Lab.Color.Red"},
		{"S", "[%CurrentDateTime%]"}, {"S", "$Base"}, {"S", "$Base/Col"}, {"S", "length('abc')"},
		{"S", "if $Bool then 1 else 2"},
		{"B", "'true'"}, {"B", "1"}, {"B", "Lab.Color.Red"}, {"B", "[%CurrentDateTime%]"},
		{"T", "true"}, {"T", "'2020-01-01'"}, {"T", "5"}, {"T", "Lab.Color.Red"},
		{"C", "'Red'"}, {"C", "1"}, {"C", "$Int"}, {"C", "$Str"}, {"C", "$Base"},
		{"C", "Lab.Size.Small"}, {"C", "[%CurrentDateTime%]"},
		{"O", "'x'"}, {"O", "5"}, {"O", "true"}, {"O", "1.5"}, {"O", "Lab.Color.Red"}, {"O", "[%CurrentDateTime%]"},
		{"Lst", "'x'"}, {"Lst", "5"}, {"Lst", "false"}, {"Lst", "Lab.Color.Red"},
	} {
		if hs := argHints(t, tc.param, tc.src); !hasCode(hs, "E009") {
			t.Errorf("%s = %s: mxbuild reports CE0117, check accepted it: %v", tc.param, tc.src, codes(hs))
		}
	}
}

// CONTROL: each of these builds at 0 errors on 11.14.0 — the coercions Mendix
// makes, `empty`, and the cases the rule cannot see into.
func TestCallArgument_MxbuildAccepts(t *testing.T) {
	for _, tc := range []struct{ param, src string }{
		{"I", "5"}, {"I", "$Lng"}, {"I", "3000000000"}, {"I", "length('abc')"}, {"I", "$Base/Num"},
		{"L", "5"}, {"L", "$Int"},
		{"D", "5"}, {"D", "$Int"}, {"D", "$Lng"}, {"D", "$Base/Num"}, {"D", "1.5"},
		{"S", "'x'"}, {"S", "toString(5)"},
		{"T", "[%CurrentDateTime%]"}, {"T", "([%CurrentDateTime%])"}, {"T", "[%BeginOfCurrentDay%]"},
		{"C", "Lab.Color.Blue"}, {"C", "$Base/Col"},
		{"O", "$Base"}, {"Lst", "$Lst"},
		{"I", "empty"}, {"S", "empty"}, {"C", "empty"}, {"O", "empty"}, {"Lst", "empty"},
		// Not measurable from here: which entity an object holds (a
		// specialization is accepted for its generalization), and a list
		// variable the flow scope types as an entity variable.
		{"O", "$Lst"}, {"Lst", "$Base"},
		// A token nested in a branch is a DateTime inferKind would call a String.
		{"T", "if $Bool then [%CurrentDateTime%] else empty"},
		// Unknown variables and values of unknown enumerations stay silent.
		{"I", "$Nope"}, {"C", "Lab.Unknown.Red"}, {"C", "Lab.Color.Green"},
	} {
		if hs := argHints(t, tc.param, tc.src); hasCode(hs, "E009") {
			t.Errorf("%s = %s builds clean, check reported it: %v", tc.param, tc.src, hs)
		}
	}
}

// The message names what mxbuild names: the parameter, the called flow, and
// Integer and Long as the one type they are there.
func TestCallArgument_MessageNamesTheCall(t *testing.T) {
	hs := argHints(t, "L", "5.5")
	if len(hs) != 1 {
		t.Fatalf("want one hint, got %v", hs)
	}
	for _, want := range []string{"Argument L of the call to Lab.Target", "Integer/Long", "Decimal", "CE0117"} {
		if !strings.Contains(hs[0].Problem, want) {
			t.Errorf("message %q lacks %q", hs[0].Problem, want)
		}
	}
	hs = argHints(t, "C", "Lab.Size.Small")
	if len(hs) != 1 || !strings.Contains(hs[0].Problem, "Enumeration Lab.Color") ||
		!strings.Contains(hs[0].Problem, "Enumeration Lab.Size") {
		t.Errorf("a wrong enumeration should name both: %v", hs)
	}
}

// A parameter typed with a bare `Mod.Name` is recorded as holding "entity"
// Mod.Name even when Mod.Name is an enumeration. Read as an Object it would be
// reported against the enumeration parameter it fits.
func TestCallArgument_EnumVariableTypedByBareName(t *testing.T) {
	withColor := func(c *Context) {
		labCtx(c)
		c.Entities = fakeEntities{"Col": "Lab.Color", "Sz": "Lab.Size"}
	}
	if hs := parseIn(t, "$Col", "CallArgument.Value:Lab.Target.C", withColor); hasCode(hs, "E009") {
		t.Errorf("an enumeration variable was rejected for its own enumeration: %v", hs)
	}
	if hs := parseIn(t, "$Sz", "CallArgument.Value:Lab.Target.C", withColor); !hasCode(hs, "E009") {
		t.Errorf("an enumeration variable of another enumeration was accepted: %v", codes(hs))
	}
}

// Without the target in the slot, or without a catalog, or for a parameter the
// catalog does not know, nothing can be compared.
func TestCallArgument_NothingToCompareAgainst(t *testing.T) {
	for _, slot := range []string{"CallArgument.Value", "CallArgument.Value:Lab.Target.Nope", "CallArgument.Value:Lab.Other.I"} {
		if hs := parseIn(t, "'5'", slot, labCtx); hasCode(hs, "E009") {
			t.Errorf("%s: reported without a known parameter: %v", slot, hs)
		}
	}
	if hs := parseIn(t, "'5'", "CallArgument.Value:Lab.Target.I"); hasCode(hs, "E009") {
		t.Errorf("reported without a catalog: %v", hs)
	}
}

// Measured on 11.14.0: a bare word for a String, an Integer, an enumeration or
// an object parameter is CE0117, in a microflow and a nanoflow call alike. It
// needs no catalog: the word is wrong whatever the parameter is.
func TestCallArgument_BareWordIsE013(t *testing.T) {
	for _, slot := range []string{"CallArgument.Value", "CallArgument.Value:Lab.Target.S"} {
		if hs := parseIn(t, "Hello", slot); !hasCode(hs, "E013") {
			t.Errorf("%s: a bare word was accepted: %v", slot, codes(hs))
		}
	}
	// CONTROL: the spellings Mendix reads are not bare words.
	for _, src := range []string{"$Hello", "'Hello'", "Lab.Color.Red", "empty", "true"} {
		if hs := parseIn(t, src, "CallArgument.Value"); hasCode(hs, "E013") {
			t.Errorf("%s was reported as a bare word: %v", src, hs)
		}
	}
}
