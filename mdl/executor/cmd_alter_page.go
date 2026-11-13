// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// execAlterPage handles ALTER PAGE/SNIPPET Module.Name { operations }.
func execAlterPage(ctx *ExecContext, s *ast.AlterPageStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	// A parameter named with a platform-reserved word is CE7247 (MDL-PARAM01,
	// ako/mxcli#1651) — refused here too, so --no-check cannot write it.
	if err := refuseReservedParameterNames(s); err != nil {
		return err
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	unitID, containerID, containerType, err := resolveAlterPageUnit(ctx, s, h)
	if err != nil {
		return err
	}

	if containerType == "layout" {
		// The same refusal CREATE LAYOUT makes, for the same reason: a
		// Marketplace update replaces the module wholesale, so an edit here is
		// gone at the next update with nothing to show it ever happened.
		if mod, _ := ctx.Backend.GetModule(containerID); isMarketplaceModule(ctx, mod) {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"layout %s is in a marketplace module — an edit there is overwritten by the next module update. "+
					"Copy it into a module of your own first: `describe layout %s`, rename it, run it, "+
					"then repoint pages with `alter pages set layout = <yours> where layout = %s`",
				s.PageName.String(), s.PageName.String(), s.PageName.String()))
		}
	}

	if containerType == "page" {
		if err := checkAddedParameterURLSegments(ctx, s, h); err != nil {
			return err
		}
	}

	// Texts are written in the project's default language (the one DESCRIBE
	// shows): resolve it before the first mutation, since the mutator reads it
	// through model.AuthoringLanguage rather than as a parameter.
	authoringLanguage(ctx)

	// Open the page for mutation via the backend
	mutator, err := ctx.Backend.OpenPageForMutation(unitID)
	if err != nil {
		return mdlerrors.NewBackend("open "+strings.ToLower(containerType)+" for mutation", err)
	}

	// Resolve module name for building new widgets
	modName := h.GetModuleName(containerID)

	// The stored document as describe prints it, parsed back — read once, and
	// only when a REPLACE needs a pluggable widget's baseline (#1247).
	var described map[string]*ast.WidgetV3
	describedOnce := false
	storedWidgets := func() map[string]*ast.WidgetV3 {
		if !describedOnce {
			describedOnce = true
			described = describedStoredWidgets(ctx, unitID, containerType, s.PageName)
		}
		return described
	}

	for _, op := range s.Operations {
		// Every target resolves through the document type's resolver before the
		// operation runs (ADR-0012): what an address means is answered once,
		// per document type, and a miss or an ambiguity stops the statement
		// before anything changes.
		if err := resolveAlterPageTargets(mutator, op); err != nil {
			return mdlerrors.NewBackend("resolve "+strings.ToLower(containerType)+" target", err)
		}
		switch o := op.(type) {
		case *ast.SetPropertyOp:
			if err := applySetPropertyMutator(ctx, mutator, o, modName, containerID); err != nil {
				return mdlerrors.NewBackend("set", err)
			}
		case *ast.InsertWidgetOp:
			if err := applyInsertWidgetMutator(ctx, mutator, o, modName, containerID); err != nil {
				return mdlerrors.NewBackend("insert", err)
			}
		case *ast.DropWidgetOp:
			if err := applyDropWidgetMutator(mutator, o); err != nil {
				return mdlerrors.NewBackend("drop", err)
			}
		case *ast.DropListViewTemplateOp:
			if err := mutator.DropListViewTemplate(o.ListView, o.Specialization); err != nil {
				return mdlerrors.NewBackend("drop TEMPLATE", err)
			}
		case *ast.ReplaceWidgetOp:
			if err := applyReplaceWidgetMutator(ctx, mutator, o, modName, containerID, storedWidgets); err != nil {
				return mdlerrors.NewBackend("replace", err)
			}
		case *ast.AddVariableOp:
			if err := mutator.AddVariable(o.Variable.Name, o.Variable.DataType, o.Variable.DefaultValue); err != nil {
				return mdlerrors.NewBackend("add VARIABLE", err)
			}
		case *ast.DropVariableOp:
			if err := mutator.DropVariable(o.VariableName); err != nil {
				return mdlerrors.NewBackend("drop VARIABLE", err)
			}
		case *ast.AddParameterOp:
			spec, err := alterParameterSpec(ctx, s, containerType, o.Parameter)
			if err != nil {
				return err
			}
			if err := mutator.AddParameter(spec); err != nil {
				return mdlerrors.NewBackend("add PARAMETER", err)
			}
		case *ast.DropParameterOp:
			if err := mutator.DropParameter(o.ParameterName); err != nil {
				return mdlerrors.NewBackend("drop PARAMETER", err)
			}
		case *ast.SetLayoutOp:
			if containerType == "snippet" {
				return mdlerrors.NewUnsupported("set Layout is not supported for snippets")
			}
			newLayoutQN := o.NewLayout.Module + "." + o.NewLayout.Name
			if err := checkRepointPlaceholders(ctx, mutator, o, newLayoutQN); err != nil {
				return err
			}
			if err := mutator.SetLayout(newLayoutQN, o.Mappings); err != nil {
				return mdlerrors.NewBackend("set Layout", err)
			}
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown alter %s operation type: %T", containerType, op))
		}
	}

	// Persist
	if err := mutator.Save(); err != nil {
		return mdlerrors.NewBackend("save modified "+strings.ToLower(containerType), err)
	}

	// The report follows what landed, not what was attempted: storage elides a
	// save whose content equals what is stored (ADR-0008), so re-running an
	// applied alter must say "Unchanged", not "Altered" (ako/mxcli#2156).
	ctx.ReportMutation("Altered", "%s %s", strings.ToLower(containerType), s.PageName.String())
	return nil
}

