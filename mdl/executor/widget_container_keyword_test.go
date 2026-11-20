// SPDX-License-Identifier: Apache-2.0

// Every object-list container keyword the .def.json generator derives must be a
// keyword the MDL grammar can actually lex.
//
// The generator builds `mdlContainer` from the widget's property key
// (customButtons → CUSTOMBUTTON), while the grammar's widgetTypeV3 carries a
// hand-maintained list. Nothing checked that the two agree, so a widget whose
// object list had no keyword described back as MDL that does not parse:
//
//	line 22:12 mismatched input 'custombutton' expecting '}'
//
// Measured on File Uploader 2.5.0: `describe page` emitted `custombutton
// custombutton1 (...)` and `exec` of that description was a parse error, so the
// page could not round-trip and the item's action slots could not be authored at
// all (#956).
package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// containerKeywordsInGrammar reads the keywords widgetTypeV3 accepts straight
// from the grammar, so the test cannot drift from it.
func containerKeywordsInGrammar(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "grammar", "domains", "MDLPage.g4"))
	if err != nil {
		t.Skipf("grammar not readable: %v", err)
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*\|\s*([A-Z][A-Z0-9]*)\s*$`).FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	if len(out) < 10 {
		t.Fatalf("only %d keywords parsed from MDLPage.g4 — the regex has drifted from the grammar", len(out))
	}
	return out
}

// parsesAsContainer reports whether kw parses as a widget-body container. The
// grammar's `| keyword` alternative of widgetTypeV3 admits a keyword token that
// is not enumerated — `attr`, `attribute`, `event`. Those three were listed here
// as knowingly unsupported (#239) after that alternative had made them writable
// (ako/mxcli#904); asking the parser keeps the test honest without a second list.
func parsesAsContainer(kw string) bool {
	_, errs := visitor.Build("create page M.P (Title: 'T', Layout: A.L) {\n" +
		"  pluggablewidget 'w.W' w1 {\n    " + strings.ToLower(kw) + " (x: 'y')\n  }\n}")
	return len(errs) == 0
}

func TestDerivedContainerKeywordsExistInTheGrammar(t *testing.T) {
	known := containerKeywordsInGrammar(t)

	// Derive the keyword the generator would produce for each object-list
	// property name seen on real widgets, using the generator's own function.
	for _, propertyKey := range []string{
		"columns", "groups", "series", "lines", "markers", "dynamicMarkers",
		"scaleColors", "customItems", "basicItems", "customButtons",
		"allowedFileFormats", "attributesList", "attributes", "events",
	} {
		kw := strings.ToUpper(deriveObjectListKeyword(propertyKey))
		if kw == "" {
			continue
		}
		if known[kw] {
			continue
		}
		if parsesAsContainer(kw) {
			continue
		}
		t.Errorf("object-list property %q derives container keyword %q, which widgetTypeV3 does not accept — "+
			"DESCRIBE will emit MDL that does not parse; add it to MDLLexer.g4 + MDLPage.g4", propertyKey, kw)
	}
}
