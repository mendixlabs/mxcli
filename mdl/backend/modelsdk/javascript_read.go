// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Codec-native JavaScript-action read (SHOW JAVASCRIPT ACTIONS / DESCRIBE
// JAVASCRIPT ACTION).
//
// Unlike Java actions (java_read.go, gen-accessor driven), JS actions are read
// from raw BSON. The gen codec for JavaScriptActions$JavaScriptAction decodes
// its children from the SDK property names — ActionReturnType, ActionParameters,
// ActionTypeParameters, ModelerActionInfo — but real Studio-Pro BSON stores them
// under the original storage names JavaReturnType, Parameters, TypeParameters and
// MicroflowActionInfo. A gen-accessor converter would therefore silently drop the
// return type, parameters, type parameters and exposed-as info (the Issue 7
// "half-shell" failure mode). Reading the four children straight from the
// normalised raw map — the empirical keys the legacy parser uses — avoids that.

// ListJavaScriptActions returns every JavaScript action, fully parsed. Mirrors
// the legacy reader's ListJavaScriptActions.
func (b *Backend) ListJavaScriptActions() ([]*types.JavaScriptAction, error) {
	units, err := b.ListRawUnitsByType("JavaScriptActions$JavaScriptAction")
	if err != nil {
		return nil, err
	}
	out := make([]*types.JavaScriptAction, 0, len(units))
	for _, u := range units {
		raw, err := b.GetRawUnit(u.ID)
		if err != nil {
			continue
		}
		out = append(out, jsActionFromRaw(raw, u.ID, u.ContainerID))
	}
	return out, nil
}

// ReadJavaScriptActionByName returns the fully-parsed JavaScript action for a
// qualified name (Module.ActionName). Used by DESCRIBE JAVASCRIPT ACTION and by
// reference validation.
func (b *Backend) ReadJavaScriptActionByName(qualifiedName string) (*types.JavaScriptAction, error) {
	dot := strings.LastIndex(qualifiedName, ".")
	if dot < 0 {
		return nil, fmt.Errorf("invalid javascript action name: %s", qualifiedName)
	}
	moduleName, actionName := qualifiedName[:dot], qualifiedName[dot+1:]
	mod, err := b.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return nil, fmt.Errorf("javascript action not found: %s", qualifiedName)
	}
	containers := b.containerSetForModule(string(mod.ID)) // module + nested folders
	units, err := b.ListRawUnitsByType("JavaScriptActions$JavaScriptAction")
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if !containers[string(u.ContainerID)] {
			continue
		}
		raw, err := b.GetRawUnit(u.ID)
		if err != nil {
			continue
		}
		if jsExtractString(raw["Name"]) != actionName {
			continue
		}
		return jsActionFromRaw(raw, u.ID, u.ContainerID), nil
	}
	return nil, fmt.Errorf("javascript action not found: %s", qualifiedName)
}

// jsActionFromRaw builds the semantic JavaScript action from a normalised raw
// unit map (the v1 shape GetRawUnit produces: map[string]any / []any /
// primitive.Binary). Inverse of the JS-action write path; mirrors the legacy
// parseJavaScriptAction so DESCRIBE output matches the legacy engine.
func jsActionFromRaw(raw map[string]any, id, containerID model.ID) *types.JavaScriptAction {
	jsa := &types.JavaScriptAction{
		ContainerID:             containerID,
		Name:                    jsExtractString(raw["Name"]),
		Documentation:           jsExtractString(raw["Documentation"]),
		Platform:                jsExtractString(raw["Platform"]),
		Excluded:                jsExtractBool(raw["Excluded"]),
		ExportLevel:             jsExtractString(raw["ExportLevel"]),
		ActionDefaultReturnName: jsExtractString(raw["ActionDefaultReturnName"]),
	}
	jsa.ID = id
	jsa.TypeName = "JavaScriptActions$JavaScriptAction"

	if rt := jsToMap(raw["JavaReturnType"]); rt != nil {
		jsa.ReturnType = parseCodeActionReturnTypeRaw(rt)
	}

	for _, p := range jsAsSlice(raw["Parameters"]) {
		if pMap := jsToMap(p); pMap != nil {
			if param := parseJavaActionParameterRaw(pMap); param != nil {
				jsa.Parameters = append(jsa.Parameters, param)
			}
		}
	}

	for _, tp := range jsAsSlice(raw["TypeParameters"]) {
		if tpMap := jsToMap(tp); tpMap != nil {
			if name := jsExtractString(tpMap["Name"]); name != "" {
				d := &types.TypeParameterDef{Name: name}
				d.ID = model.ID(jsExtractBsonID(tpMap["$ID"]))
				jsa.TypeParameters = append(jsa.TypeParameters, d)
			}
		}
	}

	if mai := jsToMap(raw["MicroflowActionInfo"]); mai != nil {
		info := &types.MicroflowActionInfo{
			Caption:       jsExtractString(mai["Caption"]),
			Category:      jsExtractString(mai["Category"]),
			IconData:      jsExtractBinary(mai["IconData"]),
			IconDataDark:  jsExtractBinary(mai["IconDataDark"]),
			ImageData:     jsExtractBinary(mai["ImageData"]),
			ImageDataDark: jsExtractBinary(mai["ImageDataDark"]),
		}
		info.ID = model.ID(jsExtractBsonID(mai["$ID"]))
		jsa.MicroflowActionInfo = info
	}

	// Resolve type-parameter names for parameters and the return type (the BSON
	// stores them as BY_ID pointers to the TypeParameterDef entries above).
	for _, param := range jsa.Parameters {
		switch pt := param.ParameterType.(type) {
		case *types.EntityTypeParameterType:
			pt.TypeParameterName = jsa.FindTypeParameterName(pt.TypeParameterID)
		case *types.TypeParameter:
			if pt.TypeParameterID != "" && pt.TypeParameter == "" {
				pt.TypeParameter = jsa.FindTypeParameterName(pt.TypeParameterID)
			}
		}
	}
	if tp, ok := jsa.ReturnType.(*types.TypeParameter); ok {
		if tp.TypeParameterID != "" && tp.TypeParameter == "" {
			tp.TypeParameter = jsa.FindTypeParameterName(tp.TypeParameterID)
		}
	}

	return jsa
}

