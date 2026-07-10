// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// Issue #583: parseAttributeType silently dropped the Length value for
// StringAttributeType when Mendix Studio Pro stored it as BSON int64.
// The previous code only handled int32, so every String attribute in a
// Studio Pro-written MPR was reported as String(unlimited) / Length: 0.
//
// Studio Pro and mxcli can both store integers as int32 or int64 depending
// on encoder choice; the parser must handle every BSON numeric width.
func TestParseAttributeType_StringLength_BsonNumericWidths(t *testing.T) {
	cases := []struct {
		name   string
		length any
		want   int
	}{
		{"int32 (mxcli writer)", int32(40), 40},
		{"int64 (Studio Pro writer)", int64(40), 40},
		{"int", int(40), 40},
		{"float64 (extended JSON)", float64(40), 40},
		{"missing field = unlimited", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{
				"$Type": "DomainModels$StringAttributeType",
			}
			if tc.length != nil {
				raw["Length"] = tc.length
			}
			at := parseAttributeType(raw)
			st, ok := at.(*domainmodel.StringAttributeType)
			if !ok {
				t.Fatalf("parseAttributeType returned %T, want *StringAttributeType", at)
			}
			if st.Length != tc.want {
				t.Errorf("Length = %d, want %d (input %T(%v))", st.Length, tc.want, tc.length, tc.length)
			}
		})
	}
}
