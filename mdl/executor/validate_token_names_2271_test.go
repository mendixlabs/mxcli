// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2271: a token name Mendix does not define passed check and exec,
// and the build refused it. Every case is a measurement on mx check 11.14.0
// (a copy of testdata/testapp, one microflow per token): CE0117 "Error(s) in
// expression." in a flow, CE0161 "Error(s) in XPath constraint." in an XPath.

func tokenViolations(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\n" + src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	var out []string
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == tokenNameRuleID {
			out = append(out, v.Message)
		}
	}
	return out
}

func TestTokenNames_RefusedInAFlowExpression(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{"[%CurrentDateTim%]", "[%CurrentDateTime%]"},  // typo, with the suggestion
		{"[%currentdatetime%]", "[%CurrentDateTime%]"}, // names are case-sensitive
		{"[%BeginOfCurrentDayUtc%]", "[%BeginOfCurrentDayUTC%]"},
		{"[%BeginOfCurrentFortnight%]", "[%BeginOfCurrentFortnight%]"}, // no such period
		{"[%Bogus%]", "[%Bogus%]"},
		{"[%BeginOfNextWeek%]", "[%BeginOfNextWeek%]"},
		{"[%BeginOfTommorow%]", "[%BeginOfTomorrow%]"}, // typo, suggested
		{"[%CurrentDeviceType%]", "[%CurrentDeviceType%]"},
		{"[%DayLength%]", "only in an XPath"},     // a length token is XPath-only
		{"[%UserRole_User%]", "only in an XPath"}, // so is a role token
		{"[%CurrentObject%]", "[%CurrentObject%]"},
	} {
		for _, kind := range []string{"microflow", "nanoflow"} {
			src := "create " + kind + " M.F () begin declare $b Boolean = " + c.expr + " != empty; end;"
			got := strings.Join(tokenViolations(t, src), "\n")
			if !strings.Contains(got, c.want) {
				t.Errorf("%s %s: want a report naming %q, got %q", kind, c.expr, c.want, got)
			}
		}
	}
}

func TestTokenNames_RefusedInAnXPath(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"create microflow M.F () begin retrieve $l from M.E where D > '[%CurentDateTime%]'; end;", "[%CurrentDateTime%]"},
		{"create microflow M.F () begin retrieve $l from M.E where D > '[%Today%]'; end;", "[%Today%]"},
		{"grant read * on entity M.E to M.User where [D > '[%BeginOfCurrentQuarter%]'];", "[%BeginOfCurrentQuarter%]"},
		{"create microflow M.F () begin retrieve $l from M.E where M.E_User = '[%CurrentObject%]'; end;", "no object in a retrieve"},
	} {
		if got := strings.Join(tokenViolations(t, c.src), "\n"); !strings.Contains(got, c.want) {
			t.Errorf("%s: want a report naming %q, got %q", c.src, c.want, got)
		}
	}
}

// Controls: every name the measurement accepted, in the contexts it accepted
// it, is silent.
func TestTokenNames_MeasuredValidOnesPass(t *testing.T) {
	var flow, xpath []string
	for _, n := range []string{"CurrentDateTime", "BeginOfCurrentMinute", "BeginOfCurrentHourUTC", "BeginOfCurrentDay",
		"EndOfCurrentWeek", "EndOfCurrentMonthUTC", "BeginOfCurrentYear", "EndOfCurrentYearUTC", "CurrentUser",
		"BeginOfTomorrow", "EndOfYesterdayUTC"} {
		flow = append(flow, "declare $b"+n+" Boolean = [%"+n+"%] != empty;")
		xpath = append(xpath, "retrieve $l"+n+" from M.E where D > '[%"+n+"%]';")
	}
	for _, n := range []string{"SecondLength", "DayLength", "MonthLength", "YearLength"} {
		xpath = append(xpath, "retrieve $l"+n+" from M.E where D > '[%CurrentDateTime%] - [%"+n+"%]';")
	}
	for _, src := range []string{
		"create microflow M.F () begin " + strings.Join(flow, " ") + " end;",
		"create nanoflow M.N () begin " + strings.Join(flow, " ") + " end;",
		"create microflow M.X () begin " + strings.Join(xpath, " ") + " end;",
		"create microflow M.R () begin retrieve $l from System.User where System.UserRoles = '[%UserRole_User%]'; end;",
		"grant read * on entity M.E to M.User where [M.E_User = '[%CurrentUser%]'];",
		// A page data source has the page's context object (measured clean).
		"create page M.P (Title: 'p', Layout: Atlas_Core.Atlas_Default, Params: ($E: M.E)) { dataview dv (DataSource: $E) { listview lv (DataSource: database from M.E where id = '[%CurrentObject%]') { dynamictext t (Content: 'x') } } };",
		// A string literal is not a token, whatever it holds.
		"create microflow M.S () begin declare $s String = '[%Bogus%]'; end;",
	} {
		if got := tokenViolations(t, src); len(got) > 0 {
			t.Errorf("measured valid, but reported:\n%s\n%v", src, got)
		}
	}
}