// parseCodeActionReturnTypeRaw maps a CodeActions return-type sub-document to the
// semantic return type. Ported from the legacy parseCodeActionReturnType.
func parseCodeActionReturnTypeRaw(raw map[string]any) types.CodeActionReturnType {
	if raw == nil {
		return nil
	}
	id := func() model.BaseElement {
		return model.BaseElement{ID: model.ID(jsExtractBsonID(raw["$ID"]))}
	}
	switch jsExtractString(raw["$Type"]) {
	case "CodeActions$VoidType":
		return &types.VoidType{BaseElement: id()}
	case "CodeActions$BooleanType":
		return &types.BooleanType{BaseElement: id()}
	case "CodeActions$IntegerType":
		return &types.IntegerType{BaseElement: id()}
	case "CodeActions$LongType":
		return &types.LongType{BaseElement: id()}
	case "CodeActions$DecimalType":
		return &types.DecimalType{BaseElement: id()}
	case "CodeActions$StringType":
		return &types.StringType{BaseElement: id()}
	case "CodeActions$DateTimeType":
		return &types.DateTimeType{BaseElement: id()}
	case "CodeActions$EntityType", "CodeActions$ConcreteEntityType":
		return &types.EntityType{BaseElement: id(), Entity: jsExtractString(raw["Entity"])}
	case "CodeActions$ListType":
		lt := &types.ListType{BaseElement: id()}
		if entity := jsExtractString(raw["Entity"]); entity != "" {
			lt.Entity = entity
		} else if param := jsToMap(raw["Parameter"]); param != nil {
			lt.Entity = jsExtractString(param["Entity"])
		}
		return lt
	case "CodeActions$FileDocumentType":
		return &types.FileDocumentType{BaseElement: id()}
	case "CodeActions$EnumerationType":
		return &types.EnumerationType{BaseElement: id(), Enumeration: jsExtractString(raw["Enumeration"])}
	case "CodeActions$TypeParameter":
		return &types.TypeParameter{BaseElement: id(), TypeParameter: jsExtractString(raw["TypeParameter"])}
	case "CodeActions$ParameterizedEntityType":
		return &types.TypeParameter{BaseElement: id(), TypeParameterID: model.ID(jsTypeParamPointer(raw))}
	}
	return nil
}

// parseJavaActionParameterRaw maps a parameter sub-document to the semantic
// parameter. Ported from the legacy parseJavaActionParameter.
func parseJavaActionParameterRaw(raw map[string]any) *types.JavaActionParameter {
	if raw == nil || raw["$ID"] == nil {
		return nil
	}
	param := &types.JavaActionParameter{
		Name:        jsExtractString(raw["Name"]),
		Description: jsExtractString(raw["Description"]),
		Category:    jsExtractString(raw["Category"]),
		IsRequired:  jsExtractBool(raw["IsRequired"]),
	}
	param.ID = model.ID(jsExtractBsonID(raw["$ID"]))
	param.TypeName = jsExtractString(raw["$Type"])
	if pt := jsToMap(raw["ParameterType"]); pt != nil {
		param.ParameterType = parseCodeActionParameterTypeRaw(pt)
	}
	return param
}

