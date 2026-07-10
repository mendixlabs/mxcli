// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow flow graph builder: core types and helpers
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// flowBuilder helps construct the flow graph from AST statements.
type flowBuilder struct {
	objects             []microflows.MicroflowObject
	flows               []*microflows.SequenceFlow
	annotationFlows     []*microflows.AnnotationFlow
	posX                int
	posY                int
	baseY               int // Base Y position (for returning after ELSE branches)
	spacing             int
	returnValue         string // Return value expression for RETURN statement (used by buildFlowGraph final EndEvent)
	returnType          *ast.MicroflowReturnType
	endsWithReturn      bool              // True if the flow already ends with EndEvent(s) from RETURN statements
	lastReturnEndID     model.ID          // Last explicit RETURN EndEvent, used as a fallback error-handler target
	varTypes            map[string]string // Variable name -> entity qualified name (for CHANGE statements)
	declaredVars        map[string]string // Declared primitive variables: name -> type (e.g., "$IsValid" -> "Boolean")
	errors              []string          // Validation errors collected during build
	measurer            *layoutMeasurer   // For measuring statement dimensions
	nextConnectionPoint model.ID          // For compound statements: the exit point differs from entry point
	// incomingRedirect, when set, instructs the next enclosing flow emission to
	// terminate at this merge/activity ID instead of the most recently emitted
	// activity. Used by retry-loop error handlers (where a merge is inserted
	// before the source activity so the normal inbound flow enters the merge
	// and the error-handler tail loops back to the same merge). Cleared after
	// the outer loop consumes it for the next flow.
	incomingRedirect model.ID
	nextFlowCase     string // If set, next connecting flow uses this case value (for merge-less splits)
	// nextFlowAnchor carries the branch-specific FlowAnchors that should be
	// applied to the flow created by the NEXT iteration of buildFlowGraph.
	// Used by guard-pattern IFs (where one branch returns and the other
	// continues) so the continuing branch's @anchor survives to the actual
	// splitID→nextActivity flow — which is emitted one iteration later by the
	// outer loop, not by addIfStatement.
	nextFlowAnchor       *ast.FlowAnchors
	backend              backend.FullBackend          // For looking up page/microflow references
	hierarchy            *ContainerHierarchy          // For resolving container IDs to module names
	pendingAnnotations   *ast.ActivityAnnotations     // Pending annotations to attach to next activity
	restServices         []*model.ConsumedRestService // Cached REST services for parameter classification
	listInputVariables   map[string]bool              // Variables later consumed by list-only actions
	objectInputVariables map[string]bool              // Variables later consumed through object/attribute access
	// previousStmtAnchor holds the Anchor annotation of the statement that
	// just emitted an activity, so the next flow's OriginConnectionIndex can
	// be overridden by the user. Cleared after each flow is created.
	previousStmtAnchor *ast.FlowAnchors
	// Cached flow lists to avoid repeated backend calls during lookups.
	microflowsCache       []*microflows.Microflow
	microflowsCacheLoaded bool
	nanoflowsCache        []*microflows.Nanoflow
	nanoflowsCacheLoaded  bool
	manualLoopBackTarget  model.ID
	isNanoflow            bool // true when building a nanoflow — default error handling is "" not "Rollback"
	// Pending custom error-handler routing uses two representations: the
	// currently active handler lives in the flat fields below, while handlers
	// postponed across branch boundaries are queued in pendingErrorHandlers.
	// Mutate this state through the helper methods in builder_flows.go so the
	// active/queued invariant stays synchronized.
	emptyErrorHandlerFrom    model.ID
	errorHandlerTailFrom     model.ID
	errorHandlerSource       model.ID
	errorHandlerSkipVar      string
	errorHandlerTailCase     string
	errorHandlerTailAnchor   *ast.FlowAnchors
	errorHandlerTailIsSource bool
	errorHandlerReturnValue  string
	pendingErrorHandlers     []pendingErrorHandlerState
}

type flowBuilderVariableState struct {
	varTypes     map[string]string
	declaredVars map[string]string
}

func (fb *flowBuilder) snapshotVariableState() flowBuilderVariableState {
	return flowBuilderVariableState{
		varTypes:     cloneStringMap(fb.varTypes),
		declaredVars: cloneStringMap(fb.declaredVars),
	}
}

