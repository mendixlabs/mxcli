// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

// ValidateMicroflow checks a microflow for common issues that don't require a project connection.
// Returns a list of structured violations with rule IDs.
func ValidateMicroflow(stmt *ast.CreateMicroflowStmt) []linter.Violation {
	v := &microflowValidator{
		mfName:     stmt.Name.String(),
		returnType: stmt.ReturnType,
		varKinds:   map[string]exprcheck.TypeKind{},
	}
	// Seed the variable→kind scope with the microflow's parameters so numeric
	// assignment checks can resolve operands like $count.
	for _, p := range stmt.Parameters {
		if k, ok := astKindToExprKind(p.Type.Kind); ok {
			v.varKinds[p.Name] = k
		}
	}
	// Validate parameter entity references — reject bare names without module prefix
	for _, p := range stmt.Parameters {
		if p.Type.EntityRef != nil && p.Type.EntityRef.Module == "" {
			v.addViolation("MDL008", linter.SeverityError,
				fmt.Sprintf("parameter '$%s': entity type '%s' is missing module prefix",
					p.Name, p.Type.EntityRef.Name),
				fmt.Sprintf("Use a qualified name like 'Module.%s' or 'System.%s'",
					p.Type.EntityRef.Name, p.Type.EntityRef.Name))
		}
	}
	v.validate(stmt.Body)
	return v.violations
}

// microflowValidator holds state for validating a single microflow.
type microflowValidator struct {
	mfName        string
	returnType    *ast.MicroflowReturnType // nil = void
	violations    []linter.Violation
	loopDepth     int             // Track nesting depth inside loops
	emptyListVars map[string]bool // List variables declared empty and never populated
	// varKinds maps in-scope variable names (params + declared) to their kind,
	// used to detect assigning a Decimal expression to an Integer/Long target.
	varKinds map[string]exprcheck.TypeKind
}

func (v *microflowValidator) addViolation(ruleID string, severity linter.Severity, message, suggestion string) {
	v.violations = append(v.violations, linter.Violation{
		RuleID:   ruleID,
		Severity: severity,
		Message:  message,
		Location: linter.Location{
			DocumentType: "microflow",
			DocumentName: v.mfName,
		},
		Suggestion: suggestion,
	})
}

// validate runs all checks on the microflow body.
func (v *microflowValidator) validate(body []ast.MicroflowStatement) {
	// Walk the body for per-statement checks (validation feedback, return value checks)
	v.emptyListVars = make(map[string]bool)
	v.walkBody(body)

	// Check 5: missing RETURN on non-void microflow paths
	if v.returnType != nil && v.returnType.Type.Kind != ast.TypeVoid {
		if !bodyReturns(body) {
			v.addViolation("MDL003", linter.SeverityError,
				fmt.Sprintf("microflow returns %s but not all code paths have a return statement",
					returnTypeString(v.returnType)),
				"Add return statements to all code paths")
		}
	}

	// Check 3: variable scope — detect variables declared inside branches but used after
	v.checkBranchScoping(body)
}

