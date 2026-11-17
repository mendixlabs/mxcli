// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// # One verdict for a `create or modify` of a stored flow
//
// exec, diff and check each answer "what happens to this statement?" for a
// `create or modify microflow|nanoflow` of a flow that is stored, and they must
// give the same answer (ako/mxcli#839, ako/mxcli#876). diff once said
// "unchanged" for a statement exec refused; check said nothing at all for
// statements exec refused under mdl 1, so a corpus migrated with
// `fmt --upgrade --header` checked clean and then stopped executing.
//
// decideFlowModify is that answer, computed once; exec acts on it
// (modifyFlowInPlace), diff reports it (spliceVerdict) and check reports it
// (checkFlowModify, cmd_check_flow_verdict.go). The wording of a refusal and
// of the mdl 0 rebuild warning is shared too, so check quotes exec.

// flowVerdict is what exec does with a `create or modify` of a flow.
// Exactly one of the cases holds:
//
//   - plan != nil: the change is spliced into the stored flow (an empty patch
//     writes nothing);
//   - why != nil: the splice cannot make it, and exec refuses the statement
//     under mdl 1 or rebuilds the whole flow under mdl 0 (refused says which);
//   - err != nil: the statement fails on its own account, under every
//     version;
//   - none of them: there is no such flow (or module) yet, so it is a create.
type flowVerdict struct {
	plan *flowPlan
	why  *notSpliceable
	err  error
	// refused is why != nil under a language version that refuses the
	// rebuild (flowRebuildRefused).
	refused bool
}

// decideFlowModify works out the verdict for d in the script's language
// (ctx.LanguageVersion), writing nothing.
func decideFlowModify(ctx *ExecContext, d *flowDecl) flowVerdict {
	p, err := planFlowModify(ctx, d)
	var why *notSpliceable
	switch {
	case errors.As(err, &why):
		return flowVerdict{why: why, refused: flowRebuildRefused.Applies(ctx.LanguageVersion)}
	case err != nil:
		return flowVerdict{err: err}
	}
	return flowVerdict{plan: p}
}

// flowRefusal is exec's refusal of a change the splice cannot make (mdl 1).
func flowRefusal(d *flowDecl, why error) error {
	var ns *notSpliceable
	if errors.As(why, &ns) && ns.loopHandle != "" {
		// alter splices inside a loop body only what create or modify does,
		// so "change activities with alter" would only lead to alter's own
		// refusal: name the one alter that makes the change, `replace body
		// of` the loop (ako/mxcli#1693). The deliberate rebuild is a route
		// here too (mendixlabs/mxcli#1517).
		return mdlerrors.NewValidation(fmt.Sprintf(
			"%s %s %s: this change cannot be spliced into the stored flow: %v. "+
				"Nothing was written: rebuilding the whole flow instead would reset what Studio Pro drew "+
				"(curves, merges, element IDs). %s. Or, %s",
			d.verb(), d.kind(), d.name, why, strings.TrimSuffix(replaceBodyAdvice(d.kind(), d.name.String(), ns.loopHandle), "."),
			deliberateRebuildAdvice(d.kind())))
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"%s %s %s: this change cannot be spliced into the stored flow: %v. "+
			"Nothing was written: rebuilding the whole flow instead would reset what Studio Pro drew "+
			"(curves, merges, element IDs). Change activities with `alter %s %s { … }`; %s",
		d.verb(), d.kind(), d.name, why, d.kind(), d.name, deliberateRebuildAdvice(d.kind())))
}

// deliberateRebuildAdvice is the route every splice refusal names for a
// rebuild the author means: drop the flow and create it in the same script.
func deliberateRebuildAdvice(kind string) string {
	return fmt.Sprintf("to rebuild the flow deliberately, drop the %s and create it in the same script "+
		"(the drop's module-role grants carry to a create in the same script or session, not to one in a later run) — "+
		"once, in a run of its own: a drop + create rewrites the flow on every run, so the script you keep should "+
		"then hold the `create or modify` of the result, which a second run finds unchanged", kind)
}

// replaceBodyAdvice is what `create or modify` and `alter` both say about a
// change inside a loop body the splice cannot make (ako/mxcli#1693): the
// splice inserts, drops and replaces plain activities in a body, and anything
// else there — a decision, a nested loop, a return — changes the body's shape.
// What makes such a change is `replace body of` the loop: the loop itself
// keeps its element ID, position, iterator and list, and the rest of the flow
// is untouched; only what is inside the loop is rebuilt, which the statement
// states. Each mode used to send the reader to the other, and then to
// `replace loop`, which rebuilt the loop as well (option (b) of the issue,
// rejected: it drops the loop's identity and layout).
// loopHandle is the stored loop's handle as `describe … with handles` prints
// it (`loop $It in $Items`, `while $N < 10`).
func replaceBodyAdvice(kind, name, loopHandle string) string {
	return fmt.Sprintf("Replace the loop's body instead, stating it as it should be "+
		"(the loop keeps its element ID, position, iterator and list, and the rest of the flow is untouched; "+
		"only what is inside the loop is rebuilt): "+
		"`alter %s %s { replace body of %s with begin … end; };`",
		kind, name, loopHandle)
}

// flowRebuildWarning is exec's MDL-V1-REBUILD warning for the whole-flow
// rebuild it runs instead under mdl 0, without the "Warning [code]: " prefix.
func flowRebuildWarning(ctx *ExecContext, d *flowDecl, why error) string {
	return fmt.Sprintf("%s %s is rebuilt as a whole: %v. %s",
		d.kind(), d.name, why, flowRebuildRefused.Warning(ctx.LanguageVersion))
}