// checkAddedParameterURLSegments refuses `add parameters` on a page whose URL
// would then lack a `{Name}` segment for the new parameter — MDL-PAGE20 on
// CREATE, CE5601 from mxbuild. Measured on 11.12.1: adding $Order to a page
// with Url 'customer-edit/{Customer}' and nothing else fails `mx check` with
// "The URL property of this Page is missing a parameter segment for parameter
// "Order"." The URL a statement ends with is what counts, so a `set (Url: …)`
// in the same statement that names the parameter satisfies it. Parameters the
// page already had are not this statement's business.
func checkAddedParameterURLSegments(ctx *ExecContext, s *ast.AlterPageStmt, h *ContainerHierarchy) error {
	var added []string
	url, urlSet := "", false
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.AddParameterOp:
			added = append(added, o.Parameter.Name)
		case *ast.SetPropertyOp:
			if o.Target.Widget != "" {
				continue
			}
			for k, v := range o.Properties {
				if strings.EqualFold(k, "Url") {
					url, _ = v.(string)
					urlSet = true
				}
			}
		}
	}
	if len(added) == 0 {
		return nil
	}
	if !urlSet {
		page, err := findPageByName(ctx, s.PageName, h)
		if err != nil {
			return err
		}
		url = page.URL
	}
	if url == "" {
		return nil
	}
	var missing []string
	suggested := url
	for _, name := range added {
		if !urlBindsParameter(url, name) {
			missing = append(missing, name)
			suggested = strings.TrimRight(suggested, "/") + "/{" + name + "}"
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf(
		"page %s has Url '%s' with no segment for parameter %s — Mendix binds each page "+
			"parameter from the URL, and mxbuild rejects the page (CE5601). Set the URL in the "+
			"same statement: `set (Url: '%s');`",
		s.PageName.String(), url, quoteList(missing), suggested)
}

// alterParameterSpec resolves an `add parameters` declaration the way CREATE
// resolves its Params clause (cmd_pages_builder_v3.go): a primitive becomes its
// DataTypes $Type, anything else must name an entity that exists, and a snippet
// takes entities only (MDL087 — mxbuild's CE0046 otherwise).
func alterParameterSpec(ctx *ExecContext, s *ast.AlterPageStmt, containerType string, p ast.PageParameter) (backend.PageParameterSpec, error) {
	spec := backend.PageParameterSpec{Name: p.Name, Optional: p.Optional}
	bsonType := pageParamBSONType(p.Type)
	if strings.EqualFold(containerType, "snippet") {
		if p.Optional {
			return spec, mdlerrors.NewValidation(optionalSnippetParameterMessage(s.PageName, p.Name))
		}
		if caption := types.SnippetParameterTypeRule(bsonType); caption != "" {
			return spec, mdlerrors.NewValidationf(
				"snippet '%s' cannot take parameter $%s with the primitive type %s — a snippet "+
					"parameter must be an entity, and mxbuild rejects a primitive one with "+
					"CE0046 (\"Invalid data type '%s'.\"). Pass the value on an object, or "+
					"keep the primitive on the calling page's parameters.",
				s.PageName.String(), p.Name, paramTypeSourceName(p.Type), caption)
		}
	}
	if bsonType != "" {
		spec.PrimitiveType = bsonType
		return spec, nil
	}
	if p.EntityType.Name == "" || p.EntityType.Module == "" {
		return spec, mdlerrors.NewValidationf(
			"parameter $%s: a page parameter is an entity (Module.Entity) or one of "+
				"String, Integer, Long, Decimal, Boolean, DateTime", p.Name)
	}
	if _, err := findEntity(ctx, p.EntityType.Module, p.EntityType.Name); err != nil {
		return spec, mdlerrors.NewBackend("resolve entity "+p.EntityType.String(), err)
	}
	spec.EntityName = p.EntityType.String()
	return spec, nil
}

// alterTargetOf converts an operation's target, as the visitor recorded it,
// into the backend's document-independent address.
func alterTargetOf(r ast.WidgetRef) backend.AlterTarget {
	if r.IsColumnAddress() {
		// The @n belongs to the column address, which carries it to the
		// mutator; on the target itself it would mean a choice among widgets.
		return backend.AlterTarget{Path: []string{r.Widget, columnRefOf(r)}}
	}
	t := backend.AlterTarget{Caption: r.Caption, Ordinal: r.Ordinal}
	if r.Caption == "" {
		t.Path = []string{r.Widget}
		if r.Column != "" {
			t.Path = append(t.Path, r.Column)
		}
	}
	return t
}

// columnRefOf is the columnRef the PageMutator takes for a target's member: the
// derived column name or region of `grid.Member`, or the explicit column
// address `grid column(…)[@n]` spelled as backend.ColumnSelector (#749). Empty
// for a plain widget target.
func columnRefOf(r ast.WidgetRef) string {
	if r.IsColumnAddress() {
		return backend.ColumnSelector{Attribute: r.ColumnAttribute, Caption: r.ColumnCaption, Ordinal: r.Ordinal}.String()
	}
	return r.Column
}

// alterPageOperationTargets lists the targets an operation addresses, in the
// order it names them. A page-level SET and the variable and layout operations
// address the document itself and have none.
func alterPageOperationTargets(op ast.AlterPageOperation) []ast.WidgetRef {
	switch o := op.(type) {
	case *ast.SetPropertyOp:
		if o.Target.Widget == "" && o.Target.Caption == "" {
			return nil
		}
		return []ast.WidgetRef{o.Target}
	case *ast.InsertWidgetOp:
		return []ast.WidgetRef{o.Target}
	case *ast.ReplaceWidgetOp:
		return []ast.WidgetRef{o.Target}
	case *ast.DropWidgetOp:
		return o.Targets
	case *ast.DropListViewTemplateOp:
		return []ast.WidgetRef{{Widget: o.ListView}}
	}
	return nil
}

// resolveAlterPageTargets resolves every target of one operation. A drop's
// targets are resolved together up front: an unresolvable second target then
// refuses the whole drop instead of leaving the first one dropped.
func resolveAlterPageTargets(resolver backend.AlterTargetResolver, op ast.AlterPageOperation) error {
	for _, ref := range alterPageOperationTargets(op) {
		if _, err := resolver.ResolveAlterTarget(alterTargetOf(ref)); err != nil {
			return err
		}
	}
	return nil
}

// resolveAlterPageUnit resolves an ALTER PAGE / SNIPPET / LAYOUT target to the
// storage unit it edits and the module holding it. One statement type, three
// document kinds — the visitor sets ContainerType from the keyword, and an empty
// one means PAGE.
//
// It is shared with the check-time dry run (validate_alter_set.go) so both
// address the same document: a pre-flight that resolved the target differently
// from exec would be checking a different page than the one about to change.
func resolveAlterPageUnit(ctx *ExecContext, s *ast.AlterPageStmt, h *ContainerHierarchy) (unitID, containerID model.ID, containerType string, err error) {
	containerType = strings.ToLower(s.ContainerType)
	if containerType == "" {
		containerType = "page"
	}
	switch containerType {
	case "snippet":
		snippet, modID, err := findSnippetByName(ctx, s.PageName, h)
		if err != nil {
			return "", "", containerType, err
		}
		return snippet.ID, modID, containerType, nil
	case "layout":
		layout, err := findLayoutByQName(ctx, s.PageName)
		if err != nil {
			return "", "", containerType, err
		}
		return layout.ID, h.FindModuleID(layout.ContainerID), containerType, nil
	default:
		page, err := findPageByName(ctx, s.PageName, h)
		if err != nil {
			return "", "", containerType, err
		}
		return page.ID, h.FindModuleID(page.ContainerID), containerType, nil
	}
}

// ============================================================================
// SET property via mutator
// ============================================================================

func applySetPropertyMutator(ctx *ExecContext, mutator backend.PageMutator, op *ast.SetPropertyOp, moduleName string, moduleID model.ID) error {
	// A column's attribute is written last. The column is addressed by its
	// attribute, so once it changes, `column(Old)` no longer finds it, and the
	// follow-up below — which re-finds the column — failed with "column(Old)
	// not found". The attribute drives none of the visibility the follow-up
	// resets, so the rest is applied and reset under the address as written
	// (ako/mxcli#1644).
	if op.Target.IsColumn() {
		for name, value := range op.Properties {
			if !strings.EqualFold(name, "Attribute") {
				continue
			}
			rest := &ast.SetPropertyOp{Target: op.Target, Properties: map[string]any{}}
			for k, v := range op.Properties {
				if k != name {
					rest.Properties[k] = v
				}
			}
			if len(rest.Properties) > 0 {
				if err := applySetPropertyMutator(ctx, mutator, rest, moduleName, moduleID); err != nil {
					return err
				}
			}
			return applySetProperties(ctx, mutator,
				&ast.SetPropertyOp{Target: op.Target, Properties: map[string]any{name: value}}, moduleName, moduleID)
		}
	}
	if err := applySetProperties(ctx, mutator, op, moduleName, moduleID); err != nil {
		return err
	}
	if err := checkDateFormatAfterSet(mutator, op); err != nil {
		return err
	}
	// A Gallery's row click is judged on the widget as the SET leaves it —
	// the statement may name only one of the three settings involved.
	if err := checkGalleryClickAfterSet(ctx, mutator, op); err != nil {
		return err
	}
	if op.Target.IsColumn() || op.Target.Item != nil {
		names := make([]string, 0, len(op.Properties))
		for k := range op.Properties {
			names = append(names, k)
		}
		sort.Strings(names)
		if op.Target.Item != nil {
			return resetHiddenItemProperties(ctx, mutator, op.Target, names)
		}
		return resetHiddenColumnProperties(ctx, mutator, op.Target, names)
	}
	return nil
}

// dateFormatChecker is the page mutator capability behind
// checkDateFormatAfterSet; engines without it are left as they were.
type dateFormatChecker interface {
	CheckDateFormat(widgetRef string) error
}

// checkDateFormatAfterSet refuses a SET that leaves a picker Custom with no
// pattern (CE0493). Only a statement that sets DateFormat or CustomDateFormat
// is checked, so an unrelated edit to a widget already in that state still
// goes through (ako/mxcli#977).
func checkDateFormatAfterSet(mutator backend.PageMutator, op *ast.SetPropertyOp) error {
	checker, ok := mutator.(dateFormatChecker)
	if !ok || op.Target.IsColumn() || op.Target.Item != nil || op.Target.Widget == "" {
		return nil
	}
	for name := range op.Properties {
		if strings.EqualFold(name, "DateFormat") || strings.EqualFold(name, "CustomDateFormat") {
			if err := checker.CheckDateFormat(op.Target.Widget); err != nil {
				return mdlerrors.NewValidation("set on " + op.Target.Name() + ": " + err.Error())
			}
			return nil
		}
	}
	return nil
}

// itemHiddenPropertyResetter is the page mutator capability behind
// resetHiddenItemProperties; engines without it are left as they were.
type itemHiddenPropertyResetter interface {
	ResetHiddenItemProperties(itemRef string, rules []types.WidgetVisibilityRule, setProps []string) ([]string, error)
}

// resetHiddenItemProperties is resetHiddenColumnProperties for an object-list
// item (`acc1 item@1`): after the SET, every item property the widget's
// editorConfig now hides goes back to how CREATE stores a hidden one.
// `set (headerRenderMode: 'custom')` hid the group's headerText and left its
// template filled: CE0463 at the Accordion. The rules are the installed
// package's, found from the item's own widget id.
func resetHiddenItemProperties(ctx *ExecContext, mutator backend.PageMutator, target ast.WidgetRef, setProps []string) error {
	resetter, ok := mutator.(itemHiddenPropertyResetter)
	if !ok {
		return nil
	}
	valuer, ok := mutator.(objectListItemValuer)
	if !ok {
		return nil
	}
	vals, ok := valuer.ObjectListItemValues(target.Widget)
	if !ok {
		return nil
	}
	rules := resolveWidgetVisibilityRules(ctx.MprPath, vals.WidgetID)
	if len(rules) == 0 {
		return nil
	}
	if _, err := resetter.ResetHiddenItemProperties(target.Widget, rules, setProps); err != nil {
		return mdlerrors.NewBackend("set on "+target.Name(), err)
	}
	return nil
}

// applySetProperties writes each property of a SET, without the column
// follow-up — what check's per-property dry run exercises, since a column's
// hidden-property rules can only be judged on the statement as a whole.
func applySetProperties(ctx *ExecContext, mutator backend.PageMutator, op *ast.SetPropertyOp, moduleName string, moduleID model.ID) error {
	// Sort property names for deterministic application order.
	propNames := make([]string, 0, len(op.Properties))
	for k := range op.Properties {
		propNames = append(propNames, k)
	}
	sort.Strings(propNames)

	for _, propName := range propNames {
		value := op.Properties[propName]
		_, isActionValue := value.(*ast.ActionV3)
		if isActionValue && propName != "Action" &&
			(op.Target.Widget == "" || op.Target.IsColumn()) {
			// A named action slot belongs to a pluggable widget. The column and
			// page-level setters would stringify the action into a scalar.
			return mdlerrors.NewValidationf(
				"`set %s = <action>` needs a pluggable widget target: `set '%s' = … on <widgetName>`",
				propName, propName)
		}
		if params, ok := value.([]ast.ParamAssignmentV3); ok && isTemplateParamsKey(propName) {
			if err := setTemplateParameters(ctx, mutator, op.Target, propName, params, moduleName, moduleID); err != nil {
				return err
			}
			continue
		}
		if op.Target.IsColumn() {
			if strings.EqualFold(propName, "Attribute") {
				b, err := resolveColumnAttribute(ctx, mutator, op.Target, value, moduleName, moduleID)
				if err != nil {
					return err
				}
				value = b
			}
			if err := mutator.SetColumnProperty(op.Target.Widget, columnRefOf(op.Target), propName, value); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		} else if propName == "DataSource" {
			// DataSource requires special handling via SetWidgetDataSource
			ds, err := convertASTDataSource(value)
			if err != nil {
				return err
			}
			if err := mutator.SetWidgetDataSource(op.Target.Widget, ds); err != nil {
				return mdlerrors.NewBackend("set DataSource on "+op.Target.Name(), err)
			}
		} else if propName == "Action" || (isActionValue && isClickActionKey(propName)) {
			// Action is a polymorphic node, not a scalar — it goes through the
			// same builder CREATE PAGE uses rather than being written as a value.
			// `onClick:` is its alias, as in CREATE PAGE; on a pluggable widget
			// both reach the slot its definition reads `onClick:` from
			// (ako/mxcli#854).
			action, err := convertASTAction(ctx, mutator, value, moduleName, moduleID, op.Target.Widget)
			if err != nil {
				return err
			}
			if err := setClickAction(mutator, op.Target.Widget, propName, action); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		} else if isActionValue {
			// Any other key carrying an action is a pluggable widget's NAMED
			// action slot — `set 'createFileAction' = microflow M.F` (#995).
			// Same builder as `Action`; the mutator checks the key is an
			// action-typed property of the stored widget. Through
			// SetWidgetProperty it would be stringified into a PrimitiveValue.
			action, err := convertASTAction(ctx, mutator, value, moduleName, moduleID, op.Target.Widget)
			if err != nil {
				return err
			}
			if err := mutator.SetWidgetNamedAction(op.Target.Widget, propName, action); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		} else if p := designPropertyForStoredWidget(
			documentTheme(ctx, mutator), mutator, op.Target.Widget, propName); p != nil {
			// An Atlas design property of THIS stored widget. It lives in
			// Appearance.DesignProperties, which SetWidgetProperty does not
			// reach, so before ako/mxcli#515 this dead-ended and ALTER STYLING
			// was the only spelling that worked.
			//
			// Routed only when the project's theme declares the key FOR THIS
			// WIDGET's type; anything else falls through to the setter below and
			// keeps that path's error, so a mistyped pluggable key is still a
			// mistyped pluggable key rather than a silently-written design
			// property.
			if err := applyDesignPropertySet(mutator, op.Target, p, value); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		} else {
			// An attribute property of a pluggable widget is bound here, by the
			// rule CREATE uses; the mutator only writes it (#1622).
			resolved, err := resolvePluggableSetValue(ctx, mutator, op.Target.Widget, propName, value, moduleName, moduleID)
			if err != nil {
				return err
			}
			if err := mutator.SetWidgetProperty(op.Target.Widget, propName, resolved); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		}
	}
	return nil
}

// resetHiddenColumnProperties puts every column property the Data grid 2 now
// hides back at its declared default, once the statement's own values are in.
// `set (ColumnWidth: autoFill)` after a manual width left `size` at 220, which
// the widget hides unless the width is manual: CE0463 (mendixlabs/mxcli#1519).
// The rules are the installed package's editorConfig — which property depends
// on which is the widget's to say, not a list kept here.
func resetHiddenColumnProperties(ctx *ExecContext, mutator backend.PageMutator, target ast.WidgetRef, setProps []string) error {
	resetter, ok := mutator.(backend.ColumnHiddenPropertyResetter)
	if !ok {
		return nil
	}
	rules := resolveWidgetVisibilityRules(ctx.MprPath, pages.WidgetIDDataGrid2)
	if len(rules) == 0 {
		return nil
	}
	if _, err := resetter.ResetHiddenColumnProperties(target.Widget, columnRefOf(target), rules, setProps); err != nil {
		return mdlerrors.NewBackend("set on "+target.Name(), err)
	}
	return nil
}

// convertASTDataSource converts an AST DataSource value to a pages.DataSource.
func convertASTDataSource(value interface{}) (pages.DataSource, error) {
	ds, ok := value.(*ast.DataSourceV3)
	if !ok {
		return nil, mdlerrors.NewValidation("DataSource value must be a datasource expression")
	}

	switch ds.Type {
	case "selection":
		return &pages.ListenToWidgetSource{WidgetName: ds.Reference}, nil
	case "database":
		if ds.AssociationPath != "" {
			// The entity it arrives at and the SourceVariable slot are resolved by
			// the CREATE PAGE builder, which REPLACE reaches (ako/mxcli#721 L5).
			return nil, mdlerrors.NewUnsupported(
				"alter page set DataSource = database from $" + ds.ContextVariable + "/" + ds.AssociationPath +
					" is not supported — use `replace <widget> with …`, which rebuilds the widget " +
					"through the CREATE PAGE path")
		}
		return &pages.DatabaseSource{EntityName: ds.Reference}, nil
	case "microflow":
		return &pages.MicroflowSource{Microflow: ds.Reference, FormValidations: ds.FormValidations}, nil
	case "nanoflow":
		return &pages.NanoflowSource{Nanoflow: ds.Reference}, nil
	case "parameter":
		// "Data from context". Only the parameter name is known here; the entity
		// it is typed with lives in the container's own Parameters list, which
		// the mutator holds — it resolves and validates the name (#855).
		return &pages.DataViewSource{ParameterName: strings.TrimPrefix(ds.Reference, "$")}, nil
	default:
		// Name the way out. The REPLACE path rebuilds the widget through the
		// CREATE PAGE builder, which handles every datasource type, so an
		// unsupported SET is an inconvenience rather than a dead end — saying so
		// beats leaving the author to discover it (#855).
		return nil, mdlerrors.NewUnsupported(fmt.Sprintf(
			"unsupported DataSource type for alter page set: %s — "+
				"use `replace <widget> with …` instead, which rebuilds the widget through the "+
				"CREATE PAGE path and supports every datasource type", ds.Type))
	}
}

// convertASTAction converts an AST action value to a pages.ClientAction.
//
// It delegates to the CREATE PAGE builder rather than reimplementing the switch,
// so every action form is supported here the day it is supported there — the
// alternative is the failure mode #855 documents for DataSource, where SET
// carried a narrower vocabulary than REPLACE and each missing case surfaced as
// its own bug report.
//
// The builder is seeded with the stored document's parameters and variables, as
// buildWidgetsFromAST is. Without them a `$Param` argument cannot be told from an
// expression and is written as one, which Studio Pro does not bind
// (mendixlabs/mxcli#1317, the #1140 rule).
//
// widgetRef is the widget the action is set on: a `returns ( … )` attribute
// target belongs to the object of the data container around it.
func convertASTAction(ctx *ExecContext, mutator backend.PageMutator, value any, moduleName string, moduleID model.ID, widgetRef string) (pages.ClientAction, error) {
	action, ok := value.(*ast.ActionV3)
	if !ok {
		return nil, mdlerrors.NewValidation("Action value must be an action expression, " +
			"for example `set (Action: microflow Module.MF) on btnSave`")
	}
	paramScope, paramEntityNames := mutator.ParamScope()
	pb := &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       moduleName,
		paramScope:       paramScope,
		paramEntityNames: paramEntityNames,
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    documentTheme(ctx, mutator),
		widgetBackend:    ctx.Backend,
		localVariables:   storedPageVariables(mutator),
		isDocumentWidget: alterDocumentWidgets(mutator, nil),
		isSnippet:        mutator.ContainerType() == backend.ContainerSnippet,
	}
	if len(action.Returns) > 0 {
		pb.entityContext, _ = alterEntityContext(ctx, mutator, widgetRef, false, moduleName, moduleID)
	}
	return pb.buildClientActionV3(action)
}

