// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ALTER builds an action against a stored document it never walks, so the
// argument context is unknown. A show page argument naming the document's own
// parameter needs no context to be bound — it is bound by name — but the unknown
// context read every $-argument as the inferred context object, so the mapping
// was left empty: `alter snippet … set (Action: show page P(Sale = $Sale))`
// stored ParameterMappings [2], describe printed `$currentObject`, and mx check
// failed CE1571 (ako/mxcli#2621, upstream mendixlabs/mxcli#1962).
func TestBuildShowPage_AlterContextBindsADocumentParameterByName_2621(t *testing.T) {
	build := func(isSnippet bool, arg string) *pages.PageClientAction {
		t.Helper()
		pb := &pageBuilder{
			execCache:  &executorCache{createdPages: map[string]*createdPageInfo{"Mod.Detail": {ID: "detail-id"}}},
			paramScope: map[string]model.ID{"SomeRef": "entity-id"},
			isSnippet:  isSnippet,
			// argCtx left zero: the alter builders' unknown context.
		}
		built, err := pb.buildClientActionV3(showPageButton(arg).GetAction())
		if err != nil {
			t.Fatalf("show page passing %s refused: %v", arg, err)
		}
		return built.(*pages.PageClientAction)
	}
	for _, tc := range []struct {
		isSnippet bool
		want      string
	}{{true, "snippet"}, {false, "parameter"}} {
		act := build(tc.isSnippet, "$SomeRef")
		if len(act.ParameterMappings) != 1 || act.ParameterMappings[0].VariableKind != tc.want {
			t.Errorf("snippet=%v: mappings %+v, want one bound by name as %s", tc.isSnippet, act.ParameterMappings, tc.want)
		}
	}
	// Control: with the context unknown, $currentObject stays inferred.
	if kind := build(true, "$currentObject").ParameterMappings[0].VariableKind; kind != "" {
		t.Errorf("$currentObject written as an explicit %q mapping, want inferred", kind)
	}
}
