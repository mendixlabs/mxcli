// SPDX-License-Identifier: Apache-2.0

// CE1571: "No argument has been selected for parameter 'X' and no default is
// available. Please select an argument manually."
//
// A microflow used as a widget's data source must be given an argument for every
// parameter it declares. mxcli accepted a bare `datasource: microflow M.F` and
// wrote a data source with no parameter mappings, so the page built to CE1571 at
// the far end of an `mx check` — while `mxcli check --references` passed, because
// the microflow itself resolves fine (ako/mxcli-maintenance-2).
//
// This is a pure model-consistency question: the microflow's signature and the
// arguments are both in hand, so it needs no runtime and no build.
//
// # What Mendix actually fills in
//
// The rule originally rested on two measurements, of which the second is true
// but not the whole story:
//
//	page with NO parameters, datasource without arguments   → CE1571
//	page WITH a $Task parameter of the parameter's exact
//	  entity type, datasource without arguments             → CE1571
//
// A page PARAMETER in scope is not the same thing as an enclosing DATA CONTEXT,
// and the two behave differently. Re-measured on mxbuild 11.13.0, five widgets
// on one project, one `mx check` (ledger #147):
//
//	nested in a dataview of the parameter's type            → no error
//	matching context two levels up, mismatched one between  → no error
//	nested in a dataview of a DIFFERENT type                → CE1571
//	no enclosing data context at all                        → CE1571
//	page parameter of the exact type, no dataview           → CE1571
//
// So an enclosing data container of a compatible type DOES supply the argument,
// at any depth, and a page parameter does not — FROM MENDIX 10.1. Before it, a
// page's or snippet's own parameter supplies it too, for data sources and
// actions alike (bisected for ako/mxcli#1206: 9.24.30 and 10.0.0 build clean,
// 10.1.0 onwards CE1571); see paramsSupplyFlowArguments. The rule now says exactly that;
// before this it rejected a page that builds, with a message asserting as fact
// that Mendix "does not fill it in, even when an object of the right type is in
// scope".
//
// Where a context's entity type cannot be resolved (an association or selection
// source), nothing is reported: guessing wrong in that direction is what
// rejected the working page in the first place.
//
// # The same CE, the other property (mendixlabs/mxcli#1082)
//
// CE1571 is not a data-source error. It is raised for any call whose parameters
// are not all supplied, and a widget's ACTION is such a call — so the rule now
// covers `Action:` / `OnClick:` / `OnChange:` and a pluggable widget's named
// action slots as well. Before this it walked GetDataSource() alone, and the
// identical fault on an action passed `mxcli check --references` clean and then
// failed the build. Measured on mxbuild 11.12.0, one project, one `mx check`:
//
//	CONTAINER action, no argument, no enclosing data context   → CE1571
//	the same page put through `mxcli check`                    → "Check passed!"
//
// The action obeys the same context rule as the data source, measured the same
// way — six containers carrying the identical fault, one `mx check`:
//
//	nested in a dataview of the parameter's type            → no error
//	nested in a dataview of a DIFFERENT type                → CE1571
//	page parameter of the exact type, no dataview           → CE1571
//	inside a data grid COLUMN (row-scoped)                  → no error
//	no enclosing data context at all                        → CE1571
//	inside a data grid CONTROL BAR                          → CE1571
//
// # A control bar is not row-scoped
//
// The last row is the reported shape, and it is the one the context walk got
// wrong: a control bar is a child of the data widget, so it used to inherit the
// row context and the check stayed silent. It applies to data sources too —
// same project, same run:
//
//	dataview whose microflow source needs an argument,
//	  placed in the grid's CONTROL BAR                      → CE1571
//	the same dataview placed in a COLUMN                    → no error
//
// So the fix belongs in the walk rather than in either rule, and it drops only
// the data widget's OWN object: an outer context still reaches the control bar
// (dataview > datagrid > controlbar > action, no argument → no error), which is
// why the control bar is walked with the context its parent was walked with
// rather than with an empty one.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// flowParam is one parameter of a microflow or nanoflow, as much of it as the
// CE1571 check needs.
//
// Object and Entity are separate because they answer different questions.
// Object says whether a data context could ever supply this parameter at all —
// a String never can, so a primitive is reported however deeply the widget is
// nested. Entity says which context would; an object parameter whose entity did
// not resolve keeps Object set and Entity empty, and is then satisfied by any
// context rather than by none, because that is the direction that does not
// reject a working page.
type flowParam struct {
	Name   string
	Entity string // entity qualified name, "" when unknown or not an object
	Object bool   // takes an object or a list of objects
	List   bool   // takes a list (Object is then true too)
}

