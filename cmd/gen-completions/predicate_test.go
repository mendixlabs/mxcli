// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A grouped keyword rule guarded by a predicate must still yield its
// completion: ERROR_MESSAGE gained one for mendixlabs/mxcli#1720 and the
// generator, reading `( E R R O R …` as an unknown part, dropped
// "ERROR MESSAGE" from the LSP keyword list.
func TestPredicatedMultiLineRuleKeepsItsKeyword(t *testing.T) {
	src := "lexer grammar L;\n" +
		"ERROR_MESSAGE: ( E R R O R WS+ M E S S A G E\n" +
		"             | E R R O R '_' M E S S A G E\n" +
		"             ) {!p.followsMemberSeparator()}?;\n" +
		"CASCADE: C A S C A D E;\n"
	path := filepath.Join(t.TempDir(), "L.g4")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := parseLexerGrammar(path)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Name] = e.Text
	}
	if got["ERROR_MESSAGE"] != "ERROR MESSAGE" {
		t.Errorf("ERROR_MESSAGE completion = %q, want %q (entries: %v)", got["ERROR_MESSAGE"], "ERROR MESSAGE", got)
	}
	if got["CASCADE"] != "CASCADE" {
		t.Errorf("control: CASCADE = %q", got["CASCADE"])
	}
}