func (fb *flowBuilder) restoreVariableState(state flowBuilderVariableState) {
	fb.varTypes = state.varTypes
	fb.declaredVars = state.declaredVars
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// addError records a validation error during flow building.
func (fb *flowBuilder) addError(format string, args ...any) {
	fb.errors = append(fb.errors, fmt.Sprintf(format, args...))
}

// addErrorWithExample records a validation error with a code example showing the fix.
func (fb *flowBuilder) addErrorWithExample(message, example string) {
	fb.errors = append(fb.errors, fmt.Sprintf("%s\n\n  Example:\n%s", message, example))
}

// GetErrors returns all validation errors collected during build.
func (fb *flowBuilder) GetErrors() []string {
	return fb.errors
}

func (fb *flowBuilder) hasDeclaredReturnValue() bool {
	return fb.returnType != nil && fb.returnType.Type.Kind != ast.TypeVoid
}

// errorExampleDeclareVariable returns an example for declaring a variable.
func errorExampleDeclareVariable(varName string) string {
	// Remove $ prefix if present for cleaner display
	cleanName := varName
	if len(varName) > 0 && varName[0] == '$' {
		cleanName = varName[1:]
	}
	return fmt.Sprintf(`    declare $%s Boolean = true;  -- or String, Integer, Decimal, DateTime
    ...
    set $%s = false;`, cleanName, cleanName)
}

// isVariableDeclared checks if a variable has been declared (either as primitive or entity).
func (fb *flowBuilder) isVariableDeclared(varName string) bool {
	name := strings.TrimPrefix(varName, "$")
	if idx := strings.IndexByte(name, '/'); idx >= 0 {
		name = name[:idx]
	}
	if _, ok := fb.varTypes[name]; ok {
		return true
	}
	if _, ok := fb.declaredVars[name]; ok {
		return true
	}
	return false
}

// registerResultVariableType records the output type of an action so later
// statements such as CHANGE, ADD TO, or attribute access can resolve members.
// When dt is nil (e.g. backend lookup failed), any stale entity/list typing is
// cleared but the variable remains declared as Unknown so downstream statements
// don't report it as undeclared.
func (fb *flowBuilder) registerResultVariableType(varName string, dt microflows.DataType) {
	if varName == "" {
		return
	}
	if dt == nil {
		if fb.varTypes != nil {
			delete(fb.varTypes, varName)
		}
		if fb.declaredVars != nil {
			fb.declaredVars[varName] = "Unknown"
		}
		return
	}

	switch t := dt.(type) {
	case *microflows.ObjectType:
		entityQName := t.EntityQualifiedName
		if entityQName == "" && t.EntityID != "" {
			entityQName = fb.resolveEntityQualifiedName(t.EntityID)
		}
		if fb.varTypes != nil && entityQName != "" {
			fb.varTypes[varName] = entityQName
			return
		}
	case *microflows.ListType:
		entityQName := t.EntityQualifiedName
		if entityQName == "" && t.EntityID != "" {
			entityQName = fb.resolveEntityQualifiedName(t.EntityID)
		}
		if fb.varTypes != nil && entityQName != "" {
			fb.varTypes[varName] = "List of " + entityQName
			return
		}
	}

	if fb.declaredVars != nil {
		fb.declaredVars[varName] = dt.GetTypeName()
	}
}

// lookupMicroflowReturnType resolves the return type of a called microflow by
// qualified name so downstream activities can infer variable types.
func (fb *flowBuilder) lookupMicroflowReturnType(qualifiedName string) microflows.DataType {
	if fb.backend == nil || qualifiedName == "" {
		return nil
	}

	// Fast path: direct lookup by qualified name avoids O(n) module walk.
	// Falls through to module walk on any error (not found, corrupt BSON, etc.).
	if rawUnit, err := fb.backend.GetRawUnitByName("microflow", qualifiedName); err == nil && rawUnit != nil && len(rawUnit.Contents) > 0 {
		if mf, err := fb.backend.ParseMicroflowBSON(rawUnit.Contents, model.ID(rawUnit.ID), ""); err == nil && mf != nil {
			return mf.ReturnType
		}
	}

	// Slow path: enumerate all microflows in the module and match by name.
	moduleName, microflowName, ok := strings.Cut(qualifiedName, ".")
	if !ok || moduleName == "" || microflowName == "" {
		return nil
	}

	module, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return nil
	}
	if !fb.microflowsCacheLoaded {
		microflowList, err := fb.backend.ListMicroflows()
		if err != nil {
			return nil
		}
		fb.microflowsCache = microflowList
		fb.microflowsCacheLoaded = true
	}

	for _, mf := range fb.microflowsCache {
		if mf == nil {
			continue
		}
		containerModuleID := mf.ContainerID
		if fb.hierarchy != nil {
			containerModuleID = fb.hierarchy.FindModuleID(mf.ContainerID)
		}
		if containerModuleID == module.ID && mf.Name == microflowName {
			return mf.ReturnType
		}
	}

	return nil
}

