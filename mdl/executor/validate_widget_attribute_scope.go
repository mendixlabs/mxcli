// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// validatePluggableAttributeScopes is the check-time half of ako/mxcli#647: a
// pluggable widget's unqualified attribute names, judged by the SAME rule exec
// writes them by (resolveBindingScope) and refused by the SAME function
// (misboundAttributeError).
//
// Before, check never asked which entity a widget attribute binds to, and exec
// answered "the last datasource" — so a two-datasource widget inside a data view
// passed check and failed mx check ten times over. Now exec binds by the
// widget's own rule, and a name that rule cannot place — an attribute of another
// entity in the widget's scope rather than of the one its property binds to — is
// refused here with the candidates, before a build cycle is spent on it.
//
// Inputs are the statically-known versions of what the engine sees live:
//
//   - the template's property links, loaded through the same backend call the
//     engine makes (LoadWidgetTemplate);
//   - the enclosing data container's entity, from the dataContext walk
//     validateFlowArguments already uses;
//   - each datasource's entity, from the same childContext rules;
//   - the domain model plus the entities this script creates.
//
// Anything it cannot work out — an unresolved context, a flow created later, a
// template that does not load — leaves the property unjudged rather than
// guessed at, so the check can only be quieter than exec, never louder.
//
// The same walk judges the BUILT-IN input widgets on the page, which need the
// same data context: an attribute of a type the widget does not take
// (MDL-WIDGET39, CE2421) and the classic drop-down on a React-client project
// (MDL-WIDGET40, CE0582). See validate_widget_attribute_type.go. Those need
// neither the widget registry nor the templates, so they run without them.
func validatePluggableAttributeScopes(ctx *ExecContext, layout string, params []ast.PageParameter, widgets []*ast.WidgetV3, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() || len(widgets) == 0 {
		return nil
	}
	registry := pageContextWidgetRegistry(ctx)
	var templates func(string) map[string]pages.PropertyTypeIDEntry
	if wb, ok := ctx.Backend.(backend.WidgetBuilderBackend); ok && registry != nil {
		templates = passTemplatePropertyTypes(ctx, wb)
	} else {
		registry = nil
	}
	sigs := buildFlowSignatures(ctx)
	if sc != nil {
		for name, sig := range sc.flowParams {
			sigs[name] = sig
		}
	}
	pageParams := make(map[string]string, len(params))
	for _, p := range params {
		if qn := p.EntityType.String(); qn != "" && qn != "." {
			pageParams[strings.ToLower(p.Name)] = qn
		}
	}
	v := &attributeScopeValidator{
		registry:    registry,
		templates:   templates,
		sigs:        sigs,
		pageParams:  pageParams,
		index:       checkAttributeIndex(ctx, sc),
		types:       checkMemberTypeIndex(ctx, sc),
		reactClient: usesReactClient(ctx) && !layoutIsNative(ctx, layout),
		assocs:      checkAssociationShapes(ctx, sc),
		pb:          &pageBuilder{ctx: ctx, backend: ctx.Backend, execCache: ctx.Cache},
	}
	if sc != nil {
		v.generalizations = sc.entityGeneralizations
		v.decls = sc.entityDecls
	}
	for _, w := range widgets {
		v.walk(w, dataContext{})
	}
	return v.errs
}

type attributeScopeValidator struct {
	registry   *WidgetRegistry
	templates  func(widgetID string) map[string]pages.PropertyTypeIDEntry
	sigs       map[string]*flowSignature
	pageParams map[string]string
	index      attributeIndex
	errs       []string

	// The built-in input widget checks (validate_widget_attribute_type.go).
	types       memberTypeIndex
	reactClient bool

	// assocs feeds MDL-ASSOCDS01 (validate_assoc_list_source.go).
	assocs map[string]assocShape

	// generalizations are the script's entities' EXTENDS targets (lower-cased
	// name -> generalization, "" for none), ahead of the project's, for
	// MDL-ASSOCDS03 (validate_assoc_hop_start.go).
	generalizations map[string]string

	// pb answers the association-as-attribute question with exec's own
	// function (validate_assoc_as_attribute.go).
	pb *pageBuilder

	// decls are the script's entity declarations, for the audit fields'
	// declared names (validate_expr_system_member.go).
	decls map[string]*ast.CreateEntityStmt
}

