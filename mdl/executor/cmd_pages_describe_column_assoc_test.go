// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// Bug 7 — DESCRIBE must reconstruct a DataGrid2 column bound to an associated
// attribute (AttributeRef.EntityRef of association steps) back into short
// `Assoc/Attr` form so the round-trip re-parses (attributePathV3 accepts bare
// segments only).
func TestColumnAttributeFromRef(t *testing.T) {
	step := func(assoc, dest string) map[string]any {
		return map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": assoc, "DestinationEntity": dest}
	}
	ref := func(attr string, steps ...map[string]any) map[string]any {
		m := map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": attr}
		if len(steps) > 0 {
			items := make([]any, len(steps))
			for i, s := range steps {
				items[i] = s
			}
			m["EntityRef"] = map[string]any{"$Type": "DomainModels$IndirectEntityRef", "Steps": items}
		}
		return m
	}
	tests := []struct {
		name string
		ref  map[string]any
		want string
	}{
		{"own attribute", ref("Sales.Order.Number"), "Number"},
		{"single-hop association (short assoc name)", ref("Sales.Customer.Name", step("Sales.Order_Customer", "Sales.Customer")), "Order_Customer/Name"},
		{"two hops", ref("A.Country.Code", step("A.Order_Customer", "A.Customer"), step("A.Customer_Country", "A.Country")), "Order_Customer/Customer_Country/Code"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := columnAttributeFromRef(tc.ref); got != tc.want {
				t.Errorf("columnAttributeFromRef = %q, want %q", got, tc.want)
			}
		})
	}
}
