// SPDX-License-Identifier: Apache-2.0

package rules

import "testing"

func TestIsPascalCase(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Customer", true},
		{"CustomerOrder", true},
		{"A", true},
		{"ABC123", true},
		{"customer", false},
		{"customerOrder", false},
		{"Customer_Order", false},
		{"customer-order", false},
		{"123Customer", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsPascalCase(tt.input)
		if got != tt.want {
			t.Errorf("IsPascalCase(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"customer", true},
		{"customerOrder", true},
		{"a", true},
		{"abc123", true},
		{"Customer", false},
		{"customer_order", false},
		{"customer-order", false},
		{"123customer", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsCamelCase(tt.input)
		if got != tt.want {
			t.Errorf("IsCamelCase(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSplitWords(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"customerOrder", []string{"customer", "Order"}},
		{"CustomerOrder", []string{"Customer", "Order"}},
		{"customer_order", []string{"customer", "order"}},
		{"customer-order", []string{"customer", "order"}},
		{"ABC", []string{"ABC"}},
		{"simple", []string{"simple"}},
		{"", nil},
	}
	for _, tt := range tests {
		got := splitWords(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitWords(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitWords(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"customer_order", "CustomerOrder"},
		{"customer-order", "CustomerOrder"},
		{"customerOrder", "CustomerOrder"},
		{"Customer", "Customer"},
		{"CUSTOMER", "Customer"},
		{"", ""},
	}
	for _, tt := range tests {
		got := toPascalCase(tt.input)
		if got != tt.want {
			t.Errorf("toPascalCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSuggestMicroflowName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ACT_do_something", "ACT_DoSomething"},
		{"SUB_helper", "SUB_Helper"},
		{"doSomething", "DoSomething"},
		{"ACT_Already", "ACT_Already"},
	}
	for _, tt := range tests {
		got := suggestMicroflowName(tt.input)
		if got != tt.want {
			t.Errorf("suggestMicroflowName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNamingConventionRule_Metadata(t *testing.T) {
	r := NewNamingConventionRule()
	if r.ID() != "MPR001" {
		t.Errorf("ID = %q, want MPR001", r.ID())
	}
	if r.Category() != "naming" {
		t.Errorf("Category = %q, want naming", r.Category())
	}
	if r.Name() != "NamingConvention" {
		t.Errorf("Name = %q, want NamingConvention", r.Name())
	}
}

func TestDefaultPatterns(t *testing.T) {
	// Entity pattern
	if !DefaultEntityPattern.MatchString("Customer") {
		t.Error("entity pattern should match 'Customer'")
	}
	if DefaultEntityPattern.MatchString("customer") {
		t.Error("entity pattern should not match 'customer'")
	}
	if DefaultEntityPattern.MatchString("Customer_Order") {
		t.Error("entity pattern should not match 'Customer_Order'")
	}

	// Microflow pattern
	if !DefaultMicroflowPattern.MatchString("ACT_CreateCustomer") {
		t.Error("microflow pattern should match 'ACT_CreateCustomer'")
	}
	if !DefaultMicroflowPattern.MatchString("ProcessOrder") {
		t.Error("microflow pattern should match 'ProcessOrder'")
	}
	if DefaultMicroflowPattern.MatchString("processOrder") {
		t.Error("microflow pattern should not match 'processOrder'")
	}

	// Page pattern
	if !DefaultPagePattern.MatchString("Customer_Edit") {
		t.Error("page pattern should match 'Customer_Edit'")
	}
	if DefaultPagePattern.MatchString("customer_edit") {
		t.Error("page pattern should not match 'customer_edit'")
	}

	// Enumeration pattern
	if !DefaultEnumerationPattern.MatchString("OrderStatus") {
		t.Error("enumeration pattern should match 'OrderStatus'")
	}
	if DefaultEnumerationPattern.MatchString("orderStatus") {
		t.Error("enumeration pattern should not match 'orderStatus'")
	}
	// Issue #715: the Mendix-recommended ENUM_ prefix must be accepted.
	if !DefaultEnumerationPattern.MatchString("ENUM_ShippingStatus") {
		t.Error("enumeration pattern should match 'ENUM_ShippingStatus'")
	}
	// A lowercase body after the prefix is still wrong.
	if DefaultEnumerationPattern.MatchString("ENUM_shippingStatus") {
		t.Error("enumeration pattern should not match 'ENUM_shippingStatus'")
	}
	// A bare prefix with no body must not pass.
	if DefaultEnumerationPattern.MatchString("ENUM_") {
		t.Error("enumeration pattern should not match 'ENUM_'")
	}
}

// Issue #715: the suggestion must preserve the ENUM_ prefix, not mangle it to
// EnumShippingStatus the way a plain toPascalCase would.
func TestSuggestEnumerationName(t *testing.T) {
	cases := map[string]string{
		"ENUM_shipping_status": "ENUM_ShippingStatus",
		"enum_shippingStatus":  "ENUM_ShippingStatus",
		"order_status":         "OrderStatus",
		"OrderStatus":          "OrderStatus",
	}
	for in, want := range cases {
		if got := suggestEnumerationName(in); got != want {
			t.Errorf("suggestEnumerationName(%q) = %q, want %q", in, got, want)
		}
	}
}
