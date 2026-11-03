// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The reporter's shape (#1914, upstream mendixlabs/mxcli#1721): a command DTO
// with a sub-DTO, both entity types, so Studio Pro links them with an
// association and offers it under "Associations to include".
const includedAssocMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Probe" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="QuantityParameterType">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.Guid" Nullable="false"/>
      </EntityType>
      <EntityType Name="CreateBufferCommand">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.Guid" Nullable="false"/>
        <NavigationProperty Name="Quantity" Type="Probe.QuantityParameterType" Nullable="true"/>
      </EntityType>
      <EntityType Name="OtherCommand">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.Guid" Nullable="false"/>
      </EntityType>
      <Action Name="CreateBuffer" IsBound="false">
        <Parameter Name="command"    Type="Probe.CreateBufferCommand" Nullable="false"/>
        <Parameter Name="additional" Type="Edm.String" Nullable="true"/>
      </Action>
      <Action Name="CreateOther" IsBound="false">
        <Parameter Name="command" Type="Probe.OtherCommand" Nullable="false"/>
      </Action>
      <EntityContainer Name="Container">
        <ActionImport Name="CreateBuffer" Action="Probe.CreateBuffer"/>
        <ActionImport Name="CreateOther"  Action="Probe.CreateOther"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// storedIncludedAssociations is the tree Studio Pro stored on ako/TestApp's
// Odata.IncludeAssociationsMF (11.16.0): Quantity ticked, Unit ticked under it.
func storedIncludedAssociations() []*microflows.IncludedAssociation {
	unit := &microflows.IncludedAssociation{Association: "Odata.Unit", IsParent: true}
	unit.ID = "ia-unit"
	qty := &microflows.IncludedAssociation{
		Association: "Odata.Quantity", IsParent: true,
		IncludedAssociations: []*microflows.IncludedAssociation{unit},
	}
	qty.ID = "ia-qty"
	return []*microflows.IncludedAssociation{qty}
}

// storedCreateBufferCall is the call as the reader returns it from Studio Pro's
// document: the command mapping holds the tree, `additional` holds none.
func storedCreateBufferCall() *microflows.ActionActivity {
	return &microflows.ActionActivity{Action: &microflows.CallExternalAction{
		ConsumedODataService: "Odata.Bug1914",
		Name:                 "CreateBuffer",
		ParameterMappings: []*microflows.ExternalActionParameterMapping{
			{ParameterName: "command", Argument: "$Cmd", ParameterEntity: "Odata.CreateBufferCommand",
				IncludedAssociations: storedIncludedAssociations()},
			{ParameterName: "additional", Argument: "empty", CanBeEmpty: true},
		},
	}}
}

