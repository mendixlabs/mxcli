// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1735 (ako/mxcli#1939): the reader had no case for
// Microflows$SendExternalObject / DeleteExternalObject, so a stored one became
// an UnsupportedAction, described as a comment and lost on every rebuild. Both
// must now write every property of the type and read back as themselves.
func TestExternalObjectActionsWriteAndReadBack(t *testing.T) {
	cases := []struct {
		in      microflows.MicroflowAction
		typ     string
		varKey  string
		roundTo func(microflows.MicroflowAction) (string, bool, microflows.ErrorHandlingType, bool)
	}{
		{
			&microflows.SendExternalObjectAction{Variable: "Customer", RefreshInClient: true},
			"Microflows$SendExternalObject", "VariableNameToBeSent",
			func(a microflows.MicroflowAction) (string, bool, microflows.ErrorHandlingType, bool) {
				s, ok := a.(*microflows.SendExternalObjectAction)
				if !ok {
					return "", false, "", false
				}
				return s.Variable, s.RefreshInClient, s.ErrorHandlingType, true
			},
		},
		{
			&microflows.DeleteExternalObjectAction{Variable: "Customer", ErrorHandlingType: microflows.ErrorHandlingTypeContinue},
			"Microflows$DeleteExternalObject", "DeleteVariableName",
			func(a microflows.MicroflowAction) (string, bool, microflows.ErrorHandlingType, bool) {
				s, ok := a.(*microflows.DeleteExternalObjectAction)
				if !ok {
					return "", false, "", false
				}
				return s.Variable, s.RefreshInClient, s.ErrorHandlingType, true
			},
		},
	}
	for _, c := range cases {
		t.Run(c.typ, func(t *testing.T) {
			el := microflowActionToGen(c.in)
			if el == nil {
				t.Fatalf("microflowActionToGen(%T) = nil: the activity would never reach BSON", c.in)
			}
			raw, err := (&codec.Encoder{}).Encode(el)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			doc := bson.Raw(raw)
			if got := rawStr(doc, "$Type"); got != c.typ {
				t.Errorf("$Type = %q, want %q", got, c.typ)
			}
			for _, k := range []string{c.varKey, "RefreshInClient", "ErrorHandlingType"} {
				if _, err := doc.LookupErr(k); err != nil {
					t.Errorf("%s missing from %s", k, doc)
				}
			}

			decoded, err := codec.NewDecoder(codec.DefaultRegistry).Decode(doc)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			back := actionFromGen(decoded)
			v, refresh, eh, ok := c.roundTo(back)
			if !ok {
				t.Fatalf("read back as %T, want the same action (not UnsupportedAction)", back)
			}
			wantV, wantR, wantEH, _ := c.roundTo(c.in)
			if wantEH == "" {
				wantEH = microflows.ErrorHandlingTypeRollback
			}
			if v != wantV || refresh != wantR || eh != wantEH {
				t.Errorf("read back (%q, %v, %q), want (%q, %v, %q)", v, refresh, eh, wantV, wantR, wantEH)
			}
		})
	}
}