// ============================================================================
// INSERT widget via mutator
// ============================================================================

func applyInsertWidgetMutator(ctx *ExecContext, mutator backend.PageMutator, op *ast.InsertWidgetOp, moduleName string, moduleID model.ID) error {
	if strings.EqualFold(op.Position, "INTO") {
		if err := refuseHiddenItemSlot(ctx, mutator, op.Target); err != nil {
			return err
		}
	}
	opWidgets, err := expandAlterFragments(ctx, op.Widgets, moduleName, moduleID)
	if err != nil {
		return err
	}
	expanded := *op
	expanded.Widgets = opWidgets
	op = &expanded

	// Check for duplicate widget names before building
	for _, w := range op.Widgets {
		if w.Name != "" && mutator.FindWidget(w.Name) {
			return mdlerrors.NewAlreadyExistsMsg("widget", w.Name, fmt.Sprintf("duplicate widget name '%s': a widget with this name already exists on the page", w.Name))
		}
	}

	// Special path: inserting columns into a DataGrid2 column. Columns are
	// CustomWidgets$WidgetObject, not Forms$* widgets, so we go through
	// InsertColumns to serialize them correctly. Column children inherit the
	// grid's data source entity as their entity context — for a flow source, the
	// flow's return entity, which the BSON walk alone does not see (#1468).
	if op.Target.IsColumn() && allColumns(op.Widgets) {
		entityCtx, _ := alterEntityContext(ctx, mutator, op.Target.Widget, true, moduleName, moduleID)
		specs, err := buildColumnSpecsFromAST(ctx, op.Widgets, moduleName, moduleID, entityCtx, mutator)
		if err != nil {
			return mdlerrors.NewBackend("build column specs", err)
		}
		return mutator.InsertColumns(op.Target.Widget, columnRefOf(op.Target), backend.InsertPosition(op.Position), specs)
	}

	// Special path: inserting specialization templates into a List View. A
	// Forms$ListViewTemplate lives in the list view's Templates array, not in its
	// Widgets array, and is not a widget — the same reason DataGrid2 columns take
	// their own path above. Routed through InsertWidget it would append a
	// non-widget to the widget list, producing a page Studio Pro cannot open.
	if allListViewTemplates(op.Widgets) {
		if !strings.EqualFold(op.Position, "INTO") {
			return mdlerrors.NewValidation(
				"a list view template can only be added with INSERT INTO <listview> { template for … }: " +
					"templates are not siblings of the widgets in the list view's body")
		}
		templates, err := buildListViewTemplatesFromAST(ctx, op.Widgets, moduleName, moduleID, mutator, op.Target.Widget)
		if err != nil {
			return err
		}
		return mutator.InsertListViewTemplates(op.Target.Widget, templates)
	}
	if hasListViewTemplate(op.Widgets) {
		return mdlerrors.NewValidation(
			"mixing `template for …` blocks with ordinary widgets in one INSERT is not supported: " +
				"they go to different places (the list view's Templates and its default body). " +
				"Use one INSERT for the templates and another for the widgets")
	}

	// Find entity context for the new widgets. For INSERT BEFORE/AFTER the target
	// is a sibling, so use its enclosing container's context; for INSERT INTO the
	// target IS the container, so the children take the target's own context (e.g.
	// a dataview's entity).
	into := strings.EqualFold(op.Position, "INTO")
	entityCtx, _ := alterEntityContext(ctx, mutator, op.Target.Widget, into, moduleName, moduleID)

	// Special path: adding tab pages to a tab container (#1215). A tab page lives
	// in the container's TabPages list and is not a widget, so the builder
	// refuses one on its own ("tabpage must be a direct child of tabcontainer")
	// and InsertWidget would put it in a widget list. INTO targets the container,
	// BEFORE/AFTER a sibling tab page; the mutator checks which the target is.
	if allTabPages(op.Widgets) {
		tabPages, err := buildTabPagesFromAST(ctx, op.Widgets, moduleName, moduleID, entityCtx, mutator)
		if err != nil {
			return mdlerrors.NewBackend("build tab pages", err)
		}
		return mutator.InsertTabPages(op.Target.Widget, backend.InsertPosition(op.Position), tabPages)
	}
	if hasTabPage(op.Widgets) {
		return mdlerrors.NewValidation(
			"mixing `tabpage` blocks with ordinary widgets in one INSERT is not supported: " +
				"tab pages go in a tab container's list of pages, widgets inside a tab page. " +
				"Use one INSERT for the tab pages and another for the widgets")
	}

	// Special path: adding rows to a layout grid (mendixlabs/mxcli#1791). A row
	// lives in the grid's Rows list and is not a widget: the builder asked for
	// a name it never stores, and InsertWidget refused the grid as "not a
	// simple container". A row has no stored name, so INTO the grid is the only
	// position the mutator can address.
	if allLayoutGridRows(op.Widgets) {
		if !into {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"a layout grid row has no name to insert %s, so rows are added with "+
					"`insert into <layoutgrid> { row { … } }`, which appends them; to place widgets "+
					"in an existing row, insert into or next to a widget in its column",
				strings.ToLower(op.Position)))
		}
		rows, err := buildLayoutGridRowsFromAST(ctx, op.Widgets, moduleName, moduleID, entityCtx, mutator)
		if err != nil {
			return mdlerrors.NewBackend("build layout grid rows", err)
		}
		return mutator.InsertLayoutGridRows(op.Target.Widget, rows)
	}
	if hasLayoutGridRow(op.Widgets) {
		return mdlerrors.NewValidation(
			"mixing `row` blocks with ordinary widgets in one INSERT is not supported: " +
				"rows go in a layout grid's list of rows, widgets inside a row's column. " +
				"Use one INSERT for the rows and another for the widgets")
	}

	// Special path: adding items to a navigation list (#1388). An item lives in
	// the list's Items and is not a widget, so the builder refuses one on its own
	// ("item must be a direct child of navigationlist") and InsertWidget would
	// put it in a widget list. The mutator checks what the target is.
	if allNavigationListItems(op.Widgets) {
		items, err := buildNavigationListItemsFromAST(ctx, op.Widgets, moduleName, moduleID, entityCtx, mutator)
		if err != nil {
			return mdlerrors.NewBackend("build navigation list items", err)
		}
		return mutator.InsertNavigationListItems(op.Target.Widget, backend.InsertPosition(op.Position), items)
	}
	if hasNavigationListItem(op.Widgets) {
		return mdlerrors.NewValidation(
			"mixing `item` blocks with ordinary widgets in one INSERT is not supported: " +
				"items go in a navigation list's list of items, widgets inside an item. " +
				"Use one INSERT for the items and another for the widgets")
	}

	// Build new widgets from AST
	widgets, err := buildWidgetsFromAST(ctx, op.Widgets, moduleName, moduleID, entityCtx, mutator)
	if err != nil {
		return mdlerrors.NewBackend("build widgets", err)
	}

	return mutator.InsertWidget(op.Target.Widget, columnRefOf(op.Target), backend.InsertPosition(op.Position), widgets)
}