// flowSignature is a flow's parameters plus the entity it returns — the return
// type is what a data container built on the flow puts into context for its
// children.
type flowSignature struct {
	Params []flowParam
	// Returns is the entity qualified name of an object or list return type,
	// "" for anything else (including a flow that returns nothing).
	Returns string
	// ReturnKind is the return type as declared, which Returns cannot express:
	// a Boolean and a void microflow both leave Returns empty, and the
	// after-startup check has to tell them apart (CE0142). TypeVoid where the
	// flow declares no return at all.
	ReturnKind ast.DataTypeKind
}

// paramNames returns the parameter names in declaration order, which is what the
// "declares X" half of the unknown-argument message prints.
func (s *flowSignature) paramNames() []string {
	names := make([]string, 0, len(s.Params))
	for _, p := range s.Params {
		names = append(names, p.Name)
	}
	return names
}

// dataContext is what encloses a widget: the entity types of the data containers
// it sits inside, plus whether any of them had a type this check could not work
// out.
type dataContext struct {
	entities   []string
	unresolved bool
}

func (c dataContext) with(entity string) dataContext {
	next := dataContext{entities: append(append([]string(nil), c.entities...), entity), unresolved: c.unresolved}
	return next
}

func (c dataContext) withUnresolved() dataContext {
	return dataContext{entities: append([]string(nil), c.entities...), unresolved: true}
}

// supplies reports whether this context fills the parameter in for Mendix.
func (c dataContext) supplies(p flowParam, compatible func(contextQN, paramQN string) bool) bool {
	if !p.Object {
		return false // a primitive is never taken from a data context
	}
	if c.unresolved {
		return true // cannot tell; do not reject a page that may well build
	}
	if p.Entity == "" {
		return len(c.entities) > 0 // object parameter, type unknown
	}
	for _, e := range c.entities {
		if compatible(e, p.Entity) {
			return true
		}
	}
	return false
}

// describe renders the enclosing context for the error message.
func (c dataContext) describe() string {
	switch {
	case len(c.entities) == 0:
		return ""
	case len(c.entities) == 1:
		return c.entities[0]
	default:
		return strings.Join(c.entities, ", ")
	}
}

// validateFlowArguments reports microflow/nanoflow calls on a widget — its data
// source and its action slots alike — whose argument list does not match the
// flow's parameters.
//
// Two distinct faults, both provable:
//
//   - a parameter no enclosing data context supplies and no argument fills —
//     CE1571, the reported case
//   - an argument naming no parameter — a typo that silently binds nothing
//
// A flow that cannot be resolved yields nothing: "not found" is already
// validateWidgetReferences' job, and reporting it twice would be noise. A flow
// created earlier in the SAME script is likewise skipped rather than guessed at,
// since its signature is not in the project yet.
func validateFlowArguments(ctx *ExecContext, params []ast.PageParameter, widgets []*ast.WidgetV3, sc *scriptContext) []string {
	if !ctx.Connected() || len(widgets) == 0 {
		return nil
	}
	sigs := buildFlowSignatures(ctx)
	// A flow the SAME script creates is not in the project yet, and that is the
	// common shape — one script writing a data source microflow and the page that
	// binds it. Script definitions win on a name clash: they are what this run
	// will leave behind.
	if sc != nil {
		for name, sig := range sc.flowParams {
			sigs[name] = sig
		}
	}
	if len(sigs) == 0 {
		return nil
	}
	return validateFlowArgumentsScoped(params, widgets, sigs, entityCompatibility(ctx),
		paramsSupplyFlowArguments(projectVersionAtLeast(ctx.Backend)))
}

// paramsSupplyFlowArguments reports whether the page's or snippet's own
// parameters supply flow arguments, as an enclosing data container does. They
// do before Mendix 10.1 and not from it (ako/mxcli#1206), bisected with one
// script — snippet and page parameter, data source and button action — across
// 9.24.30, 10.0.0 (0 errors) and 10.1.0 … 11.12.6 (CE1571 on each). An unknown
// version gets the 10.1+ rule, the one every project used to get.
func paramsSupplyFlowArguments(atLeast func(major, minor int) bool) bool {
	return atLeast != nil && !atLeast(10, 1)
}

