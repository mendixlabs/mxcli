// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	genEnum "github.com/mendixlabs/mxcli/modelsdk/gen/enumerations"
	"github.com/mendixlabs/mxcli/modelsdk/meta"

	"github.com/mendixlabs/mxcli/model"
)

// gen→model enumeration adapter (ported from engalar's convert_reader.go).
// Value captions are Texts$Text elements, decoded via textElementToModel — which
// relies on the gen/texts package being registered (blank-imported in page.go).

func (b *Backend) ListEnumerations() ([]*model.Enumeration, error) {
	units, err := listUnits[*genEnum.Enumeration](b)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Enumeration, 0, len(units)+len(meta.SystemEnumerations))
	for _, u := range units {
		out = append(out, enumToModel(u.Element, u.ContainerID))
	}
	// The System module's enumerations are platform built-ins with no stored
	// unit, so they have to be synthesized or they vanish — same reason as
	// ListJavaActions. Without them `describe enumeration System.X` reports
	// "not found" and `check --references` rejects an attribute typed against
	// one, while `describe entity` prints that very type (#1102).
	out = append(out, meta.BuildSystemEnumerationsIn(b.systemVersionAtLeast())...)
	return out, nil
}

func (b *Backend) GetEnumeration(id model.ID) (*model.Enumeration, error) {
	// Resolved against the same set ListEnumerations returns, so a caller
	// holding an ID from the listing can always look it up again — the System
	// module's synthesized enumerations included.
	enums, err := b.ListEnumerations()
	if err != nil {
		return nil, err
	}
	for _, e := range enums {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

func enumToModel(e *genEnum.Enumeration, containerID model.ID) *model.Enumeration {
	out := &model.Enumeration{
		ContainerID:   containerID,
		Name:          e.Name(),
		Documentation: e.Documentation(),
		Excluded:      e.Excluded(),
		ExportLevel:   e.ExportLevel(),
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