func (fb *flowBuilder) lookupNanoflowReturnType(qualifiedName string) microflows.DataType {
	if fb.backend == nil || qualifiedName == "" {
		return nil
	}

	// Fast path: direct lookup by qualified name avoids O(n) module walk.
	// Falls through to module walk on any error (not found, corrupt BSON, etc.).
	if rawUnit, err := fb.backend.GetRawUnitByName("nanoflow", qualifiedName); err == nil && rawUnit != nil && len(rawUnit.Contents) > 0 {
		if nf, err := fb.backend.ParseMicroflowBSON(rawUnit.Contents, model.ID(rawUnit.ID), ""); err == nil && nf != nil {
			return nf.ReturnType
		}
	}

	// Slow path: enumerate all nanoflows in the module and match by name.
	moduleName, nanoflowName, ok := strings.Cut(qualifiedName, ".")
	if !ok || moduleName == "" || nanoflowName == "" {
		return nil
	}

	module, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return nil
	}
	if !fb.nanoflowsCacheLoaded {
		nanoflowList, err := fb.backend.ListNanoflows()
		if err != nil {
			return nil
		}
		fb.nanoflowsCache = nanoflowList
		fb.nanoflowsCacheLoaded = true
	}

	for _, nf := range fb.nanoflowsCache {
		if nf == nil {
			continue
		}
		containerModuleID := nf.ContainerID
		if fb.hierarchy != nil {
			containerModuleID = fb.hierarchy.FindModuleID(nf.ContainerID)
		}
		if containerModuleID == module.ID && nf.Name == nanoflowName {
			return nf.ReturnType
		}
	}

	return nil
}

// microflowExists returns true if qualifiedName refers to a microflow present
// in the connected project. Returns true (no error) when no backend is available
// or when backend calls fail, so offline / syntax-check mode is unaffected.
func (fb *flowBuilder) microflowExists(qualifiedName string) bool {
	if fb.backend == nil {
		return true
	}
	// Fast path: name-indexed lookup; succeeds without loading the full list.
	if rawUnit, err := fb.backend.GetRawUnitByName("microflow", qualifiedName); err == nil && rawUnit != nil {
		return true
	}
	// Slow path: enumerate microflows in the resolved module.
	moduleName, mfName, ok := strings.Cut(qualifiedName, ".")
	if !ok || moduleName == "" || mfName == "" {
		return true // malformed name — not our problem to report
	}
	module, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return true // module not found — let other validation surface that
	}
	if !fb.microflowsCacheLoaded {
		list, err := fb.backend.ListMicroflows()
		if err != nil {
			return true // backend error — don't block
		}
		fb.microflowsCache = list
		fb.microflowsCacheLoaded = true
	}
	for _, mf := range fb.microflowsCache {
		if mf == nil {
			continue
		}
		containerModuleID := mf.ContainerID
		if fb.hierarchy != nil {
			containerModuleID = fb.hierarchy.FindModuleID(mf.ContainerID)
		}
		if containerModuleID == module.ID && mf.Name == mfName {
			return true
		}
	}
	return false
}

