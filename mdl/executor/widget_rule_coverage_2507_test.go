// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2507 (1): `widget describe` of a widget whose editorConfig.js
// hides properties through a local helper said "Dynamic property rules (0) …
// (none discovered)", which reads as "hides nothing". The CE0463 it caused
// went unexplained for a month.
func TestRuleCoverageSaysUnreadRulesExist_2507(t *testing.T) {
	got := ruleCoverageText(true, 0, 0)
	for _, want := range []string{"editorConfig.js", "none of its hide calls", "CE0463"} {
		if !strings.Contains(got, want) {
			t.Errorf("coverage %q lacks %q", got, want)
		}
	}
	// Controls: no editorConfig says nothing; recognised calls keep the count.
	if got := ruleCoverageText(false, 0, 0); got != "" {
		t.Errorf("no editorConfig: %q", got)
	}
	if got := ruleCoverageText(true, 2, 3); got != "2 of 3 editor hide-rules recognized" {
		t.Errorf("counted: %q", got)
	}
}

// The local helper the report's widgets use is not counted as a hide call:
// that is what makes total 0 while rules exist.
func TestLocalHideHelperIsNotAToolkitCall_2507(t *testing.T) {
	js := `function hideProperties(p,keys){return p.map(function(g){return g})} exports.getProperties=function(v,p){"none"===v.playerColor&&hideProperties(p,["notationLanguage","showNotationInput"]);return p}`
	rules, stats := extractVisibilityRulesFromJS(js)
	if len(rules) != 0 || stats.TotalHideCalls != 0 {
		t.Fatalf("rules %v, stats %+v: the premise of the coverage line changed", rules, stats)
	}
}
