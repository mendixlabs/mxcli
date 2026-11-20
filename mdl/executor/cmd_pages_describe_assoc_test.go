// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// Bug 3 — DESCRIBE must reconstruct an association-navigated contentparam
// (AttributeRef.EntityRef of association steps) back into "Assoc/.../Attr" so
// the round-trip re-parses.
func TestAssociationTemplateParamPath(t *testing.T) {
	step := func(assoc, dest string) map[string]any {
		return map[string]any{
			"$Type":             "DomainModels$EntityRefStep",
			"Association":       assoc,
			"DestinationEntity": dest,
		}
	}
	attrRefWith := func(attr string, steps ...map[string]any) map[string]any {
		items := make([]any, len(steps))
		for i, s := range steps {
			items[i] = s
		}
		return map[string]any{
			"$Type":     "DomainModels$AttributeRef",
			"Attribute": attr,
			"EntityRef": map[string]any{
				"$Type": "DomainModels$IndirectEntityRef",
				"Steps": items,
			},
		}
	}

	tests := []struct {
		name    string
		attrRef map[string]any
		want    string
	}{
		{
			name:    "single hop",
			attrRef: attrRefWith("MyFirstModule.Employee.Name", step("MyFirstModule.Expense_Employee", "MyFirstModule.Employee")),
			want:    "MyFirstModule.Expense_Employee/Name",
		},
		{
			name: "two hops",
			attrRef: attrRefWith("A.Country.Code",
				step("A.Employee_Address", "A.Address"),
				step("A.Address_Country", "A.Country")),
			want: "A.Employee_Address/A.Address_Country/Code",
		},
		{
			name:    "direct attribute (no EntityRef) → empty",
			attrRef: map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": "M.E.Attr"},
			want:    "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := associationTemplateParamPath(nil, tc.attrRef); got != tc.want {
				t.Errorf("associationTemplateParamPath = %q, want %q", got, tc.want)
			}
		})
	}
}
