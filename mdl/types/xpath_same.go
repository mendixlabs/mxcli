// SPDX-License-Identifier: Apache-2.0

package types

import "strings"

// SameXPathConstraint reports whether two XPath constraints are the same once
// whitespace outside string literals is ignored: the same constraint wrapped
// over lines, indented, or stored on one line by another writer
// (ako/mxcli#2355). Whitespace inside a literal is part of the value.
func SameXPathConstraint(a, b string) bool {
	return xpathKey(a) == xpathKey(b)
}

// xpathKey is s without whitespace outside its single-quoted literals.
func xpathKey(s string) string {
	var sb strings.Builder
	inLiteral := false
	for _, r := range s {
		if r == '\'' {
			inLiteral = !inLiteral
		}
		if !inLiteral && (r == ' ' || r == '\t' || r == '\n' || r == '\r') {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
