// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2266 (point 1), against a real project: Agent Editor 2.7 stores
// an agent's test sets in its Contents JSON, as a top-level `testSets` array
// mxcli does not model. Every statement that rewrites an existing agent builds
// Contents from the typed fields — `create or modify`, `create or replace`
// (the same handler) and `alter agent` — so a rewrite that did not carry the
// stored keys deleted the user's test sets every time the script ran again.
// `futureKey` stands in for whatever a later Agent Editor adds next: the carry
// is for any key mxcli does not model, not for testSets by name.

const agentTestSets = `[{"uuid":"5d1c0a52-7f0e-4c43-9e0a-2c4d8f6b9a01","name":"Smoke",` +
	`"testCases":[{"uuid":"c3b2a1d0-1111-4222-8333-944455556666","name":"Greets","input":"Hello <b>&</b>","expected":"Hi"}],` +
	`"evaluators":[{"documentId":"9f8e7d6c-5b4a-4321-8fed-cba987654321"}]}]`

const agentFutureKey = `{"kept":true,"n":3}`

// agentEditorBackend is the real backend on the fixture project, reporting
// the Mendix version Agent Editor 2.7 runs on: the fixture is 11.6, below the
// 11.9 gate every agent statement checks, and nothing else here depends on it.
type agentEditorBackend struct{ backend.FullBackend }

func (agentEditorBackend) ProjectVersion() *types.ProjectVersion {
	return &types.ProjectVersion{ProductVersion: "11.16.0", MajorVersion: 11, MinorVersion: 16}
}

func agentEditorProject(t *testing.T) *Executor {
	t.Helper()
	var out bytes.Buffer
	exec := New(&out)
	exec.SetBackendFactory(func() backend.FullBackend { return agentEditorBackend{modelsdkbackend.New()} })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	return exec
}

// agentContents returns the stored agent's unit ID and its Contents JSON.
func agentContents(t *testing.T, exec *Executor, name string) (model.ID, string) {
	t.Helper()
	agents, err := exec.backend.ListAgentEditorAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		if a.Name != name {
			continue
		}
		raw, err := exec.backend.GetRawUnitBytes(a.ID)
		if err != nil {
			t.Fatal(err)
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		for _, e := range doc {
			if e.Key == "Contents" {
				s, _ := e.Value.(string)
				return a.ID, s
			}
		}
		t.Fatalf("agent %s has no Contents", name)
	}
	t.Fatalf("no agent %s", name)
	return "", ""
}

// injectAgentKeys does what Agent Editor 2.7 does when the user adds a test
// set: it writes keys into the stored Contents that mxcli does not model.
func injectAgentKeys(t *testing.T, exec *Executor, id model.ID, contents string) {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(contents), &obj); err != nil {
		t.Fatal(err)
	}
	obj["testSets"] = json.RawMessage(agentTestSets)
	obj["futureKey"] = json.RawMessage(agentFutureKey)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obj); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.backend.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for i := range doc {
		if doc[i].Key == "Contents" {
			doc[i].Value = string(bytes.TrimRight(buf.Bytes(), "\n"))
		}
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.backend.UpdateRawUnit(string(id), out); err != nil {
		t.Fatal(err)
	}
}

func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		t.Fatalf("%v in %s", err, raw)
	}
	return b.String()
}

func topLevelKey(t *testing.T, contents, key string) (json.RawMessage, bool) {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(contents), &obj); err != nil {
		t.Fatalf("%v in %s", err, contents)
	}
	v, ok := obj[key]
	return v, ok
}

func TestIssue2266_AgentRewriteKeepsTestSets(t *testing.T) {
	rewrites := []struct {
		name, mdl string
		check     func(t *testing.T, contents string)
	}{
		{
			name: "create or modify",
			mdl:  `create or modify agent MyFirstModule.Triage (UsageType: Task, SystemPrompt: 'changed by the script');`,
			check: func(t *testing.T, contents string) {
				if v, _ := topLevelKey(t, contents, "systemPrompt"); compactJSON(t, v) != `"changed by the script"` {
					t.Errorf("systemPrompt %s, want the statement's", v)
				}
			},
		},
		{
			name: "create or replace",
			mdl:  `create or replace agent MyFirstModule.Triage (UsageType: Task, SystemPrompt: 'replaced');`,
			check: func(t *testing.T, contents string) {
				if v, _ := topLevelKey(t, contents, "systemPrompt"); compactJSON(t, v) != `"replaced"` {
					t.Errorf("systemPrompt %s, want the statement's", v)
				}
			},
		},
		{
			name: "alter agent",
			mdl:  `alter agent MyFirstModule.Triage set Temperature = 0.3;`,
			check: func(t *testing.T, contents string) {
				if v, _ := topLevelKey(t, contents, "temperature"); compactJSON(t, v) != `0.3` {
					t.Errorf("temperature %s, want 0.3", v)
				}
			},
		},
	}
	for _, rw := range rewrites {
		t.Run(rw.name, func(t *testing.T) {
			exec := agentEditorProject(t)
			run(t, exec, `create agent MyFirstModule.Triage (UsageType: Task, SystemPrompt: 'be helpful');`)
			id, fresh := agentContents(t, exec, "Triage")

			// Control: a fresh agent gets no test sets mxcli invented — the
			// encoder does not write the key, and must not start to.
			if v, ok := topLevelKey(t, fresh, "testSets"); ok {
				t.Fatalf("a fresh create wrote testSets %s", v)
			}

			injectAgentKeys(t, exec, id, fresh)
			run(t, exec, rw.mdl)
			_, got := agentContents(t, exec, "Triage")
			rw.check(t, got)

			for key, want := range map[string]string{"testSets": agentTestSets, "futureKey": agentFutureKey} {
				v, ok := topLevelKey(t, got, key)
				if !ok {
					t.Errorf("%s deleted %s from the stored Contents:\n%s", rw.name, key, got)
					continue
				}
				if compactJSON(t, v) != want {
					t.Errorf("%s changed %s:\n got %s\nwant %s", rw.name, key, compactJSON(t, v), want)
				}
			}
		})
	}
}