// expandAlterFragments expands `use fragment` / `use building block` sentinels
// in the widgets an INSERT or REPLACE carries, the same expansion CREATE PAGE
// applies to its body. It runs before anything else looks at the widgets, so
// the duplicate-name check, the column/template routing and the builder all see
// the fragment's widgets rather than the sentinel (#572). The input is cloned:
// expansion rewrites Children in place and the statement's AST is not ours.
func expandAlterFragments(ctx *ExecContext, widgets []*ast.WidgetV3, moduleName string, moduleID model.ID) ([]*ast.WidgetV3, error) {
	pb := &pageBuilder{
		ctx:        ctx,
		backend:    ctx.Backend,
		moduleID:   moduleID,
		moduleName: moduleName,
		execCache:  ctx.Cache,
		fragments:  ctx.Fragments,
	}
	return pb.expandFragments(cloneWidgets(widgets))
}

// ============================================================================
// DROP widget via mutator
// ============================================================================

func applyDropWidgetMutator(mutator backend.PageMutator, op *ast.DropWidgetOp) error {
	refs := make([]backend.WidgetRef, len(op.Targets))
	for i, t := range op.Targets {
		refs[i] = backend.WidgetRef{Widget: t.Widget, Column: columnRefOf(t)}
	}
	return mutator.DropWidget(refs)
}