// walkBody recursively walks microflow body statements looking for per-statement issues.
func (v *microflowValidator) walkBody(body []ast.MicroflowStatement) {
	for _, s := range body {
		switch stmt := s.(type) {
		case *ast.ValidationFeedbackStmt:
			if isEmptyMessage(stmt.Message) {
				v.addViolation("MDL007", linter.SeverityWarning,
					"validation feedback has empty message template. "+
						"Mendix requires a non-empty feedback message (CE0091).",
					"Add a message template to the validation feedback action")
			}
		case *ast.ReturnStmt:
			v.checkReturn(stmt)
		case *ast.IfStmt:
			v.walkBody(stmt.ThenBody)
			v.walkBody(stmt.ElseBody)
		case *ast.EnumSplitStmt:
			// Mendix enumeration splits map to exclusive splits with one outgoing
			// flow per enum value. Multiple values per branch and a default (else)
			// flow are not supported — Studio Pro will reject both with CE errors.
			if len(stmt.ElseBody) > 0 {
				v.addViolation("MDL008", linter.SeverityError,
					fmt.Sprintf("case statement on '$%s' has an else branch; "+
						"Mendix enumeration splits do not support a default case. "+
						"Add an explicit when branch for every enum value instead.",
						stmt.Variable),
					"Add an explicit when branch for every enum value instead of using else")
			}
			for _, c := range stmt.Cases {
				if len(c.Values) > 1 {
					v.addViolation("MDL009", linter.SeverityError,
						fmt.Sprintf("case statement on '$%s': when branch lists %d values (%s); "+
							"Mendix enumeration splits require exactly one value per branch.",
							stmt.Variable, len(c.Values), strings.Join(c.Values, ", ")),
						"Split into separate when branches, one per enum value")
				}
				v.walkBody(c.Body)
			}
			v.walkBody(stmt.ElseBody)
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				v.walkBody(c.Body)
			}
			v.walkBody(stmt.ElseBody)
		case *ast.DeclareStmt:
			if stmt.Type.Kind == ast.TypeListOf {
				// A `declare` maps to a Create Variable activity, which cannot
				// produce a list — Studio Pro rejects it with CE0053 ("type not
				// allowed") and CE0038 ("value required"). Lists must come from a
				// microflow parameter, a `retrieve`, or a `create list`. (#607)
				v.addViolation("MDL040", linter.SeverityError,
					fmt.Sprintf("declare '$%s' creates a list variable, but Mendix does not allow the "+
						"Create Variable activity to produce a list (CE0053/CE0038). "+
						"Pass the list as a microflow parameter, populate it with retrieve, or use create list.",
						stmt.Variable),
					"Accept the list as a parameter, use retrieve, or use create list — do not declare a list variable")
				// Track list variables declared as empty (candidates for the empty-list-in-loop anti-pattern)
				if isEmptyInit(stmt.InitialValue) {
					v.emptyListVars[stmt.Variable] = true
				}
			}
			// Register the declared variable's kind for later assignment checks,
			// and flag a Decimal initial value assigned to an Integer/Long declare.
			if k, ok := astKindToExprKind(stmt.Type.Kind); ok {
				v.varKinds[stmt.Variable] = k
				if stmt.InitialValue != nil {
					v.checkNumericAssignment("$"+stmt.Variable, k, stmt.InitialValue)
				}
			}
		case *ast.MfSetStmt:
			// SET on a plain variable target (not $var/Member = …, which is a
			// member change). Flag a Decimal value assigned to an Integer/Long var.
			if !strings.Contains(stmt.Target, "/") {
				if k, ok := v.varKinds[stmt.Target]; ok {
					v.checkNumericAssignment("$"+stmt.Target, k, stmt.Value)
				}
			}
		case *ast.RetrieveStmt:
			// RETRIEVE populates a list variable — remove from empty tracking
			delete(v.emptyListVars, stmt.Variable)
		case *ast.LoopStmt:
			// Check: @caption on a loop is silently dropped — Mendix for-loops
			// have no caption (Microflows$LoopedActivity has no Caption
			// property; Studio Pro auto-labels them from the iterator). The
			// supported way to label a loop is an annotation note.
			if stmt.Annotations != nil && stmt.Annotations.Caption != "" {
				v.addViolation("MDL042", linter.SeverityWarning,
					"@caption on a loop has no effect — Mendix loops have no caption "+
						"(the loop activity has no Caption property, so it is dropped). "+
						"Use @annotation to attach a note to the loop instead.",
					"Replace @caption with @annotation to label the loop")
			}
			// Check: nested loop anti-pattern
			if v.loopDepth > 0 {
				v.addViolation("MDL001", linter.SeverityWarning,
					"nested loop detected (loop inside a loop). "+
						"Use retrieve $Match from $List where ... limit 1 for list matching instead of nested loops (O(N^2) performance).",
					"Replace nested loop with retrieve ... where ... limit 1 for O(N) lookup")
			}
			// Check: loop over empty declared list
			if v.emptyListVars[stmt.ListVariable] {
				v.addViolation("MDL002", linter.SeverityWarning,
					fmt.Sprintf("loop iterates over '$%s' which was declared as an empty list and never populated. "+
						"Pass the list as a microflow parameter instead of creating an empty variable.",
						stmt.ListVariable),
					"Pass the list as a microflow parameter instead of creating an empty variable")
			}
			v.loopDepth++
			v.walkBody(stmt.Body)
			v.loopDepth--
		}
		// Check error handling inside loops
		if eh := stmtErrorHandling(s); eh != nil {
			v.checkErrorHandlingInLoop(s, eh)
			// Also walk ON ERROR bodies
			if len(eh.Body) > 0 {
				v.walkBody(eh.Body)
			}
		}
	}
}

