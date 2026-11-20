// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// The targets every case below calls, one parameter each. They are created in
// the SAME script as the caller, which is the shape the issue reports: the
// catalog only knows the stored project, so a flow the script creates must be
// overlaid onto it or every argument it takes stays unchecked.
const callArgTargets2096 = `
CREATE ENUMERATION MyFirstModule.Color (Red 'Red', Blue 'Blue');
CREATE ENUMERATION MyFirstModule.Size (Small 'Small', Large 'Large');
CREATE PERSISTENT ENTITY MyFirstModule.Order (Qty: Integer, Color: Enumeration(MyFirstModule.Color));
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Int ($P: Integer) BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Long ($P: Long) BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Dec ($P: Decimal) BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Str ($P: String) BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Bool ($P: Boolean) BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Date ($P: DateTime) BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Color ($P: enum MyFirstModule.Color) BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Order ($P: MyFirstModule.Order) BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Orders ($P: List of MyFirstModule.Order) BEGIN END;
CREATE OR REPLACE NANOFLOW MyFirstModule.N_Int ($P: Integer) BEGIN END;
`

// callArgCaller wraps one call in a caller whose parameters supply a value of
// every kind the matrix varies.
func callArgCaller2096(call string) string {
	return `
CREATE OR REPLACE MICROFLOW MyFirstModule.Caller (
  $L: Long, $I: Integer, $Dv: Decimal, $O: MyFirstModule.Order,
  $Os: List of MyFirstModule.Order, $Sz: enum MyFirstModule.Size, $C: enum MyFirstModule.Color)
BEGIN
  ` + call + `
END;
`
}

// TestTypeCheckCallArgumentMismatch2096 is ako/mxcli#2096 (mendixlabs/mxcli#1797):
// `check -p` passed a call argument whose type cannot match the called flow's
// parameter, and the build then failed CE0117 "Error(s) in expression" at the
// call activity. Every case here was measured CE0117 on mxbuild 11.14.0.
func TestTypeCheckCallArgumentMismatch2096(t *testing.T) {
	cases := []struct{ name, call string }{
		{"string to Integer (the reported case)", `call microflow MyFirstModule.T_Int (P = '5');`},
		{"Decimal to Integer", `call microflow MyFirstModule.T_Int (P = $Dv);`},
		{"Decimal literal to Long", `call microflow MyFirstModule.T_Long (P = 5.5);`},
		{"Boolean to DateTime", `call microflow MyFirstModule.T_Date (P = true);`},
		{"Integer to String", `call microflow MyFirstModule.T_Str (P = $I);`},
		{"DateTime token to String", `call microflow MyFirstModule.T_Str (P = [%CurrentDateTime%]);`},
		{"string to Boolean", `call microflow MyFirstModule.T_Bool (P = 'true');`},
		{"value of another enumeration", `call microflow MyFirstModule.T_Color (P = MyFirstModule.Size.Small);`},
		{"variable of another enumeration", `call microflow MyFirstModule.T_Color (P = $Sz);`},
		{"string to enumeration", `call microflow MyFirstModule.T_Color (P = 'Red');`},
		{"enumeration to Integer", `call microflow MyFirstModule.T_Int (P = $C);`},
		{"primitive to object", `call microflow MyFirstModule.T_Order (P = 5);`},
		{"primitive to list", `call microflow MyFirstModule.T_Orders (P = 'x');`},
		{"bare word", `call microflow MyFirstModule.T_Dec (P = Five);`},
		{"nanoflow argument", `call nanoflow MyFirstModule.N_Int (P = '5');`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := typeCheckFixture(t)
			got := typeCheck(t, exec, callArgTargets2096+callArgCaller2096(tc.call))
			var hit bool
			for _, v := range got {
				if v.Location.DocumentName == "MyFirstModule.Caller" || strings.Contains(v.Message, "Caller") {
					hit = true
				}
			}
			if !hit {
				t.Fatalf("check passed a call argument mxbuild reports as CE0117: %s\nviolations: %+v", tc.call, got)
			}
		})
	}
}

