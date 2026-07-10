// SPDX-License-Identifier: Apache-2.0
//
// Supplement: raw BSON field getters for legacy storage key fallbacks.
//
// gen-typed getters bind to a single BSON key chosen by the codegen
// schema, but real MPRs frequently store the same value under a legacy
// key (e.g. `XpathConstraint` vs `XPathConstraint`, `Parameters` vs
// `Arguments`). Describe-path executor code needs those fallbacks while
// staying out of the codec import — so we expose typed wrappers and a
// generic raw-field helper here.

package microflows

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
)

func readField(raw []byte, key string) string {
	v, _ := codec.ReadBSONFieldString(raw, key)
	return v
}

// RawFieldString reads a BSON string field from raw element bytes.
// Use when the receiver decoded as *element.Base because the gen type
// is unregistered, or when the field has no typed getter at all.
func RawFieldString(raw []byte, key string) string {
	return readField(raw, key)
}

// RawFieldStringFromBase reads a BSON string field from an *element.Base.
func RawFieldStringFromBase(b *element.Base, key string) string {
	if b == nil {
		return ""
	}
	return readField(b.Raw(), key)
}

// ────────────────────────────────────────────────────────
// CastAction
// ────────────────────────────────────────────────────────

// CastActionOutputVariableName reads the legacy "OutputVariableName" key.
func CastActionOutputVariableName(a *CastAction) string {
	return readField(a.Raw(), "OutputVariableName")
}

// CastActionVariableName reads the legacy "VariableName" fallback key.
func CastActionVariableName(a *CastAction) string {
	return readField(a.Raw(), "VariableName")
}

// CastActionObjectVariableName reads "ObjectVariableName" (no gen getter).
func CastActionObjectVariableName(a *CastAction) string {
	return readField(a.Raw(), "ObjectVariableName")
}

// ────────────────────────────────────────────────────────
// RetrieveAction
// ────────────────────────────────────────────────────────

// RetrieveActionResultVariableName reads the legacy "ResultVariableName" key.
func RetrieveActionResultVariableName(a *RetrieveAction) string {
	return readField(a.Raw(), "ResultVariableName")
}

// ────────────────────────────────────────────────────────
// DatabaseRetrieveSource / AssociationRetrieveSource
// ────────────────────────────────────────────────────────

// DatabaseRetrieveSourceXpathConstraint reads the legacy lowercase-`p`
// "XpathConstraint" key (gen binds to "XPathConstraint").
func DatabaseRetrieveSourceXpathConstraint(s *DatabaseRetrieveSource) string {
	return readField(s.Raw(), "XpathConstraint")
}

// AssociationRetrieveSourceAssociationId reads "AssociationId" (gen binds
// to "Association").
func AssociationRetrieveSourceAssociationId(s *AssociationRetrieveSource) string {
	return readField(s.Raw(), "AssociationId")
}

// ────────────────────────────────────────────────────────
// ConstantRange (no gen getters for limit/offset)
// ────────────────────────────────────────────────────────

// ConstantRangeLimitExpression reads "LimitExpression".
func ConstantRangeLimitExpression(r *ConstantRange) string {
	return readField(r.Raw(), "LimitExpression")
}

// ConstantRangeOffsetExpression reads "OffsetExpression".
func ConstantRangeOffsetExpression(r *ConstantRange) string {
	return readField(r.Raw(), "OffsetExpression")
}

// ────────────────────────────────────────────────────────
// ValidationFeedbackAction
// ────────────────────────────────────────────────────────

// ValidationFeedbackActionValidationVariableName reads the legacy key
// "ValidationVariableName" (gen binds to "ObjectVariableName").
func ValidationFeedbackActionValidationVariableName(a *ValidationFeedbackAction) string {
	return readField(a.Raw(), "ValidationVariableName")
}

// ────────────────────────────────────────────────────────
// ResultHandling / nested calls
// ────────────────────────────────────────────────────────

// ResultHandlingResultVariableName reads the legacy "ResultVariableName"
// key off a ResultHandling element (gen binds OutputVariableName to
// "OutputVariableName"; real MPRs use "ResultVariableName").
func ResultHandlingResultVariableName(rh *ResultHandling) string {
	if rh == nil {
		return ""
	}
	return readField(rh.Raw(), "ResultVariableName")
}

// ImportMappingCallReturnValueMapping reads the legacy alias
// "ReturnValueMapping" off an ImportMappingCall.
func ImportMappingCallReturnValueMapping(c *ImportMappingCall) string {
	if c == nil {
		return ""
	}
	return readField(c.Raw(), "ReturnValueMapping")
}

// ────────────────────────────────────────────────────────
// RestOperationParameterMapping
// ────────────────────────────────────────────────────────

// RestOperationParameterMappingQueryParameter reads the legacy key
// "QueryParameter" (gen binds to "Parameter").
func RestOperationParameterMappingQueryParameter(pm *RestOperationParameterMapping) string {
	if pm == nil {
		return ""
	}
	return readField(pm.Raw(), "QueryParameter")
}

// ────────────────────────────────────────────────────────
// WebServiceCallAction
// ────────────────────────────────────────────────────────

// WebServiceCallActionImportedService reads the legacy "ImportedService"
// key (gen binds to "ImportedWebService").
func WebServiceCallActionImportedService(a *WebServiceCallAction) string {
	if a == nil {
		return ""
	}
	return readField(a.Raw(), "ImportedService")
}

