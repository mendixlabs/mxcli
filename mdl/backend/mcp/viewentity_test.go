// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

func TestBuildEntityValue_ViewEntity(t *testing.T) {
	b := &Backend{}
	e := &domainmodel.Entity{
		Name:              "LocView",
		Persistable:       true,
		Source:            "DomainModels$OqlViewEntitySource",
		SourceDocumentRef: "MyFirstModule.LocView",
		Attributes: []*domainmodel.Attribute{{
			Name:  "LocName",
			Type:  &domainmodel.StringAttributeType{Length: 200},
			Value: &domainmodel.AttributeValue{ViewReference: "LocName"},
		}},
	}
	v, err := b.buildEntityValue(e)
	if err != nil {
		t.Fatalf("buildEntityValue: %v", err)
	}
	raw, _ := json.Marshal(v)
	for _, want := range []string{
		`"$Type":"DomainModels$OqlViewEntitySource"`,
		`"sourceDocument":"MyFirstModule.LocView"`,
		`"$Type":"DomainModels$OqlViewValue"`,
		`"reference":"LocName"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("view entity value missing %s: %s", want, raw)
		}
	}
}

func TestBuildEntityValue_ViewEntity_RequiresSourceRef(t *testing.T) {
	b := &Backend{}
	e := &domainmodel.Entity{
		Name:        "Bad",
		Persistable: true,
		Source:      "DomainModels$OqlViewEntitySource",
		// SourceDocumentRef intentionally empty
	}
	if _, err := b.buildEntityValue(e); err == nil {
		t.Fatal("expected error when a view entity has no source document reference")
	}
}

// View entities are not authorable over MCP: PED can only sync a view entity's
// attributes to its OQL via the LLM-backed oql_generate tool, so the deterministic
// path leaves the entity out of sync (CE6770). CreateViewEntitySourceDocument is
// the executor's first view-entity backend call and must reject before any PED
// write, leaving nothing half-created.
func TestCreateViewEntitySourceDocument_Rejected(t *testing.T) {
	b := &Backend{} // no client/server needed — it rejects before any PED call
	_, err := b.CreateViewEntitySourceDocument("m1", "MyFirstModule", "LocView", "select 1 as X", "")
	if err == nil {
		t.Fatal("expected view-entity creation to be rejected over MCP")
	}
	if !strings.Contains(err.Error(), "not authorable") {
		t.Errorf("error should explain the view entity is not authorable: %v", err)
	}
}

func TestDeleteViewEntitySourceDocumentByName_NoOp(t *testing.T) {
	b := &Backend{}
	if err := b.DeleteViewEntitySourceDocumentByName("MyFirstModule", "LocView"); err != nil {
		t.Fatalf("should be a no-op, got: %v", err)
	}
}
