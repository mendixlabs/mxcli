// SPDX-License-Identifier: Apache-2.0

// Package texttemplate reads the text of a Mendix text template — the
// 'Order {1} of {2}' a log message, a show message, a validation feedback, a
// REST call's URL and custom body, a Java action's StringTemplate argument, a
// send email subject and body, a workflow's display and description, a
// dynamic text's content and a button's caption store beside their
// parameters — with the grammar mxbuild applies to every one of them.
//
// Measured with `mx check -j` on mxbuild 10.24.24, 11.13.0 and 11.15.0, one
// document per text in each of those slots (ako/mxcli#1397,
// mendixlabs/mxcli#1515). Every slot gave the same answer for every input, on
// every version:
//
//	{{                 a literal {
//	}  and  }}         literal text — a closing brace needs no escape
//	{ at the very end  literal text
//	{digits}           a placeholder; {01} is {1}
//	{ then not a digit or {     CE0712 Brace should be followed by a number of digits or another brace.
//	{digits at the end          CE0713 Missing close brace at end of string.
//	{digits then not }          CE0714 Placeholder should end in closing brace.
//	{0}                         CE0716 Place holder indices start at one (1).
//	{digits} past int32         CE0717 Place holder index N too big.
//	{n} with fewer than n parameters   CE0720 Place holder index N is greater than P, the number of parameter(s).
//
// mxbuild reports one of these per text: the first the text holds, reading
// left to right, and CE0720 only for a text with none of the others. "Digit"
// is a Unicode decimal digit, as .NET's char.IsDigit has it — `{١}` is read as
// a placeholder and refused as too big, because it does not parse as a number.
//
// A REST client document's `{name}` path parameters and an agent prompt's
// `{{variable}}` are other grammars; nothing here applies to them.
package texttemplate

import (
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// Problem is the first thing mxbuild refuses in a template text.
type Problem struct {
	// Code is the CE code mxbuild reports, e.g. "CE0712".
	Code string
	// Message is mxbuild's wording for it.
	Message string
	// At is the text from the brace the problem is at, cut to a few
	// characters, so a message can show which brace of a long body it means.
	At string
}

// mxbuild's messages (Mendix.Modeler.Texts.dll, the same on all three versions).
const (
	msgBrace      = "Brace should be followed by a number of digits or another brace."
	msgUnclosed   = "Missing close brace at end of string."
	msgNotClosed  = "Placeholder should end in closing brace."
	msgZero       = "Place holder indices start at one (1)."
	msgTooBigFmt  = "Place holder index %s too big."
	msgBeyondFmt  = "Place holder index %d is greater than %d, the number of parameter(s)."
	excerptLength = 12
)

// placeholder is one {n} of a text: its index and where it starts.
type placeholder struct {
	index int
	start int
}

// scan reads text left to right as mxbuild does and returns its placeholders,
// or the first problem.
func scan(text string) ([]placeholder, *Problem) {
	var out []placeholder
	for i := 0; i < len(text); {
		if text[i] != '{' {
			i++
			continue
		}
		start := i
		i++
		if i >= len(text) {
			break // a brace that ends the text is text
		}
		if text[i] == '{' {
			i++ // {{ is a literal brace
			continue
		}
		digitsStart := i
		for i < len(text) {
			r, size := utf8.DecodeRuneInString(text[i:])
			if !unicode.IsDigit(r) {
				break
			}
			i += size
		}
		if i == digitsStart {
			return nil, problem("CE0712", msgBrace, text, start)
		}
		if i >= len(text) {
			return nil, problem("CE0713", msgUnclosed, text, start)
		}
		if text[i] != '}' {
			return nil, problem("CE0714", msgNotClosed, text, start)
		}
		digits := text[digitsStart:i]
		i++
		n, err := strconv.ParseInt(digits, 10, 32)
		if err != nil {
			return nil, problem("CE0717", fmt.Sprintf(msgTooBigFmt, digits), text, start)
		}
		if n == 0 {
			return nil, problem("CE0716", msgZero, text, start)
		}
		out = append(out, placeholder{index: int(n), start: start})
	}
	return out, nil
}

func problem(code, msg, text string, at int) *Problem {
	return &Problem{Code: code, Message: msg, At: excerpt(text[at:])}
}

// excerpt is s, which starts at a brace, up to the brace that closes it — or
// up to the next opening brace, or a few characters, marked as cut.
func excerpt(s string) string {
	n := 0
	for i, r := range s {
		if i > 0 && r == '{' || n == excerptLength {
			return s[:i] + "…"
		}
		n++
		if r == '}' && i > 0 {
			return s[:i+1]
		}
	}
	return s
}

// Check returns the problem mxbuild reports for text filled by params
// parameters, or nil when it builds.
func Check(text string, params int) *Problem {
	phs, p := scan(text)
	if p != nil {
		return p
	}
	var highest *placeholder
	for i := range phs {
		if highest == nil || phs[i].index > highest.index {
			highest = &phs[i]
		}
	}
	if highest != nil && highest.index > params {
		return &Problem{
			Code:    "CE0720",
			Message: fmt.Sprintf(msgBeyondFmt, highest.index, params),
			At:      excerpt(text[highest.start:]),
		}
	}
	return nil
}

// Placeholders returns the indices of text's placeholders in order of
// appearance, or the problem that makes text no template mxbuild accepts (and
// then no indices). An escaped brace is not a placeholder: `{{1}}` is the
// literal text {1}.
func Placeholders(text string) ([]int, *Problem) {
	phs, p := scan(text)
	if p != nil {
		return nil, p
	}
	var out []int
	for _, ph := range phs {
		out = append(out, ph.index)
	}
	return out, nil
}

// MaxPlaceholder is the highest placeholder index text uses, 0 for none or for
// a text whose grammar mxbuild refuses (Check reports that).
func MaxPlaceholder(text string) int {
	idx, _ := Placeholders(text)
	m := 0
	for _, n := range idx {
		if n > m {
			m = n
		}
	}
	return m
}
