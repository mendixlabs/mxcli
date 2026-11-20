// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// checkedPages is n pages over the project's Parent entity, each with a data
// view bound to a page parameter, an input on one of its attributes and a
// button calling a microflow: the statement whose check needs the domain
// models (the entity, its attributes) and the flows (the button's signature).
func checkedPages(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, `create or modify page M.Memo%02d
(
  params: ( $Item: M.Parent ),
  title: 'Memo %d',
  layout: Atlas_Core.PopupLayout
)
{
  dataview dv1 (datasource: $Item) {
    textbox txtName (label: 'Name', attribute: Name)
    actionbutton btnRun (caption: 'Run', action: call microflow M.ExistingMF)
  }
};
`, i, i)
	}
	return sb.String()
}

// ako/mxcli#2160, the check's side: every page statement rebuilt the project's
// flow signatures and its entity indexes, each from a full listing of the
// microflows or the domain models, so a check cost statements × project. The
// project does not change during a check, so one pass lists each kind once.
//
// The assertion is that six statements list no more than two do. A count that
// grows with the statements is the defect, whatever its size.
func TestCheck_AddedStatementsDoNotListTheProjectAgain(t *testing.T) {
	listingsFor := func(statements int) map[string]int {
		ctx, _ := setupProjectConflictCtx(t)
		counter := countListings(ctx.Backend.(*mock.MockBackend))
		prog, errs := visitor.Build(checkedPages(statements))
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs[0])
		}
		validateProgramWithWarnings(ctx, prog)
		counter.mu.Lock()
		defer counter.mu.Unlock()
		out := map[string]int{}
		for k, v := range counter.calls {
			out[k] = v
		}
		return out
	}

	two, six := listingsFor(2), listingsFor(6)
	for _, kind := range []string{"ListDomainModels", "ListMicroflows", "ListNanoflows"} {
		if two[kind] == 0 {
			t.Errorf("control: checking a page never called %s, so its count proves nothing", kind)
		}
		if six[kind] > two[kind] {
			t.Errorf("%s: %d calls for 2 statements, %d for 6 — the check lists the project per statement", kind, two[kind], six[kind])
		}
	}
	t.Logf("2 statements: %v", two)
	t.Logf("6 statements: %v", six)
}