// rewriteExternalActionFlow runs CREATE OR MODIFY of Odata.ACT_Buffer with body
// against a stored microflow holding stored (nil: there is no stored
// microflow), and returns the CallExternalActions the rewrite wrote.
func rewriteExternalActionFlow(t *testing.T, stored *microflows.ActionActivity, body string) []*microflows.CallExternalAction {
	t.Helper()
	mod := mkModule("Odata")
	h := mkHierarchy(mod)
	var existing []*microflows.Microflow
	if stored != nil {
		mf := mkMicroflow(mod.ID, "ACT_Buffer")
		withContainer(h, mf.ContainerID, mod.ID)
		mf.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{stored}}
		existing = append(existing, mf)
	}
	entity := func(name string) *domainmodel.Entity {
		return &domainmodel.Entity{
			BaseElement:       model.BaseElement{ID: nextID("ent")},
			Name:              name,
			RemoteServiceName: "Odata.Bug1914",
			RemoteEntityName:  name,
		}
	}
	var written *microflows.Microflow
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return existing, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				ContainerID: mod.ID,
				Entities:    []*domainmodel.Entity{entity("CreateBufferCommand"), entity("QuantityParameterType"), entity("OtherCommand")},
			}}, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{{
				BaseElement: model.BaseElement{ID: "svc-1914"},
				ContainerID: mod.ID,
				Name:        "Bug1914",
				Metadata:    includedAssocMetadata,
			}}, nil
		},
		ListConsumedRestServicesFunc: func() ([]*model.ConsumedRestService, error) { return nil, nil },
		CreateMicroflowFunc:          func(mf *microflows.Microflow) error { written = mf; return nil },
		UpdateMicroflowFunc:          func(mf *microflows.Microflow) error { written = mf; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	prog, errs := visitor.Build("create or modify microflow Odata.ACT_Buffer ($Cmd: Odata.CreateBufferCommand, $Other: Odata.OtherCommand)\nbegin\n" +
		body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if err := execCreateMicroflow(ctx, prog.Statements[0].(*ast.CreateMicroflowStmt)); err != nil {
		t.Fatalf("create or modify: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	var calls []*microflows.CallExternalAction
	for _, obj := range written.ObjectCollection.Objects {
		if act, ok := obj.(*microflows.ActionActivity); ok {
			if c, ok := act.Action.(*microflows.CallExternalAction); ok {
				calls = append(calls, c)
			}
		}
	}
	return calls
}

func mappingNamed(c *microflows.CallExternalAction, name string) *microflows.ExternalActionParameterMapping {
	for _, pm := range c.ParameterMappings {
		if pm.ParameterName == name {
			return pm
		}
	}
	return nil
}

// #1914: MDL has no syntax for "Associations to include", so a CREATE OR MODIFY
// that restates the call wrote it with none, and the OData request silently
// lost the associated objects at runtime. The stored tree is carried onto the
// rewritten call, $IDs and nesting intact.
func TestCreateOrModify_CarriesIncludedAssociations(t *testing.T) {
	calls := rewriteExternalActionFlow(t, storedCreateBufferCall(),
		"call external action Odata.Bug1914.CreateBuffer(command = $Cmd, additional = empty);")
	if len(calls) != 1 {
		t.Fatalf("wrote %d calls, want 1", len(calls))
	}
	cmd := mappingNamed(calls[0], "command")
	if cmd == nil {
		t.Fatal("no command mapping")
	}
	got := cmd.IncludedAssociations
	if len(got) != 1 || got[0].Association != "Odata.Quantity" || !got[0].IsParent || got[0].ID != "ia-qty" ||
		len(got[0].IncludedAssociations) != 1 || got[0].IncludedAssociations[0].Association != "Odata.Unit" ||
		got[0].IncludedAssociations[0].ID != "ia-unit" {
		t.Errorf("command mapping's associations to include were not carried: %+v", got)
	}
	if add := mappingNamed(calls[0], "additional"); add == nil || len(add.IncludedAssociations) != 0 {
		t.Errorf("additional mapping gained associations: %+v", add)
	}
}

// Controls: the carry keeps what the statement still means, and nothing else.
func TestCreateOrModify_IncludedAssociationsCarryControls(t *testing.T) {
	t.Run("parameter now binds another entity", func(t *testing.T) {
		stored := storedCreateBufferCall()
		stored.Action.(*microflows.CallExternalAction).Name = "CreateOther"
		stored.Action.(*microflows.CallExternalAction).ParameterMappings[0].ParameterEntity = "Odata.CreateBufferCommand"
		calls := rewriteExternalActionFlow(t, stored,
			"call external action Odata.Bug1914.CreateOther(command = $Other);")
		if cmd := mappingNamed(calls[0], "command"); cmd == nil || len(cmd.IncludedAssociations) != 0 {
			t.Errorf("associations of CreateBufferCommand were carried onto an OtherCommand argument: %+v", cmd)
		}
	})
	t.Run("call to another action", func(t *testing.T) {
		calls := rewriteExternalActionFlow(t, storedCreateBufferCall(),
			"call external action Odata.Bug1914.CreateOther(command = $Other);")
		if cmd := mappingNamed(calls[0], "command"); cmd == nil || len(cmd.IncludedAssociations) != 0 {
			t.Errorf("a different action inherited CreateBuffer's associations: %+v", cmd)
		}
	})
	t.Run("new microflow", func(t *testing.T) {
		calls := rewriteExternalActionFlow(t, nil,
			"call external action Odata.Bug1914.CreateBuffer(command = $Cmd, additional = empty);")
		if cmd := mappingNamed(calls[0], "command"); cmd == nil || len(cmd.IncludedAssociations) != 0 {
			t.Errorf("a new call has associations nobody ticked: %+v", cmd)
		}
	})
}

// Two stored calls to one action: the carry cannot pair them by position, so it
// carries a tree they share and refuses when they differ.
func TestCreateOrModify_IncludedAssociationsTwoCalls(t *testing.T) {
	twoCalls := func(second []*microflows.IncludedAssociation) *microflows.LoopedActivity {
		b := storedCreateBufferCall()
		b.Action.(*microflows.CallExternalAction).ParameterMappings[0].IncludedAssociations = second
		return &microflows.LoopedActivity{ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{storedCreateBufferCall(), b},
		}}
	}
	run := func(t *testing.T, stored *microflows.LoopedActivity) ([]*microflows.CallExternalAction, error) {
		t.Helper()
		var calls []*microflows.CallExternalAction
		rebuilt := &microflows.MicroflowObjectCollection{}
		for range 2 {
			a := storedCreateBufferCall()
			a.Action.(*microflows.CallExternalAction).ParameterMappings[0].IncludedAssociations = nil
			rebuilt.Objects = append(rebuilt.Objects, a)
			calls = append(calls, a.Action.(*microflows.CallExternalAction))
		}
		err := carryIncludedAssociations(rebuilt,
			&microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{stored}}, "Odata.ACT_Buffer")
		return calls, err
	}

	t.Run("same tree", func(t *testing.T) {
		calls, err := run(t, twoCalls(storedIncludedAssociations()))
		if err != nil {
			t.Fatal(err)
		}
		a := calls[0].ParameterMappings[0].IncludedAssociations
		b := calls[1].ParameterMappings[0].IncludedAssociations
		if len(a) != 1 || len(b) != 1 || a[0].Association != "Odata.Quantity" || b[0].Association != "Odata.Quantity" {
			t.Fatalf("both calls should keep the tree: %+v / %+v", a, b)
		}
		if a[0].ID != "ia-qty" || b[0].ID != "" || b[0].IncludedAssociations[0].ID != "" {
			t.Errorf("the second copy must not reuse the stored $IDs: %q, %q", a[0].ID, b[0].ID)
		}
	})
	t.Run("different trees", func(t *testing.T) {
		qtyOnly := storedIncludedAssociations()
		qtyOnly[0].IncludedAssociations = nil
		if _, err := run(t, twoCalls(qtyOnly)); err == nil || !strings.Contains(err.Error(), "Odata.Bug1914.CreateBuffer(command)") {
			t.Errorf("want a refusal naming the call, got %v", err)
		}
	})
}
