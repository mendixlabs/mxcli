// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// newUnknownObject creates an UnknownElement that preserves raw BSON fields
// for unrecognized $Type values, preventing silent data loss.
// FieldKinds is populated by inferPropertyKind so callers can see the inferred
// Mendix property kind for each field without inspecting the SDK JS source.
func newUnknownObject(typeName string, raw map[string]any) *model.UnknownElement {
	id := ""
	if raw != nil {
		id = extractBsonID(raw["$ID"])
	}
	// convert map to bson.D for storage
	doc := make(bson.D, 0, len(raw))
	for k, v := range raw {
		doc = append(doc, bson.E{Key: k, Value: v})
	}
	elem := &model.UnknownElement{
		BaseElement: model.BaseElement{ID: model.ID(id), TypeName: typeName},
		RawDoc:      doc,
	}
	if raw != nil {
		elem.Position = parsePoint(raw["RelativeMiddlePoint"])
		elem.Name = extractString(raw["Name"])
		elem.Caption = extractString(raw["Caption"])
		elem.FieldKinds = make(map[string]string, len(raw))
		for k, v := range raw {
			elem.FieldKinds[k] = inferPropertyKind(k, v)
		}
	}
	return elem
}

// newUnknownObjectFromD creates an UnknownElement from a bson.D document,
// preserving field ordering for round-trip fidelity.
func newUnknownObjectFromD(typeName string, raw bson.D) *model.UnknownElement {
	elem := &model.UnknownElement{
		BaseElement: model.BaseElement{TypeName: typeName},
		RawDoc:      raw,
	}
	if len(raw) > 0 {
		elem.FieldKinds = make(map[string]string, len(raw))
		for _, e := range raw {
			switch e.Key {
			case "$ID":
				elem.ID = model.ID(extractBsonID(e.Value))
			case "Name":
				elem.Name = extractString(e.Value)
			case "Caption":
				elem.Caption = extractString(e.Value)
			case "RelativeMiddlePoint":
				elem.Position = parsePoint(e.Value)
			}
			elem.FieldKinds[e.Key] = inferPropertyKind(e.Key, e.Value)
		}
	}
	return elem
}