// ============================================================================
// REPLACE widget via mutator
// ============================================================================

func applyReplaceWidgetMutator(ctx *ExecContext, mutator backend.PageMutator, op *ast.ReplaceWidgetOp, moduleName string, moduleID model.ID, storedWidgets func() map[string]*ast.WidgetV3) error {
	if op.Target.Item != nil && op.Target.Column != "" {
		if err := refuseHiddenItemSlot(ctx, mutator, op.Target); err != nil {
			return err
		}
	}
	newWidgets, err := expandAlterFragments(ctx, op.NewWidgets, moduleName, moduleID)
	if err != nil {
		return err
	}
	expanded := *op
	expanded.NewWidgets = unwrapFooterReplacement(op.Target, newWidgets)
	op = &expanded
	footerRegion := isFooterRegionTarget(op.Target)

	// The widgets the replace removes are not duplicates of what replaces them:
	// restating a footer's or a container's own children under their names is
	// the ordinary edit, and was refused as "duplicate widget name" (#293).
	replaced := replacedWidgetNames(mutator, op.Target)

	// Check for duplicate widget names (skip the widget being replaced)
	for _, w := range op.NewWidgets {
		if w.Name != "" && w.Name != op.Target.Widget && w.Name != columnRefOf(op.Target) && !replaced[w.Name] && mutator.FindWidget(w.Name) {
			return mdlerrors.NewAlreadyExistsMsg("widget", w.Name, fmt.Sprintf("duplicate widget name '%s': a widget with this name already exists on the page", w.Name))
		}
	}

	// Special path: replacing a DataGrid2 column with new columns. Columns are
	// CustomWidgets$WidgetObject, not Forms$* widgets. Column children inherit
	// the grid's data source entity as their entity context. A microflow or
	// nanoflow source names no entity in the BSON, so it is resolved through
	// the flow's return type; reading the BSON alone left a replacement column's
	// Attribute unqualified, it was not written, and mxbuild failed with "An
	// attribute is required when column sorting is enabled" (#1468).
	if op.Target.IsColumn() && allColumns(op.NewWidgets) {
		entityCtx, _ := alterEntityContext(ctx, mutator, op.Target.Widget, true, moduleName, moduleID)
		// What the replaced column contains — its filter, its custom content —
		// leaves the name scope with it, so the replacement may reuse the names.
		exclude := []string{op.Target.Widget, columnRefOf(op.Target)}
		for n := range replaced {
			exclude = append(exclude, n)
		}
		specs, err := buildColumnSpecsFromAST(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, mutator, exclude...)
		if err != nil {
			return mdlerrors.NewBackend("build replacement column specs", err)
		}
		return mutator.ReplaceColumn(op.Target.Widget, columnRefOf(op.Target), specs)
	}

	// Find entity context from enclosing DataView/DataGrid/ListView for regular
	// widget replace. A data view footer's widgets sit INSIDE the data view, so
	// they take its own context, as INSERT INTO does.
	entityCtx, _ := alterEntityContext(ctx, mutator, op.Target.Widget, footerRegion, moduleName, moduleID)

	// Build new widgets from AST, excluding the target widget/column — and what
	// it contains — from the duplicate-name scope so a same-name replacement is
	// allowed.
	exclude := []string{op.Target.Widget, columnRefOf(op.Target)}
	for n := range replaced {
		exclude = append(exclude, n)
	}

	// Special path: replacing one navigation list item with items (#1388). An
	// item lives in the list's Items, so it is not a widget REPLACE; the mutator
	// checks the target is an item.
	if allNavigationListItems(op.NewWidgets) {
		items, err := buildNavigationListItemsFromAST(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, mutator, exclude...)
		if err != nil {
			return mdlerrors.NewBackend("build replacement navigation list items", err)
		}
		return mutator.ReplaceNavigationListItem(op.Target.Widget, items)
	}
	if hasNavigationListItem(op.NewWidgets) {
		return mdlerrors.NewValidation(
			"mixing `item` blocks with ordinary widgets in one REPLACE is not supported: " +
				"items go in a navigation list's list of items, widgets inside an item")
	}
	widgets, err := buildWidgetsFromAST(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, mutator, exclude...)
	if err != nil {
		return mdlerrors.NewBackend("build replacement widgets", err)
	}

	// One pluggable widget replaced by one of the same package keeps what the
	// statement does not state (mendixlabs/mxcli#1247): the stored widget, as
	// describe prints it, is built beside the replacement, and the mutator keeps
	// every stored property the two builds agree on.
	if handled, err := replacePluggableKeepingUnstated(ctx, mutator, op, widgets, storedWidgets, moduleName, moduleID, entityCtx, exclude); handled || err != nil {
		return err
	}

	return mutator.ReplaceWidget(op.Target.Widget, columnRefOf(op.Target), widgets)
}

