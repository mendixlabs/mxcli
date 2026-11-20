// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// ako/mxcli#2464 (mendixlabs/mxcli#1909): a create/change member's value was
// never compared with the attribute's type — the expectations table resolves
// it "AttributeOf:Parent" and nothing resolved it — so `change $O (Code = $I)`
// with an Integer for a String attribute passed check and exec, and failed the
// build with CE0117. The cases are measured on mxbuild 11.12.6 (7 CE0117 in
// 13 microflows): the same rule as a call argument's (#2096).
func TestMemberValueKind_Issue2464(t *testing.T) {
	cat := lookupCatalog{
		kinds: map[string]TypeKind{
			"M.T|S": KindString, "M.T|I": KindInteger, "M.T|L": KindLong,
			"M.T|D": KindDecimal, "M.T|B": KindBoolean, "M.T|C": KindEnumeration,
		},
		enums: map[string]string{"M.T|C": "M.Color"},
		cases: map[string][]string{"M.Color": {"Red"}, "M.Size": {"Small"}},
	}
	p := NewParser()
	for _, tc := range []struct {
		attr, value string
		wantErr     bool
	}{
		{"S", "5", true},      // measured CE0117
		{"I", "'x'", true},    // measured CE0117
		{"I", "1.5", true},    // Decimal → Integer, measured CE0117
		{"B", "1", true},      // measured CE0117
		{"D", "5", false},     // Integer widens into Decimal, measured clean
		{"S", "'ok'", false},  // measured clean
		{"S", "empty", false}, // measured clean
		{"S", "toString(5)", false},
	} {
		for _, slot := range []string{"ChangeItem.Value:M.T.", "CreateItem.Value:M.T."} {
			_, hs := p.Parse(tc.value, Context{SlotPath: slot + tc.attr, Slots: DefaultSlotResolver(), Catalog: cat})
			if got := hasCode(hs, "E009"); got != tc.wantErr {
				t.Errorf("%s%s = %s: E009 = %v, want %v (%+v)", slot, tc.attr, tc.value, got, tc.wantErr, hs)
			}
		}
	}
}

// An enumeration attribute given a string is already E001; it is not reported
// twice.
func TestMemberValueKind_EnumStringIsReportedOnce_Issue2464(t *testing.T) {
	cat := lookupCatalog{
		kinds: map[string]TypeKind{"M.T|C": KindEnumeration},
		enums: map[string]string{"M.T|C": "M.Color"},
		cases: map[string][]string{"M.Color": {"Red"}},
	}
	_, hs := NewParser().Parse("'Red'", Context{SlotPath: "ChangeItem.Value:M.T.C", Slots: DefaultSlotResolver(), Catalog: cat})
	if hasCode(hs, "E009") {
		t.Errorf("E009 beside E001 for the same value: %+v", hs)
	}
}
