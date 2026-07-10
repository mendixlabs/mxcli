// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDt "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatransformers"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
)

// ListDataTransformers reads every DataTransformers$DataTransformer unit (identity
// plus source type and transform steps). Used by SHOW DATA TRANSFORMERS, the
// CREATE OR MODIFY existence check, and DROP.
func (b *Backend) ListDataTransformers() ([]*model.DataTransformer, error) {
	units, err := mprread.ListUnitsWithContainer[*genDt.DataTransformer](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*model.DataTransformer, 0, len(units))
	for _, u := range units {
		g := u.Element
		dt := &model.DataTransformer{ContainerID: model.ID(u.ContainerID), Name: g.Name(), Excluded: g.Excluded()}
		dt.ID = model.ID(g.ID())
		dt.TypeName = "DataTransformers$DataTransformer"
		if src := g.Source(); src != nil {
			if js, ok := src.(*genDt.JsonSource); ok {
				dt.SourceType, dt.SourceJSON = "JSON", js.Content()
			} else if strings.Contains(src.TypeName(), "Xml") {
				dt.SourceType = "XML"
			} else {
				dt.SourceType = "JSON"
			}
		}
		for _, el := range g.StepsItems() {
			st, ok := el.(*genDt.Step)
			if !ok {
				continue
			}
			step := &model.DataTransformerStep{Technology: "JSLT"}
			if act := st.Action(); act != nil {
				if ja, ok := act.(*genDt.JsltAction); ok {
					step.Expression = ja.Jslt()
				} else if strings.Contains(act.TypeName(), "Xslt") {
					step.Technology = "XSLT"
				}
			}
			dt.Steps = append(dt.Steps, step)
		}
		out = append(out, dt)
	}
	return out, nil
}

func init() {
	// A data transformer's Elements and Steps lists, and a StructureObject's
	// Attributes list, all serialize with marker 2 (verified vs legacy). The gen
	// DataTransformer/Step types bind the pointer keys wrongly (RootElement vs the
	// real RootElementPointer), so these documents are built directly with the
	// verified storage keys; register the per-$Type / per-list markers here.
	codec.RegisterListMarker("DataTransformers$StructureObject", 2)
	codec.RegisterListMarker("DataTransformers$Step", 2)
	codec.RegisterTypeDefaults("DataTransformers$DataTransformer", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Elements": 2, "Steps": 2},
	})
	codec.RegisterTypeDefaults("DataTransformers$StructureObject", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Attributes": 2},
	})
}

// CreateDataTransformer inserts a new DataTransformers$DataTransformer document
// (a source, a single structure root element, and the transform steps that point
// at it). Mirrors the legacy serializer.
func (b *Backend) CreateDataTransformer(dt *model.DataTransformer) error {
	if dt == nil {
		return fmt.Errorf("CreateDataTransformer: nil data transformer")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateDataTransformer: not connected for writing")
	}
	if dt.ID == "" {
		dt.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := (&codec.Encoder{}).Encode(dataTransformerToGen(dt))
	if err != nil {
		return fmt.Errorf("CreateDataTransformer: encode: %w", err)
	}
	return b.writer.InsertUnit(string(dt.ID), string(dt.ContainerID), "Documents", "DataTransformers$DataTransformer", contents)
}

// UpdateDataTransformer rewrites an existing data transformer in place.
func (b *Backend) UpdateDataTransformer(dt *model.DataTransformer) error {
	if dt == nil {
		return fmt.Errorf("UpdateDataTransformer: nil data transformer")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateDataTransformer: not connected for writing")
	}
	contents, err := (&codec.Encoder{}).Encode(dataTransformerToGen(dt))
	if err != nil {
		return fmt.Errorf("UpdateDataTransformer: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(dt.ID), contents)
}

// DeleteDataTransformer removes a data transformer unit by ID.
func (b *Backend) DeleteDataTransformer(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteDataTransformer: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

func dataTransformerToGen(dt *model.DataTransformer) element.Element {
	// The single root structure element; every step's input/output and the
	// transformer's RootElementPointer reference it by ID.
	rootID := mmpr.GenerateID()
	root := newElem("DataTransformers$StructureObject", rootID)
	addPartList(root, "Attributes", nil)

	source := newElem("DataTransformers$JsonSource", "")
	if strings.EqualFold(dt.SourceType, "XML") {
		source = newElem("DataTransformers$XmlSource", "")
	}
	addStr(source, "Content", dt.SourceJSON)

	steps := make([]element.Element, 0, len(dt.Steps))
	for _, st := range dt.Steps {
		var action *element.Base
		if strings.EqualFold(st.Technology, "XSLT") {
			action = newElem("DataTransformers$XsltAction", "")
			addStr(action, "Xslt", st.Expression)
		} else {
			action = newElem("DataTransformers$JsltAction", "")
			addStr(action, "Jslt", st.Expression)
		}
		s := newElem("DataTransformers$Step", "")
		addPart(s, "Action", action)
		addIDRef(s, "InputElementPointer", model.ID(rootID))
		addIDRef(s, "OutputElementPointer", model.ID(rootID))
		steps = append(steps, s)
	}

	g := newElem("DataTransformers$DataTransformer", string(dt.ID))
	addStr(g, "Name", dt.Name)
	addStr(g, "Documentation", "")
	addBool(g, "Excluded", dt.Excluded)
	addStr(g, "ExportLevel", "Hidden")
	addPart(g, "Source", source)
	addPartList(g, "Elements", []element.Element{root})
	addIDRef(g, "RootElementPointer", model.ID(rootID))
	addPartList(g, "Steps", steps)
	return g
}

// addIDRef adds a by-ID reference property (BSON key = name). The referenced ID is
// encoded as a binary UUID pointer (same as $ID / association ParentPointer).
func addIDRef(b *element.Base, name string, id model.ID) {
	p := property.NewByIdRef[element.Element](name)
	b.AddProperty(p, uint(len(b.Properties())))
	p.SetID(element.ID(id))
}