// pluggableKeepingReplacer is the PageMutator half of
// replacePluggableKeepingUnstated; mutators without it replace as before.
type pluggableKeepingReplacer interface {
	ReplacePluggableKeepingUnstated(widgetRef string, replacement, baseline pages.Widget) (bool, error)
}

// replacePluggableKeepingUnstated handles `replace <pluggable> with { <same
// pluggable kind> }`. handled is false when it does not apply — not exactly one
// widget for one, no description of the stored one, or a baseline that does
// not build — and the caller replaces as before.
func replacePluggableKeepingUnstated(ctx *ExecContext, mutator backend.PageMutator, op *ast.ReplaceWidgetOp,
	widgets []pages.Widget, storedWidgets func() map[string]*ast.WidgetV3,
	moduleName string, moduleID model.ID, entityCtx string, exclude []string) (bool, error) {
	keeper, ok := mutator.(pluggableKeepingReplacer)
	if !ok {
		return false, nil
	}
	baseline, ok := pluggableReplaceBaseline(ctx, mutator, op, widgets, storedWidgets, moduleName, moduleID, entityCtx, exclude)
	if !ok {
		return false, nil
	}
	return keeper.ReplacePluggableKeepingUnstated(op.Target.Widget, widgets[0], baseline)
}

// pluggableReplaceBaseline builds the stored widget as describe prints it, the
// baseline the #1247 merge compares the replacement with; ok is false when the
// merge does not apply — not exactly one pluggable widget for one of the same
// kind, no description of the stored one, or a baseline that does not build.
func pluggableReplaceBaseline(ctx *ExecContext, mutator backend.PageMutator, op *ast.ReplaceWidgetOp,
	widgets []pages.Widget, storedWidgets func() map[string]*ast.WidgetV3,
	moduleName string, moduleID model.ID, entityCtx string, exclude []string) (pages.Widget, bool) {
	if storedWidgets == nil || op.Target.Column != "" || op.Target.IsColumnAddress() ||
		len(widgets) != 1 || len(op.NewWidgets) != 1 {
		return nil, false
	}
	if _, isPluggable := widgets[0].(*pages.CustomWidget); !isPluggable {
		return nil, false
	}
	stored := storedWidgets()[op.Target.Widget]
	if stored == nil || !strings.EqualFold(stored.Type, op.NewWidgets[0].Type) {
		return nil, false
	}
	baseline, err := buildWidgetsFromAST(ctx, []*ast.WidgetV3{replaceBaseline(stored, op.NewWidgets[0])}, moduleName, moduleID, entityCtx, mutator, exclude...)
	if err != nil || len(baseline) != 1 {
		return nil, false
	}
	return baseline[0], true
}

// pluggableReplaceChecker is the check-time half of pluggableKeepingReplacer:
// the refusal ReplacePluggableKeepingUnstated would give, without the write.
type pluggableReplaceChecker interface {
	CheckPluggableReplace(widgetRef string, replacement, baseline pages.Widget) error
}

// refuseStaleContentReplace is the check -p half of the refusal a pluggable
// REPLACE gives for content it would carry onto another entity
// (backend.StaleWidgetContentError, ako/mxcli#1563): the replacement built
// as exec builds it, and the stored document asked, without writing. A
// replacement that does not build here is exec's to report, so nothing is
// said for it.
func refuseStaleContentReplace(ctx *ExecContext, mutator backend.PageMutator, op *ast.ReplaceWidgetOp,
	moduleName string, moduleID model.ID, storedWidgets func() map[string]*ast.WidgetV3) error {
	checker, ok := mutator.(pluggableReplaceChecker)
	if !ok || op.Target.Item != nil || isFooterRegionTarget(op.Target) {
		return nil
	}
	newWidgets, err := expandAlterFragments(ctx, op.NewWidgets, moduleName, moduleID)
	if err != nil {
		return nil
	}
	expanded := *op
	expanded.NewWidgets = newWidgets
	op = &expanded
	entityCtx, _ := alterEntityContext(ctx, mutator, op.Target.Widget, false, moduleName, moduleID)
	exclude := []string{op.Target.Widget, columnRefOf(op.Target)}
	for n := range replacedWidgetNames(mutator, op.Target) {
		exclude = append(exclude, n)
	}
	widgets, err := buildWidgetsFromAST(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, mutator, exclude...)
	if err != nil {
		return nil
	}
	baseline, ok := pluggableReplaceBaseline(ctx, mutator, op, widgets, storedWidgets, moduleName, moduleID, entityCtx, exclude)
	if !ok {
		return nil
	}
	return checker.CheckPluggableReplace(op.Target.Widget, widgets[0], baseline)
}

// replaceBaselineSystemProps are the widget-level settings describe prints for
// a pluggable widget (its visibility and editability). The merge reads "the
// replacement differs from the baseline" as "the statement states it", so one
// the statement leaves out must be left out of the baseline too, or the
// replacement's default would overwrite the stored value.
var replaceBaselineSystemProps = []string{"Editable", "EditableIf", "EditableWhen", "Visible", "VisibleIf", "VisibleWhen", "VisibleRoles"}

// replaceBaseline is the stored widget's description with the visibility and
// editability properties the statement does not state removed.
func replaceBaseline(stored, stmt *ast.WidgetV3) *ast.WidgetV3 {
	c := cloneWidget(stored)
	for _, k := range replaceBaselineSystemProps {
		if _, stated := lookupPropCI(stmt, k); stated {
			continue
		}
		for key := range c.Properties {
			if strings.EqualFold(key, k) {
				delete(c.Properties, key)
			}
		}
	}
	return c
}

// describedStoredWidgets describes the document an ALTER edits, in the
// script's language, and indexes its widgets by name; nil when it cannot.
func describedStoredWidgets(ctx *ExecContext, unitID model.ID, containerType string, name ast.QualifiedName) map[string]*ast.WidgetV3 {
	var describe func() error
	switch containerType {
	case "page":
		describe = func() error { return describePage(ctx, name) }
	case "snippet":
		describe = func() error { return describeSnippet(ctx, name) }
	default:
		return nil
	}
	out, err := describedWidgets(ctx, func() error {
		prev := ctx.describeID
		ctx.describeID = unitID
		defer func() { ctx.describeID = prev }()
		return describe()
	})
	if err != nil {
		return nil
	}
	return out
}

// storedPageVariables is the stored document's page variables, so a widget an
// ALTER adds can bind to one (`Attribute: $ShowAll`, mendixlabs/mxcli#1235) and
// a `$name` in a template resolves as it does in CREATE.
func storedPageVariables(mutator backend.PageMutator) map[string]bool {
	vars := map[string]bool{}
	if lister, ok := mutator.(interface{ PageVariableNames() []string }); ok {
		for _, n := range lister.PageVariableNames() {
			vars[n] = true
		}
	}
	return vars
}

