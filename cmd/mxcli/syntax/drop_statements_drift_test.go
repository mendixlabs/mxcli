// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1716 (ako/mxcli#1911): `DROP ASSOCIATION Module.Name;` parses
// and executes, but a reader looking for it found nothing: `mxcli syntax` lists no
// drop topic, and `mxcli syntax drop` answered with three unrelated topics that
// merely had "drop" in a path segment. DROP ENTITY, DROP MICROFLOW, DROP NANOFLOW
// and DROP SNIPPET were documented nowhere at all.
//
// The grammar's dropStatement rule is the authority: every statement it accepts
// must be spelled out in the `drop` topic, so a DROP added to the parser cannot
// ship undocumented. Same shape as widget_keywords_drift_test.go.

// dropKeywordRules maps the grammar's shared keyword sub-rules to the canonical
// spelling — the first alternative; the others are deprecated aliases.
var dropKeywordRules = map[string]string{
	"taskQueueKw":             "TASK QUEUE",
	"consumedODataServiceKw":  "CONSUMED ODATA SERVICE",
	"publishedODataServiceKw": "PUBLISHED ODATA SERVICE",
	"consumedRestServiceKw":   "CONSUMED REST SERVICE",
	"aiModelKw":               "AI MODEL",
}

// dropStatementForms reads the DROP <words> head of every alternative of the
// dropStatement rule in the committed grammar.
func dropStatementForms(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "mdl", "grammar", "MDLParser.g4")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	rule := regexp.MustCompile(`(?s)\ndropStatement\s*\n\s*:(.*?)\n\s*;`).FindStringSubmatch(string(b))
	if rule == nil {
		t.Fatal("dropStatement rule not found — if it was renamed, re-point this guard rather than deleting it")
	}
	var out []string
	for _, line := range strings.Split(rule[1], "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "|"))
		if len(fields) == 0 || fields[0] != "DROP" {
			continue
		}
		words := []string{"DROP"}
		for _, f := range fields[1:] {
			if kw, ok := dropKeywordRules[f]; ok {
				words = append(words, kw)
				continue
			}
			if f != strings.ToUpper(f) || strings.Contains(f, "LITERAL") || f == "AT_KW" {
				break // ifExists?, qualifiedName, a literal: the head is complete
			}
			words = append(words, strings.TrimSuffix(f, "_KW"))
		}
		if len(words) == 1 {
			t.Fatalf("dropStatement alternative with an unknown keyword rule: %q — add it to dropKeywordRules", line)
		}
		out = append(out, strings.Join(words, " "))
	}
	if len(out) < 30 {
		t.Fatalf("read only %d dropStatement alternatives — the parse of the rule is broken", len(out))
	}
	return out
}

func TestDropTopicListsEveryDropStatement(t *testing.T) {
	f := ByPath("drop")
	if f == nil {
		t.Fatal(`no "drop" topic: DROP statements are not discoverable from mxcli syntax (#1716)`)
	}
	syntax := strings.ToUpper(f.Syntax)
	for _, form := range dropStatementForms(t) {
		if !strings.Contains(syntax, form+" ") {
			t.Errorf("the grammar accepts %q but the drop topic does not list it", form)
		}
	}
}

// The reported lookups: `mxcli syntax drop` must land on the topic, and
// `mxcli syntax drop association` must reach a page that spells the statement.
func TestDropLookupFindsTheStatement(t *testing.T) {
	m := Lookup([]string{"drop"})
	if !m.Exact || len(m.Features) == 0 || m.Features[0].Path != "drop" {
		t.Fatalf("syntax drop: got exact=%v %v, want the drop topic", m.Exact, paths(m.Features))
	}
	for _, q := range [][]string{{"drop", "association"}, {"drop", "entity"}, {"drop", "microflow"}} {
		m := Lookup(q)
		want := "DROP " + strings.ToUpper(q[1]) + " "
		found := false
		for _, f := range m.Features {
			if strings.Contains(strings.ToUpper(f.Syntax), want) {
				found = true
			}
		}
		if !found {
			t.Errorf("syntax %s: no topic among %v spells %q", strings.Join(q, " "), paths(m.Features), want)
		}
	}
}

func paths(fs []SyntaxFeature) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Path)
	}
	return out
}
