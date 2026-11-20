// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1762 (mendixlabs/mxcli#1668), the reporter's script. Reported: check
// and exec both pass, and mx check says "[CE0463] The definition of this widget
// has changed" at Timeline 'tlD' (groupByDayOptions, hidden in custom mode) and
// 'tlG' (groupByMonthOptions, hidden when groupByKey is 'day'). exec must refuse
// it before writing anything, naming both — as it already did for the one
// Timeline rule it knew, `groupEvents: false`.
const timeline1762Setup = `create module ModTL;
create non-persistent entity ModTL.Ev ( Line: String(200), WhoText: String(200), WhenText: String(50), At: DateTime );
create microflow ModTL.DS_Events () returns List of ModTL.Ev as $Events
begin
  $Events = create list of ModTL.Ev;
  $E = create ModTL.Ev ( Line = 'Created', WhoText = 'someone', WhenText = '10:00', At = [%CurrentDateTime%] );
  add $E to $Events;
  return $Events;
end;
`

func timeline1762Pages(dayOptions, monthOptions string) string {
	return `create page ModTL.PageD ( Title: 'D', Layout: Atlas_Core.Atlas_Default ) {
  timeline tlD (
    data: MICROFLOW ModTL.DS_Events,
    customVisualization: true, groupEvents: true, groupAttribute: At,
    groupByKey: 'day', ` + dayOptions + `ungroupedEventsPosition: 'end'
  ) {
    customtitle d2 { dynamictext d2t (Content: '{1}', ContentParams: ({1} = Line)) }
  }
};
create page ModTL.PageG ( Title: 'G', Layout: Atlas_Core.Atlas_Default ) {
  timeline tlG (
    data: MICROFLOW ModTL.DS_Events,
    title: '{1}', titleParams: ({1} = $currentObject/Line),
    customVisualization: false, groupEvents: true, groupAttribute: At,
    groupByKey: 'day', ` + monthOptions + `ungroupedEventsPosition: 'end'
  )
};
`
}

func TestExecPreflightRefusesHiddenTimelineEnumerations(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "expr-checker")
	if _, err := os.Stat(filepath.Join(src, "minimal.mpr")); err != nil {
		t.Skipf("expr-checker fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := testutil.CopyFixture(dir, src); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "minimal.mpr")
	exec := executor.New(io.Discard)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })

	preflight := func(script string) (string, string) {
		t.Helper()
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		var out bytes.Buffer
		refusal := execPreflight(exec, prog, mpr, "s.mdl", false, false, false, deprecation.Warn, &out, false)
		return refusal, out.String()
	}

	refusal, out := preflight(timeline1762Setup +
		timeline1762Pages("groupByDayOptions: 'dayMonth', ", "groupByMonthOptions: 'monthYear', "))
	if refusal == "" || !strings.Contains(refusal, "Nothing was written") {
		t.Fatalf("want the pre-flight to refuse the script, got %q\n%s", refusal, out)
	}
	for _, want := range []string{"`tlD`", "`groupByDayOptions`", "`tlG`", "`groupByMonthOptions`"} {
		if !strings.Contains(out, want) {
			t.Errorf("pre-flight output must name %s:\n%s", want, out)
		}
	}

	// CONTROL: the same pages without the hidden values (variant b of the
	// report, which mx check passes) run.
	if refusal, out := preflight(timeline1762Setup + timeline1762Pages("", "")); refusal != "" {
		t.Errorf("the pages without hidden values were refused: %s\n%s", refusal, out)
	}
}
