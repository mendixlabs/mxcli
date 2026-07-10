// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

func enumVal(name, caption string) model.EnumerationValue {
	v := model.EnumerationValue{Name: name}
	if caption != "" {
		v.Caption = &model.Text{Translations: map[string]string{"en_US": caption}}
	}
	return v
}

func TestEnumCaption(t *testing.T) {
	if got := enumCaption(enumVal("Open", "Is Open")); got != "Is Open" {
		t.Errorf("with caption: got %q", got)
	}
	if got := enumCaption(enumVal("Open", "")); got != "Open" {
		t.Errorf("fallback to name: got %q", got)
	}
}

func TestBuildEnumContent(t *testing.T) {
	enum := &model.Enumeration{
		Name:   "OrderState",
		Values: []model.EnumerationValue{enumVal("Open", "Open"), enumVal("Paid", "Paid")},
	}
	content := buildEnumContent(enum)
	raw, _ := json.Marshal(content)
	for _, want := range []string{`"name":"OrderState"`, `"name":"Open"`, `"caption":"Paid"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("enum content missing %s: %s", want, raw)
		}
	}
}

func TestPedCreateDocument_SendsEnumConstructor(t *testing.T) {
	f := newFakePED(t, func(string, map[string]any) (string, bool) { return "SUCCESS", false })
	b := &Backend{client: f.connectClient(t), dirty: map[string]bool{}}

	enum := &model.Enumeration{Name: "OrderState", Values: []model.EnumerationValue{enumVal("Open", "Open")}}
	if err := b.pedCreateDocument("MyFirstModule", enumerationDocType, enum.Name, buildEnumContent(enum), ""); err != nil {
		t.Fatalf("pedCreateDocument: %v", err)
	}
	call, ok := f.callByName("ped_create_document")
	if !ok {
		t.Fatal("ped_create_document was not called")
	}
	raw, _ := json.Marshal(call.Args["documents"])
	for _, want := range []string{
		`"documentType":"Enumerations$Enumeration"`,
		`"moduleName":"MyFirstModule"`,
		`"documentName":"OrderState"`,
		`"name":"Open"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("create-document args missing %s: %s", want, raw)
		}
	}
	if !b.dirty["MyFirstModule"] {
		t.Error("creating a document should mark its module dirty")
	}
}

func TestListEnumerations_SessionTakesPrecedence(t *testing.T) {
	// Pure merge logic check via the session registry (no reader needed: an
	// empty local set is simulated by not calling the reader path — exercise
	// the dedup helper directly).
	a := &model.Enumeration{Name: "A", ContainerID: "m1"}
	dup := &model.Enumeration{Name: "A", ContainerID: "m1"}
	if enumKey(a) != enumKey(dup) {
		t.Fatalf("enumKey should match for same module+name: %q vs %q", enumKey(a), enumKey(dup))
	}
	other := &model.Enumeration{Name: "A", ContainerID: "m2"}
	if enumKey(a) == enumKey(other) {
		t.Fatalf("enumKey should differ across modules")
	}
}

func TestDeleteEnumeration_RequiresConcord(t *testing.T) {
	// DROP routes to Concord's delete_document; without --mcp-concord it must
	// give an actionable error (not "no delete tool ever"). Use a session module
	// + enum so the resolve path needs no local reader.
	b := &Backend{}
	mod := &model.Module{Name: "M"}
	mod.ID = model.ID("mcp~module~M")
	b.sessionModules = append(b.sessionModules, mod)
	enum := &model.Enumeration{Values: []model.EnumerationValue{}}
	enum.ID = model.ID("mcp~enum~M~E")
	enum.Name = "E"
	enum.ContainerID = mod.ID
	b.sessionEnums = append(b.sessionEnums, enum)

	err := b.DeleteEnumeration(enum.ID)
	if err == nil || !strings.Contains(err.Error(), "Concord") {
		t.Fatalf("DROP without Concord should error pointing at --mcp-concord, got: %v", err)
	}
}
