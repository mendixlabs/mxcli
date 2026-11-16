// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2293: `check --references` did not resolve an agent's references.
// `create agent … (Model: M.Missing, Entity: M.Missing)` passed check and exec
// then refused it — "model not found", and since ako/mxcli#2291 "entity not
// found". The same held for a tool's Document (microflow or consumed MCP
// service), a knowledge base tool's Source, and every alter agent form that
// names one. check now runs exec's own resolution, so the two refuse the same
// statements with the same message.

const issue2293Setup = `create persistent entity MyFirstModule.Ticket ( "Subject": String(200) );
create model MyFirstModule.Support ( Provider: MxCloudGenAI );
create knowledge base MyFirstModule.Docs ( Provider: MxCloudGenAI );
create consumed mcp service MyFirstModule.Weather ( ProtocolVersion: v2025_03_26 );
create microflow MyFirstModule.ToolMf ()
begin
  return;
end;
create agent MyFirstModule.Stored ( UsageType: Task, SystemPrompt: 'x' );`

// issue2293Check runs check --references over src against the project exec
// is connected to, and returns its error messages.
func issue2293Check(t *testing.T, exec *Executor, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", src, errs)
	}
	var out []string
	for _, err := range exec.ValidateProgram(prog) {
		out = append(out, err.Error())
	}
	return out
}

// issue2293Exec runs src statement by statement, as exec's handlers do, and
// returns the first refusal ("" when every statement ran).
func issue2293Exec(t *testing.T, exec *Executor, src string) string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", src, errs)
	}
	for _, stmt := range prog.Statements {
		if err := exec.Execute(stmt); err != nil {
			return err.Error()
		}
	}
	return ""
}

