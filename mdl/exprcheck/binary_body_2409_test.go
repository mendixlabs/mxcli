// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// mendixlabs/mxcli#1897 (ako/mxcli#2409): `call rest service … (Body: BINARY
// $File)` with $File a FileDocument passed check and exec, and failed the
// build with CE0117. A binary body sends the bytes of a Binary expression.
// Measured on 11.12.6, one microflow each: $File/Contents, $Img/Contents and
// empty build; $File, $Img, 'abc' and $File/Name are CE0117.

type binaryCatalog struct{}

func (binaryCatalog) AttributeKind(entityQN, attr string) (TypeKind, bool) {
	switch attr {
	case "Contents":
		return KindBinary, true
	case "Name":
		return KindString, true
	}
	return KindUnknown, false
}
func (binaryCatalog) AttributeEnumQN(string, string) (string, bool)   { return "", false }
func (binaryCatalog) EnumCases(string) ([]string, bool)               { return nil, false }
func (binaryCatalog) MicroflowReturn(string) (TypeKind, bool)         { return KindUnknown, false }
func (binaryCatalog) MicroflowParam(string, string) (TypeKind, bool)  { return KindUnknown, false }
func (binaryCatalog) MicroflowParamRef(string, string) (string, bool) { return "", false }
func (binaryCatalog) Lookup(string) (TypeKind, bool)                  { return KindUnknown, false }

type binaryScope struct{}

func (binaryScope) VariableEntity(name string) (string, bool) {
	switch name {
	case "File":
		return "System.FileDocument", true
	case "Img":
		return "System.Image", true
	}
	return "", false
}
func (binaryScope) AssociationTarget(string, string) (string, bool) { return "", false }

func TestBinaryRestBody_Issue2409(t *testing.T) {
	ctx := Context{
		SlotPath: "RestCall.BinaryBody", Slots: DefaultSlotResolver(),
		Catalog: binaryCatalog{}, Entities: binaryScope{}, Target: KindBinary,
	}
	for src, wantErr := range map[string]bool{
		"$File":          true,  // measured CE0117 (the report)
		"$Img":           true,  // measured CE0117
		"'abc'":          true,  // measured CE0117
		"$File/Name":     true,  // measured CE0117
		"$File/Contents": false, // measured clean
		"$Img/Contents":  false, // measured clean
		"empty":          false, // measured clean
		"$Unknown":       false, // not typed here: silence, not a guess
	} {
		_, hs := NewParser().Parse(src, ctx)
		if got := hasCode(hs, "E009"); got != wantErr {
			t.Errorf("BINARY %s: E009 = %v, want %v (%+v)", src, got, wantErr, hs)
		}
	}
	// The object case says how to fix it.
	_, hs := NewParser().Parse("$File", ctx)
	for _, h := range hs {
		if h.Code == "E009" && !contains(h.Fix, "$File/Contents") {
			t.Errorf("fix = %q, want it to name $File/Contents", h.Fix)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
