// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#2252: an attribute path that starts with the entity it is read
// from — `"ServiceDesk"."Product"/System.changedBy/System.User/Name`, the way a
// microflow writes `$X/…` paths fully qualified — was not resolved, and the
// builder wrote the whole string as one flat attribute. mxbuild then aborted
// the build ("An error occurred when trying to set the 'Attribute' property").
// The leading step naming the context entity (or one of its generalizations)
// is the start itself; the path resolves as the short form does.
func TestResolveAssociationPath_LeadingContextEntity(t *testing.T) {
	const modID = model.ID("mod")
	pb := &pageBuilder{
		entityContext: "M.Product",
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "M"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: modID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: "e-base"}, Name: "Item", HasChangedBy: true},
					{BaseElement: model.BaseElement{ID: "e-prod"}, Name: "Product", GeneralizationRef: "M.Item", HasChangedBy: true},
					{BaseElement: model.BaseElement{ID: "e-model"}, Name: "PartModel"},
				},
				Associations: []*domainmodel.Association{
					{Name: "PartModel_Product", ParentID: "e-model", ChildID: "e-prod", Type: domainmodel.AssociationTypeReference},
				},
			}},
		},
	}
	short, shortSteps, ok := pb.resolveAssociationAttributePath("System.changedBy/Name")
	if !ok {
		t.Fatal("control: the short form does not resolve")
	}
	for _, path := range []string{
		"M.Product/System.changedBy/System.User/Name",
		"M.Product/System.changedBy/Name",
		"M.Item/System.changedBy/Name", // a generalization of the context
	} {
		got, steps, ok := pb.resolveAssociationAttributePath(path)
		if !ok || got != short || len(steps) != len(shortSteps) || steps[0].Association != shortSteps[0].Association {
			t.Errorf("%s: got %q %+v ok=%v, want %q %+v like the short form", path, got, steps, ok, short, shortSteps)
		}
	}
	// Control: an entity the context is not stays unresolved, never guessed.
	if _, _, ok := pb.resolveAssociationAttributePath("M.PartModel/System.changedBy/Name"); ok {
		t.Error("a path starting at an unrelated entity was resolved")
	}
}
