// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1018 has 26 statement types in scope — every one that accepts
// a `/** … */` doc comment AND has a rewrite path. Three are fixed. This test
// exists so the other 23 are a number somebody can read rather than a thing that
// was quietly not done: patching the reported instances and leaving the class is
// the failure the wiki calls duplicate-resolver-drift.
//
// It fails when a statement type gains a rewrite flag and a Documentation field
// without either carrying the presence bit or being listed below. Adding a new
// doctype therefore has to make a decision about documentation, rather than
// inheriting the bug by default.

// docCarryDone are the statement types whose rewrite path preserves an unstated
// doc comment. Move a type here when its executor carry lands AND
// TestDocumentation_SurvivesRewrite covers it.
var docCarryDone = map[string]bool{
	"CreateEntityStmt":               true,
	"CreateMicroflowStmt":            true,
	"CreateEnumerationStmt":          true,
	"CreateQueueStmt":                true,
	"CreateRegularExpressionStmt":    true,
	"CreateScheduledEventStmt":       true,
	"CreateNanoflowStmt":             true,
	"CreateRuleStmt":                 true,
	"CreateJsonStructureStmt":        true,
	"CreateImageCollectionStmt":      true,
	"CreateWorkflowStmt":             true,
	"CreateConstantStmt":             true,
	"CreateAssociationStmt":          true,
	"CreateViewEntityStmt":           true,
	"CreateBusinessEventServiceStmt": true,
	"CreateJavaActionStmt":           true,
	"CreateJavaScriptActionStmt":     true,
	"CreateMenuStmt":                 true,
	"CreateODataServiceStmt":         true,
	"CreatePageStmtV3":               true,
	"CreateSnippetStmtV3":            true,
	"CreateLayoutStmt":               true,
	"CreateODataClientStmt":          true,
	"CreateExternalEntityStmt":       true,
	"CreateRestClientStmt":           true,
	"CreateModelStmt":                true,
	"CreateKnowledgeBaseStmt":        true,
	"CreateConsumedMCPServiceStmt":   true,
	"CreatePublishedMCPServiceStmt":  true,
	"CreateAgentStmt":                true,
	"CreateModuleStmt":               true,
	// mendixlabs/mxcli#1510: covered by TestIntegrationDocumentation_Issue1510
	// (unit, on PedApp) as well as TestDocumentation_SurvivesRewrite.
	"CreateImportMappingStmt":        true,
	"CreateExportMappingStmt":        true,
	"CreatePublishedRestServiceStmt": true,
	"CreateDataTransformerStmt":      true,
}

// docCarryPending is empty: every statement type in scope is carried AND
// covered by TestDocumentation_SurvivesRewrite.
//
// It is kept rather than deleted because it is the mechanism that made the
// remaining work visible while there was any, and because the next doctype
// added has to land in one list or the other.
//
// Worth recording why it emptied. The last seven entries carried BLOCKERS —
// "agent editor needs AgentEditorCommons and Mendix 11.9+", "needs a reachable
// $metadata", "needs an OpenAPI spec" — and every one of them was an assumption
// written down as if it were a measurement. Tested directly, all seven author
// fine offline against an ordinary 11.13 project. A blocker nobody has tried is
// a guess with a citation.
var docCarryPending = map[string]string{}

var (
	// `(?:V\d)?` is load-bearing: CreatePageStmtV3 and CreateSnippetStmtV3 were
	// invisible to the first version of this pattern, so two in-scope doctypes
	// were absent from a list whose entire job is to be complete.
	stmtRe = regexp.MustCompile(`(?s)type (\w+Stmt(?:V\d)?) struct \{(.*?)\n\}`)
	// A rewrite flag is spelled CreateOrModify / CreateOrReplace on most
	// statements and IsReplace / IsModify on a few (CreateLayoutStmt). Matching
	// only the first spelling made this test disagree with the enumeration it
	// was built from, which is the kind of drift it exists to catch.
	rewriteRe = regexp.MustCompile(`CreateOr(Modify|Replace)|Is(Replace|Modify)\s+bool`)
)

func TestDocumentationCarry_EveryRewritableDoctypeIsAccountedFor(t *testing.T) {
	entries, err := os.ReadDir("../ast")
	if err != nil {
		t.Fatal(err)
	}
	var unaccounted []string
	seen := map[string]bool{}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "ast_") || !strings.HasSuffix(e.Name(), ".go") ||
			strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join("../ast", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range stmtRe.FindAllStringSubmatch(string(src), -1) {
			name, body := m[1], m[2]
			// Image collections spell it `Comment`; everything else `Documentation`.
			if (!strings.Contains(body, "Documentation") && !strings.Contains(body, "Comment")) ||
				!rewriteRe.MatchString(body) {
				continue
			}
			seen[name] = true
			if docCarryDone[name] || docCarryPending[name] != "" {
				continue
			}
			unaccounted = append(unaccounted, name)
		}
	}
	sort.Strings(unaccounted)
	for _, n := range unaccounted {
		t.Errorf("%s takes a doc comment and has a rewrite path, but is in neither "+
			"docCarryDone nor docCarryPending — its rewrite silently deletes documentation "+
			"(#1018). Carry the stored value when !DocumentationSet, or list it as pending.", n)
	}

	// A type that no longer exists must not linger in either list, or the
	// remaining-work count is fiction.
	for name := range docCarryPending {
		if !seen[name] {
			t.Errorf("docCarryPending lists %s, which no longer matches a statement type", name)
		}
	}
	for name := range docCarryDone {
		if !seen[name] {
			t.Errorf("docCarryDone lists %s, which no longer matches a statement type", name)
		}
	}
	t.Logf("#1018 documentation carry: %d done, %d pending (of %d in scope)",
		len(docCarryDone), len(docCarryPending), len(seen))
}

// A type in docCarryDone must actually carry the bit, or the list is a claim
// rather than a record.
func TestDocumentationCarry_DoneTypesHaveThePresenceBit(t *testing.T) {
	entries, _ := os.ReadDir("../ast")
	found := map[string]bool{}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "ast_") || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, _ := os.ReadFile(filepath.Join("../ast", e.Name()))
		for _, m := range stmtRe.FindAllStringSubmatch(string(src), -1) {
			if docCarryDone[m[1]] && strings.Contains(m[2], "DocumentationSet") {
				found[m[1]] = true
			}
		}
	}
	for name := range docCarryDone {
		if !found[name] {
			t.Errorf("%s is in docCarryDone but has no DocumentationSet field", name)
		}
	}
}
