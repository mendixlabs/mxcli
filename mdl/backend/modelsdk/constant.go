// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	genConst "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/constants"
	genDT "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatypes"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// gen→model constant adapter (ported from engalar's convert_reader.go).
// Importing gen/datatypes both supplies the DataTypes$* concrete types used by
// constantDataTypeToModel and registers them in the codec.

func (b *Backend) ListConstants() ([]*model.Constant, error) {
	units, err := mprread.ListUnitsWithContainer[*genConst.Constant](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Constant, 0, len(units))
	for _, u := range units {
		out = append(out, constToModel(u.Element, u.ContainerID))
	}
	return out, nil
}

func (b *Backend) GetConstant(id model.ID) (*model.Constant, error) {
	units, err := mprread.ListUnitsWithContainer[*genConst.Constant](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return constToModel(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

func constToModel(c *genConst.Constant, containerID model.ID) *model.Constant {
	out := &model.Constant{
		ContainerID:     containerID,
		Name:            c.Name(),
		Documentation:   c.Documentation(),
		Type:            constantDataTypeToModel(c.Type()),
		DefaultValue:    c.DefaultValue(),
		ExposedToClient: c.ExposedToClient(),
		Excluded:        c.Excluded(),
		ExportLevel:     c.ExportLevel(),
	}
	out.ID = model.ID(c.ID())
	out.TypeName = "Constants$Constant"
	return out
}

func constantDataTypeToModel(typ element.Element) model.ConstantDataType {
	dt := model.ConstantDataType{Kind: "Unknown"}
	if typ == nil {
		return dt
	}
	switch typ.TypeName() {
	case "DataTypes$StringType":
		dt.Kind = "String"
	case "DataTypes$IntegerType":
		dt.Kind = "Integer"
	case "DataTypes$LongType":
		dt.Kind = "Long"
	case "DataTypes$DecimalType":
		dt.Kind = "Decimal"
	case "DataTypes$BooleanType":
		dt.Kind = "Boolean"
	case "DataTypes$DateTimeType":
		dt.Kind = "DateTime"
	case "DataTypes$BinaryType":
		dt.Kind = "Binary"
	case "DataTypes$FloatType":
		dt.Kind = "Float"
	case "DataTypes$EnumerationType":
		dt.Kind = "Enumeration"
		if et, ok := typ.(*genDT.EnumerationType); ok {
			dt.EnumRef = et.EnumerationQualifiedName()
		}
	case "DataTypes$ObjectType":
		dt.Kind = "Object"
		if ot, ok := typ.(*genDT.ObjectType); ok {
			dt.EntityRef = ot.EntityQualifiedName()
		}
	case "DataTypes$ListType":
		dt.Kind = "List"
		if lt, ok := typ.(*genDT.ListType); ok {
			dt.EntityRef = lt.EntityQualifiedName()
		}
	}
	return dt
}