// paramEntity is the entity of the document parameter `$name`, or "".
func (v *attributeScopeValidator) paramEntity(name string) string {
	return v.pageParams[strings.ToLower(name)]
}

func (v *attributeScopeValidator) walk(w *ast.WidgetV3, enclosing dataContext) {
	if w == nil {
		return
	}
	inner := childContext(enclosing, w.GetDataSource(), v.sigs, v.pageParams)
	v.checkInputBinding(w, enclosing)
	v.checkTemplateParams(w, enclosing)
	v.checkAssocListSource(w, enclosing)
	v.checkViewAssocReverse(w, enclosing)
	v.checkAssocHopStart(w, enclosing)
	v.checkReactUnsupported(w)
	v.checkExprSystemMembers(w, enclosing)
	if v.registry == nil {
		// No registry: the pluggable half cannot run, and a pluggable widget's
		// slots cannot be told apart, so nothing inside one is judged rather
		// than judged against the wrong context.
		if w.Type == "pluggablewidget" || w.Type == "customwidget" {
			inner = enclosing.withUnresolved()
		}
	} else if def := lookupWidgetDef(w, v.registry); def != nil {
		named := v.checkWidget(w, def, enclosing)
		if named {
			// A widget with datasources named by key gives each child slot the
			// context its own link says; this walk does not model slots, so it
			// judges nothing inside rather than judging it wrongly.
			inner = enclosing.withUnresolved()
		}
	}
	for _, c := range w.Children {
		if isControlBar(c) {
			v.walk(c, enclosing)
			continue
		}
		v.walk(c, inner)
	}
}

// checkWidget judges one pluggable widget's attribute properties. It reports
// whether the widget names any datasource by its schema key.
func (v *attributeScopeValidator) checkWidget(w *ast.WidgetV3, def *WidgetDefinition, enclosing dataContext) (namedDataSource bool) {
	ids := v.templates(def.WidgetID)
	engine := &PluggableWidgetEngine{currentDef: def}
	mappings, _, err := engine.selectMappings(def, w)
	if err != nil {
		return false
	}

	// Each datasource's entity, keyed as the engine records it.
	dsEntities := map[string]string{}
	dsKeys := dataSourceMappingKeys(mappings)
	for _, m := range mappings {
		if m.Source != "DataSource" || m.PropertyKey == "" {
			continue
		}
		ds := namedDataSourceValue(m, w)
		if ds != nil {
			namedDataSource = true
		} else if len(dsKeys) <= 1 {
			ds = w.GetDataSource()
		}
		if entity := v.dataSourceEntity(ds); entity != "" {
			dsEntities[m.PropertyKey] = entity
		}
	}
	if len(dsKeys) == 0 && w.GetDataSource() != nil {
		// The engine's auto-datasource path: the template's one datasource.
		if keys := dataSourceTypedTemplateKeys(ids); len(keys) == 1 {
			if entity := v.dataSourceEntity(w.GetDataSource()); entity != "" {
				dsEntities[keys[0]] = entity
			}
		}
	}
	if len(ids) == 0 {
		return namedDataSource
	}

	outer := ""
	if !enclosing.unresolved && len(enclosing.entities) > 0 {
		outer = enclosing.entities[len(enclosing.entities)-1]
	}

	// The value each attribute property ends up with: the mapping pass first,
	// then a property authored by its own schema key, which the engine applies
	// after it (and which therefore wins).
	values := map[string]string{}
	var order []string
	set := func(key, val string) {
		if _, seen := values[key]; !seen {
			order = append(order, key)
		}
		values[key] = val
	}
	sources := map[string]bool{}
	fromMapping := map[string]bool{}
	for _, m := range mappings {
		if m.Source != "" {
			sources[m.Source] = true
		}
		if m.Source != "Attribute" {
			continue
		}
		val := namedPropValue(m, w)
		if val == "" && engine.isPrimaryAttributeMapping(m) {
			val = w.GetAttribute()
		}
		if val != "" {
			set(m.PropertyKey, val)
			fromMapping[m.PropertyKey] = true
		}
	}
	for propName, propVal := range w.Properties {
		if sources[propName] || isBuiltinPropName(propName) {
			continue
		}
		entry, ok := ids[propName]
		if !ok || entry.ValueType != "Attribute" {
			continue
		}
		if s, ok := propVal.(string); ok && s != "" {
			set(propName, s)
		}
	}

	sort.Strings(order)
	for _, key := range order {
		entry, declared := ids[key]
		scope := resolveBindingScope(entry.DataSourceProperty, declared, outer, dsEntities, "")
		// An association where the attribute belongs: exec refuses it in the
		// engine's Attribute mapping, so only a value that mapping reads is
		// judged (mendixlabs/mxcli#1440).
		if fromMapping[key] {
			if err := associationAsAttributeError(v.pb, w, key, values[key], scope.Entity, v.paramEntity); err != nil {
				v.errs = append(v.errs, err.Error())
				continue
			}
		}
		if err := misboundAttributeError(
			"widget `"+w.Name+"` property `"+key+"`",
			values[key], scope, outer, dsEntities, v.index,
		); err != nil {
			v.errs = append(v.errs, err.Error())
		}
	}
	return namedDataSource
}

