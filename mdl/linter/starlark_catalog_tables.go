// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Starlark builtins over the catalog tables that had no typed accessor
// (mendixlabs/mxcli#1265, #1269). See context_catalog_tables.go for the
// iterators and the filtering they apply.

func (r *StarlarkRule) builtinAssociations(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for a := range r.ctx.Associations() {
		out = append(out, associationToStarlark(a))
	}
	return starlark.NewList(out), nil
}

func (r *StarlarkRule) builtinEntityEventHandlers(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for h := range r.ctx.EntityEventHandlers() {
		out = append(out, entityEventHandlerToStarlark(h))
	}
	return starlark.NewList(out), nil
}

func (r *StarlarkRule) builtinNavigationMenuItems(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for n := range r.ctx.NavigationMenuItems() {
		out = append(out, navigationMenuItemToStarlark(n))
	}
	return starlark.NewList(out), nil
}

func (r *StarlarkRule) builtinJarDependencies(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for j := range r.ctx.JarDependencies() {
		out = append(out, jarDependencyToStarlark(j))
	}
	return starlark.NewList(out), nil
}

// builtinStrings is strings(language = None). The table is filled only by a
// FULL catalog build, which is why "strings" is in fullBuiltins.
func (r *StarlarkRule) builtinStrings(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var lang starlark.Value = starlark.None
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "language?", &lang); err != nil {
		return nil, err
	}
	language := ""
	switch v := lang.(type) {
	case starlark.NoneType:
	case starlark.String:
		language = string(v)
	default:
		return nil, fmt.Errorf("%s: language must be a string or None, got %s", b.Name(), lang.Type())
	}
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for s := range r.ctx.Strings(language) {
		out = append(out, catalogStringToStarlark(s))
	}
	return starlark.NewList(out), nil
}

func (r *StarlarkRule) builtinLayouts(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for l := range r.ctx.Layouts() {
		out = append(out, layoutToStarlark(l))
	}
	return starlark.NewList(out), nil
}

func (r *StarlarkRule) builtinPublishedRestOperations(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for o := range r.ctx.PublishedRestOperations() {
		out = append(out, publishedRestOperationToStarlark(o))
	}
	return starlark.NewList(out), nil
}

func (r *StarlarkRule) builtinModules(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for m := range r.ctx.Modules() {
		out = append(out, moduleToStarlark(m))
	}
	return starlark.NewList(out), nil
}

func associationToStarlark(a Association) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("association"), starlark.StringDict{
		"name":                      starlark.String(a.Name),
		"qualified_name":            starlark.String(a.QualifiedName),
		"module_name":               starlark.String(a.ModuleName),
		"from_entity":               starlark.String(a.FromEntity),
		"to_entity":                 starlark.String(a.ToEntity),
		"type":                      starlark.String(a.Type),
		"owner":                     starlark.String(a.Owner),
		"storage_format":            starlark.String(a.StorageFormat),
		"description":               starlark.String(a.Description),
		"to_delete_behavior":        starlark.String(a.ToDeleteBehavior),
		"from_delete_behavior":      starlark.String(a.FromDeleteBehavior),
		"to_delete_error_message":   starlark.String(a.ToDeleteErrorMessage),
		"from_delete_error_message": starlark.String(a.FromDeleteErrorMessage),
	})
}

func entityEventHandlerToStarlark(h EntityEventHandler) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("entity_event_handler"), starlark.StringDict{
		"entity":               starlark.String(h.Entity),
		"module_name":          starlark.String(h.ModuleName),
		"moment":               starlark.String(h.Moment),
		"event":                starlark.String(h.Event),
		"microflow":            starlark.String(h.Microflow),
		"raise_error_on_false": starlark.Bool(h.RaiseErrorOnFalse),
		"pass_event_object":    starlark.Bool(h.PassEventObject),
	})
}

func navigationMenuItemToStarlark(n NavigationMenuItem) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("navigation_menu_item"), starlark.StringDict{
		"profile":          starlark.String(n.Profile),
		"item_path":        starlark.String(n.ItemPath),
		"depth":            starlark.MakeInt(n.Depth),
		"caption":          starlark.String(n.Caption),
		"action_type":      starlark.String(n.ActionType),
		"target_page":      starlark.String(n.TargetPage),
		"target_microflow": starlark.String(n.TargetMicroflow),
	})
}

func jarDependencyToStarlark(j JarDependency) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("jar_dependency"), starlark.StringDict{
		"module_name": starlark.String(j.ModuleName),
		"group_id":    starlark.String(j.GroupID),
		"artifact_id": starlark.String(j.ArtifactID),
		"version":     starlark.String(j.Version),
		"coordinate":  starlark.String(j.Coordinate),
		"is_included": starlark.Bool(j.IsIncluded),
	})
}

func catalogStringToStarlark(s CatalogString) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("catalog_string"), starlark.StringDict{
		"qualified_name": starlark.String(s.QualifiedName),
		"object_type":    starlark.String(s.ObjectType),
		"value":          starlark.String(s.Value),
		"context":        starlark.String(s.Context),
		"language":       starlark.String(s.Language),
		"element_id":     starlark.String(s.ElementID),
		"module_name":    starlark.String(s.ModuleName),
	})
}

func layoutToStarlark(l Layout) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("layout"), starlark.StringDict{
		"name":           starlark.String(l.Name),
		"qualified_name": starlark.String(l.QualifiedName),
		"module_name":    starlark.String(l.ModuleName),
		"folder":         starlark.String(l.Folder),
		"layout_type":    starlark.String(l.LayoutType),
		"platform":       starlark.String(l.Platform),
		"description":    starlark.String(l.Description),
		// Named class_name because class is a Starlark keyword, as on a widget.
		"class_name":      starlark.String(l.Class),
		"style":           starlark.String(l.Style),
		"dynamic_classes": starlark.String(l.DynamicClasses),
	})
}

func publishedRestOperationToStarlark(o PublishedRestOperation) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("published_rest_operation"), starlark.StringDict{
		"service":     starlark.String(o.Service),
		"resource":    starlark.String(o.Resource),
		"http_method": starlark.String(o.HTTPMethod),
		"path":        starlark.String(o.Path),
		"summary":     starlark.String(o.Summary),
		"microflow":   starlark.String(o.Microflow),
		"deprecated":  starlark.Bool(o.Deprecated),
		"module_name": starlark.String(o.ModuleName),
	})
}

func moduleToStarlark(m Module) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("module"), starlark.StringDict{
		"id":                         starlark.String(m.ID),
		"name":                       starlark.String(m.Name),
		"domain_model_documentation": starlark.String(m.DomainModelDocumentation),
	})
}
