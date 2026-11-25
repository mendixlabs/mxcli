// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func meterFlow(body string) string {
	return "create microflow M.F ($N: Integer, $D: Decimal, $R: String) begin " + body + " end;"
}

func exprSource(e ast.Expression) string {
	if s, ok := e.(*ast.SourceExpr); ok {
		return s.Source
	}
	if l, ok := e.(*ast.LiteralExpr); ok {
		if s, ok := l.Value.(string); ok {
			return "'" + s + "'"
		}
	}
	return ""
}

// The three metrics activities: `increment counter` without `by` is Increment
// Counter, with `by` it is Counter, `set gauge … to` is Gauge. Name, value and
// tags are expressions — the constant form is the one Studio Pro 11.16.1 stored
// in ako/TestApp.
func TestMeterStatementsBuild(t *testing.T) {
	prog := mustBuild(t, meterFlow(`
  increment counter 'acme.orders' (Tags: ('region': $R, 'channel': 'web'));
  increment counter @Metrics.NAME_COUNTER by $N;
  set gauge 'acme.depth' to $D (Tags: ());
`))
	body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
	if len(body) != 3 {
		t.Fatalf("got %d statements, want 3", len(body))
	}
	inc, ok := body[0].(*ast.IncrementCounterStmt)
	if !ok || inc.By != nil {
		t.Fatalf("first statement = %#v, want IncrementCounterStmt without By", body[0])
	}
	if len(inc.Tags) != 2 || exprSource(inc.Tags[0].Key) != "'region'" {
		t.Errorf("tags = %+v", inc.Tags)
	}
	ctr, ok := body[1].(*ast.IncrementCounterStmt)
	if !ok || ctr.By == nil || len(ctr.Tags) != 0 {
		t.Fatalf("second statement = %#v, want IncrementCounterStmt with By and no tags", body[1])
	}
	if _, ok := body[2].(*ast.SetGaugeStmt); !ok {
		t.Fatalf("third statement = %T, want SetGaugeStmt", body[2])
	}
}

// R11: the settings list is new syntax, so an unknown or repeated key and a Tags
// value that is not a map are errors.
func TestMeterSettingsAreStrict(t *testing.T) {
	cases := map[string]struct{ stmt, want string }{
		"unknown key":  {"increment counter 'a' (Tag: ('k': 'v'));", "did you mean 'Tags'"},
		"repeated key": {"set gauge 'a' to 1 (Tags: ('k': 'v'), Tags: ());", "sets Tags twice"},
		"tags not map": {"increment counter 'a' by 2 (Tags: 'k');", "Tags takes a map"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, errs := Build(meterFlow(tc.stmt))
			var all []string
			for _, e := range errs {
				all = append(all, e.Error())
			}
			if !strings.Contains(strings.Join(all, "\n"), tc.want) {
				t.Errorf("errors %q, want one containing %q", all, tc.want)
			}
		})
	}
	// CONTROL: the well-formed statement builds clean.
	if _, errs := Build(meterFlow("increment counter 'a' (Tags: ('k': 'v'));")); len(errs) != 0 {
		t.Errorf("valid statement reported: %v", errs)
	}
}