// dataSourceEntity is the entity a datasource supplies, by the childContext
// rules; "" when that cannot be worked out.
func (v *attributeScopeValidator) dataSourceEntity(ds *ast.DataSourceV3) string {
	if ds == nil {
		return ""
	}
	c := childContext(dataContext{}, ds, v.sigs, v.pageParams)
	if c.unresolved || len(c.entities) != 1 {
		return ""
	}
	return c.entities[0]
}

// passTemplatePropertyTypes is templatePropertyTypes shared by the statements
// of one check. Each call of templatePropertyTypes starts an empty cache, and
// it was called per statement, so "once per widget for the run" was once per
// widget per statement: a template load, with its .mpk augmentation, for every
// widget of every page checked (ako/mxcli#2160).
func passTemplatePropertyTypes(ctx *ExecContext, wb backend.WidgetBuilderBackend) func(string) map[string]pages.PropertyTypeIDEntry {
	memo := ctx.checkMemo
	if memo == nil {
		return templatePropertyTypes(wb, ctx.Backend.Path())
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	if memo.templates == nil {
		memo.templates = templatePropertyTypes(wb, ctx.Backend.Path())
	}
	return memo.templates
}

// templatePropertyTypes loads a widget's template property metadata through the
// backend call the engine uses, once per widget for the run.
func templatePropertyTypes(wb backend.WidgetBuilderBackend, projectPath string) func(string) map[string]pages.PropertyTypeIDEntry {
	var mu sync.Mutex
	cache := map[string]map[string]pages.PropertyTypeIDEntry{}
	return func(widgetID string) map[string]pages.PropertyTypeIDEntry {
		mu.Lock()
		defer mu.Unlock()
		if ids, ok := cache[widgetID]; ok {
			return ids
		}
		var ids map[string]pages.PropertyTypeIDEntry
		if builder, err := wb.LoadWidgetTemplate(widgetID, projectPath); err == nil && builder != nil {
			ids = builder.PropertyTypeIDs()
		}
		cache[widgetID] = ids
		return ids
	}
}

// checkAttributeIndex is the attributeIndex over the project's domain model,
// overlaid with the entities this script creates.
func checkAttributeIndex(ctx *ExecContext, sc *scriptContext) attributeIndex {
	ix := attributeIndex{owners: map[string]map[string]bool{}, parents: map[string]string{}}
	for qn, ent := range buildEntityIndex(ctx) {
		attrs := make(map[string]bool, len(ent.Attributes))
		for _, a := range ent.Attributes {
			attrs[strings.ToLower(a.Name)] = true
		}
		ix.owners[qn] = attrs
		if ent.GeneralizationRef != "" {
			ix.parents[qn] = ent.GeneralizationRef
		}
	}
	if sc != nil {
		for qn, names := range sc.entityAttrs {
			attrs := make(map[string]bool, len(names))
			for n := range names {
				attrs[strings.ToLower(n)] = true
			}
			ix.owners[qn] = attrs
			delete(ix.parents, qn)
			if gen := sc.entityGeneralizations[strings.ToLower(qn)]; gen != "" {
				ix.parents[qn] = gen
			}
		}
	}
	return ix
}