// checkNumericAssignment flags assigning a Decimal-typed expression to an
// Integer or Long target. Mendix integer division (`div`) yields a Decimal, so
// `set $IntVar = $a * 100 div $b;` fails mx check with CE0117 even though the
// syntax is valid. Only Integer/Long targets with a provably-Decimal value are
// flagged (unknown inference never fires), keeping false positives out.
func (v *microflowValidator) checkNumericAssignment(targetLabel string, targetKind exprcheck.TypeKind, value ast.Expression) {
	if targetKind != exprcheck.KindInteger && targetKind != exprcheck.KindLong {
		return
	}
	src := microflowExprSource(value)
	if src == "" {
		return
	}
	// Only flag a raw arithmetic Decimal (e.g. `$a div $b`); a rounding function
	// result assigned to Integer is accepted by Mendix and must not be flagged.
	if !exprcheck.SourceIsArithmeticDecimal(src, v.varKinds) {
		return
	}
	target := "Integer"
	if targetKind == exprcheck.KindLong {
		target = "Long"
	}
	v.addViolation("MDL041", linter.SeverityError,
		fmt.Sprintf("assigning a Decimal expression to %s variable '%s' — Mendix rejects this with CE0117. "+
			"Integer division ('div') always yields a Decimal.", target, targetLabel),
		fmt.Sprintf("Declare '%s' as Decimal, or round the value (e.g. round(%s) or floor(%s)).", targetLabel, src, src))
}

// microflowExprSource returns the Mendix source text of a microflow value
// expression: the preserved raw source when available, otherwise the structured
// expression rendered back to a string. Returns "" when nothing is available.
func microflowExprSource(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	if se, ok := expr.(*ast.SourceExpr); ok && se.Source != "" {
		return se.Source
	}
	return expressionToString(expr)
}

// astKindToExprKind maps an MDL primitive data-type kind to an exprcheck kind.
// Returns false for non-primitive / unmappable kinds (entities, lists, void).
func astKindToExprKind(k ast.DataTypeKind) (exprcheck.TypeKind, bool) {
	switch k {
	case ast.TypeString, ast.TypeStringTemplate:
		return exprcheck.KindString, true
	case ast.TypeInteger, ast.TypeAutoNumber:
		return exprcheck.KindInteger, true
	case ast.TypeLong:
		return exprcheck.KindLong, true
	case ast.TypeDecimal:
		return exprcheck.KindDecimal, true
	case ast.TypeBoolean:
		return exprcheck.KindBoolean, true
	case ast.TypeDateTime, ast.TypeDate:
		return exprcheck.KindDateTime, true
	case ast.TypeBinary:
		return exprcheck.KindBinary, true
	case ast.TypeEnumeration:
		return exprcheck.KindEnumeration, true
	default:
		return exprcheck.KindUnknown, false
	}
}

