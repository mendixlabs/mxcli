// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"strings"
	"unicode"
)

// TableToEntityName converts a table name to a Mendix entity name (PascalCase, singular).
// Examples: "employees" → "Employee", "order_items" → "OrderItem"
func TableToEntityName(tableName string) string {
	name := toPascalCase(tableName)
	return singularize(name)
}

// ColumnToAttributeName converts a column name to a Mendix attribute name (PascalCase).
// Examples: "first_name" → "FirstName", "employee_id" → "EmployeeId"
func ColumnToAttributeName(colName string) string {
	return toPascalCase(colName)
}

// TableToQueryName returns the default query name for a table.
func TableToQueryName(tableName string) string {
	return "GetAll" + TableToEntityName(tableName) + "s"
}

// toPascalCase converts a snake_case or camelCase name to PascalCase.
func toPascalCase(s string) string {
	if s == "" {
		return s
	}

	// Handle already-PascalCase names (no separators, starts with upper)
	if !strings.ContainsAny(s, "_- .") {
		// Just capitalize first letter
		runes := []rune(s)
		runes[0] = unicode.ToUpper(runes[0])
		return string(runes)
	}

	var b strings.Builder
	capitalize := true
	for _, r := range s {
		// A dot separates words here. Left in, a table named after a Kafka
		// topic produced `ENTITY LakeSql.LocalCdc.cdcpublish.customer.1.local`,
		// which is not a Mendix name at all (ako/mxcli#2422).
		if r == '_' || r == '-' || r == ' ' || r == '.' {
			capitalize = true
			continue
		}
		if capitalize {
			b.WriteRune(unicode.ToUpper(r))
			capitalize = false
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// singularize applies simple English singularization rules.
// No NLP dependency — just common patterns.
func singularize(s string) string {
	if len(s) <= 2 {
		return s
	}

	lower := strings.ToLower(s)

	// Don't singularize words ending in "ss" (address, process)
	if strings.HasSuffix(lower, "ss") {
		return s
	}

	// Don't singularize words ending in "us" (status, campus)
	if strings.HasSuffix(lower, "us") {
		return s
	}

	// "ies" → "y" (categories → category)
	if strings.HasSuffix(lower, "ies") {
		return s[:len(s)-3] + "y"
	}

	// "ses" → "s" (addresses → address) — but skip "ses" when base is "se" (cases → case)
	if strings.HasSuffix(lower, "sses") {
		return s[:len(s)-2] // addresses → address
	}

	// "xes", "zes", "ches", "shes" → drop "es"
	if strings.HasSuffix(lower, "xes") || strings.HasSuffix(lower, "zes") ||
		strings.HasSuffix(lower, "ches") || strings.HasSuffix(lower, "shes") {
		return s[:len(s)-2]
	}

	// "ses" → drop "s" (cases → case, phases → phase)
	if strings.HasSuffix(lower, "ses") {
		return s[:len(s)-1]
	}

	// Generic "s" → drop (employees → employee)
	if strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") {
		return s[:len(s)-1]
	}

	return s
}
