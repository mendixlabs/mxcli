// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2583: inside a loop body @position is in the loop BOX's
// coordinates (#2564). A body written in canvas coordinates passed check and
// exec, and the builder sized the box to reach it — measured on the
// mendixlabs/mxcli#1935 repro: loop at 600,200 with its body at 600,300 stored
// Size 710;380, at 1200 1310;380; the same body at 210,100 stored 320;180.

func loopCoordViolations(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\n" + src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	var out []string
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == loopBodyCoordsRuleID {
			out = append(out, v.Message+" | "+v.Suggestion)
		}
	}
	return out
}

func loopFlow(loopPos, body, extra string) string {
	return "create microflow Repro.F ($Items: List of Repro.Item) begin @start(60, 200) " + loopPos +
		" loop $Item in $Items " + extra + " begin " + body + " end loop; @position(1900, 200) return; end;"
}

func TestLoopBodyInCanvasCoordinatesIsReported(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the #1935 repro, small", loopFlow("@position(600, 200)", "@position(600, 300) log info node 'R' 'x';", ""), "@position(210, 80)"},
		{"the #1935 repro, large", loopFlow("@position(1200, 200)", "@position(1200, 300) log info node 'R' 'x';", ""), "@position(210, 80)"},
		{"two body nodes", loopFlow("@position(600, 200)", "@position(600, 300) log info node 'R' 'x'; @position(600, 400) log info node 'R' 'y';", ""), "@position(210, 80)"},
		{"only too far down", loopFlow("@position(600, 200)", "@position(210, 400) log info node 'R' 'x';", ""), "@position(210, 80)"},
		{"a while loop", "create microflow Repro.W () begin declare $i Integer = 0; @position(900, 200) while $i < 3 begin @position(900, 300) set $i = $i + 1; end while; end;", "@position(210, 80)"},
		{"a nanoflow", "create nanoflow Repro.N ($Items: List of Repro.Item) begin @position(900, 200) loop $Item in $Items begin @position(900, 300) log info node 'R' 'x'; end loop; end;", "@position(210, 80)"},
	} {
		got := strings.Join(loopCoordViolations(t, c.src), "\n")
		if !strings.Contains(got, c.want) || !strings.Contains(got, "$Items") && !strings.Contains(got, "while") {
			t.Errorf("%s: want a report suggesting %s, got %q", c.name, c.want, got)
		}
	}
}

// Controls: box coordinates, a stated box, and a body the builder places.
func TestLoopBodyInBoxCoordinatesPasses(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"box coordinates", loopFlow("@position(1200, 200)", "@position(210, 100) log info node 'R' 'x';", "")},
		{"a wide body in box coordinates", loopFlow("@position(1200, 200)", "@position(210, 80) log info node 'R' 'x'; @position(900, 80) log info node 'R' 'y';", "")},
		{"a stated @size", loopFlow("@position(1200, 200) @size(1310, 380)", "@position(1200, 300) log info node 'R' 'x';", "")},
		{"no body @position", loopFlow("@position(1200, 200)", "log info node 'R' 'x';", "")},
		{"a modest margin", loopFlow("@position(600, 200)", "@position(260, 150) log info node 'R' 'x';", "")},
	} {
		if got := loopCoordViolations(t, c.src); len(got) > 0 {
			t.Errorf("%s: reported %v", c.name, got)
		}
	}
}