// checkErrorHandlingInLoop warns if custom error handling is used inside a loop.
// Mendix requires error handling to be 'Rollback' inside looped activities (CE0644, CE6035).
func (v *microflowValidator) checkErrorHandlingInLoop(stmt ast.MicroflowStatement, eh *ast.ErrorHandlingClause) {
	if v.loopDepth == 0 {
		return // Not inside a loop
	}

	// Only Rollback is allowed inside loops
	if eh.Type != ast.ErrorHandlingRollback && eh.Type != "" {
		activityName := stmtActivityName(stmt)
		v.addViolation("MDL006", linter.SeverityWarning,
			fmt.Sprintf("%s has error handling type '%s' inside a loop. "+
				"Mendix requires error handling to be 'Rollback' inside looped activities (CE0644).",
				activityName, eh.Type),
			"Extract the activity with custom error handling into a submicroflow")
	}
}

// stmtActivityName returns a human-readable name for a statement type.
func stmtActivityName(stmt ast.MicroflowStatement) string {
	switch stmt.(type) {
	case *ast.CreateObjectStmt:
		return "create"
	case *ast.DeleteObjectStmt:
		return "delete"
	case *ast.MfCommitStmt:
		return "commit"
	case *ast.RetrieveStmt:
		return "retrieve"
	case *ast.CallMicroflowStmt:
		return "call microflow"
	case *ast.CallNanoflowStmt:
		return "call nanoflow"
	case *ast.CallJavaActionStmt:
		return "call java action"
	case *ast.CallJavaScriptActionStmt:
		return "call javascript action"
	case *ast.CallWebServiceStmt:
		return "call web service"
	case *ast.ExecuteDatabaseQueryStmt:
		return "execute database query"
	default:
		return "Activity"
	}
}

// checkReturn validates a RETURN statement against the microflow's return type.
func (v *microflowValidator) checkReturn(stmt *ast.ReturnStmt) {
	isVoid := v.returnType == nil || v.returnType.Type.Kind == ast.TypeVoid
	hasValue := stmt.Value != nil

	// Check 1: RETURN with no value when microflow has a return type
	if !isVoid && !hasValue {
		v.addViolation("MDL004", linter.SeverityError,
			fmt.Sprintf("return requires a value because microflow returns %s",
				returnTypeString(v.returnType)),
			fmt.Sprintf("Add a return value of type %s", returnTypeString(v.returnType)))
		return
	}

	// Check 2: RETURN with value when microflow returns Void
	if isVoid && hasValue {
		// Allow RETURN empty; on void microflows (it's a no-op)
		if lit, ok := stmt.Value.(*ast.LiteralExpr); ok {
			if lit.Kind == ast.LiteralEmpty || lit.Kind == ast.LiteralNull {
				return
			}
		}
		v.addViolation("MDL004", linter.SeverityError,
			"return has a value but microflow does not declare a return type",
			"Remove the return value or add a return type to the microflow")
		return
	}

	// Check 4: literal RETURN from entity-typed microflow
	if !isVoid && hasValue {
		retKind := v.returnType.Type.Kind
		if retKind == ast.TypeEntity || retKind == ast.TypeListOf {
			if isScalarLiteral(stmt.Value) {
				v.addViolation("MDL004", linter.SeverityError,
					fmt.Sprintf("return has a %s literal but microflow returns %s",
						literalKindName(stmt.Value), returnTypeString(v.returnType)),
					fmt.Sprintf("Return an object of type %s instead of a scalar literal", returnTypeString(v.returnType)))
			}
		}
	}
}

// isScalarLiteral returns true if the expression is a string, integer, boolean, or decimal literal.
func isScalarLiteral(expr ast.Expression) bool {
	lit, ok := expr.(*ast.LiteralExpr)
	if !ok {
		return false
	}
	switch lit.Kind {
	case ast.LiteralString, ast.LiteralInteger, ast.LiteralDecimal, ast.LiteralBoolean:
		return true
	}
	return false
}

