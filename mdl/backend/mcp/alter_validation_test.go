// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// contactAlterPED scripts a live entity "Contact" (index 0) with two attributes,
// FirstName and Email, so UpdateEntity's entityIndex + liveAttributeNames reads
// resolve. Everything else succeeds.
func contactAlterPED(t *testing.T) *fakePED {
	return newFakePED(t, func(name string, args map[string]any) (string, bool) {
		switch name {
		case "ped_read_document":
			p := ""
			if ps, ok := args["paths"].([]any); ok && len(ps) > 0 {
				p, _ = ps[0].(string)
			}
			switch p {
			case "/entities":
				return `{"results":[{"result":[{"name":"Contact"}]}]}`, false
			case "/entities/0/attributes":
				return `{"results":[{"result":[{"$QualifiedName":"M.Contact.FirstName"},{"$QualifiedName":"M.Contact.Email"}]}]}`, false
			}
			return `{"results":[{"result":[]}]}`, false
		case "ped_check_errors":
			return "No errors found.", false
		default: // ped_get_schema, ped_update_document
			return "SUCCESS", false
		}
	})
}

func contactWithRules(rules ...*domainmodel.ValidationRule) *domainmodel.Entity {
	fn := attr("FirstName", &domainmodel.StringAttributeType{Length: 100})
	fn.ID = "a1"
	em := attr("Email", &domainmodel.StringAttributeType{Length: 200})
	em.ID = "a2"
	st := attr("Status", &domainmodel.StringAttributeType{Length: 50})
	st.ID = "a3"
	e := newPersistentEntity("Contact", fn, em, st)
	e.ValidationRules = rules
	return e
}

// TestUpdateEntity_AddAttr_PreexistingRulesPass guards Issue 6: ALTER ENTITY ADD
// ATTRIBUTE on an entity that already has NOT NULL/UNIQUE rules must succeed —
// pre-existing rules (on already-live attributes) are untouched, not a blocker.
func TestUpdateEntity_AddAttr_PreexistingRulesPass(t *testing.T) {
	f := contactAlterPED(t)
	b := &Backend{client: f.connectClient(t), schemaFetched: map[string]bool{}, dirty: map[string]bool{}}

	// Rules reference the LIVE attributes FirstName (a1) and Email (a2); the added
	// attribute Status (a3) has none.
	e := contactWithRules(
		&domainmodel.ValidationRule{AttributeID: "a1", Type: "Required"},
		&domainmodel.ValidationRule{AttributeID: "a2", Type: "Unique"},
	)

	if err := b.UpdateEntity(model.ID("mcp~dm~M"), e); err != nil {
		t.Fatalf("ALTER ADD ATTRIBUTE on a constrained entity must succeed, got: %v", err)
	}
	call, ok := f.callByName("ped_update_document")
	if !ok {
		t.Fatal("expected an attribute-add write")
	}
	raw, _ := json.Marshal(call.Args["operations"])
	if !strings.Contains(string(raw), `"path":"/entities/0/attributes"`) || !strings.Contains(string(raw), `"Status"`) {
		t.Errorf("expected add of Status, got: %s", raw)
	}
}

// A NEW attribute carrying a validation rule still can't be authored on ALTER
// (the rule would have to be created, which the entity slice doesn't do) — reject
// it explicitly rather than silently dropping the constraint.
func TestUpdateEntity_AddConstrainedAttr_Rejected(t *testing.T) {
	f := contactAlterPED(t)
	b := &Backend{client: f.connectClient(t), schemaFetched: map[string]bool{}, dirty: map[string]bool{}}

	e := contactWithRules(&domainmodel.ValidationRule{AttributeID: "a3", Type: "Required"}) // a3 = the new Status attr

	err := b.UpdateEntity(model.ID("mcp~dm~M"), e)
	if err == nil || !strings.Contains(err.Error(), "new attribute") {
		t.Fatalf("adding a constrained NEW attribute via ALTER should be rejected, got: %v", err)
	}
	if _, sent := f.callByName("ped_update_document"); sent {
		t.Error("a rejected ALTER must not write")
	}
}
