// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/sdk/mpr"
)

// ---------------------------------------------------------------------------
// Conversion helpers: sdk/mpr -> mdl/types
// ---------------------------------------------------------------------------

func convertMPRVersion(v mpr.MPRVersion) types.MPRVersion { return types.MPRVersion(v) }

func convertFolderInfoSlice(in []*mpr.FolderInfo, err error) ([]*types.FolderInfo, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.FolderInfo, len(in))
	for i, f := range in {
		out[i] = &types.FolderInfo{ID: f.ID, ContainerID: f.ContainerID, Name: f.Name}
	}
	return out, nil
}

func convertUnitInfoSlice(in []*mpr.UnitInfo, err error) ([]*types.UnitInfo, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.UnitInfo, len(in))
	for i, u := range in {
		out[i] = &types.UnitInfo{
			ID: u.ID, ContainerID: u.ContainerID,
			ContainmentName: u.ContainmentName, Type: u.Type,
		}
	}
	return out, nil
}

func convertRenameHitSlice(in []mpr.RenameHit, err error) ([]types.RenameHit, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]types.RenameHit, len(in))
	for i, h := range in {
		out[i] = types.RenameHit{UnitID: h.UnitID, UnitType: h.UnitType, Name: h.Name, Count: h.Count}
	}
	return out, nil
}

func convertRawUnitSlice(in []*mpr.RawUnit, err error) ([]*types.RawUnit, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.RawUnit, len(in))
	for i, r := range in {
		out[i] = &types.RawUnit{
			ID: r.ID, ContainerID: r.ContainerID, Type: r.Type, Contents: r.Contents,
		}
	}
	return out, nil
}

func convertRawUnitInfoSlice(in []*mpr.RawUnitInfo, err error) ([]*types.RawUnitInfo, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.RawUnitInfo, len(in))
	for i, r := range in {
		out[i] = &types.RawUnitInfo{
			ID: r.ID, QualifiedName: r.QualifiedName, Type: r.Type,
			ModuleName: r.ModuleName, Contents: r.Contents,
		}
	}
	return out, nil
}

func convertRawUnitInfoPtr(in *mpr.RawUnitInfo, err error) (*types.RawUnitInfo, error) {
	if err != nil || in == nil {
		return nil, err
	}
	return &types.RawUnitInfo{
		ID: in.ID, QualifiedName: in.QualifiedName, Type: in.Type,
		ModuleName: in.ModuleName, Contents: in.Contents,
	}, nil
}

func convertRawCustomWidgetTypePtr(in *mpr.RawCustomWidgetType, err error) (*types.RawCustomWidgetType, error) {
	if err != nil || in == nil {
		return nil, err
	}
	return &types.RawCustomWidgetType{
		WidgetID: in.WidgetID, RawType: in.RawType, RawObject: in.RawObject,
		UnitID: in.UnitID, UnitName: in.UnitName, WidgetName: in.WidgetName,
	}, nil
}

func convertRawCustomWidgetTypeSlice(in []*mpr.RawCustomWidgetType, err error) ([]*types.RawCustomWidgetType, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.RawCustomWidgetType, len(in))
	for i, w := range in {
		out[i] = &types.RawCustomWidgetType{
			WidgetID: w.WidgetID, RawType: w.RawType, RawObject: w.RawObject,
			UnitID: w.UnitID, UnitName: w.UnitName, WidgetName: w.WidgetName,
		}
	}
	return out, nil
}

func convertJavaActionSlice(in []*mpr.JavaAction, err error) ([]*types.JavaAction, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.JavaAction, len(in))
	for i, ja := range in {
		out[i] = &types.JavaAction{
			BaseElement:   ja.BaseElement,
			ContainerID:   ja.ContainerID,
			Name:          ja.Name,
			Documentation: ja.Documentation,
		}
	}
	return out, nil
}

func convertJavaScriptActionSlice(in []*mpr.JavaScriptAction, err error) ([]*types.JavaScriptAction, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.JavaScriptAction, len(in))
	for i, jsa := range in {
		out[i] = convertJavaScriptAction(jsa)
	}
	return out, nil
}

func convertJavaScriptActionPtr(in *mpr.JavaScriptAction, err error) (*types.JavaScriptAction, error) {
	if err != nil || in == nil {
		return nil, err
	}
	return convertJavaScriptAction(in), nil
}

func convertJavaScriptAction(in *mpr.JavaScriptAction) *types.JavaScriptAction {
	return &types.JavaScriptAction{
		BaseElement:             in.BaseElement,
		ContainerID:             in.ContainerID,
		Name:                    in.Name,
		Documentation:           in.Documentation,
		Platform:                in.Platform,
		Excluded:                in.Excluded,
		ExportLevel:             in.ExportLevel,
		ActionDefaultReturnName: in.ActionDefaultReturnName,
		ReturnType:              in.ReturnType,
		Parameters:              in.Parameters,
		TypeParameters:          in.TypeParameters,
		MicroflowActionInfo:     in.MicroflowActionInfo,
	}
}