// literalKindName returns a human-readable name for a literal expression's kind.
func literalKindName(expr ast.Expression) string {
	lit, ok := expr.(*ast.LiteralExpr)
	if !ok {
		return "unknown"
	}
	switch lit.Kind {
	case ast.LiteralString:
		return "String"
	case ast.LiteralInteger:
		return "Integer"
	case ast.LiteralDecimal:
		return "Decimal"
	case ast.LiteralBoolean:
		return "Boolean"
	default:
		return "unknown"
	}
}

// returnTypeString formats a MicroflowReturnType for display in messages.
func returnTypeString(rt *ast.MicroflowReturnType) string {
	if rt == nil {
		return "Void"
	}
	switch rt.Type.Kind {
	case ast.TypeEntity:
		if rt.Type.EntityRef != nil {
			return rt.Type.EntityRef.String()
		}
		return "Entity"
	case ast.TypeListOf:
		if rt.Type.EntityRef != nil {
			return "List of " + rt.Type.EntityRef.String()
		}
		return "List"
	default:
		return rt.Type.Kind.String()
	}
}

// bodyReturns returns true if all execution paths in the body end with a RETURN.
func bodyReturns(stmts []ast.MicroflowStatement) bool {
	if len(stmts) == 0 {
		return false
	}
	// Check from the last statement backwards for a RETURN or exhaustive IF/ELSE
	last := stmts[len(stmts)-1]
	switch s := last.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.IfStmt:
		// Both branches must return, and ELSE must be present
		return len(s.ElseBody) > 0 && bodyReturns(s.ThenBody) && bodyReturns(s.ElseBody)
	case *ast.WhileStmt:
		return isUnconditionalTrueWhile(s) && !containsBreakForCurrentLoop(s.Body)
	case *ast.EnumSplitStmt:
		// else is not supported by Mendix; treat the split as exhaustive if
		// every explicit case ends with a return. Unhandled enum values fall
		// through to the next statement, so callers should add a return after
		// end case when the split may not cover all values.
		if len(s.Cases) == 0 {
			return false
		}
		for _, c := range s.Cases {
			if !bodyReturns(c.Body) {
				return false
			}
		}
		return true
	case *ast.InheritanceSplitStmt:
		if len(s.Cases) == 0 || len(s.ElseBody) == 0 || !bodyReturns(s.ElseBody) {
			return false
		}
		for _, c := range s.Cases {
			if !bodyReturns(c.Body) {
				return false
			}
		}
		return true
	}
	return false
}

func isUnconditionalTrueWhile(s *ast.WhileStmt) bool {
	if s == nil {
		return false
	}
	lit, ok := s.Condition.(*ast.LiteralExpr)
	if !ok || lit.Kind != ast.LiteralBoolean {
		return false
	}
	value, ok := lit.Value.(bool)
	return ok && value
}