// WebServiceLegacyMappings extracts (sendMapping, receiveMapping) from a
// WebServiceCallAction's raw BSON document. Legacy reads:
//   - send:    RequestHandling -> ExportMappingCall -> Mapping
//   - receive: NewResultHandling -> ImportMappingCall -> ReturnValueMapping
//     (with a Mapping fallback)
//
// Returns ("", "") when neither chain is present.
func WebServiceLegacyMappings(a *WebServiceCallAction) (send string, receive string) {
	if a == nil {
		return "", ""
	}
	doc := unmarshalBSONDoc(a.Raw())
	if doc == nil {
		return "", ""
	}
	if call := lookupBSONMap(doc, "RequestHandling"); call != nil {
		if inner := lookupBSONMap(call, "ExportMappingCall"); inner != nil {
			send, _ = inner["Mapping"].(string)
		}
	}
	if call := lookupBSONMap(doc, "NewResultHandling"); call != nil {
		if inner := lookupBSONMap(call, "ImportMappingCall"); inner != nil {
			receive, _ = inner["ReturnValueMapping"].(string)
			if receive == "" {
				receive, _ = inner["Mapping"].(string)
			}
		}
	}
	return send, receive
}

// WebServiceRequestBodyExportMapping reads the
// ExportMappingCall.Mapping value from a RequestBodyHandling element's
// raw BSON document (Mendix MPRs store the mapping reference under this
// nested chain when the gen schema did not model it).
func WebServiceRequestBodyExportMapping(b *element.Base) string {
	if b == nil {
		return ""
	}
	doc := unmarshalBSONDoc(b.Raw())
	if doc == nil {
		return ""
	}
	if call := lookupBSONMap(doc, "ExportMappingCall"); call != nil {
		if v, _ := call["Mapping"].(string); v != "" {
			return v
		}
	}
	return ""
}

// ────────────────────────────────────────────────────────
// SortItemList legacy raw-BSON walk (NewSortings → Sortings)
// ────────────────────────────────────────────────────────

// SortPartFromRaw is one decoded sort item from the legacy raw-BSON walk:
// a fully-qualified attribute name plus a Descending flag.
type SortPartFromRaw struct {
	AttributeName string
	Descending    bool
}

// SortPartsFromRawBSON decodes the legacy `NewSortings → Sortings` array
// out of a DatabaseRetrieveSource raw BSON document. Each Sortings entry
// is a Microflows$SortItem with one of `Attribute` (BY_NAME_REFERENCE
// qualified name), `AttributePath` (string), or a nested
// `AttributeRef.Attribute` (gen-typed). Direction is taken from the
// `SortOrder` enum string (with a legacy `Direction` fallback).
//
// The first array element is a BSON int32 versioning prefix; non-document
// entries are skipped silently.
func SortPartsFromRawBSON(raw []byte) []SortPartFromRaw {
	if len(raw) == 0 {
		return nil
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	listMap := lookupBSONMap(doc, "NewSortings", "Sortings", "SortItemList", "sortItemList")
	if listMap == nil {
		return nil
	}
	itemsArr := lookupBSONArray(listMap, "Sortings", "Items", "items")
	if itemsArr == nil {
		return nil
	}
	var parts []SortPartFromRaw
	for _, it := range itemsArr {
		m, ok := it.(bson.M)
		if !ok {
			continue
		}
		attrName, _ := m["Attribute"].(string)
		if attrName == "" {
			attrName, _ = m["AttributePath"].(string)
		}
		if attrName == "" {
			if refMap, ok := m["AttributeRef"].(bson.M); ok {
				attrName, _ = refMap["Attribute"].(string)
			}
		}
		if attrName == "" {
			continue
		}
		descending := false
		if dir, _ := m["SortOrder"].(string); dir == string(SortOrderEnumDescending) {
			descending = true
		}
		// Legacy parsers also recognise a "Direction" key on older
		// Sortings shapes; mirror that for parity even though modern MPRs
		// use SortOrder.
		if dir, _ := m["Direction"].(string); dir == "Descending" {
			descending = true
		}
		parts = append(parts, SortPartFromRaw{AttributeName: attrName, Descending: descending})
	}
	return parts
}

// ────────────────────────────────────────────────────────
// StringTemplate legacy raw-BSON walk (Parameters → Expression)
// ────────────────────────────────────────────────────────

// StringTemplateArgsFromRaw decodes the legacy `Parameters` (or
// `Arguments`) array on a raw StringTemplate document. Each entry is a
// Microflows$TemplateArgument with an `Expression` string field. Empty
// expressions are skipped.
func StringTemplateArgsFromRaw(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	arr := lookupBSONArray(doc, "Parameters", "Arguments")
	if arr == nil {
		return nil
	}
	var exprs []string
	for _, it := range arr {
		m, ok := it.(bson.M)
		if !ok {
			continue
		}
		if expr, _ := m["Expression"].(string); expr != "" {
			exprs = append(exprs, expr)
		}
	}
	return exprs
}

// lookupBSONMap returns the first non-nil bson.M value found under any
// of the candidate keys, mirroring the legacy parser's field-name
// fallback chain.
func lookupBSONMap(doc bson.M, keys ...string) bson.M {
	for _, k := range keys {
		switch v := doc[k].(type) {
		case bson.M:
			return v
		case bson.D:
			// v2: nested docs inside bson.M decode as bson.D.
			m := make(bson.M, len(v))
			for _, e := range v {
				m[e.Key] = e.Value
			}
			return m
		}
	}
	return nil
}

// lookupBSONArray returns the first non-nil bson.A value found under
// any of the candidate keys.
func lookupBSONArray(doc bson.M, keys ...string) bson.A {
	for _, k := range keys {
		if v, ok := doc[k].(bson.A); ok {
			return v
		}
	}
	return nil
}

// unmarshalBSONDoc decodes a raw BSON document into a bson.M for nested
// key lookups. Returns nil for empty input or unmarshal errors so the
// caller can chain `lookupBSONMap` safely.
func unmarshalBSONDoc(raw []byte) bson.M {
	if len(raw) == 0 {
		return nil
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	return doc
}