// nanoflowExists returns true if qualifiedName refers to a nanoflow present
// in the connected project. Same fallback-to-true semantics as microflowExists.
func (fb *flowBuilder) nanoflowExists(qualifiedName string) bool {
	if fb.backend == nil {
		return true
	}
	if rawUnit, err := fb.backend.GetRawUnitByName("nanoflow", qualifiedName); err == nil && rawUnit != nil {
		return true
	}
	moduleName, nfName, ok := strings.Cut(qualifiedName, ".")
	if !ok || moduleName == "" || nfName == "" {
		return true
	}
	module, err := fb.backend.GetModuleByName(moduleName)
	if err != nil || module == nil {
		return true
	}
	if !fb.nanoflowsCacheLoaded {
		list, err := fb.backend.ListNanoflows()
		if err != nil {
			return true
		}
		fb.nanoflowsCache = list
		fb.nanoflowsCacheLoaded = true
	}
	for _, nf := range fb.nanoflowsCache {
		if nf == nil {
			continue
		}
		containerModuleID := nf.ContainerID
		if fb.hierarchy != nil {
			containerModuleID = fb.hierarchy.FindModuleID(nf.ContainerID)
		}
		if containerModuleID == module.ID && nf.Name == nfName {
			return true
		}
	}
	return false
}

func (fb *flowBuilder) resolveEntityQualifiedName(entityID model.ID) string {
	if fb.backend == nil || entityID == "" {
		return ""
	}

	domainModels, err := fb.backend.ListDomainModels()
	if err != nil {
		return ""
	}

	for _, dm := range domainModels {
		if dm == nil {
			continue
		}

		moduleName := ""
		if fb.hierarchy != nil {
			moduleName = fb.hierarchy.GetModuleName(dm.ContainerID)
		}
		if moduleName == "" {
			if mod, err := fb.backend.GetModule(dm.ContainerID); err == nil && mod != nil {
				moduleName = mod.Name
			}
		}
		if moduleName == "" {
			continue
		}

		for _, entity := range dm.Entities {
			if entity != nil && entity.ID == entityID {
				return moduleName + "." + entity.Name
			}
		}
	}

	return ""
}

// exprToString converts an AST Expression to a Mendix expression string,
// resolving association navigation paths to include the target entity qualifier.
// e.g. $Order/MyModule.Order_Customer/Name → $Order/MyModule.Order_Customer/MyModule.Customer/Name
func (fb *flowBuilder) exprToString(expr ast.Expression) string {
	resolved := fb.resolveAssociationPaths(expr)
	return expressionToString(resolved)
}

// resolveAssociationPaths walks an expression tree and, for any AttributePathExpr
// whose path contains an association (qualified name like Module.AssocName), inserts
// the association's target entity after the association segment.
func (fb *flowBuilder) resolveAssociationPaths(expr ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.AttributePathExpr:
		resolved := fb.resolvePathSegments(e.Path)
		return &ast.AttributePathExpr{
			Variable: e.Variable,
			Path:     resolved,
			Segments: e.Segments,
		}
	case *ast.BinaryExpr:
		return &ast.BinaryExpr{
			Left:     fb.resolveAssociationPaths(e.Left),
			Operator: e.Operator,
			Right:    fb.resolveAssociationPaths(e.Right),
		}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{
			Operator: e.Operator,
			Operand:  fb.resolveAssociationPaths(e.Operand),
		}
	case *ast.FunctionCallExpr:
		args := make([]ast.Expression, len(e.Arguments))
		for i, arg := range e.Arguments {
			args[i] = fb.resolveAssociationPaths(arg)
		}
		return &ast.FunctionCallExpr{
			Name:      e.Name,
			Arguments: args,
		}
	case *ast.ParenExpr:
		return &ast.ParenExpr{Inner: fb.resolveAssociationPaths(e.Inner)}
	case *ast.IfThenElseExpr:
		return &ast.IfThenElseExpr{
			Condition: fb.resolveAssociationPaths(e.Condition),
			ThenExpr:  fb.resolveAssociationPaths(e.ThenExpr),
			ElseExpr:  fb.resolveAssociationPaths(e.ElseExpr),
		}
	case *ast.SourceExpr:
		if e.Source != "" {
			// Non-empty Source is the exact expression text to write back.
			// Rebuilding it here would defeat the whitespace-preservation
			// purpose of SourceExpr, so keep the parsed tree only for callers
			// that need semantic inspection.
			return e
		}
		return fb.resolveAssociationPaths(e.Expression)
	default:
		return expr
	}
}