// checkBranchScoping detects variables declared inside IF/ELSE branches that are
// referenced in subsequent statements at the same level.
func (v *microflowValidator) checkBranchScoping(body []ast.MicroflowStatement) {
	// Collect variables that are only declared inside branches
	branchVars := make(map[string]string) // varName -> "IF branch" / "ELSE branch" / "ON ERROR body"

	for i, s := range body {
		switch stmt := s.(type) {
		case *ast.IfStmt:
			// Collect vars declared in THEN branch
			for varName := range collectDeclaredVars(stmt.ThenBody) {
				branchVars[varName] = "if branch"
			}
			// Collect vars declared in ELSE branch
			for varName := range collectDeclaredVars(stmt.ElseBody) {
				branchVars[varName] = "else branch"
			}
			// Recurse into branches for nested scoping checks
			v.checkBranchScoping(stmt.ThenBody)
			v.checkBranchScoping(stmt.ElseBody)
		case *ast.EnumSplitStmt:
			for _, c := range stmt.Cases {
				for varName := range collectDeclaredVars(c.Body) {
					branchVars[varName] = "enum split branch"
				}
				v.checkBranchScoping(c.Body)
			}
			for varName := range collectDeclaredVars(stmt.ElseBody) {
				branchVars[varName] = "enum split else branch"
			}
			v.checkBranchScoping(stmt.ElseBody)
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				for varName := range collectDeclaredVars(c.Body) {
					branchVars[varName] = "split type branch"
				}
				v.checkBranchScoping(c.Body)
			}
			for varName := range collectDeclaredVars(stmt.ElseBody) {
				branchVars[varName] = "split type else branch"
			}
			v.checkBranchScoping(stmt.ElseBody)
		case *ast.LoopStmt:
			v.checkBranchScoping(stmt.Body)
		}

		// Check ON ERROR bodies
		if eh := stmtErrorHandling(s); eh != nil && len(eh.Body) > 0 {
			for varName := range collectDeclaredVars(eh.Body) {
				branchVars[varName] = "on error body"
			}
			v.checkBranchScoping(eh.Body)
		}

		// After processing this statement, check if subsequent statements reference branch vars
		if len(branchVars) > 0 {
			for _, subsequent := range body[i+1:] {
				for _, refVar := range referencedVars(subsequent) {
					if scope, ok := branchVars[refVar]; ok {
						v.addViolation("MDL005", linter.SeverityWarning,
							fmt.Sprintf("variable '$%s' is declared inside %s but used outside",
								refVar, scope),
							fmt.Sprintf("Declare '$%s' before the if/else block", refVar))
						// Remove to avoid duplicate warnings
						delete(branchVars, refVar)
					}
				}
			}
		}
	}
}

// collectDeclaredVars returns the set of variable names declared in a body.
func collectDeclaredVars(body []ast.MicroflowStatement) map[string]bool {
	vars := make(map[string]bool)
	for _, s := range body {
		switch stmt := s.(type) {
		case *ast.DeclareStmt:
			vars[stmt.Variable] = true
		case *ast.CreateObjectStmt:
			if stmt.Variable != "" {
				vars[stmt.Variable] = true
			}
		case *ast.RetrieveStmt:
			if stmt.Variable != "" {
				vars[stmt.Variable] = true
			}
		case *ast.CallMicroflowStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.CallNanoflowStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.CallJavaActionStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.CallJavaScriptActionStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.ExecuteDatabaseQueryStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.ListOperationStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.AggregateListStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.CreateListStmt:
			if stmt.Variable != "" {
				vars[stmt.Variable] = true
			}
		case *ast.EnumSplitStmt:
		case *ast.CastObjectStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				for varName := range collectDeclaredVars(c.Body) {
					vars[varName] = true
				}
			}
			for varName := range collectDeclaredVars(stmt.ElseBody) {
				vars[varName] = true
			}
		}
	}
	return vars
}

// referencedVars returns the variable names referenced in a statement (SET targets, RETURN values, etc.).
func referencedVars(stmt ast.MicroflowStatement) []string {
	var refs []string
	switch s := stmt.(type) {
	case *ast.MfSetStmt:
		// SET $Var = expr — the target variable is a reference
		refs = append(refs, extractVarName(s.Target))
		refs = append(refs, exprVarRefs(s.Value)...)
	case *ast.ReturnStmt:
		if s.Value != nil {
			refs = append(refs, exprVarRefs(s.Value)...)
		}
	case *ast.ChangeObjectStmt:
		refs = append(refs, s.Variable)
	case *ast.MfCommitStmt:
		refs = append(refs, s.Variable)
	case *ast.DeleteObjectStmt:
		refs = append(refs, s.Variable)
	case *ast.AddToListStmt:
		if s.Value != nil {
			refs = append(refs, exprVarRefs(s.Value)...)
		} else {
			refs = append(refs, s.Item)
		}
		refs = append(refs, s.List)
	case *ast.RemoveFromListStmt:
		refs = append(refs, s.Item, s.List)
	case *ast.LogStmt:
		refs = append(refs, exprVarRefs(s.Node)...)
		refs = append(refs, exprVarRefs(s.Message)...)
	case *ast.EnumSplitStmt:
		refs = append(refs, extractVarName(s.Variable))
	case *ast.CastObjectStmt:
		if s.ObjectVariable != "" {
			refs = append(refs, s.ObjectVariable)
		}
	case *ast.InheritanceSplitStmt:
		refs = append(refs, s.Variable)
		for _, c := range s.Cases {
			for _, nested := range c.Body {
				refs = append(refs, referencedVars(nested)...)
			}
		}
		for _, nested := range s.ElseBody {
			refs = append(refs, referencedVars(nested)...)
		}
	}
	return refs
}