func convertNavDocSlice(in []*mpr.NavigationDocument, err error) ([]*types.NavigationDocument, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.NavigationDocument, len(in))
	for i, nd := range in {
		out[i] = convertNavDoc(nd)
	}
	return out, nil
}

func convertNavDocPtr(in *mpr.NavigationDocument, err error) (*types.NavigationDocument, error) {
	if err != nil || in == nil {
		return nil, err
	}
	return convertNavDoc(in), nil
}

func convertNavDoc(in *mpr.NavigationDocument) *types.NavigationDocument {
	nd := &types.NavigationDocument{
		BaseElement: in.BaseElement,
		ContainerID: in.ContainerID,
		Name:        in.Name,
	}
	if in.Profiles != nil {
		nd.Profiles = make([]*types.NavigationProfile, len(in.Profiles))
		for i, p := range in.Profiles {
			nd.Profiles[i] = convertNavProfile(p)
		}
	}
	return nd
}

func convertNavProfile(in *mpr.NavigationProfile) *types.NavigationProfile {
	p := &types.NavigationProfile{
		Name:         in.Name,
		Kind:         in.Kind,
		IsNative:     in.IsNative,
		LoginPage:    in.LoginPage,
		NotFoundPage: in.NotFoundPage,
	}
	if in.HomePage != nil {
		p.HomePage = &types.NavHomePage{Page: in.HomePage.Page, Microflow: in.HomePage.Microflow}
	}
	if in.RoleBasedHomePages != nil {
		p.RoleBasedHomePages = make([]*types.NavRoleBasedHome, len(in.RoleBasedHomePages))
		for i, rbh := range in.RoleBasedHomePages {
			p.RoleBasedHomePages[i] = &types.NavRoleBasedHome{
				UserRole: rbh.UserRole, Page: rbh.Page, Microflow: rbh.Microflow,
			}
		}
	}
	if in.MenuItems != nil {
		p.MenuItems = make([]*types.NavMenuItem, len(in.MenuItems))
		for i, mi := range in.MenuItems {
			p.MenuItems[i] = convertNavMenuItem(mi)
		}
	}
	if in.OfflineEntities != nil {
		p.OfflineEntities = make([]*types.NavOfflineEntity, len(in.OfflineEntities))
		for i, oe := range in.OfflineEntities {
			p.OfflineEntities[i] = &types.NavOfflineEntity{
				Entity: oe.Entity, SyncMode: oe.SyncMode, Constraint: oe.Constraint,
			}
		}
	}
	return p
}

func convertNavMenuItem(in *mpr.NavMenuItem) *types.NavMenuItem {
	mi := &types.NavMenuItem{
		Caption: in.Caption, Page: in.Page, Microflow: in.Microflow, ActionType: in.ActionType,
	}
	if in.Items != nil {
		mi.Items = make([]*types.NavMenuItem, len(in.Items))
		for i, sub := range in.Items {
			mi.Items[i] = convertNavMenuItem(sub)
		}
	}
	return mi
}

// ---------------------------------------------------------------------------
// Conversion helpers: mdl/types -> sdk/mpr (for write methods)
// ---------------------------------------------------------------------------

// unconvertNavProfileSpec is now a pass-through since mpr.NavigationProfileSpec
// is aliased to types.NavigationProfileSpec.
func unconvertNavProfileSpec(s types.NavigationProfileSpec) mpr.NavigationProfileSpec {
	return s
}

func unconvertEntityMemberAccessSlice(in []types.EntityMemberAccess) []mpr.EntityMemberAccess {
	if in == nil {
		return nil
	}
	out := make([]mpr.EntityMemberAccess, len(in))
	for i, ma := range in {
		out[i] = mpr.EntityMemberAccess{
			AttributeRef: ma.AttributeRef, AssociationRef: ma.AssociationRef, AccessRights: ma.AccessRights,
		}
	}
	return out
}

func unconvertEntityAccessRevocation(in types.EntityAccessRevocation) mpr.EntityAccessRevocation {
	return mpr.EntityAccessRevocation{
		RevokeCreate:       in.RevokeCreate,
		RevokeDelete:       in.RevokeDelete,
		RevokeReadMembers:  in.RevokeReadMembers,
		RevokeWriteMembers: in.RevokeWriteMembers,
		RevokeReadAll:      in.RevokeReadAll,
		RevokeWriteAll:     in.RevokeWriteAll,
	}
}

func convertJsonStructureSlice(in []*mpr.JsonStructure, err error) ([]*types.JsonStructure, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.JsonStructure, len(in))
	for i, js := range in {
		out[i] = convertJsonStructure(js)
	}
	return out, nil
}

