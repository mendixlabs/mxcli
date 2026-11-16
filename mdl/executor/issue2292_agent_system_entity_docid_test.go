// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// ako/mxcli#2292: an agent whose context entity is a System entity was written
// as {"documentId":"","qualifiedName":"System.User"} — the System module is not
// in the project, so mxcli had no $ID to resolve. Studio Pro fills one: a
// Studio Pro 11.16.1 save of an AgentCommons agent stores
// "entity":{"documentId":"83d81a7b-4a84-416e-a64f-0ffa981c8408","qualifiedName":"System.Language"}.
// That is System.Language's element $ID, fixed in mxbuild and the same in
// 10.24.4 and 11.16 builds (meta.SystemEntityID).

const issue2292Agent = `create or modify agent MyFirstModule.ForUsers (
  UsageType: Task,
  Entity: System.User,
  SystemPrompt: 'be helpful'
);`

func TestIssue2292_AgentSystemEntityDocumentID(t *testing.T) {
	exec, out := agentEditorProjectOut(t)
	run(t, exec, issue2289Setup)

	// The Studio Pro-authored reference.
	run(t, exec, `create agent MyFirstModule.Translate (UsageType: Task, Entity: System.Language);`)
	_, contents := agentContents(t, exec, "Translate")
	if got := agentRef(t, contents, "entity"); got.DocumentID != "83d81a7b-4a84-416e-a64f-0ffa981c8408" || got.QualifiedName != "System.Language" {
		t.Errorf("System.Language ref %+v, want Studio Pro's documentId 83d81a7b-4a84-416e-a64f-0ffa981c8408", got)
	}

	run(t, exec, issue2292Agent)
	id, contents := agentContents(t, exec, "ForUsers")
	if got := agentRef(t, contents, "entity"); got.DocumentID != "0e439bf8-87c0-406f-ad3f-630ec563f770" || got.QualifiedName != "System.User" {
		t.Errorf("System.User ref %+v, want documentId 0e439bf8-87c0-406f-ad3f-630ec563f770", got)
	}

	// Re-running the same statement writes nothing.
	before, err := exec.backend.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	run(t, exec, issue2292Agent)
	after, _ := exec.backend.GetRawUnitBytes(id)
	if !bytes.Equal(before, after) {
		t.Errorf("re-running create or modify rewrote the agent")
	}
	if !strings.Contains(out.String(), "Unchanged") {
		t.Errorf("re-run reported %q, want Unchanged", out.String())
	}

	// Control: a project entity still gets its own $ID, not a System one.
	run(t, exec, `alter agent MyFirstModule.ForUsers set Entity = MyFirstModule.Ticket;`)
	_, contents = agentContents(t, exec, "ForUsers")
	if got, want := agentRef(t, contents, "entity"), entityIDString(t, exec, "MyFirstModule.Ticket"); got.DocumentID != want {
		t.Errorf("project entity ref %+v, want its own $ID %s", got, want)
	}

	// alter … set Entity back to a System entity resolves its id too.
	run(t, exec, `alter agent MyFirstModule.ForUsers set Entity = System.Language;`)
	_, contents = agentContents(t, exec, "ForUsers")
	if got := agentRef(t, contents, "entity"); got.DocumentID != "83d81a7b-4a84-416e-a64f-0ffa981c8408" {
		t.Errorf("alter set Entity = System.Language wrote %+v", got)
	}

	// A System entity whose $ID mxbuild mints at load has no fixed id to write:
	// it stays by name alone rather than getting an invented one.
	run(t, exec, `create agent MyFirstModule.Thumbs (UsageType: Task, Entity: System.Thumbnail);`)
	_, contents = agentContents(t, exec, "Thumbs")
	if got := agentRef(t, contents, "entity"); got.DocumentID != "" || got.QualifiedName != "System.Thumbnail" {
		t.Errorf("System.Thumbnail ref %+v, want by name with no documentId", got)
	}
}