// validateFlowArgumentsIn is the whole rule with its two project-dependent
// inputs — the flow signatures and the entity-compatibility test — handed in, so
// it can be measured against the shapes `mx check` was run on. It is the 10.1+
// rule; validateFlowArgumentsScoped takes the version's answer.
func validateFlowArgumentsIn(
	pageParams []ast.PageParameter,
	widgets []*ast.WidgetV3,
	sigs map[string]*flowSignature,
	compatible func(contextQN, paramQN string) bool,
) []string {
	return validateFlowArgumentsScoped(pageParams, widgets, sigs, compatible, false)
}

// validateFlowArgumentsScoped is validateFlowArgumentsIn with paramsInScope:
// when set, the parameters' entities are the root data context.
func validateFlowArgumentsScoped(
	pageParams []ast.PageParameter,
	widgets []*ast.WidgetV3,
	sigs map[string]*flowSignature,
	compatible func(contextQN, paramQN string) bool,
	paramsInScope bool,
) []string {
	paramEntities := make(map[string]string, len(pageParams))
	for _, p := range pageParams {
		if qn := p.EntityType.String(); qn != "" && qn != "." {
			paramEntities[strings.ToLower(p.Name)] = qn
		}
	}

	var errs []string
	var walk func(w *ast.WidgetV3, enclosing dataContext, controlBarOf string)
	walk = func(w *ast.WidgetV3, enclosing dataContext, controlBarOf string) {
		if w == nil {
			return
		}
		ds := w.GetDataSource()
		if ds != nil {
			// The context checked is what ENCLOSES the widget: a data source
			// cannot supply its own parameter.
			errs = append(errs, dataSourceArgErrors(w.Name, ds, sigs, enclosing, compatible)...)
		}
		inner := childContext(enclosing, ds, sigs, paramEntities)
		errs = append(errs, actionArgErrors(w, sigs, enclosing, inner, compatible, controlBarOf)...)

		// A widget that establishes its own data context ends the control bar's
		// reach: inside a dataview placed in a control bar, $currentObject is
		// that dataview's object like anywhere else.
		childControlBarOf := controlBarOf
		if ds != nil {
			childControlBarOf = ""
		}
		for _, c := range w.Children {
			if isControlBar(c) {
				// Not row-scoped: the data widget's own object is out of scope
				// here, everything above it is not. Measured — see the header.
				walk(c, enclosing, w.Name)
				continue
			}
			walk(c, inner, childControlBarOf)
		}
	}
	root := dataContext{}
	if paramsInScope {
		for _, p := range pageParams {
			if qn := p.EntityType.String(); qn != "" && qn != "." {
				root = root.with(qn)
			}
		}
	}
	for _, w := range widgets {
		walk(w, root, "")
	}
	return errs
}

// isControlBar reports whether the widget is a data widget's control bar, the
// one child that does not inherit its parent's row context.
func isControlBar(w *ast.WidgetV3) bool {
	return w != nil && strings.EqualFold(w.Type, "controlbar")
}

// childContext extends the enclosing context with what this widget's data source
// puts in scope for its children.
//
// A widget with no data source passes its own context down unchanged; that is
// what makes a container between a dataview and a grid transparent.
func childContext(enclosing dataContext, ds *ast.DataSourceV3, sigs map[string]*flowSignature, pageParams map[string]string) dataContext {
	if ds == nil {
		return enclosing
	}
	switch ds.Type {
	case "microflow", "nanoflow":
		if sig, ok := sigs[strings.ToLower(ds.Reference)]; ok && sig.Returns != "" {
			return enclosing.with(sig.Returns)
		}
		return enclosing.withUnresolved()
	case "database":
		if ds.Reference != "" {
			return enclosing.with(ds.Reference)
		}
	case "parameter":
		if qn, ok := pageParams[strings.ToLower(strings.TrimPrefix(ds.Reference, "$"))]; ok {
			return enclosing.with(qn)
		}
	case "association", "selection":
		// Resolvable in principle, not resolved here. Marking the context
		// unresolved suppresses the report rather than guessing at a mismatch.
	default:
		return enclosing
	}
	return enclosing.withUnresolved()
}

