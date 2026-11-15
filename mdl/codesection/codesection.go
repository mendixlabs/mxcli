// SPDX-License-Identifier: Apache-2.0

// Package codesection is the one rule for the text of a Java or JavaScript
// action's code section, so a `$$ … $$` body and the user code read back from
// the action's source file agree.
//
// The writer indents every line of the body uniformly (one tab in a .js
// function, two in a Java executeAction). So the body must be relative: line
// endings LF, the blank lines around it dropped, and the indentation common to
// every line removed. describe did this for Java and not for JavaScript, and the
// parser trimmed only the FIRST line's leading whitespace — so a body written
// with every line indented kept the indentation on all lines but the first, the
// writer added its tab on top, and each describe → exec moved the code one tab
// further right (mendixlabs/mxcli#1812, ako/mxcli#2131).
package codesection

import "strings"

// Normalize returns s as a relative code section: LF line endings, no blank
// lines at either end, the indentation common to every non-blank line removed,
// and no trailing whitespace on any line.
func Normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	indent := ""
	first := true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		lead := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if first {
			indent, first = lead, false
			continue
		}
		for !strings.HasPrefix(lead, indent) {
			indent = indent[:len(indent)-1]
		}
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(strings.TrimPrefix(l, indent), " \t")
	}
	return strings.Join(lines, "\n")
}
