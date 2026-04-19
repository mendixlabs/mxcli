// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/mpr/version"
)

// ---------------------------------------------------------------------------
// Conversion helpers: sdk/mpr -> mdl/types
// ---------------------------------------------------------------------------

func convertMPRVersion(v mpr.MPRVersion) types.MPRVersion { return types.MPRVersion(v) }

func convertProjectVersion(v *version.ProjectVersion) *types.ProjectVersion {
	if v == nil {
		return nil
	}
	return &types.ProjectVersion{
		ProductVersion: v.ProductVersion,
		BuildVersion:   v.BuildVersion,
		FormatVersion:  v.FormatVersion,
		SchemaHash:     v.SchemaHash,
		MajorVersion:   v.MajorVersion,
		MinorVersion:   v.MinorVersion,
		PatchVersion:   v.PatchVersion,
	}
}

// passthrough helpers for types that sdk/mpr already returns as types.*.
func convertFolderInfoSlice(in []*types.FolderInfo, err error) ([]*types.FolderInfo, error) {
	return in, err
}

func convertUnitInfoSlice(in []*types.UnitInfo, err error) ([]*types.UnitInfo, error) {
	return in, err
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

func convertRawUnitSlice(in []*types.RawUnit, err error) ([]*types.RawUnit, error) {
	return in, err
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

func convertJavaActionSlice(in []*types.JavaAction, err error) ([]*types.JavaAction, error) {
	return in, err
}

func convertJavaScriptActionSlice(in []*types.JavaScriptAction, err error) ([]*types.JavaScriptAction, error) {
	return in, err
}

func convertJavaScriptActionPtr(in *types.JavaScriptAction, err error) (*types.JavaScriptAction, error) {
	return in, err
}

func convertNavDocSlice(in []*types.NavigationDocument, err error) ([]*types.NavigationDocument, error) {
	return in, err
}

func convertNavDocPtr(in *types.NavigationDocument, err error) (*types.NavigationDocument, error) {
	return in, err
}

func convertJsonStructureSlice(in []*types.JsonStructure, err error) ([]*types.JsonStructure, error) {
	return in, err
}

func convertJsonStructurePtr(in *types.JsonStructure, err error) (*types.JsonStructure, error) {
	return in, err
}

func convertImageCollectionSlice(in []*types.ImageCollection, err error) ([]*types.ImageCollection, error) {
	return in, err
}

// ---------------------------------------------------------------------------
// Conversion helpers: mdl/types -> sdk/mpr (for write methods)
// ---------------------------------------------------------------------------

func unconvertNavProfileSpec(s types.NavigationProfileSpec) mpr.NavigationProfileSpec {
	out := mpr.NavigationProfileSpec{
		LoginPage:    s.LoginPage,
		NotFoundPage: s.NotFoundPage,
		HasMenu:      s.HasMenu,
	}
	if s.HomePages != nil {
		out.HomePages = make([]mpr.NavHomePageSpec, len(s.HomePages))
		for i, hp := range s.HomePages {
			out.HomePages[i] = mpr.NavHomePageSpec{IsPage: hp.IsPage, Target: hp.Target, ForRole: hp.ForRole}
		}
	}
	if s.MenuItems != nil {
		out.MenuItems = make([]mpr.NavMenuItemSpec, len(s.MenuItems))
		for i, mi := range s.MenuItems {
			out.MenuItems[i] = unconvertNavMenuItemSpec(mi)
		}
	}
	return out
}

func unconvertNavMenuItemSpec(in types.NavMenuItemSpec) mpr.NavMenuItemSpec {
	out := mpr.NavMenuItemSpec{Caption: in.Caption, Page: in.Page, Microflow: in.Microflow}
	if in.Items != nil {
		out.Items = make([]mpr.NavMenuItemSpec, len(in.Items))
		for i, sub := range in.Items {
			out.Items[i] = unconvertNavMenuItemSpec(sub)
		}
	}
	return out
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
