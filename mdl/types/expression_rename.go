// SPDX-License-Identifier: Apache-2.0

package types

import "strings"

// RewriteExpressionReferences renames the references to oldQN that sit inside a
// Mendix expression (or an XPath, which spells them the same way), returning the
// rewritten text and how many references it rewrote.
//
// The rename sweep matches a stored string that IS a qualified name or starts
// with one. Two kinds of reference never look like that:
//
//   - a constant, which an expression always writes @Module.Name — so the sweep
//     missed every use of one, even a constant that was the whole expression;
//   - an enumeration value, Module.Enum.Value, once it is part of a longer
//     expression (`$S = Module.Enum.Value and …`).
//
// Both left the renamed element's uses naming the old one: CE1613 for a
// constant, CE0117 for an enumeration value (ako/mxcli#1671).
//
// A reference is rewritten only as a whole token: @oldQN not followed by a name
// character, or oldQN followed by `.` and a member name and not preceded by a
// name character, `.`, `/`, `$` or `@` (so neither a longer name, nor another
// module's element of the same name, nor an association step in a path). Text
// inside a string literal is data and is left alone; Mendix quotes with ' and
// escapes a quote by doubling it.
//
// oldQN must be a qualified name. A module rename arrives as a prefix pair
// ("Old." → "New.") or a bare module name; neither is handled here.
func RewriteExpressionReferences(text, oldQN, newQN string) (string, int) {
	if !isQualifiedName(oldQN) || !isQualifiedName(newQN) || !strings.Contains(text, oldQN) {
		return text, 0
	}
	var b strings.Builder
	count := 0
	inLiteral := false
	for i := 0; i < len(text); {
		c := text[i]
		if c == '\'' {
			// Inside a literal, '' is an escaped quote, not its end.
			if inLiteral && i+1 < len(text) && text[i+1] == '\'' {
				b.WriteString("''")
				i += 2
				continue
			}
			inLiteral = !inLiteral
			b.WriteByte(c)
			i++
			continue
		}
		if !inLiteral {
			// @oldQN: a constant.
			if c == '@' && strings.HasPrefix(text[i+1:], oldQN) && !isNameByte(byteAt(text, i+1+len(oldQN))) {
				b.WriteString("@" + newQN)
				i += 1 + len(oldQN)
				count++
				continue
			}
			// oldQN.Member: an enumeration value (or another member access).
			end := i + len(oldQN)
			if strings.HasPrefix(text[i:], oldQN) && startsReference(text, i) &&
				byteAt(text, end) == '.' && isNameStartByte(byteAt(text, end+1)) {
				b.WriteString(newQN)
				i = end
				count++
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	if count == 0 {
		return text, 0
	}
	return b.String(), count
}

// isQualifiedName reports whether s is Module.Name: two non-empty parts.
func isQualifiedName(s string) bool {
	dot := strings.IndexByte(s, '.')
	return dot > 0 && dot < len(s)-1
}

// startsReference reports whether a reference may begin at text[i]: not in the
// middle of a name, a path, a variable or a constant.
func startsReference(text string, i int) bool {
	if i == 0 {
		return true
	}
	p := text[i-1]
	return !isNameByte(p) && p != '.' && p != '/' && p != '$' && p != '@'
}

func byteAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

func isNameStartByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isNameByte(c byte) bool {
	return isNameStartByte(c) || c >= '0' && c <= '9'
}

// RewritePathSteps renames oldQN where it is a step of a path — in an XPath
// constraint (`[Src.Order_Customer/Src.Customer/Name = 'x']`) or an expression
// (`$O/Src.Order_Customer/Name`) — returning the rewritten text and how many
// steps it rewrote.
//
// RewriteExpressionReferences deliberately leaves path steps alone: an
// enumeration renamed must not rewrite an association that shares its name.
// This is the rewrite for when the renamed element IS an entity or an
// association, which is the caller's to establish (ako/mxcli#1836: a MOVE of
// entities between modules left every access-rule XPath naming the old module).
//
// A step is oldQN as a whole token: not preceded by a name character, `.`, `$`
// or `@`, and not followed by a name character or `.` — so neither a longer
// name, another module's element, a variable, a constant nor a member access.
// Text inside a string literal is data and is left alone.
func RewritePathSteps(text, oldQN, newQN string) (string, int) {
	if !isQualifiedName(oldQN) || !isQualifiedName(newQN) || !strings.Contains(text, oldQN) {
		return text, 0
	}
	var b strings.Builder
	count := 0
	inLiteral := false
	for i := 0; i < len(text); {
		c := text[i]
		if c == '\'' {
			if inLiteral && i+1 < len(text) && text[i+1] == '\'' {
				b.WriteString("''")
				i += 2
				continue
			}
			inLiteral = !inLiteral
			b.WriteByte(c)
			i++
			continue
		}
		if !inLiteral && strings.HasPrefix(text[i:], oldQN) && startsPathStep(text, i) {
			if next := byteAt(text, i+len(oldQN)); !isNameByte(next) && next != '.' {
				b.WriteString(newQN)
				i += len(oldQN)
				count++
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	if count == 0 {
		return text, 0
	}
	return b.String(), count
}

// startsPathStep reports whether a path step may begin at text[i]: after a
// step separator or an operator, not in the middle of a name, a variable or a
// constant.
func startsPathStep(text string, i int) bool {
	if i == 0 {
		return true
	}
	p := text[i-1]
	return !isNameByte(p) && p != '.' && p != '$' && p != '@'
}
