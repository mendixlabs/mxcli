// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// Contents saved by Studio Pro 11.16, free text shortened. GraphExplorer was
// created in Agent Editor (mx-test-projects/ICXGraphPilot); WorkInstructionAgent
// and KB_WorkInstructions come from mx-test-projects/FactoryManagement.
const (
	graphExplorerFixture        = "testdata/agenteditor/ICXGraphPilot.GraphExplorer.11_16.json"
	workInstructionAgentFixture = "testdata/agenteditor/FactoryManagement.WorkInstructionAgent.11_16.json"
	workInstructionsKBFixture   = "testdata/agenteditor/FactoryManagement.KB_WorkInstructions.11_16.json"
)

func readContentsFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(b), "\n")
}

func decodeMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("%v in %s", err, s)
	}
	return m
}

// typedAgent is what the reader makes of stored Contents: only the keys mxcli
// models.
func typedAgent(t *testing.T, contents string) *agenteditor.Agent {
	t.Helper()
	a := &agenteditor.Agent{}
	if err := json.Unmarshal([]byte(contents), a); err != nil {
		t.Fatal(err)
	}
	return a
}

// An `alter agent … set Temperature` rewrote Contents from the typed fields,
// and the streaming switches, the selected model and each tool's approval and
// display title were gone. mxcli models all of them now; a rewrite still keeps
// any key it does not model ("futureKey" stands in for what a later Agent
// Editor adds), at its stored position.
func TestCarryStoredContents_AgentKeepsWhatItDoesNotModel(t *testing.T) {
	stored := strings.Replace(readContentsFixture(t, graphExplorerFixture),
		`"enableBackendStreaming": true,`, `"enableBackendStreaming": true,
  "futureKey": "kept",`, 1)
	if !strings.Contains(stored, "futureKey") {
		t.Fatal("fixture changed; futureKey not injected")
	}
	a := typedAgent(t, stored)
	temp := 0.3
	a.Temperature = &temp

	rebuilt, err := encodeAgentContents(a)
	if err != nil {
		t.Fatal(err)
	}
	// Control: the rebuild alone loses it, so the carry below is what keeps it.
	if m := decodeMap(t, rebuilt); m["futureKey"] != nil {
		t.Fatalf("the encoder writes futureKey itself; this test no longer covers the carry: %s", rebuilt)
	}

	got, err := carryStoredContents(stored, rebuilt, agentContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	want := decodeMap(t, stored)
	want["temperature"] = 0.3
	if g := decodeMap(t, got); !reflect.DeepEqual(g, want) {
		t.Errorf("carried contents differ from stored + temperature:\n got %s\nwant %v", got, want)
	}
	// Studio Pro's key order is kept, and the new key goes last.
	if order := decodeObjectKeys(t, got); !reflect.DeepEqual(order, append(decodeObjectKeys(t, stored), "temperature")) {
		t.Errorf("key order %v", order)
	}
}

// ako/mxcli#2031: Agent Editor 2.2+ refuses to import an agent without
// selectedModel ("Model with name  is referenced but could not be found"), and
// mxcli could not write one. It is modelled now: the encoder writes it where
// Studio Pro does (after model, before entity), and clearing it removes it.
func TestEncodeAgentContents_SelectedModel(t *testing.T) {
	stored := readContentsFixture(t, graphExplorerFixture)
	a := typedAgent(t, stored)
	if a.SelectedModel == nil || a.SelectedModel.ID != "eu.anthropic.claude-sonnet-5" || a.SelectedModel.Name != "Anthropic Claude Sonnet 5" {
		t.Fatalf("selectedModel not read: %+v", a.SelectedModel)
	}
	rebuilt, err := encodeAgentContents(a)
	if err != nil {
		t.Fatal(err)
	}
	sm, _ := decodeMap(t, rebuilt)["selectedModel"].(map[string]any)
	if sm["id"] != "eu.anthropic.claude-sonnet-5" || sm["name"] != "Anthropic Claude Sonnet 5" {
		t.Errorf("selectedModel not written: %s", rebuilt)
	}
	keys := decodeObjectKeys(t, rebuilt)
	pos := map[string]int{}
	for i, k := range keys {
		pos[k] = i
	}
	if !(pos["model"] < pos["selectedModel"] && pos["selectedModel"] < pos["entity"]) {
		t.Errorf("selectedModel not between model and entity: %v", keys)
	}

	a.SelectedModel = nil
	rebuilt, _ = encodeAgentContents(a)
	got, err := carryStoredContents(stored, rebuilt, agentContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decodeMap(t, got)["selectedModel"]; ok {
		t.Errorf("a cleared selectedModel came back from the stored contents: %s", got)
	}
}

// A key on a knowledge base tool that mxcli does not model is kept, paired
// with its stored entry by id. ("futureKey" stands in for what a later Agent
// Editor adds: every key the fixture holds is modelled now.)
func TestCarryStoredContents_KnowledgeBaseToolByID(t *testing.T) {
	stored := strings.Replace(readContentsFixture(t, workInstructionAgentFixture),
		`"minSimilarity":0.2`, `"minSimilarity":0.2,"futureKey":"kept"`, 1)
	if !strings.Contains(stored, "futureKey") {
		t.Fatal("fixture no longer holds minSimilarity 0.2")
	}
	a := typedAgent(t, stored)
	a.KBTools[0].MaxResults = 7
	rebuilt, err := encodeAgentContents(a)
	if err != nil {
		t.Fatal(err)
	}
	got, err := carryStoredContents(stored, rebuilt, agentContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	kb := decodeMap(t, got)["knowledgebaseTools"].([]any)[0].(map[string]any)
	if kb["futureKey"] != "kept" || kb["minSimilarity"] != 0.2 || kb["maxResults"] != float64(7) {
		t.Errorf("knowledge base tool %v, want futureKey and minSimilarity kept and maxResults 7 written", kb)
	}

	// Control: a tool the statement adds pairs with nothing and gets nothing.
	a.KBTools[0].ID = "new-tool"
	rebuilt, _ = encodeAgentContents(a)
	got, _ = carryStoredContents(stored, rebuilt, agentContentsSpec)
	if kb := decodeMap(t, got)["knowledgebaseTools"].([]any)[0].(map[string]any); kb["futureKey"] != nil {
		t.Errorf("an unpaired tool got the stored tool's futureKey: %v", kb)
	}
}

// A modelled optional key the statement cleared stays cleared: carrying it
// back would make the value impossible to remove.
func TestCarryStoredContents_ClearedModelledKeyStaysOut(t *testing.T) {
	stored := readContentsFixture(t, graphExplorerFixture)
	a := typedAgent(t, stored)
	a.Model = nil
	rebuilt, _ := encodeAgentContents(a)
	got, err := carryStoredContents(stored, rebuilt, agentContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decodeMap(t, got)["model"]; ok {
		t.Errorf("a cleared model came back from the stored contents: %s", got)
	}
}

// `alter knowledge base` wrote the Portal knowledge base name as "".
func TestEncodeKnowledgeBaseContents_KeepsName(t *testing.T) {
	stored := readContentsFixture(t, workInstructionsKBFixture)
	k := &agenteditor.KnowledgeBase{InnerName: "WorkInstructions", Provider: "MxCloudGenAI", ModelName: "cohere.embed-english-v3"}
	rebuilt, err := encodeKnowledgeBaseContents(k)
	if err != nil {
		t.Fatal(err)
	}
	got, err := carryStoredContents(stored, rebuilt, knowledgeBaseContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	if n := decodeMap(t, got)["name"]; n != "WorkInstructions" {
		t.Errorf("name = %v, want WorkInstructions", n)
	}
}

// A restatement that changes nothing writes the stored bytes back: Agent
// Editor 2.6 saves Contents indented (GraphExplorer), older documents compact
// (WorkInstructionAgent). Rewriting one layout in the other made every
// restatement of an unchanged agent a write.
func TestCarryStoredContents_UnchangedIsByteIdentical(t *testing.T) {
	for _, path := range []string{graphExplorerFixture, workInstructionAgentFixture} {
		stored := readContentsFixture(t, path)
		rebuilt, err := encodeAgentContents(typedAgent(t, stored))
		if err != nil {
			t.Fatal(err)
		}
		got, err := carryStoredContents(stored, rebuilt, agentContentsSpec)
		if err != nil {
			t.Fatal(err)
		}
		if got != stored {
			t.Errorf("%s: an unchanged agent is rewritten:\n got %s\nwant %s", path, got, stored)
		}
	}
	// Control: the indented fixture really is indented, the other compact.
	if !strings.HasPrefix(readContentsFixture(t, graphExplorerFixture), "{\n  ") ||
		strings.Contains(readContentsFixture(t, workInstructionAgentFixture), "\n") {
		t.Fatal("the fixtures no longer cover both layouts")
	}
}

func decodeObjectKeys(t *testing.T, s string) []string {
	t.Helper()
	obj, ok := decodeObject(json.RawMessage(s))
	if !ok {
		t.Fatalf("not an object: %s", s)
	}
	return obj.keys
}