func TestIssue2293_AgentReferencesResolveInCheck(t *testing.T) {
	exec, _ := agentEditorProjectOut(t)
	run(t, exec, issue2293Setup)

	// Controls: references the project holds, a System entity (written by
	// name, ako/mxcli#2291), and references the same script creates first.
	passing := map[string]string{
		"stored references": `create agent MyFirstModule.A ( UsageType: Task, Model: MyFirstModule.Support, SelectedModel: 'gpt-4o', Entity: MyFirstModule.Ticket )
{
  tool T ( Document: MyFirstModule.ToolMf )
  mcp service MyFirstModule.Weather ( Enabled: true )
  knowledge base K ( Source: MyFirstModule.Docs )
};`,
		"system entity": `create agent MyFirstModule.A ( UsageType: Task, Entity: System.User );`,
		"created earlier in the script": `create persistent entity MyFirstModule.NewEnt ( "Name": String(20) );
create model MyFirstModule.NewModel ( Provider: MxCloudGenAI );
create knowledge base MyFirstModule.NewKb ( Provider: MxCloudGenAI );
create consumed mcp service MyFirstModule.NewSvc ( ProtocolVersion: v2025_03_26 );
create microflow MyFirstModule.NewMf ()
begin
  return;
end;
create agent MyFirstModule.A ( UsageType: Task, Model: MyFirstModule.NewModel, SelectedModel: 'gpt-4o', Entity: MyFirstModule.NewEnt )
{
  tool T ( Document: MyFirstModule.NewMf )
  mcp service MyFirstModule.NewSvc ( Enabled: true )
  knowledge base K ( Source: MyFirstModule.NewKb )
};
alter agent MyFirstModule.A set Model = MyFirstModule.NewModel, Entity = MyFirstModule.NewEnt;`,
		"alter with stored references": `alter agent MyFirstModule.Stored set Model = MyFirstModule.Support, Entity = MyFirstModule.Ticket
  add tool T2 ( Document: MyFirstModule.ToolMf )
  add mcp service MyFirstModule.Weather ( Enabled: true )
  add knowledge base K2 ( Source: MyFirstModule.Docs );`,
		"alter of an agent created earlier": `create agent MyFirstModule.Fresh ( UsageType: Task );
alter agent MyFirstModule.Fresh set Entity = MyFirstModule.Ticket;`,
	}
	for name, src := range passing {
		t.Run("passes/"+name, func(t *testing.T) {
			if errs := issue2293Check(t, exec, src); len(errs) > 0 {
				t.Errorf("check refused it:\n  %s", strings.Join(errs, "\n  "))
			}
		})
	}

	// Each refused by check and by exec, with exec's message.
	refused := []struct{ name, src, want string }{
		{"missing model", `create agent MyFirstModule.B ( UsageType: Task, Model: MyFirstModule.Missing, SelectedModel: 'gpt-4o' );`,
			"create agent MyFirstModule.B: model not found: MyFirstModule.Missing"},
		{"missing entity", `create agent MyFirstModule.B ( UsageType: Task, Entity: MyFirstModule.Missing );`,
			"create agent MyFirstModule.B: entity not found: MyFirstModule.Missing"},
		{"missing tool microflow", `create agent MyFirstModule.B ( UsageType: Task ) { tool T ( Document: MyFirstModule.NoMf ) };`,
			"create agent MyFirstModule.B: tool T: microflow not found: MyFirstModule.NoMf"},
		{"missing mcp service", `create agent MyFirstModule.B ( UsageType: Task ) { mcp service MyFirstModule.NoSvc ( Enabled: true ) };`,
			"create agent MyFirstModule.B: consumed mcp service not found: MyFirstModule.NoSvc"},
		{"missing knowledge base", `create agent MyFirstModule.B ( UsageType: Task ) { knowledge base K ( Source: MyFirstModule.NoKb ) };`,
			"create agent MyFirstModule.B: knowledge base not found: MyFirstModule.NoKb"},
		{"created later in the script", `create agent MyFirstModule.B ( UsageType: Task, Model: MyFirstModule.Later, SelectedModel: 'gpt-4o' );
create model MyFirstModule.Later ( Provider: MxCloudGenAI );`,
			"create agent MyFirstModule.B: model not found: MyFirstModule.Later"},
		{"alter set model", `alter agent MyFirstModule.Stored set Model = MyFirstModule.Missing;`,
			"alter agent Stored: model not found: MyFirstModule.Missing"},
		{"alter set entity", `alter agent MyFirstModule.Stored set Entity = MyFirstModule.Missing;`,
			"alter agent Stored: entity not found: MyFirstModule.Missing"},
		{"alter add tool", `alter agent MyFirstModule.Stored add tool T3 ( Document: MyFirstModule.NoMf );`,
			"alter agent MyFirstModule.Stored: tool T3: microflow not found: MyFirstModule.NoMf"},
		{"alter add mcp service", `alter agent MyFirstModule.Stored add mcp service MyFirstModule.NoSvc ( Enabled: true );`,
			"alter agent MyFirstModule.Stored: consumed mcp service not found: MyFirstModule.NoSvc"},
		{"alter add knowledge base", `alter agent MyFirstModule.Stored add knowledge base K3 ( Source: MyFirstModule.NoKb );`,
			"alter agent MyFirstModule.Stored: knowledge base not found: MyFirstModule.NoKb"},
		{"alter missing agent", `alter agent MyFirstModule.NoAgent set Description = 'x';`,
			"agent not found: MyFirstModule.NoAgent"},
		// Last: its exec half drops the stored model.
		{"model dropped earlier in the script", `drop model MyFirstModule.Support;
create agent MyFirstModule.B ( UsageType: Task, Model: MyFirstModule.Support, SelectedModel: 'gpt-4o' );`,
			"create agent MyFirstModule.B: model not found: MyFirstModule.Support"},
	}
	for _, tc := range refused {
		t.Run("refused/"+tc.name, func(t *testing.T) {
			errs := issue2293Check(t, exec, tc.src)
			if len(errs) != 1 || !strings.Contains(errs[0], tc.want) {
				t.Errorf("check reported %q, want one error containing %q", errs, tc.want)
			}
			if got := issue2293Exec(t, exec, tc.src); !strings.Contains(got, tc.want) {
				t.Errorf("exec refused with %q, want %q", got, tc.want)
			}
		})
	}
}
