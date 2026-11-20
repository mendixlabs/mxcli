// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"regexp"
	"strings"
)

// leadingVisibleRolesRe is `roles (Mod.A, Mod.B) and <rest>` at the start of a
// visibility expression.
var leadingVisibleRolesRe = regexp.MustCompile(
	`(?is)^\s*roles\s*\(\s*([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+(?:\s*,\s*[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+)*)\s*\)\s+and\s+(.+?)\s*$`)

// visibleRolesOnlyRe is `roles (Mod.A, Mod.B)` and nothing else.
var visibleRolesOnlyRe = regexp.MustCompile(
	`(?is)^\s*roles\s*\(\s*([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+(?:\s*,\s*[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)+)*)\s*\)\s*$`)

// splitVisibleRoles is splitLeadingVisibleRoles that also takes a bare
// `roles (…)`, whose condition is empty. `alter page … set (visible: …)`
// receives both as expression text (ako/mxcli#2371).
func splitVisibleRoles(expr string) (roles []string, cond string, ok bool) {
	if m := visibleRolesOnlyRe.FindStringSubmatch(expr); m != nil {
		for _, r := range strings.Split(m[1], ",") {
			roles = append(roles, strings.TrimSpace(r))
		}
		return roles, "", true
	}
	return splitLeadingVisibleRoles(expr)
}

// splitLeadingVisibleRoles splits `roles (…) and <condition>` into the role
// names and the condition (ako/mxcli#2319). It declines when the condition
// has a top-level `or`: `roles (A) and x or y` reads (roles and x) or y,
// which is not "these roles, and this condition", and is left to be refused.
func splitLeadingVisibleRoles(expr string) (roles []string, cond string, ok bool) {
	m := leadingVisibleRolesRe.FindStringSubmatch(expr)
	if m == nil || hasTopLevelOr(m[2]) {
		return nil, "", false
	}
	for _, r := range strings.Split(m[1], ",") {
		roles = append(roles, strings.TrimSpace(r))
	}
	return roles, m[2], true
}

// hasTopLevelOr reports whether expr has an `or` outside parentheses and
// string literals.
func hasTopLevelOr(expr string) bool {
	depth, inStr := 0, false
	lower := strings.ToLower(expr)
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch {
		case inStr:
			if c == '\'' {
				inStr = false
			}
		case c == '\'':
			inStr = true
		case c == '(':
			depth++
		case c == ')':
			depth--
		case depth == 0 && strings.HasPrefix(lower[i:], "or") &&
			(i == 0 || !isWordByte(expr[i-1])) && (i+2 == len(expr) || !isWordByte(expr[i+2])):
			return true
		}
	}
	return false
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