// resolvePathSegments processes path segments in an attribute path expression.
// For each segment that is a qualified association name (Module.AssocName), it looks up
// the association's target entity and inserts it after the association.
func (fb *flowBuilder) resolvePathSegments(path []string) []string {
	if fb.backend == nil || len(path) == 0 {
		return path
	}

	var resolved []string
	for i, segment := range path {
		resolved = append(resolved, segment)

		// A qualified name (contains ".") that isn't the last segment might be an association
		if !strings.Contains(segment, ".") {
			continue
		}
		// If the next segment is already a qualified name, the target entity is already present
		if i+1 < len(path) && strings.Contains(path[i+1], ".") {
			continue
		}
		// If this is the last segment, nothing to insert after
		if i == len(path)-1 {
			continue
		}

		// Look up association target entity
		parts := strings.SplitN(segment, ".", 2)
		if len(parts) != 2 {
			continue
		}
		result := fb.lookupAssociation(parts[0], parts[1])
		if result != nil && result.childEntityQN != "" {
			resolved = append(resolved, result.childEntityQN)
		}
	}
	return resolved
}

// buildSplitCondition constructs the right SplitCondition variant for an IF
// statement. When the condition is a qualified call into a rule, it emits a
// RuleSplitCondition (nested RuleCall with ParameterMappings). Everything else
// falls back to ExpressionSplitCondition.
//
// Studio Pro enforces this distinction: a rule reference stored as an
// expression fails validation with CE0117, which is the regression this
// helper prevents on describe → exec roundtrips.
func (fb *flowBuilder) buildSplitCondition(expr ast.Expression, fallbackExpression string) microflows.SplitCondition {
	if ruleCond := fb.tryBuildRuleSplitCondition(expr); ruleCond != nil {
		return ruleCond
	}
	return &microflows.ExpressionSplitCondition{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Expression:  fallbackExpression,
	}
}

// tryBuildRuleSplitCondition returns a RuleSplitCondition when the expression
// is a qualified function call that resolves to a rule via the backend.
// Returns nil if the expression isn't a qualified call, if the backend is
// unavailable, or if the name doesn't resolve to a rule.
func (fb *flowBuilder) tryBuildRuleSplitCondition(expr ast.Expression) *microflows.RuleSplitCondition {
	if fb.backend == nil {
		return nil
	}
	call := unwrapParenCall(expr)
	if call == nil {
		return nil
	}
	// Only qualified names (Module.Name) can refer to rules; bare identifiers
	// are built-ins (length, contains, etc.).
	if !strings.Contains(call.Name, ".") {
		return nil
	}
	isRule, err := fb.backend.IsRule(call.Name)
	if err != nil || !isRule {
		return nil
	}

	cond := &microflows.RuleSplitCondition{
		BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
		RuleQualifiedName: call.Name,
	}
	for _, arg := range call.Arguments {
		name, value := extractNamedArg(arg)
		if name == "" {
			// Positional arguments aren't representable in RuleCall — skip
			// rather than fabricate a parameter mapping that Studio Pro
			// would reject.
			continue
		}
		cond.ParameterMappings = append(cond.ParameterMappings, &microflows.RuleCallParameterMapping{
			BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
			ParameterName: call.Name + "." + name,
			Argument:      fb.exprToString(value),
		})
	}
	return cond
}

// unwrapParenCall peels outer ParenExprs and returns the inner FunctionCallExpr
// if present. Describer output wraps rule calls in parens when they sit inside
// boolean expressions, so we must see through them.
func unwrapParenCall(expr ast.Expression) *ast.FunctionCallExpr {
	for {
		switch e := expr.(type) {
		case *ast.FunctionCallExpr:
			return e
		case *ast.ParenExpr:
			expr = e.Inner
		case *ast.SourceExpr:
			expr = e.Expression
		default:
			return nil
		}
	}
}

// extractNamedArg recognises `Name = value` BinaryExprs and returns the
// parameter name + value. Anything else returns "", nil.
//
// The left side of a named-arg expression can surface as either an
// IdentifierExpr (bare parameter name) or an AttributePathExpr with an empty
// Variable — both forms come out of the visitor depending on surrounding
// context, so handle them both.
func extractNamedArg(expr ast.Expression) (string, ast.Expression) {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Operator != "=" {
		return "", nil
	}
	switch left := bin.Left.(type) {
	case *ast.IdentifierExpr:
		return left.Name, bin.Right
	case *ast.AttributePathExpr:
		if left.Variable == "" && len(left.Path) == 1 {
			return left.Path[0], bin.Right
		}
	}
	return "", nil
}