// TestTypeCheckCallArgumentLegal2096 is the control: the coercions Mendix
// makes, `empty`, and everything the checker cannot type must stay silent.
// Each case was measured 0 errors on mxbuild 11.14.0 (or is unresolvable, where
// a refusal would block exec on a guess).
func TestTypeCheckCallArgumentLegal2096(t *testing.T) {
	cases := []struct{ name, call string }{
		{"Integer to Integer", `call microflow MyFirstModule.T_Int (P = 5);`},
		{"Long to Integer", `call microflow MyFirstModule.T_Int (P = $L);`},
		{"Integer to Long", `call microflow MyFirstModule.T_Long (P = $I);`},
		{"Integer to Decimal", `call microflow MyFirstModule.T_Dec (P = 5);`},
		{"Long to Decimal", `call microflow MyFirstModule.T_Dec (P = $L);`},
		{"attribute to Decimal", `call microflow MyFirstModule.T_Dec (P = $O/Qty);`},
		{"string to String", `call microflow MyFirstModule.T_Str (P = '5');`},
		{"DateTime token to DateTime", `call microflow MyFirstModule.T_Date (P = [%CurrentDateTime%]);`},
		{"matching enumeration value", `call microflow MyFirstModule.T_Color (P = MyFirstModule.Color.Red);`},
		{"matching enumeration variable", `call microflow MyFirstModule.T_Color (P = $C);`},
		{"matching enumeration attribute", `call microflow MyFirstModule.T_Color (P = $O/Color);`},
		{"object of the right entity", `call microflow MyFirstModule.T_Order (P = $O);`},
		{"list of the right entity", `call microflow MyFirstModule.T_Orders (P = $Os);`},
		{"empty to Integer", `call microflow MyFirstModule.T_Int (P = empty);`},
		{"empty to enumeration", `call microflow MyFirstModule.T_Color (P = empty);`},
		{"empty to object", `call microflow MyFirstModule.T_Order (P = empty);`},
		{"empty to list", `call microflow MyFirstModule.T_Orders (P = empty);`},
		{"empty to Boolean", `call microflow MyFirstModule.T_Bool (P = empty);`},
		{"unknown variable", `call microflow MyFirstModule.T_Int (P = $Nope);`},
		{"unknown flow", `call microflow MyFirstModule.NotThere (P = '5');`},
		{"unknown parameter", `call microflow MyFirstModule.T_Int (Other = '5');`},
		{"nanoflow argument", `call nanoflow MyFirstModule.N_Int (P = 5);`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := typeCheckFixture(t)
			got := typeCheck(t, exec, callArgTargets2096+callArgCaller2096(tc.call))
			if len(got) != 0 {
				t.Fatalf("a call argument that builds was refused: %s\nviolations: %+v", tc.call, got)
			}
		})
	}
}

// TestTypeCheckCallArgumentStoredTarget2096 covers a target that is already in
// the project rather than created by the script: its parameter type comes from
// the catalog, with no overlay.
func TestTypeCheckCallArgumentStoredTarget2096(t *testing.T) {
	exec := typeCheckFixture(t)
	run(t, exec, `CREATE MICROFLOW MyFirstModule.Stored ($Qty: Integer) BEGIN END;`)

	bad := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.Caller () BEGIN
  call microflow MyFirstModule.Stored (Qty = '5');
END;`)
	if len(bad) == 0 {
		t.Fatalf("a String argument to a stored flow's Integer parameter passed check")
	}
	good := typeCheck(t, exec, `
CREATE OR REPLACE MICROFLOW MyFirstModule.Caller () BEGIN
  call microflow MyFirstModule.Stored (Qty = 5);
END;`)
	if len(good) != 0 {
		t.Fatalf("an Integer argument to an Integer parameter was refused: %+v", good)
	}
}
