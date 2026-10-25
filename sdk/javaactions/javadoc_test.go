// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1653: Studio Pro and mxbuild write a Java action's
// documentation as a javadoc block directly above the class; GenerateSource
// wrote none, so every `create or modify java action` removed the block and
// the next build put it back. The format is the one measured on generated
// files: `/**`, one ` * <line>` per line (an empty line keeps the trailing
// space), ` */`, CRLF like the rest of the file.
func TestGenerateSource_WritesTheDocumentationAsClassJavadoc(t *testing.T) {
	src := GenerateSource("MyFirstModule", "FormatAmount", "return null;", nil, &StringType{}, nil, "",
		"Line one.\n\nLine three.")
	want := "\r\n/**\r\n * Line one.\r\n * \r\n * Line three.\r\n */\r\npublic class FormatAmount extends UserAction<java.lang.String>\r\n"
	if !strings.Contains(src, want) {
		t.Errorf("want the javadoc directly above the class:\n%q\ngot:\n%s", want, src)
	}
}

// Documentation stored with CRLF line ends (as Studio Pro may store it) gives
// the same block, not one with stray carriage returns.
func TestGenerateSource_JavadocFromCRLFDocumentation(t *testing.T) {
	lf := GenerateSource("MyFirstModule", "A", "return null;", nil, &StringType{}, nil, "", "One.\nTwo.")
	crlf := GenerateSource("MyFirstModule", "A", "return null;", nil, &StringType{}, nil, "", "One.\r\nTwo.")
	if lf != crlf {
		t.Errorf("CRLF documentation gives a different file:\n%s", lineDiff(lf, crlf))
	}
}

// Control: no documentation, no javadoc — the file mxbuild writes for an
// undocumented action (the goldens in testdata/mxbuild).
func TestGenerateSource_NoDocumentationNoJavadoc(t *testing.T) {
	src := GenerateSource("MyFirstModule", "A", "return null;", nil, &StringType{}, nil, "", "")
	head, _, _ := strings.Cut(src, "public class ")
	if strings.Contains(head, "/**") {
		t.Errorf("javadoc written for an action without documentation:\n%s", head)
	}
}

// Regenerating over a file that already has the javadoc — what a build left
// behind — gives that file back, so a rewrite with unchanged documentation
// leaves no diff.
func TestGenerateSource_RegeneratesOverAFileWithJavadoc(t *testing.T) {
	doc := "Formats an amount as text with two decimals.\n\nSecond paragraph."
	params := []*JavaActionParameter{param("Amount", &DecimalType{})}
	first := GenerateSource("MyFirstModule", "FormatAmount", `return String.format("%.2f", Amount);`,
		params, &StringType{}, nil, "", doc)
	imports, extra := RetainSections(first, nil, "")
	again := GenerateSource("MyFirstModule", "FormatAmount", `return String.format("%.2f", Amount);`,
		params, &StringType{}, imports, extra, doc)
	if again != first {
		t.Errorf("regenerating over a file with javadoc changed it:\n%s", lineDiff(first, again))
	}
}