// dataSourceArgErrors compares one data source against the flow's signature,
// treating the enclosing data context as supplying what it supplies.
func dataSourceArgErrors(
	widgetName string,
	ds *ast.DataSourceV3,
	sigs map[string]*flowSignature,
	enclosing dataContext,
	compatible func(contextQN, paramQN string) bool,
) []string {
	if ds.Type != "microflow" && ds.Type != "nanoflow" {
		return nil
	}
	sig, known := sigs[strings.ToLower(ds.Reference)]
	if !known || sig == nil {
		return nil // resolution is validateWidgetReferences' job
	}

	missing, unknown, missingEntity := argDiff(ds.Args, sig, enclosing, compatible)

	var out []string
	if len(missing) > 0 {
		msg := fmt.Sprintf(
			"widget '%s': %s data source %s has no argument for %s %s — Mendix rejects this with CE1571. "+
				"Write it as `%s(%s: $Value)`",
			widgetName, ds.Type, ds.Reference,
			plural(len(missing), "parameter", "parameters"),
			quoteJoin(missing), ds.Reference, missing[0])
		if hint := contextHint(enclosing, missingEntity); hint != "" {
			msg += hint
		}
		out = append(out, msg)
	}
	if len(unknown) > 0 {
		out = append(out, fmt.Sprintf(
			"widget '%s': %s data source %s has no %s %s (it declares %s)",
			widgetName, ds.Type, ds.Reference,
			plural(len(unknown), "parameter", "parameters"),
			quoteJoin(unknown), quoteJoin(sig.paramNames())))
	}
	return out
}