// isFooterRegionTarget reports whether an ALTER target is `<widget>.footer` —
// a data view's footer region (ako/mxcli#528). Whether the owner IS a data view
// is the mutator's call; on anything else the dotted form keeps its other
// meanings (a scroll-container region, a grid column).
func isFooterRegionTarget(t ast.WidgetRef) bool {
	return t.Widget != "" && !t.IsColumnAddress() && strings.EqualFold(t.Column, "footer")
}

// unwrapFooterReplacement lets `replace dv.footer with { footer { … } }` — the
// footer block as describe prints it — mean its content: a footer region holds
// widgets, and a `footer` built outside a data view would be a container nested
// inside the region rather than the region's content.
func unwrapFooterReplacement(target ast.WidgetRef, widgets []*ast.WidgetV3) []*ast.WidgetV3 {
	if !isFooterRegionTarget(target) || len(widgets) != 1 || !strings.EqualFold(widgets[0].Type, "footer") {
		return widgets
	}
	return widgets[0].Children
}

// replacedWidgetNames lists the names a REPLACE removes along with its target:
// everything inside it. Only mutators that can walk the stored tree answer; the
// others keep the old, stricter scope.
func replacedWidgetNames(mutator backend.PageMutator, target ast.WidgetRef) map[string]bool {
	walker, ok := mutator.(interface {
		ContainedWidgetNames(widgetRef, columnRef string) []string
	})
	if !ok {
		return nil
	}
	names := map[string]bool{}
	for _, n := range walker.ContainedWidgetNames(target.Widget, columnRefOf(target)) {
		names[n] = true
	}
	return names
}

// allColumns returns true if all widgets in the slice have type "column".
// Used to dispatch ALTER PAGE INSERT/REPLACE into a DataGrid2 column to the
// column-specific mutator path.
// allListViewTemplates reports whether every inserted node is a `template for
// Module.Entity { … }` block.
func allListViewTemplates(widgets []*ast.WidgetV3) bool {
	if len(widgets) == 0 {
		return false
	}
	for _, w := range widgets {
		if w.Specialization == "" {
			return false
		}
	}
	return true
}

// hasListViewTemplate reports whether ANY inserted node is a template, so a
// mixed insert can be refused rather than silently sending the templates down
// the widget path.
func hasListViewTemplate(widgets []*ast.WidgetV3) bool {
	for _, w := range widgets {
		if w.Specialization != "" {
			return true
		}
	}
	return false
}

// buildListViewTemplatesFromAST builds specialization templates for INSERT INTO.
//
// Each template's children are built in the SPECIALIZATION's entity context, not
// the list view's: an attribute only the specialization has must resolve inside
// its own template.
func buildListViewTemplatesFromAST(ctx *ExecContext, nodes []*ast.WidgetV3, moduleName string, moduleID model.ID, mutator backend.PageMutator, listViewRef string) ([]*pages.ListViewTemplate, error) {
	// The specialization check needs the domain model, which the mutator does not
	// have — it sees raw BSON. A minimal builder over the same cache answers it.
	checker := &pageBuilder{ctx: ctx, backend: ctx.Backend, moduleID: moduleID, moduleName: moduleName, execCache: ctx.Cache}
	listEntity := mutator.EnclosingEntityForChildren(listViewRef)

	seen := make(map[string]bool, len(nodes))
	out := make([]*pages.ListViewTemplate, 0, len(nodes))
	for _, node := range nodes {
		spec := node.Specialization
		if seen[spec] {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"this INSERT adds two templates for %s", spec))
		}
		seen[spec] = true

		// Refuse a template nothing will ever reach, by the same rule CREATE PAGE
		// applies — one function, so the two cannot drift apart.
		if err := checker.checkListViewTemplateSpecialization(spec, listEntity, listViewRef); err != nil {
			return nil, err
		}

		widgets, err := buildWidgetsFromAST(ctx, node.Children, moduleName, moduleID, spec, mutator)
		if err != nil {
			return nil, mdlerrors.NewBackend("build template widgets", err)
		}
		out = append(out, &pages.ListViewTemplate{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ListViewTemplate",
			},
			Specialization: spec,
			Widgets:        widgets,
		})
	}
	return out, nil
}

func allColumns(widgets []*ast.WidgetV3) bool {
	if len(widgets) == 0 {
		return false
	}
	for _, w := range widgets {
		if !strings.EqualFold(w.Type, "column") {
			return false
		}
	}
	return true
}

// buildColumnSpecsFromAST converts AST column widgets to DataGridColumnSpec
// instances using the executor's pageBuilder for attribute path resolution
// and child widget construction.
func buildColumnSpecsFromAST(ctx *ExecContext, widgets []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator, excludeFromScope ...string) ([]*backend.DataGridColumnSpec, error) {
	paramScope, paramEntityNames := mutator.ParamScope()
	widgetScope := mutator.WidgetScope()
	for _, name := range excludeFromScope {
		delete(widgetScope, name)
	}

	pb := &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       moduleName,
		entityContext:    entityContext,
		widgetScope:      widgetScope,
		paramScope:       paramScope,
		paramEntityNames: paramEntityNames,
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    documentTheme(ctx, mutator),
		widgetBackend:    ctx.Backend,
		localVariables:   storedPageVariables(mutator),
		isDocumentWidget: alterDocumentWidgets(mutator, widgets),
		isSnippet:        mutator.ContainerType() == backend.ContainerSnippet,
	}

	var result []*backend.DataGridColumnSpec
	for _, w := range widgets {
		spec, err := pb.buildColumnSpecFromAST(w)
		if err != nil {
			return nil, mdlerrors.NewBackend("build column "+w.Name, err)
		}
		result = append(result, spec)
	}
	return result, nil
}

// ============================================================================
// Widget building from AST (domain logic stays in executor)
// ============================================================================

// alterEntityContext is the entity an INSERT or REPLACE builds its widgets
// against, plus the flow that was meant to supply it when a flow data source is
// where the scope comes from. forChildren is INSERT INTO: the target IS the
// container, so its own data source decides; otherwise (INSERT BEFORE/AFTER,
// REPLACE) the target is a sibling and the nearest ENCLOSING source does.
//
// A microflow/nanoflow datasource contributes no entity to the BSON walk (its
// entity is the flow's RETURN type), so it is resolved via the model — otherwise
// a widget inserted into a flow-sourced list binds nothing (CE0402/CE1613, #55).
//
// Shared with the check-time pass (validate_alter_unscoped.go), so check and exec
// cannot disagree about which entity is in scope.
func alterEntityContext(ctx *ExecContext, mutator backend.PageMutator, widgetRef string, forChildren bool, moduleName string, moduleID model.ID) (entity, flow string) {
	if forChildren {
		entity = mutator.EnclosingEntityForChildren(widgetRef)
	} else {
		entity = mutator.EnclosingEntity(widgetRef)
	}
	if entity != "" {
		return entity, ""
	}
	mfQN, nfQN := mutator.EnclosingDataSourceFlow(widgetRef, forChildren)
	flow = mfQN
	if flow == "" {
		flow = nfQN
	}
	return resolveDataSourceFlowEntity(ctx, moduleName, moduleID, mfQN, nfQN), flow
}

// resolveDataSourceFlowEntity resolves the entity context contributed by a
// microflow/nanoflow datasource — its RETURN entity — for ALTER PAGE widget
// builds. A flow datasource stores no entity in its own BSON (the entity lives
// in the flow document), so the BSON walk yields "" and a bare inserted
// attribute would bind nothing. Returns "" when neither flow qualified name
// resolves (e.g. a void-returning flow). (FINDINGS #55)
func resolveDataSourceFlowEntity(ctx *ExecContext, moduleName string, moduleID model.ID, mfQN, nfQN string) string {
	if mfQN == "" && nfQN == "" {
		return ""
	}
	pb := &pageBuilder{
		ctx:        ctx,
		backend:    ctx.Backend,
		moduleID:   moduleID,
		moduleName: moduleName,
		execCache:  ctx.Cache,
	}
	if mfQN != "" {
		return pb.getMicroflowReturnEntityName(mfQN)
	}
	return pb.getNanoflowReturnEntityName(nfQN)
}

