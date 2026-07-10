// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// CreateNanoflow inserts a new Microflows$Nanoflow document unit. A nanoflow
// shares the microflow flow model (parameters + object collection + sequence
// flows), so it reuses the microflow object/flow converters; only the top-level
// field set differs (no ExportLevel / concurrency / URL fields).
func (b *Backend) CreateNanoflow(nf *microflows.Nanoflow) error {
	if nf == nil {
		return fmt.Errorf("CreateNanoflow: nil nanoflow")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateNanoflow: not connected for writing")
	}
	if nf.ID == "" {
		nf.ID = model.ID(mmpr.GenerateID())
	}
	g := nanoflowToGen(nf, b.majorVersion())
	g.SetID(element.ID(nf.ID))
	assignNanoflowIDs(g)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateNanoflow: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(nf.ID), string(nf.ContainerID), "Documents", "Microflows$Nanoflow", contents); err != nil {
		return fmt.Errorf("CreateNanoflow: insert: %w", err)
	}
	return nil
}

// UpdateNanoflow rebuilds a nanoflow document (the CREATE OR REPLACE path).
func (b *Backend) UpdateNanoflow(nf *microflows.Nanoflow) error {
	if nf == nil {
		return fmt.Errorf("UpdateNanoflow: nil nanoflow")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateNanoflow: not connected for writing")
	}
	g := nanoflowToGen(nf, b.majorVersion())
	g.SetID(element.ID(nf.ID))
	assignNanoflowIDs(g)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdateNanoflow: encode: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(nf.ID), contents); err != nil {
		return fmt.Errorf("UpdateNanoflow: update: %w", err)
	}
	return nil
}

// DeleteNanoflow removes the nanoflow unit.
func (b *Backend) DeleteNanoflow(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteNanoflow: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// nanoflowToGen builds a gen Nanoflow from the model, mirroring the legacy
// serializeNanoflow field set (AllowedModuleRoles, Documentation, Excluded,
// Flows, MarkAsUsed, MicroflowReturnType [only when non-void], Name,
// ObjectCollection with parameters merged in first).
func nanoflowToGen(nf *microflows.Nanoflow, major int) *genMf.Nanoflow {
	out := genMf.NewNanoflow()
	out.SetName(nf.Name)
	out.SetDocumentation(nf.Documentation)
	out.SetExcluded(nf.Excluded)
	out.SetMarkAsUsed(nf.MarkAsUsed)
	out.SetAllowedModuleRolesQualifiedNames(moduleRoleNames(nf.AllowedModuleRoles))
	if nf.ReturnType != nil {
		out.SetMicroflowReturnType(microflowDataTypeToGen(nf.ReturnType))
	}

	oc := genMf.NewMicroflowObjectCollection()
	for i, p := range nf.Parameters {
		oc.AddObjects(microflowParameterToGen(p, i, major))
	}
	if nf.ObjectCollection != nil {
		for _, obj := range nf.ObjectCollection.Objects {
			if g := microflowObjectToGen(obj); g != nil {
				oc.AddObjects(g)
			}
		}
	}
	out.SetObjectCollection(oc)

	if nf.ObjectCollection != nil {
		for _, f := range nf.ObjectCollection.Flows {
			out.AddFlows(sequenceFlowToGen(f, major))
		}
	}
	return out
}

// assignNanoflowIDs assigns fresh IDs to the nanoflow's return type, object
// collection, and sequence flows (parallels assignMicroflowIDs minus the
// microflow-only concurrency-error message).
func assignNanoflowIDs(n *genMf.Nanoflow) {
	if rt := n.MicroflowReturnType(); rt != nil {
		assignID(rt)
	}
	if oc, ok := n.ObjectCollection().(*genMf.MicroflowObjectCollection); ok {
		assignObjectCollectionIDs(oc)
	}
	for _, el := range n.FlowsItems() {
		assignID(el)
		if sf, ok := el.(*genMf.SequenceFlow); ok {
			for _, cv := range sf.CaseValuesItems() {
				assignID(cv)
			}
			assignID(sf.Line())
		}
	}
}
