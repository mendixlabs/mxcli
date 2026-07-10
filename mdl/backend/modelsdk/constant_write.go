// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genConst "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/constants"
	genDT "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatypes"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

// CreateConstant adds a new constant document (a top-level unit, like an
// enumeration): build the gen element, encode, and insert a new unit.
func (b *Backend) CreateConstant(c *model.Constant) error {
	if c == nil {
		return fmt.Errorf("CreateConstant: nil constant")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateConstant: not connected for writing")
	}
	if c.ID == "" {
		c.ID = model.ID(mmpr.GenerateID())
	}
	gc := constToGen(c)
	gc.SetID(element.ID(c.ID))
	assignConstIDs(gc)
	contents, err := (&codec.Encoder{}).Encode(gc)
	if err != nil {
		return fmt.Errorf("CreateConstant: encode: %w", err)
	}
	return b.writer.InsertUnit(string(c.ID), string(c.ContainerID), "Documents", "Constants$Constant", contents)
}

// UpdateConstant rebuilds a constant document from the model and rewrites its
// unit (the CREATE OR MODIFY CONSTANT path; constants have no ALTER statement).
func (b *Backend) UpdateConstant(c *model.Constant) error {
	if c == nil {
		return fmt.Errorf("UpdateConstant: nil constant")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateConstant: not connected for writing")
	}
	gc := constToGen(c)
	gc.SetID(element.ID(c.ID))
	assignConstIDs(gc)
	contents, err := (&codec.Encoder{}).Encode(gc)
	if err != nil {
		return fmt.Errorf("UpdateConstant: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(c.ID), contents)
}

// DeleteConstant removes the constant unit.
func (b *Backend) DeleteConstant(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteConstant: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// constToGen builds a gen Constant from the model, mirroring the legacy
// serializer's field set (Name/Documentation/Type/DefaultValue/ExposedToClient/
// Excluded/ExportLevel). The legacy-only "DataType" primitive is intentionally
// not set — real Studio-Pro constants carry only the "Type" element (verified
// against test7-app FeedbackModule.ClientIdentifier).
func constToGen(c *model.Constant) *genConst.Constant {
	out := genConst.NewConstant()
	out.SetName(c.Name)
	out.SetDocumentation(c.Documentation)
	out.SetExcluded(c.Excluded)
	out.SetExportLevel(c.ExportLevel)
	out.SetType(constantDataTypeToGen(c.Type))
	out.SetDefaultValue(c.DefaultValue)
	out.SetExposedToClient(c.ExposedToClient)
	return out
}

// constantDataTypeToGen is the reverse of constantDataTypeToModel: a
// model.ConstantDataType to a gen DataTypes$* element. Long maps to IntegerType
// and Date to DateTimeType (storage has no LongType; matches the legacy writer).
func constantDataTypeToGen(dt model.ConstantDataType) element.Element {
	switch dt.Kind {
	case "String":
		return genDT.NewStringType()
	case "Integer", "Long":
		return genDT.NewIntegerType()
	case "Decimal":
		return genDT.NewDecimalType()
	case "Boolean":
		return genDT.NewBooleanType()
	case "DateTime", "Date":
		return genDT.NewDateTimeType()
	case "Binary":
		return genDT.NewBinaryType()
	case "Float":
		return genDT.NewFloatType()
	case "Enumeration":
		t := genDT.NewEnumerationType()
		t.SetEnumerationQualifiedName(dt.EnumRef)
		return t
	case "Object":
		t := genDT.NewObjectType()
		t.SetEntityQualifiedName(dt.EntityRef)
		return t
	case "List":
		t := genDT.NewListType()
		t.SetEntityQualifiedName(dt.EntityRef)
		return t
	default:
		return genDT.NewStringType()
	}
}

// assignConstIDs gives the constant and its data-type element fresh IDs where
// empty (assignID leaves non-empty IDs untouched).
func assignConstIDs(c *genConst.Constant) {
	assignID(c)
	assignID(c.Type())
}
