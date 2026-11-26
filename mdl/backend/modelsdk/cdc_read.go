// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genCdc "github.com/mendixlabs/mxcli/modelsdk/gen/changedatacapture"
)

// ListChangeDataCaptureServices reads every ChangeDataCapture$EntityChangeDataCaptureService
// in the project.
//
// The document is two-sided and both sides are read: Sources is the live model,
// Baseline the copy Studio Pro's "accept changes" froze, and mxbuild raises
// CE1585 when they disagree. The baseline names what it tracks by **GUID** —
// the identity the runtime keys the database on — so the GUIDs are carried into
// the semantic model verbatim rather than resolved to names here. A reader that
// resolved them would make the write path's hardest invariant invisible
// (CLAUDE.md, and ako/mxcli#2395 for the decode that made these readable at all).
func (b *Backend) ListChangeDataCaptureServices() ([]*model.ChangeDataCaptureService, error) {
	units, err := listUnits[*genCdc.EntityChangeDataCaptureService](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ChangeDataCaptureService, 0, len(units))
	for _, u := range units {
		g := u.Element
		svc := &model.ChangeDataCaptureService{
			ContainerID:   model.ID(u.ContainerID),
			Name:          g.Name(),
			Documentation: g.Documentation(),
			Excluded:      g.Excluded(),
			ExportLevel:   g.ExportLevel(),
			ServiceName:   g.ServiceName(),
			AppName:       g.AppName(),
		}
		svc.ID = model.ID(g.ID())
		svc.TypeName = "ChangeDataCapture$EntityChangeDataCaptureService"

		for _, el := range g.SourcesItems() {
			src, ok := el.(*genCdc.EntityChangeSource)
			if !ok {
				continue
			}
			svc.Sources = append(svc.Sources, changeSourceFromGen(src))
		}
		if bl, ok := g.Baseline().(*genCdc.VersionedEntityChangeDataCaptureService); ok && bl != nil {
			svc.Baseline = changeBaselineFromGen(bl)
		}
		out = append(out, svc)
	}
	return out, nil
}

func changeSourceFromGen(g *genCdc.EntityChangeSource) *model.ChangeSource {
	src := &model.ChangeSource{
		Entity:            g.EntityQualifiedName(),
		ExposedName:       g.ExposedName(),
		VersionedSourceID: model.ID(g.VersionedSourceRefID()),
	}
	src.ID = model.ID(g.ID())
	src.TypeName = "ChangeDataCapture$EntityChangeSource"

	for _, el := range g.PropertiesItems() {
		// Which concrete type it is *is* the information: an attribute and an
		// association are tracked differently and bump the version differently.
		switch p := el.(type) {
		case *genCdc.AttributeProperty:
			prop := &model.ChangeProperty{
				ExposedName:         p.ExposedName(),
				Attribute:           p.AttributeQualifiedName(),
				VersionedPropertyID: model.ID(p.VersionedPropertyRefID()),
			}
			prop.ID = model.ID(p.ID())
			prop.TypeName = "ChangeDataCapture$AttributeProperty"
			src.Properties = append(src.Properties, prop)
		case *genCdc.AssociationProperty:
			prop := &model.ChangeProperty{
				ExposedName:         p.ExposedName(),
				Association:         p.AssociationQualifiedName(),
				VersionedPropertyID: model.ID(p.VersionedPropertyRefID()),
			}
			prop.ID = model.ID(p.ID())
			prop.TypeName = "ChangeDataCapture$AssociationProperty"
			src.Properties = append(src.Properties, prop)
		}
	}
	return src
}

func changeBaselineFromGen(g *genCdc.VersionedEntityChangeDataCaptureService) *model.ChangeBaseline {
	bl := &model.ChangeBaseline{Version: changeVersionFromGen(g.Version())}
	bl.ID = model.ID(g.ID())
	bl.TypeName = "ChangeDataCapture$VersionedEntityChangeDataCaptureService"
	if raw := g.Raw(); raw != nil {
		if _, err := raw.LookupErr("DecimalScale"); err == nil {
			ds := int(g.DecimalScale())
			bl.DecimalScale = &ds
		}
	}

	for _, el := range g.SourcesItems() {
		vs, ok := el.(*genCdc.VersionedEntityChangeSource)
		if !ok {
			continue
		}
		src := &model.VersionedChangeSource{
			EntityGUID:  vs.EntityGuid(),
			ExposedName: vs.ExposedName(),
			Version:     changeVersionFromGen(vs.Version()),
		}
		src.ID = model.ID(vs.ID())
		src.TypeName = "ChangeDataCapture$VersionedEntityChangeSource"

		for _, pel := range vs.PropertiesItems() {
			vp, ok := pel.(*genCdc.VersionedProperty)
			if !ok {
				continue
			}
			prop := &model.VersionedChangeProperty{
				ExposedName:          vp.ExposedName(),
				PropertyGUID:         vp.PropertyGuid(),
				AssociatedEntityGUID: vp.AssociatedEntityGuid(),
				Type:                 vp.Type(),
			}
			if raw := vp.Raw(); raw != nil {
				if _, err := raw.LookupErr("MaxLength"); err == nil {
					prop.Facts = &model.ChangePropertyFacts{
						AssociationMultiplicity: vp.AssociationMultiplicity(),
						IsNullable:              vp.IsNullable(),
						MaxLength:               int(vp.MaxLength()),
					}
				}
			}
			prop.ID = model.ID(vp.ID())
			prop.TypeName = "ChangeDataCapture$VersionedProperty"
			src.Properties = append(src.Properties, prop)
		}
		bl.Sources = append(bl.Sources, src)
	}
	return bl
}

func changeVersionFromGen(el element.Element) *model.ChangeVersion {
	v, ok := el.(*genCdc.Version)
	if !ok || v == nil {
		return nil
	}
	out := &model.ChangeVersion{Major: int(v.Major()), Minor: int(v.Minor())}
	out.ID = model.ID(v.ID())
	out.TypeName = "ChangeDataCapture$Version"
	return out
}
