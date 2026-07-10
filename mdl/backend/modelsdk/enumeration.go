// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	genEnum "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/enumerations"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// gen→model enumeration adapter (ported from engalar's convert_reader.go).
// Value captions are Texts$Text elements, decoded via textElementToModel — which
// relies on the gen/texts package being registered (blank-imported in page.go).

func (b *Backend) ListEnumerations() ([]*model.Enumeration, error) {
	units, err := mprread.ListUnitsWithContainer[*genEnum.Enumeration](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Enumeration, 0, len(units))
	for _, u := range units {
		out = append(out, enumToModel(u.Element, u.ContainerID))
	}
	return out, nil
}

func (b *Backend) GetEnumeration(id model.ID) (*model.Enumeration, error) {
	units, err := mprread.ListUnitsWithContainer[*genEnum.Enumeration](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return enumToModel(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

func enumToModel(e *genEnum.Enumeration, containerID model.ID) *model.Enumeration {
	out := &model.Enumeration{
		ContainerID:   containerID,
		Name:          e.Name(),
		Documentation: e.Documentation(),
	}
	out.ID = model.ID(e.ID())
	out.TypeName = "Enumerations$Enumeration"
	for _, item := range e.ValuesItems() {
		if ev, ok := item.(*genEnum.EnumerationValue); ok {
			out.Values = append(out.Values, enumValueToModel(ev))
		}
	}
	return out
}

func enumValueToModel(v *genEnum.EnumerationValue) model.EnumerationValue {
	out := model.EnumerationValue{Name: v.Name()}
	out.ID = model.ID(v.ID())
	out.TypeName = "Enumerations$EnumerationValue"
	if cap := v.Caption(); cap != nil {
		out.Caption = textElementToModel(cap)
	}
	return out
}