// parseCodeActionParameterTypeRaw maps a CodeActions parameter-type sub-document
// to the semantic parameter type. Ported from the legacy
// parseCodeActionParameterType (+ parseInnerParameterType for BasicParameterType).
func parseCodeActionParameterTypeRaw(raw map[string]any) types.CodeActionParameterType {
	if raw == nil {
		return nil
	}
	id := func() model.BaseElement {
		return model.BaseElement{ID: model.ID(jsExtractBsonID(raw["$ID"]))}
	}
	switch jsExtractString(raw["$Type"]) {
	case "CodeActions$BasicParameterType":
		if inner := jsToMap(raw["Type"]); inner != nil {
			return parseCodeActionParameterTypeRaw(inner)
		}
		return nil
	case "CodeActions$BooleanType":
		return &types.BooleanType{BaseElement: id()}
	case "CodeActions$IntegerType":
		return &types.IntegerType{BaseElement: id()}
	case "CodeActions$LongType":
		return &types.LongType{BaseElement: id()}
	case "CodeActions$DecimalType":
		return &types.DecimalType{BaseElement: id()}
	case "CodeActions$StringType":
		return &types.StringType{BaseElement: id()}
	case "CodeActions$DateTimeType":
		return &types.DateTimeType{BaseElement: id()}
	case "CodeActions$EntityType", "CodeActions$ConcreteEntityType":
		return &types.EntityType{BaseElement: id(), Entity: jsExtractString(raw["Entity"])}
	case "CodeActions$ListType":
		lt := &types.ListType{BaseElement: id()}
		if entity := jsExtractString(raw["Entity"]); entity != "" {
			lt.Entity = entity
		} else if param := jsToMap(raw["Parameter"]); param != nil {
			lt.Entity = jsExtractString(param["Entity"])
		}
		return lt
	case "CodeActions$StringTemplateParameterType":
		return &types.StringTemplateParameterType{BaseElement: id(), Grammar: jsExtractString(raw["Grammar"])}
	case "CodeActions$FileDocumentType":
		return &types.FileDocumentType{BaseElement: id()}
	case "CodeActions$EnumerationType":
		return &types.EnumerationType{BaseElement: id(), Enumeration: jsExtractString(raw["Enumeration"])}
	case "CodeActions$MicroflowType", "JavaActions$MicroflowJavaActionParameterType":
		return &types.MicroflowType{BaseElement: id()}
	case "CodeActions$TypeParameter":
		return &types.TypeParameter{BaseElement: id(), TypeParameter: jsExtractString(raw["TypeParameter"])}
	case "CodeActions$EntityTypeParameterType":
		return &types.EntityTypeParameterType{BaseElement: id(), TypeParameterID: model.ID(jsTypeParamPointer(raw))}
	case "CodeActions$ParameterizedEntityType":
		return &types.TypeParameter{BaseElement: id(), TypeParameterID: model.ID(jsTypeParamPointer(raw))}
	case "JavaScriptActions$NanoflowJavaScriptActionParameterType":
		return &types.NanoflowType{BaseElement: id()}
	}
	return nil
}

// jsTypeParamPointer reads a type-parameter BY_ID reference, preferring the
// Studio-Pro "TypeParameterPointer" key with a "TypeParameter" fallback.
func jsTypeParamPointer(raw map[string]any) string {
	if id := jsExtractBsonID(raw["TypeParameterPointer"]); id != "" {
		return id
	}
	return jsExtractBsonID(raw["TypeParameter"])
}

// --- raw-map extraction helpers (operate on GetRawUnit's normalised v1 shape) ---

func jsExtractString(v any) string {
	s, _ := v.(string)
	return s
}

// jsExtractBinary returns the bytes of a BSON binary value, or nil when the
// field is absent/null/non-binary (tolerating the legacy MicroflowActionInfo
// shape on read). See issue #656.
func jsExtractBinary(v any) []byte {
	if b, ok := v.(primitive.Binary); ok {
		return b.Data
	}
	return nil
}

func jsExtractBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// jsToMap normalises a BSON sub-document to map[string]any (GetRawUnit already
// converts bson.D → map, but keep the primitive.D fallback for robustness).
func jsToMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case primitive.D:
		out := make(map[string]any, len(m))
		for _, e := range m {
			out[e.Key] = e.Value
		}
		return out
	}
	return nil
}

// jsAsSlice normalises a BSON array to []any (GetRawUnit converts bson.A → []any).
func jsAsSlice(v any) []any {
	switch a := v.(type) {
	case []any:
		return a
	case primitive.A:
		return []any(a)
	}
	return nil
}

// jsExtractBsonID converts a BSON id/pointer (string, []byte, or 16-byte
// primitive.Binary UUID) to its canonical string form.
func jsExtractBsonID(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return jsBlobToUUID(val)
	case primitive.Binary:
		return jsBlobToUUID(val.Data)
	}
	return ""
}

// jsBlobToUUID renders a 16-byte UUID blob in the same byte order the codec
// reader uses, so type-parameter pointers match their TypeParameterDef IDs.
func jsBlobToUUID(blob []byte) string {
	if len(blob) != 16 {
		return ""
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		blob[3], blob[2], blob[1], blob[0],
		blob[5], blob[4],
		blob[7], blob[6],
		blob[8], blob[9],
		blob[10], blob[11], blob[12], blob[13], blob[14], blob[15])
}
