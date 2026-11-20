// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#2289 (mendixlabs/mxcli#1867), against a real project: an agent's
// context entity was written as {"documentId":"","qualifiedName":…}. Studio
// Pro stores the entity element's $ID there, as canonical GUID text —
// measured on AgentCommons' example agents, whose
// "entity":{"documentId":"262e6f3b-129e-4748-a5ec-a8fd6cb02f27",…} is the
// $ID of AgentCommons.InformationExtractor_EXAMPLE in its domain model. The
// old comment called it "an opaque agent-editor-internal ID" that Agent
// Editor filled at runtime; it is neither.

// agentEditorCountingBackend is agentEditorBackend that also reports the
// backend's write counters, so a re-run can say Unchanged.
type agentEditorCountingBackend struct{ agentEditorBackend }

func (b agentEditorCountingBackend) WriteStats() backend.WriteStats {
	if r, ok := b.FullBackend.(backend.WriteStatsReporter); ok {
		return r.WriteStats()
	}
	return backend.WriteStats{}
}

func agentEditorProjectOut(t *testing.T) (*Executor, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	exec := New(&out)
	exec.SetBackendFactory(func() backend.FullBackend {
		return agentEditorCountingBackend{agentEditorBackend{modelsdkbackend.New()}}
	})
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	return exec, &out
}

type storedDocRef struct {
	DocumentID    string `json:"documentId"`
	QualifiedName string `json:"qualifiedName"`
}

func agentRef(t *testing.T, contents, key string) storedDocRef {
	t.Helper()
	raw, ok := topLevelKey(t, contents, key)
	if !ok {
		t.Fatalf("Contents has no %s:\n%s", key, contents)
	}
	var r storedDocRef
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func entityIDString(t *testing.T, exec *Executor, qn string) string {
	t.Helper()
	e, ok := lookupEntity(exec.newExecContext(nil), qn)
	if !ok {
		t.Fatalf("no entity %s", qn)
	}
	return string(e.ID)
}

const issue2289Setup = `create persistent entity MyFirstModule.Ticket ( "Subject": String(200) );
create persistent entity MyFirstModule.Other ( "Name": String(200) );
create model MyFirstModule.Support ( Provider: MxCloudGenAI );`

const issue2289Agent = `create or modify agent MyFirstModule.Triage (
  UsageType: Task,
  Model: MyFirstModule.Support,
  SelectedModel: 'eu.anthropic.claude-sonnet-5-5',
  Entity: MyFirstModule.Ticket,
  SystemPrompt: 'be helpful'
);`

func TestIssue2289_AgentEntityDocumentID(t *testing.T) {
	exec, out := agentEditorProjectOut(t)
	run(t, exec, issue2289Setup)
	run(t, exec, issue2289Agent)

	id, contents := agentContents(t, exec, "Triage")
	want := entityIDString(t, exec, "MyFirstModule.Ticket")
	if want == "" || !strings.Contains(want, "-") {
		t.Fatalf("entity $ID %q is not GUID text", want)
	}
	if got := agentRef(t, contents, "entity"); got.DocumentID != want || got.QualifiedName != "MyFirstModule.Ticket" {
		t.Errorf("entity ref %+v, want documentId %s (the entity's $ID)", got, want)
	}

	// Control: the model reference was already written with its unit id.
	models, err := exec.backend.ListAgentEditorModels()
	if err != nil || len(models) != 1 {
		t.Fatalf("models %v %v", models, err)
	}
	if got := agentRef(t, contents, "model"); got.DocumentID != string(models[0].ID) {
		t.Errorf("model ref %+v, want documentId %s", got, models[0].ID)
	}

	// Re-running the same statement writes nothing.
	before, err := exec.backend.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	run(t, exec, issue2289Agent)
	after, _ := exec.backend.GetRawUnitBytes(id)
	if !bytes.Equal(before, after) {
		t.Errorf("re-running create or modify rewrote the agent")
	}
	if !strings.Contains(out.String(), "Unchanged") {
		t.Errorf("re-run reported %q, want Unchanged", out.String())
	}

	// alter agent … set Entity resolves the new entity's $ID too.
	run(t, exec, `alter agent MyFirstModule.Triage set Entity = MyFirstModule.Other;`)
	_, contents = agentContents(t, exec, "Triage")
	if got, want := agentRef(t, contents, "entity"), entityIDString(t, exec, "MyFirstModule.Other"); got.DocumentID != want {
		t.Errorf("alter set Entity wrote %+v, want documentId %s", got, want)
	}
}

func TestIssue2289_AgentEntityNotFound(t *testing.T) {
	exec, _ := agentEditorProjectOut(t)
	run(t, exec, issue2289Setup)
	prog, errs := visitor.Build(`create agent MyFirstModule.Triage (UsageType: Task, Entity: MyFirstModule.Missing);`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	err := exec.Execute(prog.Statements[0])
	if err == nil || !strings.Contains(err.Error(), "entity not found: MyFirstModule.Missing") {
		t.Fatalf("got %v, want entity not found", err)
	}
	// A System entity is not in the project, but it is not refused: its $ID
	// is the measured one (ako/mxcli#2292).
	run(t, exec, `create agent MyFirstModule.ForUsers (UsageType: Task, Entity: System.User);`)
	_, contents := agentContents(t, exec, "ForUsers")
	if got := agentRef(t, contents, "entity"); got.QualifiedName != "System.User" || got.DocumentID != "0e439bf8-87c0-406f-ad3f-630ec563f770" {
		t.Errorf("System entity ref %+v, want System.User with its measured $ID", got)
	}

	run(t, exec, `create agent MyFirstModule.Triage (UsageType: Task, Entity: MyFirstModule.Ticket);`)
	prog, _ = visitor.Build(`alter agent MyFirstModule.Triage set Entity = MyFirstModule.Missing;`)
	if err := exec.Execute(prog.Statements[0]); err == nil || !strings.Contains(err.Error(), "entity not found") {
		t.Fatalf("alter: got %v, want entity not found", err)
	}
}