// buildWidgetsFromAST converts AST widgets to pages.Widget domain objects.
// It uses the mutator for scope resolution (WidgetScope, ParamScope).
// excludeFromScope removes named widgets from the duplicate-detection scope,
// used when replacing a widget so the new one may reuse the target's name.
func buildWidgetsFromAST(ctx *ExecContext, widgets []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator, excludeFromScope ...string) ([]pages.Widget, error) {
	pb := newAlterPageBuilder(ctx, moduleName, moduleID, entityContext, mutator, excludeFromScope...)
	pb.isDocumentWidget = alterDocumentWidgets(mutator, widgets)

	var result []pages.Widget
	for _, w := range widgets {
		widget, err := pb.buildWidgetV3(w)
		if err != nil {
			return nil, mdlerrors.NewBackend("build widget "+w.Name, err)
		}
		if widget == nil {
			continue
		}
		result = append(result, widget)
	}
	return result, nil
}

// buildTabPagesFromAST builds the tab pages an INSERT adds to a tab container,
// with the same builder CREATE PAGE uses for a tab container's children.
func buildTabPagesFromAST(ctx *ExecContext, nodes []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator) ([]*pages.TabPage, error) {
	pb := newAlterPageBuilder(ctx, moduleName, moduleID, entityContext, mutator)
	pb.isDocumentWidget = alterDocumentWidgets(mutator, nodes)
	out := make([]*pages.TabPage, 0, len(nodes))
	for _, node := range nodes {
		tp, err := pb.buildTabPageV3(node)
		if err != nil {
			return nil, mdlerrors.NewBackend("build tab page "+node.Name, err)
		}
		out = append(out, tp)
	}
	return out, nil
}

// buildNavigationListItemsFromAST builds the items an INSERT adds to a
// navigation list, with the same builder CREATE uses for a list's children.
func buildNavigationListItemsFromAST(ctx *ExecContext, nodes []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator, excludeFromScope ...string) ([]*pages.NavigationListItem, error) {
	pb := newAlterPageBuilder(ctx, moduleName, moduleID, entityContext, mutator, excludeFromScope...)
	pb.isDocumentWidget = alterDocumentWidgets(mutator, nodes)
	out := make([]*pages.NavigationListItem, 0, len(nodes))
	for _, node := range nodes {
		item, err := pb.buildNavigationListItemV3(node)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// allNavigationListItems reports whether every inserted node is a navigation
// list item.
func allNavigationListItems(widgets []*ast.WidgetV3) bool {
	if len(widgets) == 0 {
		return false
	}
	for _, w := range widgets {
		if !strings.EqualFold(w.Type, "item") {
			return false
		}
	}
	return true
}

// hasNavigationListItem reports whether ANY inserted node is a navigation list
// item, so a mixed insert is refused rather than half-applied.
func hasNavigationListItem(widgets []*ast.WidgetV3) bool {
	for _, w := range widgets {
		if strings.EqualFold(w.Type, "item") {
			return true
		}
	}
	return false
}

// buildLayoutGridRowsFromAST builds the rows an INSERT adds to a layout grid,
// with the same builder CREATE PAGE uses for a layout grid's children.
func buildLayoutGridRowsFromAST(ctx *ExecContext, nodes []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator) ([]*pages.LayoutGridRow, error) {
	pb := newAlterPageBuilder(ctx, moduleName, moduleID, entityContext, mutator)
	pb.isDocumentWidget = alterDocumentWidgets(mutator, nodes)
	out := make([]*pages.LayoutGridRow, 0, len(nodes))
	for _, node := range nodes {
		if err := refuseNativeWidgetProps(pb.ctx, "layoutgrid row", node); err != nil {
			return nil, err
		}
		row, err := pb.buildLayoutGridRowV3(node)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// allLayoutGridRows reports whether every inserted node is a layout grid row.
func allLayoutGridRows(widgets []*ast.WidgetV3) bool {
	if len(widgets) == 0 {
		return false
	}
	for _, w := range widgets {
		if !strings.EqualFold(w.Type, "row") {
			return false
		}
	}
	return true
}

// hasLayoutGridRow reports whether ANY inserted node is a layout grid row, so
// a mixed insert is refused rather than half-applied.
func hasLayoutGridRow(widgets []*ast.WidgetV3) bool {
	for _, w := range widgets {
		if strings.EqualFold(w.Type, "row") {
			return true
		}
	}
	return false
}

// allTabPages reports whether every inserted node is a tab page.
func allTabPages(widgets []*ast.WidgetV3) bool {
	if len(widgets) == 0 {
		return false
	}
	for _, w := range widgets {
		if !strings.EqualFold(w.Type, "tabpage") {
			return false
		}
	}
	return true
}

// hasTabPage reports whether ANY inserted node is a tab page, so a mixed insert
// is refused rather than sending the tab pages down the widget path.
func hasTabPage(widgets []*ast.WidgetV3) bool {
	for _, w := range widgets {
		if strings.EqualFold(w.Type, "tabpage") {
			return true
		}
	}
	return false
}

// alterDocumentWidgets is isDocumentWidget for an ALTER: a widget the stored
// page already has, or one the statement adds with it (ako/mxcli#1560).
func alterDocumentWidgets(mutator backend.PageMutator, added []*ast.WidgetV3) func(name string) bool {
	names := map[string]bool{}
	collectWidgetNames(added, names)
	return func(name string) bool { return names[name] || mutator.FindWidget(name) }
}

// newAlterPageBuilder returns a page builder scoped to the stored page an ALTER
// edits: its parameters, its widget names and its local variables.
func newAlterPageBuilder(ctx *ExecContext, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator, excludeFromScope ...string) *pageBuilder {
	paramScope, paramEntityNames := mutator.ParamScope()
	widgetScope := mutator.WidgetScope()
	for _, name := range excludeFromScope {
		delete(widgetScope, name)
	}

	return &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       moduleName,
		entityContext:    entityContext,
		widgetScope:      widgetScope,
		paramScope:       paramScope,
		paramEntityNames: paramEntityNames,
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    documentTheme(ctx, mutator),
		widgetBackend:    ctx.Backend,
		localVariables:   storedPageVariables(mutator),
		isDocumentWidget: alterDocumentWidgets(mutator, nil),
		isSnippet:        mutator.ContainerType() == backend.ContainerSnippet,
	}
}

// templateParameterSetter is the page mutator capability behind
// `set (CaptionParams: …)` / `set (ContentParams: …)`.
type templateParameterSetter interface {
	SetWidgetTemplateParameters(widgetRef, prop string, params []*pages.ClientTemplateParameter) error
}

func isTemplateParamsKey(prop string) bool {
	return strings.EqualFold(prop, "CaptionParams") || strings.EqualFold(prop, "ContentParams")
}

// setTemplateParameters writes a text template's parameters on a stored
// built-in widget (ako/mxcli#1966), resolved by the builder CREATE PAGE uses
// against the entity of the data container the widget sits in — so a bare
// attribute name means here what it means in `create page`.
func setTemplateParameters(ctx *ExecContext, mutator backend.PageMutator, target ast.WidgetRef, prop string,
	params []ast.ParamAssignmentV3, moduleName string, moduleID model.ID) error {
	setter, ok := mutator.(templateParameterSetter)
	if !ok || target.Widget == "" || target.IsColumn() || target.Item != nil {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"alter page set %s is supported on a button or a dynamic text — use `replace <widget> with …` here", prop))
	}
	pb := newAlterPageBuilder(ctx, moduleName, moduleID, mutator.EnclosingEntity(target.Widget), mutator)
	resolved := pb.buildClientTemplateParams(params)
	if err := setter.SetWidgetTemplateParameters(target.Widget, prop, resolved); err != nil {
		return mdlerrors.NewBackend("set "+prop+" on "+target.Name(), err)
	}
	return nil
}
