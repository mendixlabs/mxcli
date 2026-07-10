// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// The gen ConsumedODataService / PublishedODataService2 accessors don't surface
// (or bind to mismatched storage keys for) the HttpConfiguration sub-document and
// the EntityTypes/EntitySets member tree. A read that drops them makes any
// CREATE OR MODIFY / ALTER round-trip lossy — dropping the ServiceUrl (CE5111) or
// stripping the published entity tree (NullReferenceException on load). These
// helpers read the affected sub-documents straight from the normalised raw unit,
// mirroring the legacy parser's keys and the modelsdk writer's vocabulary so the
// read→modify→write round-trip is faithful.

// httpConfigFromRaw parses a Microflows$HttpConfiguration sub-document. Keys and
// field mapping mirror sdk/mpr.parseODataHttpConfiguration (the real Studio Pro
// storage names, which the modelsdk writer also emits).
func odataHttpConfigFromRaw(raw map[string]any) *model.HttpConfiguration {
	cfg := &model.HttpConfiguration{}
	cfg.ID = model.ID(jsExtractBsonID(raw["$ID"]))
	cfg.TypeName = jsExtractString(raw["$Type"])
	cfg.UseAuthentication = jsExtractBool(raw["UseHttpAuthentication"])
	cfg.Username = jsExtractString(raw["HttpAuthenticationUserName"])
	cfg.Password = jsExtractString(raw["HttpAuthenticationPassword"])
	cfg.HttpMethod = jsExtractString(raw["HttpMethod"])
	cfg.OverrideLocation = jsExtractBool(raw["OverrideLocation"])
	cfg.CustomLocation = jsExtractString(raw["CustomLocation"])
	cfg.ClientCertificate = jsExtractString(raw["ClientCertificate"])
	for _, h := range jsAsSlice(raw["HttpHeaderEntries"]) {
		hm := jsToMap(h)
		if hm == nil {
			continue
		}
		cfg.HeaderEntries = append(cfg.HeaderEntries, &model.HttpHeaderEntry{
			BaseElement: model.BaseElement{ID: model.ID(jsExtractBsonID(hm["$ID"])), TypeName: jsExtractString(hm["$Type"])},
			Key:         jsExtractString(hm["Key"]),
			Value:       jsExtractString(hm["Value"]),
		})
	}
	return cfg
}

// publishedEntityTreeFromRaw parses the EntityTypes + EntitySets of a published
// OData service from its raw unit, resolving each entity set's EntityTypePointer
// back to the entity-type name (mirrors sdk/mpr.parsePublishedODataService).
func publishedEntityTreeFromRaw(raw map[string]any) ([]*model.PublishedEntityType, []*model.PublishedEntitySet) {
	var ets []*model.PublishedEntityType
	byID := map[string]*model.PublishedEntityType{}
	for _, e := range jsAsSlice(raw["EntityTypes"]) {
		em := jsToMap(e)
		if em == nil {
			continue
		}
		et := publishedEntityTypeFromRaw(em)
		ets = append(ets, et)
		byID[string(et.ID)] = et
	}

	var sets []*model.PublishedEntitySet
	for _, e := range jsAsSlice(raw["EntitySets"]) {
		sm := jsToMap(e)
		if sm == nil {
			continue
		}
		sets = append(sets, publishedEntitySetFromRaw(sm, byID))
	}
	return ets, sets
}

func publishedEntityTypeFromRaw(raw map[string]any) *model.PublishedEntityType {
	et := &model.PublishedEntityType{
		BaseElement: model.BaseElement{ID: model.ID(jsExtractBsonID(raw["$ID"])), TypeName: jsExtractString(raw["$Type"])},
		Entity:      jsExtractString(raw["Entity"]),
		ExposedName: jsExtractString(raw["ExposedName"]),
		Summary:     jsExtractString(raw["Summary"]),
		Description: jsExtractString(raw["Description"]),
	}
	for _, m := range jsAsSlice(raw["ChildMembers"]) {
		mm := jsToMap(m)
		if mm == nil {
			continue
		}
		et.Members = append(et.Members, publishedMemberFromRaw(mm))
	}
	return et
}

func publishedMemberFromRaw(raw map[string]any) *model.PublishedMember {
	m := &model.PublishedMember{
		BaseElement: model.BaseElement{ID: model.ID(jsExtractBsonID(raw["$ID"])), TypeName: jsExtractString(raw["$Type"])},
		ExposedName: jsExtractString(raw["ExposedName"]),
		Filterable:  jsExtractBool(raw["Filterable"]),
		Sortable:    jsExtractBool(raw["Sortable"]),
		IsPartOfKey: jsExtractBool(raw["IsPartOfKey"]),
	}
	switch m.TypeName {
	case "ODataPublish$PublishedAttribute":
		m.Kind = "attribute"
		m.Name = jsExtractString(raw["Attribute"])
		m.EdmType = jsExtractString(raw["EdmType"])
	case "ODataPublish$PublishedAssociationEnd":
		m.Kind = "association"
		m.Name = jsExtractString(raw["Association"])
		m.AssociationTargetEntity = jsExtractString(raw["Entity"])
		m.ExposedAssociationName = jsExtractString(raw["ExposedAssociationName"])
		m.IsMany = jsExtractBool(raw["IsMany"])
	case "ODataPublish$PublishedId":
		m.Kind = "id"
		m.Name = jsExtractString(raw["Attribute"])
	default:
		m.Kind = "unknown"
	}
	return m
}

func publishedEntitySetFromRaw(raw map[string]any, byID map[string]*model.PublishedEntityType) *model.PublishedEntitySet {
	es := &model.PublishedEntitySet{
		BaseElement: model.BaseElement{ID: model.ID(jsExtractBsonID(raw["$ID"])), TypeName: jsExtractString(raw["$Type"])},
		ExposedName: jsExtractString(raw["ExposedName"]),
		UsePaging:   jsExtractBool(raw["UsePaging"]),
		PageSize:    jsExtractInt(raw["PageSize"]),
		ReadMode:    parseODataModeRaw(raw["ReadMode"]),
		InsertMode:  parseODataModeRaw(raw["InsertMode"]),
		UpdateMode:  parseODataModeRaw(raw["UpdateMode"]),
		DeleteMode:  parseODataModeRaw(raw["DeleteMode"]),
	}
	if et, ok := byID[jsExtractBsonID(raw["EntityTypePointer"])]; ok {
		es.EntityTypeName = et.Entity
	}
	return es
}

// parseODataModeRaw maps a Read/Change source sub-document to the string the
// modelsdk writer expects (odataReadModeToGen/odataChangeModeToGen), keeping the
// ALTER round-trip faithful. Mirrors sdk/mpr.parseChangeMode.
func parseODataModeRaw(v any) string {
	mm := jsToMap(v)
	if mm == nil {
		return ""
	}
	switch jsExtractString(mm["$Type"]) {
	case "ODataPublish$ReadSource":
		return "ReadFromDatabase"
	case "ODataPublish$ChangeSource":
		return "ChangeFromDatabase"
	case "ODataPublish$ChangeNotSupported":
		return "NotSupported"
	case "ODataPublish$CallMicroflowToRead", "ODataPublish$CallMicroflowToChange":
		if mf := jsExtractString(mm["Microflow"]); mf != "" {
			return "CallMicroflow:" + mf
		}
		return "CallMicroflow"
	default:
		return jsExtractString(mm["$Type"])
	}
}

// jsExtractInt reads an int from a normalised raw value (int32/int64/float64).
func jsExtractInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
