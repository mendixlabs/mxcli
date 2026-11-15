// SPDX-License-Identifier: Apache-2.0

// Package doccomment is the escape a `/** … */` doc comment needs to hold any
// documentation text.
//
// The lexer ends a doc comment at the first `*/`, so documentation that holds
// one — a CSS comment in a page's notes, `/* Adjust the value */` — ended the
// comment early and the rest of the describe output did not parse
// (ako/mxcli#2113). Describe writes every `*/` as `*\/` and the visitor reads it
// back as `*/`.
//
// The escape is lossless: a `*`, any run of backslashes, then `/` gains one
// backslash on the way out and loses one on the way in. So text that already
// holds `*\/` is written `*\\/` and still reads back as itself.
package doccomment

import "regexp"

var (
	unescaped = regexp.MustCompile(`\*(\\*)/`)
	escaped   = regexp.MustCompile(`\*\\(\\*)/`)
)

// Escape returns text as a doc comment can hold it.
func Escape(text string) string {
	return unescaped.ReplaceAllString(text, `*\$1/`)
}

// Unescape returns the documentation a doc comment's text states.
func Unescape(text string) string {
	return escaped.ReplaceAllString(text, `*$1/`)
}
