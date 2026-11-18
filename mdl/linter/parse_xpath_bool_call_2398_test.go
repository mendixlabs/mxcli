// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ako/mxcli#2398 (mendixlabs/mxcli#1894): parse_xpath stopped at `false()`,
// left `() or …` on the stream, and returned `(IsConfidential = false)` as if
// it were the whole constraint — so a rule over an access rule's `or` branches
// judged a fragment, depending on the order of the terms.

func parseXPathForTest(t *testing.T, src string) *starlarkstruct.Struct {
	t.Helper()
	r := &StarlarkRule{}
	v, err := r.builtinParseXPath(nil, nil, starlark.Tuple{starlark.String(src)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return v.(*starlarkstruct.Struct)
}

func attrString(t *testing.T, s *starlarkstruct.Struct, name string) string {
	t.Helper()
	v, err := s.Attr(name)
	if err != nil {
		return ""
	}
	if str, ok := v.(starlark.String); ok {
		return string(str)
	}
	return ""
}

func TestParseXPath_BooleanCallKeepsTheRest_Issue2398(t *testing.T) {
	for _, src := range []string{
		"[IsConfidential = false() or Mod.Report_Owner = '[%CurrentUser%]']",
		"[Mod.Report_Owner = '[%CurrentUser%]' or IsConfidential = true()]",
		// CONTROL: the bare form always parsed whole.
		"[IsConfidential = false or Mod.Report_Owner = '[%CurrentUser%]']",
	} {
		root := parseXPathForTest(t, src)
		if k := attrString(t, root, "kind"); k != "bin" {
			t.Fatalf("%s: root kind = %q, want bin", src, k)
		}
		if op := attrString(t, root, "op"); !strings.EqualFold(op, "or") {
			t.Errorf("%s: root op = %q, want or — a branch was dropped", src, op)
		}
	}
}

// A parse that stopped early is a recovered root, not a partial tree.
func TestParseXPath_TrailingTokensIsRecovered_Issue2398(t *testing.T) {
	root := parseXPathForTest(t, "[A = 1 ) or B = 2]")
	if k := attrString(t, root, "kind"); k != "recovered" {
		t.Errorf("root kind = %q, want recovered: the rule must see the parse stopped early", k)
	}
	if src := attrString(t, root, "source"); src == "" {
		t.Error("recovered root has no source")
	}
}
