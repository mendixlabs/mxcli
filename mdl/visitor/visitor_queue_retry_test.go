// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1729 (ako/mxcli#1931): "No spelling is accepted" — try1 was
// `line 4:81 mismatched input 'retry' expecting ';'`.
func TestQueueRetryClauseParses(t *testing.T) {
	src := `create microflow Ops.ACT ()
begin
  call microflow Ops.POST_Message(Body = $Body) in queue Ops.SendQueue
    retry exponential (Retries: 20, InitialInterval: 1, MaximumInterval: 60, IntervalType: Minutes);
  call java action Ops.Refresh(Url = $Url) in queue Ops.SendQueue
    retry fixed (Retries: 5, Interval: 10, IntervalType: minutes);
  call microflow Ops.Plain() in queue Ops.SendQueue;
end;`
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
	mf := body[0].(*ast.CallMicroflowStmt)
	want := ast.QueueRetry{Kind: "Exponential", Retries: 20, InitialInterval: 1, MaximumInterval: 60, IntervalType: "Minutes"}
	if mf.QueueRetry == nil || *mf.QueueRetry != want {
		t.Errorf("exponential = %+v, want %+v", mf.QueueRetry, want)
	}
	ja := body[1].(*ast.CallJavaActionStmt)
	wantF := ast.QueueRetry{Kind: "Fixed", Retries: 5, Interval: 10, IntervalType: "Minutes"}
	if ja.QueueRetry == nil || *ja.QueueRetry != wantF {
		t.Errorf("fixed = %+v, want %+v", ja.QueueRetry, wantF)
	}
	if body[2].(*ast.CallMicroflowStmt).QueueRetry != nil {
		t.Error("a call without a retry clause must have no retry")
	}
}

func TestQueueRetryClauseRefusals(t *testing.T) {
	for clause, want := range map[string]string{
		"retry linear (Retries: 3)":                                         `retry "linear"`,
		"retry fixed (Retries: 3, Interval: 10)":                            "needs IntervalType",
		"retry fixed (Retries: 3, Interval: 10, IntervalType: Days)":        "IntervalType is Seconds, Minutes, Hours",
		"retry fixed (Retries: 3, InitialInterval: 1, IntervalType: Hours)": `no property "InitialInterval"`,
		"retry fixed (Retries: 0, Interval: 10, IntervalType: Hours)":       "CE7044",
	} {
		_, errs := Build("create microflow Ops.ACT ()\nbegin\n  call microflow Ops.F() in queue Ops.Q " + clause + ";\nend;")
		var got []string
		for _, e := range errs {
			got = append(got, e.Error())
		}
		if !strings.Contains(strings.Join(got, "\n"), want) {
			t.Errorf("%s: want an error containing %q, got %v", clause, want, got)
		}
	}
}
