// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The unsupported-activity guards (validate_unsupported_actions.go) on Studio
// Pro's own flows: WorkflowCommons in ako/TestApp holds
// Microflows$GenerateJumpToOptionsAction and ApplyJumpToOptionAction, which
// mxcli has no MDL for. A drop + create from the description and the mdl 0
// whole-flow rebuild must be refused before anything is written — the rebuild
// in the verdict pass too (FlowUnsupportedRule), so a script is not left
// half-applied; the in-place splice under mdl 1 keeps the activity and stays
// allowed (mendixlabs/mxcli#1735, #1738).

const unsupportedFlow = "WorkflowCommons.ACT_Workflow_JumpTo"

// describeQualified prints describe output for a flow named Module.Name.
func describeQualified(t *testing.T, exec *Executor, kind, name string) string {
	t.Helper()
	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	defer func() { exec.output = saved }()
	if err := afRun(t, exec, "describe "+kind+" "+name+";"); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func verdictRules(t *testing.T, exec *Executor, src string) map[string]string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	got := map[string]string{}
	for _, v := range exec.CheckFlowVerdicts(prog) {
		got[v.RuleID] += v.Message + "\n"
	}
	return got
}

// withoutHeader strips a leading `mdl n;` line from describe output.
func withoutHeader(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 && strings.HasPrefix(strings.TrimSpace(s), "mdl ") {
		return s[i+1:]
	}
	return s
}

func dropRecreateRefusals(t *testing.T, exec *Executor, src string) []error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	return validateDropRecreateKeepsActivities(exec.newExecContext(context.Background()), prog)
}

func TestUnsupportedActionGuard_DropAndCreateIsRefused(t *testing.T) {
	exec, _ := openTestAppCopy(t)
	described := describeQualified(t, exec, "microflow", unsupportedFlow)
	if !strings.Contains(described, "Unsupported action: Microflows$GenerateJumpToOptionsAction") {
		t.Fatalf("control: %s no longer holds the unsupported action:\n%s", unsupportedFlow, described)
	}
	src := "mdl 1;\ndrop microflow " + unsupportedFlow + ";\n" + withoutHeader(described)
	errs := dropRecreateRefusals(t, exec, src)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Microflows$GenerateJumpToOptionsAction") {
		t.Fatalf("drop + create of %s: want one refusal naming the action, got %v", unsupportedFlow, errs)
	}
}

// Control: a drop + create of a flow with nothing unsupported is a deliberate
// rebuild and stays allowed.
func TestUnsupportedActionGuard_PlainRebuildIsAllowed(t *testing.T) {
	exec, _ := openTestAppCopy(t)
	const plain = "WorkflowCommons.SUB_WorkflowTaskDetail_Delete"
	described := describeQualified(t, exec, "microflow", plain)
	if strings.Contains(described, "Unsupported action") {
		t.Fatalf("control subject %s holds an unsupported action:\n%s", plain, described)
	}
	src := "mdl 1;\ndrop microflow " + plain + ";\n" + withoutHeader(described)
	if errs := dropRecreateRefusals(t, exec, src); len(errs) != 0 {
		t.Fatalf("a flow without unsupported actions was refused: %v", errs)
	}
}

// Under mdl 0 a `create or modify` the splice cannot make is rebuilt as a
// whole (MDL-V1-REBUILD warning) — the same loss, refused the same way. The
// insert at the start does not fit between the stored start event and the
// decision, which the script draws where they are, so the splice refuses it
// and mdl 0 falls back to the rebuild.
func TestUnsupportedActionGuard_Mdl0RebuildIsRefused(t *testing.T) {
	exec, _ := openTestAppCopy(t)
	// ACT_Workflow_JumpTo cannot be built from its description at all — the
	// unsupported action declares a variable the rest reads — so its verdict
	// is "unpredictable", not "rebuilt". This one builds.
	const applyFlow = "WorkflowCommons.ACT_WorkflowJumpToDetails_Apply"
	described := withoutHeader(describeQualified(t, exec, "microflow", applyFlow))
	// Taking the then-branch's return away changes where the decision's paths
	// end, which the splice cannot make: mdl 1 refuses it, mdl 0 rebuilds the
	// flow (MDL-V1-REBUILD warning).
	i := strings.Index(described, "show page WorkflowCommons.Workflow_ActionConfirmation")
	if i < 0 {
		t.Fatalf("control: the then-branch's show page is gone:\n%s", described)
	}
	start := i + strings.Index(described[i:], "\n") + 1
	end := start + strings.Index(described[start:], "return;\n") + len("return;\n")
	src := "mdl 0;\n" + described[:start] + described[end:]
	if _, errs := visitor.Build(src); len(errs) > 0 {
		t.Fatalf("the edited script does not parse: %v\n%s", errs, src)
	}
	rules := verdictRules(t, exec, src)
	if _, rebuilt := rules[FlowRebuildRule]; !rebuilt {
		t.Fatalf("control: this edit no longer reaches the mdl 0 rebuild; verdicts: %v", rules)
	}
	if msg, ok := rules[FlowUnsupportedRule]; !ok || !strings.Contains(msg, "Microflows$ApplyJumpToOptionAction") {
		t.Fatalf("the mdl 0 rebuild of %s was not refused naming the action; verdicts: %v", applyFlow, rules)
	}

	// exec without the verdict pass (--no-check, or the executor driven
	// directly) refuses the rewrite too, and the activity stays.
	if err := agreeExec(t, exec, src); err == nil || !strings.Contains(err.Error(), "cannot express") {
		t.Fatalf("exec of the rebuild = %v, want the refusal", err)
	}
	if got := describeQualified(t, exec, "microflow", applyFlow); !strings.Contains(got, "Unsupported action: Microflows$ApplyJumpToOptionAction") {
		t.Fatalf("the refused rebuild still lost the activity:\n%s", got)
	}
}

// Control: under mdl 1 the in-place splice keeps the unsupported activity, so
// an edit of the flow is not refused, and exec keeps the action.
func TestUnsupportedActionGuard_SpliceKeepsTheActivity(t *testing.T) {
	exec, out := openTestAppCopy(t)
	described := describeQualified(t, exec, "microflow", unsupportedFlow)
	const old = "'It is not allowed to apply jump to activity for this workflow.'"
	if !strings.Contains(described, old) {
		t.Fatalf("control: the message to edit is gone:\n%s", described)
	}
	src := "mdl 1;\n" + strings.Replace(withoutHeader(described), old, "'Jump to is not allowed for this workflow.'", 1)
	if msg, ok := verdictRules(t, exec, src)[FlowUnsupportedRule]; ok {
		t.Fatalf("an in-place splice was refused:\n%s", msg)
	}
	if err := agreeExec(t, exec, src); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	if got := describeQualified(t, exec, "microflow", unsupportedFlow); !strings.Contains(got, "Jump to is not allowed") ||
		!strings.Contains(got, "Unsupported action: Microflows$GenerateJumpToOptionsAction") {
		t.Fatalf("after the splice the flow lacks the edit or the unsupported activity:\n%s", got)
	}
}

// loopEditFlowOfQualified loads a stored microflow by its qualified name.
func loopEditFlowOfQualified(t *testing.T, exec *Executor, qn string) *microflows.Microflow {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if h.GetModuleName(h.FindModuleID(m.ContainerID))+"."+m.Name == qn {
			mf, err := ctx.Backend.GetMicroflow(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return mf
		}
	}
	t.Fatalf("%s not found", qn)
	return nil
}
