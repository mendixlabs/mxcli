// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// An edit of a stored flow in place changes no unit's container, so the
// container hierarchy built for the statement before still holds. It was
// discarded after every `create or modify`, and rebuilding it lists every
// unit of the project — per statement (ako/mxcli#2160).
//
// A statement that moves the flow to another folder does change a container,
// and must still discard it: the control here, and the reason the test then
// reads the flow back by its name.
func TestHierarchy_KeptAcrossAnEditInPlaceAndDroppedByAMove(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const flow = `microflow MyFirstModule.HierarchyProbe ($Count: integer) returns integer
begin
  declare $Total integer = $Count;
  log info node 'Probe' '%s';
  return $Total;
end;`
	stmt := func(prefix, folder, text string) string {
		body := strings.Replace(flow, "%s", text, 1)
		if folder != "" {
			body = strings.Replace(body, "\nbegin", "\nfolder '"+folder+"'\nbegin", 1)
		}
		return "mdl 1;\n" + prefix + " " + body + "\n"
	}
	run := func(script string) {
		t.Helper()
		out.Reset()
		if err := runScript(t, exec, script); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
	}
	run(stmt("create", "", "one"))

	run(stmt("create or modify", "", "two"))
	if !strings.Contains(out.String(), "Modified microflow") {
		t.Fatalf("the edit was not made in place:\n%s", out.String())
	}
	kept := exec.cache.hierarchy
	if kept == nil {
		t.Fatal("the hierarchy was discarded by an edit in place")
	}
	run(stmt("create or modify", "", "three"))
	if exec.cache.hierarchy != kept {
		t.Fatal("the hierarchy was rebuilt between two edits in place")
	}

	// Control: a move changes the flow's container.
	run(stmt("create or modify", "Moved/Here", "four"))
	if exec.cache.hierarchy == kept {
		t.Fatal("the hierarchy was kept across a move to another folder")
	}
	run("describe microflow MyFirstModule.HierarchyProbe;\n")
	if got := out.String(); !strings.Contains(got, "MyFirstModule.HierarchyProbe") || !strings.Contains(got, "'four'") || !strings.Contains(got, "Moved/Here") {
		t.Fatalf("the moved flow is not described as moved and edited:\n%s", got)
	}
	// And an edit in place after the move keeps the new one.
	run(stmt("create or modify", "Moved/Here", "five"))
	after := exec.cache.hierarchy
	run(stmt("create or modify", "Moved/Here", "six"))
	if after == nil || exec.cache.hierarchy != after {
		t.Fatal("the hierarchy was not kept across an edit in place after the move")
	}
}