// extractVarName extracts the base variable name from a target that may include
// a $ prefix or attribute path (e.g., "$Var/Attr" → "Var").
func extractVarName(target string) string {
	name := strings.TrimPrefix(target, "$")
	if before, _, ok := strings.Cut(name, "/"); ok {
		return before
	}
	return name
}

// exprVarRefs extracts variable names referenced in an expression.
func exprVarRefs(expr ast.Expression) []string {
	if expr == nil {
		return nil
	}
	var refs []string
	switch e := expr.(type) {
	case *ast.VariableExpr:
		refs = append(refs, e.Name)
	case *ast.AttributePathExpr:
		refs = append(refs, e.Variable)
	case *ast.BinaryExpr:
		refs = append(refs, exprVarRefs(e.Left)...)
		refs = append(refs, exprVarRefs(e.Right)...)
	case *ast.UnaryExpr:
		refs = append(refs, exprVarRefs(e.Operand)...)
	case *ast.FunctionCallExpr:
		for _, arg := range e.Arguments {
			refs = append(refs, exprVarRefs(arg)...)
		}
	case *ast.ParenExpr:
		refs = append(refs, exprVarRefs(e.Inner)...)
	case *ast.IfThenElseExpr:
		refs = append(refs, exprVarRefs(e.Condition)...)
		refs = append(refs, exprVarRefs(e.ThenExpr)...)
		refs = append(refs, exprVarRefs(e.ElseExpr)...)
	case *ast.SourceExpr:
		refs = append(refs, exprVarRefs(e.Expression)...)
	}
	return refs
}

// stmtErrorHandling returns the ErrorHandlingClause for statements that support it.
func stmtErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	switch s := stmt.(type) {
	case *ast.CreateObjectStmt:
		return s.ErrorHandling
	case *ast.DeleteObjectStmt:
		return s.ErrorHandling
	case *ast.MfCommitStmt:
		return s.ErrorHandling
	case *ast.RetrieveStmt:
		return s.ErrorHandling
	case *ast.CallMicroflowStmt:
		return s.ErrorHandling
	case *ast.CallNanoflowStmt:
		return s.ErrorHandling
	case *ast.CallJavaActionStmt:
		return s.ErrorHandling
	case *ast.DownloadFileStmt:
		return s.ErrorHandling
	case *ast.CallJavaScriptActionStmt:
		return s.ErrorHandling
	case *ast.CallWebServiceStmt:
		return s.ErrorHandling
	case *ast.ExecuteDatabaseQueryStmt:
		return s.ErrorHandling
	}
	return nil
}

// isEmptyInit checks if a variable initializer is empty/nil (used to detect "DECLARE $List List of ... = empty").
func isEmptyInit(expr ast.Expression) bool {
	if expr == nil {
		return true
	}
	if lit, ok := expr.(*ast.LiteralExpr); ok {
		return lit.Kind == ast.LiteralEmpty || lit.Kind == ast.LiteralNull
	}
	return false
}

// isEmptyMessage checks if a message expression is empty or nil.
func isEmptyMessage(expr ast.Expression) bool {
	if expr == nil {
		return true
	}
	if lit, ok := expr.(*ast.LiteralExpr); ok {
		if lit.Kind == ast.LiteralString {
			if s, ok := lit.Value.(string); ok && s == "" {
				return true
			}
		}
		if lit.Kind == ast.LiteralEmpty || lit.Kind == ast.LiteralNull {
			return true
		}
	}
	return false
}