func convertJsonStructurePtr(in *mpr.JsonStructure, err error) (*types.JsonStructure, error) {
	if err != nil || in == nil {
		return nil, err
	}
	return convertJsonStructure(in), nil
}

func convertJsonStructure(in *mpr.JsonStructure) *types.JsonStructure {
	js := &types.JsonStructure{
		BaseElement:   in.BaseElement,
		ContainerID:   in.ContainerID,
		Name:          in.Name,
		Documentation: in.Documentation,
		JsonSnippet:   in.JsonSnippet,
		Excluded:      in.Excluded,
		ExportLevel:   in.ExportLevel,
	}
	if in.Elements != nil {
		js.Elements = make([]*types.JsonElement, len(in.Elements))
		for i, e := range in.Elements {
			js.Elements[i] = convertJsonElement(e)
		}
	}
	return js
}

func convertJsonElement(in *mpr.JsonElement) *types.JsonElement {
	e := &types.JsonElement{
		ExposedName: in.ExposedName, ExposedItemName: in.ExposedItemName,
		Path: in.Path, ElementType: in.ElementType, PrimitiveType: in.PrimitiveType,
		MinOccurs: in.MinOccurs, MaxOccurs: in.MaxOccurs, Nillable: in.Nillable,
		IsDefaultType: in.IsDefaultType, MaxLength: in.MaxLength,
		FractionDigits: in.FractionDigits, TotalDigits: in.TotalDigits,
		OriginalValue: in.OriginalValue,
	}
	if in.Children != nil {
		e.Children = make([]*types.JsonElement, len(in.Children))
		for i, c := range in.Children {
			e.Children[i] = convertJsonElement(c)
		}
	}
	return e
}

func unconvertJsonStructure(in *types.JsonStructure) *types.JsonStructure {
	js := &types.JsonStructure{
		BaseElement:   in.BaseElement,
		ContainerID:   in.ContainerID,
		Name:          in.Name,
		Documentation: in.Documentation,
		JsonSnippet:   in.JsonSnippet,
		Excluded:      in.Excluded,
		ExportLevel:   in.ExportLevel,
	}
	if in.Elements != nil {
		js.Elements = make([]*types.JsonElement, len(in.Elements))
		for i, e := range in.Elements {
			js.Elements[i] = unconvertJsonElement(e)
		}
	}
	return js
}

func unconvertJsonElement(in *types.JsonElement) *types.JsonElement {
	e := &types.JsonElement{
		ExposedName: in.ExposedName, ExposedItemName: in.ExposedItemName,
		Path: in.Path, ElementType: in.ElementType, PrimitiveType: in.PrimitiveType,
		MinOccurs: in.MinOccurs, MaxOccurs: in.MaxOccurs, Nillable: in.Nillable,
		IsDefaultType: in.IsDefaultType, MaxLength: in.MaxLength,
		FractionDigits: in.FractionDigits, TotalDigits: in.TotalDigits,
		OriginalValue: in.OriginalValue,
	}
	if in.Children != nil {
		e.Children = make([]*types.JsonElement, len(in.Children))
		for i, c := range in.Children {
			e.Children[i] = unconvertJsonElement(c)
		}
	}
	return e
}

func convertImageCollectionSlice(in []*mpr.ImageCollection, err error) ([]*types.ImageCollection, error) {
	if err != nil || in == nil {
		return nil, err
	}
	out := make([]*types.ImageCollection, len(in))
	for i, ic := range in {
		out[i] = convertImageCollection(ic)
	}
	return out, nil
}

func convertImageCollection(in *mpr.ImageCollection) *types.ImageCollection {
	ic := &types.ImageCollection{
		BaseElement:   in.BaseElement,
		ContainerID:   in.ContainerID,
		Name:          in.Name,
		ExportLevel:   in.ExportLevel,
		Documentation: in.Documentation,
	}
	if in.Images != nil {
		ic.Images = make([]types.Image, len(in.Images))
		for i, img := range in.Images {
			ic.Images[i] = types.Image{ID: img.ID, Name: img.Name, Data: img.Data, Format: img.Format}
		}
	}
	return ic
}

func unconvertImageCollection(in *types.ImageCollection) *types.ImageCollection {
	ic := &types.ImageCollection{
		BaseElement:   in.BaseElement,
		ContainerID:   in.ContainerID,
		Name:          in.Name,
		ExportLevel:   in.ExportLevel,
		Documentation: in.Documentation,
	}
	if in.Images != nil {
		ic.Images = make([]types.Image, len(in.Images))
		for i, img := range in.Images {
			ic.Images[i] = types.Image{ID: img.ID, Name: img.Name, Data: img.Data, Format: img.Format}
		}
	}
	return ic
}
