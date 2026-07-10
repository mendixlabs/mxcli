// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genEnum "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/enumerations"
	genTexts "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/texts"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

func init() {
	// Studio Pro serializes an enumeration's RemoteSource and each value's
	// RemoteValue as BSON null when unset (confirmed against the legacy writer,
	// which is byte-faithful for enums). An (empty) caption Text always emits its
	// Items marker. The Enumeration/EnumerationValue scalars (Excluded/ExportLevel/
	// Image) are set explicitly in the converters.
	codec.RegisterTypeDefaults("Enumerations$Enumeration", codec.TypeDefaults{NullFields: []string{"RemoteSource"}})
	codec.RegisterTypeDefaults("Enumerations$EnumerationValue", codec.TypeDefaults{NullFields: []string{"RemoteValue"}})
	codec.RegisterTypeDefaults("Texts$Text", codec.TypeDefaults{MandatoryLists: []string{"Items"}})
}

// CreateEnumeration adds a new enumeration document. Unlike entities (children of
// the domain-model unit), an enumeration is a top-level unit, so this builds the
// gen element, encodes it, and inserts a new unit under the module.
func (b *Backend) CreateEnumeration(enum *model.Enumeration) error {
	if enum == nil {
		return fmt.Errorf("CreateEnumeration: nil enumeration")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateEnumeration: not connected for writing")
	}
	if enum.ID == "" {
		enum.ID = model.ID(mmpr.GenerateID())
	}
	ge := enumToGen(enum)
	ge.SetID(element.ID(enum.ID))
	assignEnumIDs(ge)
	contents, err := (&codec.Encoder{}).Encode(ge)
	if err != nil {
		return fmt.Errorf("CreateEnumeration: encode: %w", err)
	}
	return b.writer.InsertUnit(string(enum.ID), string(enum.ContainerID), "Documents", "Enumerations$Enumeration", contents)
}

// UpdateEnumeration rebuilds an enumeration document from the (lossless) model and
// rewrites its unit. The whole document is small and rebuilt wholesale, matching
// the legacy writer's full re-serialize.
func (b *Backend) UpdateEnumeration(enum *model.Enumeration) error {
	if enum == nil {
		return fmt.Errorf("UpdateEnumeration: nil enumeration")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateEnumeration: not connected for writing")
	}
	ge := enumToGen(enum)
	ge.SetID(element.ID(enum.ID))
	assignEnumIDs(ge)
	contents, err := (&codec.Encoder{}).Encode(ge)
	if err != nil {
		return fmt.Errorf("UpdateEnumeration: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(enum.ID), contents)
}

// DeleteEnumeration removes the enumeration unit.
func (b *Backend) DeleteEnumeration(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteEnumeration: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// enumToGen builds a gen Enumeration from the model. Excluded=false and
// ExportLevel="Hidden" mirror the legacy serializer; RemoteSource (null) comes
// from the registered default.
func enumToGen(enum *model.Enumeration) *genEnum.Enumeration {
	out := genEnum.NewEnumeration()
	out.SetName(enum.Name)
	out.SetDocumentation(enum.Documentation)
	out.SetExcluded(false)
	out.SetExportLevel("Hidden")
	for _, v := range enum.Values {
		out.AddValues(enumValueToGen(v))
	}
	return out
}

// enumValueToGen builds a gen EnumerationValue. The caption is always a Texts$Text
// (empty when no translations); Image="" and RemoteValue (null) mirror legacy.
func enumValueToGen(v model.EnumerationValue) *genEnum.EnumerationValue {
	out := genEnum.NewEnumerationValue()
	out.SetName(v.Name)
	out.SetCaption(captionToGen(v.Caption))
	out.SetImageQualifiedName("")
	return out
}

// captionToGen returns a gen Texts$Text for the caption, empty when nil.
func captionToGen(t *model.Text) *genTexts.Text {
	if t == nil || len(t.Translations) == 0 {
		return genTexts.NewText()
	}
	return textToGen(t)
}

// assignEnumIDs gives the enumeration, its values, captions, and translations
// fresh IDs where empty (assignID leaves non-empty IDs untouched).
func assignEnumIDs(e *genEnum.Enumeration) {
	assignID(e)
	for _, el := range e.ValuesItems() {
		assignID(el)
		v, ok := el.(*genEnum.EnumerationValue)
		if !ok {
			continue
		}
		if cap := v.Caption(); cap != nil {
			assignID(cap)
			if txt, ok := cap.(*genTexts.Text); ok {
				for _, tr := range txt.TranslationsItems() {
					assignID(tr)
				}
			}
		}
	}
}
