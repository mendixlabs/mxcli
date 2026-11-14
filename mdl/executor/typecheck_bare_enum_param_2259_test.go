// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#2259 (part 2): a flow parameter typed by a bare `Module.Name` —
// `($P: "MyFirstModule"."Color")` — that names an enumeration was read as an
// Object, so every enumeration value passed to it was E009 ("is an Object …,
// but the argument passed to it is an Enumeration"). The writer resolves the
// bare name to the enumeration (describe prints `enum MyFirstModule.Color`)
// and mx check is clean. The control: a wrong argument is still refused, and a
// bare name of an entity still types an object.
func TestTypeCheckCallArgumentBareEnumerationParam2259(t *testing.T) {
	const targets = `
CREATE ENUMERATION MyFirstModule.Color (Red 'Red', Blue 'Blue');
CREATE PERSISTENT ENTITY MyFirstModule.Order (Qty: Integer);
CREATE OR REPLACE MICROFLOW MyFirstModule.T_Bare ($P: "MyFirstModule"."Color") BEGIN END;
CREATE OR REPLACE MICROFLOW MyFirstModule.T_BareEntity ($P: MyFirstModule.Order) BEGIN END;
`
	caller := func(call string) string {
		return `CREATE OR REPLACE MICROFLOW MyFirstModule.Caller ($O: MyFirstModule.Order) BEGIN ` + call + ` END;`
	}
	for _, tc := range []struct {
		name, call string
		refused    bool
	}{
		{"enumeration value to a bare enumeration type", `call microflow MyFirstModule.T_Bare (P = MyFirstModule.Color.Red);`, false},
		{"string to a bare enumeration type", `call microflow MyFirstModule.T_Bare (P = 'Red');`, true},
		{"object to a bare entity type", `call microflow MyFirstModule.T_BareEntity (P = $O);`, false},
		{"primitive to a bare entity type", `call microflow MyFirstModule.T_BareEntity (P = 5);`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := typeCheckFixture(t)
			got := typeCheck(t, exec, targets+caller(tc.call))
			if refused := len(got) > 0; refused != tc.refused {
				t.Fatalf("refused=%v, want %v: %s\nviolations: %+v", refused, tc.refused, tc.call, got)
			}
		})
	}
}