// argDiff compares a call's arguments against a flow's parameters, treating the
// enclosing data context as supplying what it supplies.
//
// Shared by the data source and the action, because CE1571 does not distinguish
// them: both are a call whose parameters must all be filled. Returns the
// parameters nothing fills, the arguments naming no parameter, and the entity of
// the first missing parameter (what the context hint is written against).
func argDiff(
	args []ast.FlowArgV3,
	sig *flowSignature,
	enclosing dataContext,
	compatible func(contextQN, paramQN string) bool,
) (missing []string, unknown []string, missingEntity string) {
	given := make(map[string]bool, len(args))
	for _, a := range args {
		given[strings.ToLower(a.Name)] = true
	}
	wanted := make(map[string]bool, len(sig.Params))
	for _, p := range sig.Params {
		wanted[strings.ToLower(p.Name)] = true
	}

	for _, p := range sig.Params {
		if given[strings.ToLower(p.Name)] || enclosing.supplies(p, compatible) {
			continue
		}
		missing = append(missing, p.Name)
		if missingEntity == "" {
			missingEntity = p.Entity
		}
	}
	for _, a := range args {
		if !wanted[strings.ToLower(a.Name)] {
			unknown = append(unknown, a.Name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	return missing, unknown, missingEntity
}

// widgetActions returns the widget's microflow and nanoflow actions, each with
// the property key it was written under.
//
// Every action slot goes through one Properties sweep rather than a list of
// known keys: `Action:`/`OnClick:` land on "Action", `OnChange:` on "OnChange",
// and a pluggable widget's named slot on its own key (`createFileAction:`), all
// as *ast.ActionV3. A list of keys would have covered the reported one and
// quietly missed the rest — the same shape as the bug being fixed.
//
// Keys are sorted so a widget with two faulty slots reports them in a stable
// order. A chained THEN action is followed: `create_object E then show_page P`
// carries a second call, and only the flow ones are kept here.
func widgetActions(w *ast.WidgetV3) []namedAction {
	if w == nil || len(w.Properties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []namedAction
	for _, k := range keys {
		a, ok := w.Properties[k].(*ast.ActionV3)
		if !ok {
			continue
		}
		for ; a != nil; a = a.ThenAction {
			if a.Type == "microflow" || a.Type == "nanoflow" {
				out = append(out, namedAction{Property: k, Action: a})
			}
		}
	}
	return out
}

// namedAction is one action slot: the property it was written under, and the
// call in it.
type namedAction struct {
	Property string
	Action   *ast.ActionV3
}

// actionArgErrors reports a widget's action slots whose arguments do not match
// the flow's parameters — CE1571 and the silent-typo argument, the same two
// faults dataSourceArgErrors reports for a data source.
//
// row is the context the widget's own data source puts in scope, which is what
// its row action is judged in (see isRowAction); every other slot is judged in
// enclosing, the context around the widget.
//
// controlBarOf names the data widget whose control bar this widget sits in, and
// is "" everywhere else. It only changes the advice: the remedy there is the
// grid's selection, which is the one thing a control-bar author needs told and
// the thing the reported issue went looking for in the grammar.
func actionArgErrors(
	w *ast.WidgetV3,
	sigs map[string]*flowSignature,
	enclosing, row dataContext,
	compatible func(contextQN, paramQN string) bool,
	controlBarOf string,
) []string {
	var out []string
	for _, na := range widgetActions(w) {
		sig, known := sigs[strings.ToLower(na.Action.Target)]
		if !known || sig == nil {
			continue // resolution is validateWidgetReferences' job
		}
		judgedIn := enclosing
		if isRowAction(w, na.Property) {
			judgedIn = row
		}
		missing, unknown, missingEntity := argDiff(na.Action.Args, sig, judgedIn, compatible)

		if len(missing) > 0 {
			msg := fmt.Sprintf(
				"widget '%s': %s action %s (%s:) has no argument for %s %s — Mendix rejects this with CE1571. "+
					"Write it as `%s: %s %s(%s: $Value)`",
				w.Name, na.Action.Type, na.Action.Target, na.Property,
				plural(len(missing), "parameter", "parameters"),
				quoteJoin(missing),
				na.Property, na.Action.Type, na.Action.Target, missing[0])
			if hint := controlBarHint(controlBarOf); hint != "" {
				msg += hint
			} else if hint := contextHint(judgedIn, missingEntity); hint != "" {
				msg += hint
			}
			out = append(out, msg)
		}
		if len(unknown) > 0 {
			out = append(out, fmt.Sprintf(
				"widget '%s': %s action %s (%s:) has no %s %s (it declares %s)",
				w.Name, na.Action.Type, na.Action.Target, na.Property,
				plural(len(unknown), "parameter", "parameters"),
				quoteJoin(unknown), quoteJoin(sig.paramNames())))
		}
	}
	return out
}

// isRowAction reports whether a widget's action slot fires per row of the
// widget's own data source, so that the row — not what surrounds the widget —
// is what Mendix fills the flow's parameter in from.
//
// That is a list widget's own click: `Action:` / `OnClick:` on a list view,
// data grid or gallery, both spellings landing on "Action". Studio Pro stores it
// with no parameter mappings (TestApp ConflictedWorkflowDefinitionView
// dataGrid2_2), and judged in the enclosing context it was reported as CE1571
// on pages mxbuild 10.24.25 builds clean (ako/mxcli#1560). A slot that does not
// fire per row — a gallery's onSelectionChange, whose template links no data
// source — keeps the enclosing context. It is the rule argContextForOwnAction
// applies to show page (#552).
func isRowAction(w *ast.WidgetV3, property string) bool {
	return property == "Action" && w.GetDataSource() != nil
}

// controlBarHint is the advice for an action in a control bar, which replaces
// the ordinary "nest it in a data container" hint: a control bar is not
// row-scoped, so nesting is not the remedy — the grid's selection is.
func controlBarHint(controlBarOf string) string {
	if controlBarOf == "" {
		return ""
	}
	return fmt.Sprintf(
		". A control bar is not row-scoped, so no enclosing row fills it in: pass the selection of `%s` "+
			"(`$%s`, once the widget has `Selection:` set), or move the widget into a column, which is row-scoped",
		controlBarOf, controlBarOf)
}

// contextHint says why the enclosing context did not fill the parameter in.
// Without it the reader of a nested widget's error has to work out for themselves
// that the surrounding data container is the wrong type.
func contextHint(enclosing dataContext, paramEntity string) string {
	if paramEntity == "" {
		return ""
	}
	if have := enclosing.describe(); have != "" {
		return fmt.Sprintf(", or nest the widget in a data container of type %s — the enclosing context is %s, "+
			"which Mendix does not use for a %s parameter", paramEntity, have, paramEntity)
	}
	return fmt.Sprintf(", or nest the widget in a data container of type %s, which Mendix fills the parameter in from",
		paramEntity)
}

func quoteJoin(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = "'" + n + "'"
	}
	return strings.Join(q, ", ")
}

// entityCompatibility returns the test for "an object of type A satisfies a
// parameter of type B".
//
// A specialization satisfies its generalization, so the chain is walked. The
// reverse is accepted too: a generalization in context does NOT in fact satisfy a
// specialization parameter, but accepting it costs a missed CE1571 that mxbuild
// still catches, while refusing it would reject a page over a hierarchy this
// check read wrong.
func entityCompatibility(ctx *ExecContext) func(contextQN, paramQN string) bool {
	return func(contextQN, paramQN string) bool {
		if strings.EqualFold(contextQN, paramQN) {
			return true
		}
		return inheritsFrom(ctx, contextQN, paramQN) || inheritsFrom(ctx, paramQN, contextQN)
	}
}

// inheritsFrom reports whether child is derived (directly or not) from ancestor.
func inheritsFrom(ctx *ExecContext, childQN, ancestorQN string) bool {
	if ctx == nil || ctx.Backend == nil {
		return false
	}
	seen := map[string]bool{}
	for qn := childQN; qn != "" && !seen[strings.ToLower(qn)]; {
		seen[strings.ToLower(qn)] = true
		entity, ok := findEntityByQN(ctx.Backend, qn)
		if !ok || entity == nil {
			return false
		}
		if strings.EqualFold(entity.GeneralizationRef, ancestorQN) {
			return true
		}
		qn = entity.GeneralizationRef
	}
	return false
}

// buildFlowSignatures maps each microflow and nanoflow qualified name
// (lower-cased, because Mendix name resolution is case-insensitive) to its
// parameters and return entity.
//
// A flow with no parameters is present with an empty list, which is the point:
// "known, and takes nothing" has to be distinguishable from "not found", or a
// typo in the flow name would be reported as a missing argument.
func buildFlowSignatures(ctx *ExecContext) map[string]*flowSignature {
	result := make(map[string]*flowSignature)
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return result
	}
	if mfs, err := projectMicroflows(ctx); err == nil {
		for _, mf := range liveFirst(mfs, microflowExcluded) {
			if mf == nil {
				continue
			}
			key := strings.ToLower(h.GetQualifiedName(mf.ContainerID, mf.Name))
			if _, seen := result[key]; !seen {
				result[key] = sdkFlowSignature(mf.Parameters, mf.ReturnType)
			}
		}
	}
	if nfs, err := projectNanoflows(ctx); err == nil {
		for _, nf := range liveFirst(nfs, nanoflowExcluded) {
			if nf == nil {
				continue
			}
			key := strings.ToLower(h.GetQualifiedName(nf.ContainerID, nf.Name))
			if _, seen := result[key]; !seen {
				result[key] = sdkFlowSignature(nf.Parameters, nf.ReturnType)
			}
		}
	}
	return result
}

func sdkFlowSignature(params []*microflows.MicroflowParameter, ret microflows.DataType) *flowSignature {
	sig := &flowSignature{Returns: sdkDataTypeEntity(ret)}
	for _, p := range params {
		if p == nil {
			continue
		}
		entity := sdkDataTypeEntity(p.Type)
		sig.Params = append(sig.Params, flowParam{
			Name:   p.Name,
			Entity: entity,
			Object: entity != "",
			List:   isSDKListType(p.Type),
		})
	}
	return sig
}

func isSDKListType(dt microflows.DataType) bool {
	switch dt.(type) {
	case *microflows.ListType, microflows.ListType:
		return true
	}
	return false
}

// sdkDataTypeEntity returns the entity qualified name behind an object or list
// data type, and "" for everything else.
func sdkDataTypeEntity(dt microflows.DataType) string {
	switch t := dt.(type) {
	case *microflows.ObjectType:
		return t.EntityQualifiedName
	case *microflows.ListType:
		return t.EntityQualifiedName
	case microflows.ObjectType:
		return t.EntityQualifiedName
	case microflows.ListType:
		return t.EntityQualifiedName
	}
	return ""
}

// astFlowSignature builds a signature from a CREATE MICROFLOW / CREATE NANOFLOW
// statement, for a flow this script has not written yet.
func astFlowSignature(params []ast.MicroflowParam, ret *ast.MicroflowReturnType) *flowSignature {
	sig := &flowSignature{ReturnKind: ast.TypeVoid}
	if ret != nil {
		sig.Returns = astDataTypeEntity(ret.Type)
		sig.ReturnKind = ret.Type.Kind
	}
	for _, p := range params {
		entity := astDataTypeEntity(p.Type)
		sig.Params = append(sig.Params, flowParam{
			Name:   p.Name,
			Entity: entity,
			Object: entity != "",
			List:   p.Type.Kind == ast.TypeListOf,
		})
	}
	return sig
}

// astDataTypeEntity extracts the entity qualified name from a parsed data type.
//
// A bare `Module.Name` parses as TypeEnumeration with EnumRef set — the visitor
// cannot tell an entity from an enumeration there — so EnumRef is the documented
// fallback, unless the type was written with the unambiguous `ENUM …` form.
func astDataTypeEntity(dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeEntity, ast.TypeListOf:
		if dt.EntityRef != nil {
			return dt.EntityRef.String()
		}
	case ast.TypeEnumeration:
		if !dt.ExplicitEnum && dt.EnumRef != nil {
			return dt.EnumRef.String()
		}
	}
	if dt.EntityRef != nil {
		return dt.EntityRef.String()
	}
	return ""
}
