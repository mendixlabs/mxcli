// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/exprcheck/adapters"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateMicroflow checks a microflow for common issues that don't require a project connection.
// Returns a list of structured violations with rule IDs.
func ValidateMicroflow(stmt *ast.CreateMicroflowStmt) []linter.Violation {
	return validateMicroflowWith(stmt, nil)
}

// validateMicroflowWith is ValidateMicroflow with a resolver for the return
// types of the Java/JavaScript actions the body calls, so a call to a void
// action is not counted as declaring its output name (MDL063, #953). A nil
// resolver knows no action, and every named call output counts.
func validateMicroflowWith(stmt *ast.CreateMicroflowStmt, voids *voidCodeActions) []linter.Violation {
	v := &microflowValidator{
		mfName:     stmt.Name.String(),
		docType:    "microflow",
		returnType: stmt.ReturnType,
		varKinds:   map[string]exprcheck.TypeKind{},
		voids:      voids,
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
	v.params = stmt.Parameters
	v.excluded = stmt.Excluded
	v.validate(stmt.Body)
	return v.violations
}

// checkQualifiedEntityRef reports an entity reference written without a module
// (MDL008), the rule that already covered microflow parameters.
//
// MDL has no implicit module context, and an unqualified name is not merely
// unresolvable — it is written as NOTHING. A `create` with a bare entity stores
// no Entity key at all, so its member assignments cannot be qualified and reach
// disk as bare words, and the project stops loading:
//
//	ERROR: … The text 'Name' is not a valid AttributeIdentifier.
//
// A `retrieve` stores the equally invalid ".Thing". The reference check never
// saw either, because it skipped any reference whose Module was empty — which is
// what made this silent (upstream #973).
func (v *microflowValidator) checkQualifiedEntityRef(site string, entity ast.QualifiedName) {
	if entity.Name == "" || entity.Module != "" {
		return
	}
	v.addViolation("MDL008", linter.SeverityError,
		fmt.Sprintf("%s: entity '%s' is missing its module prefix — MDL has no implicit module "+
			"context, and an unqualified name is written as no reference at all, which Mendix "+
			"cannot load", site, entity.Name),
		fmt.Sprintf("Use a qualified name like 'Module.%s' or 'System.%s'.", entity.Name, entity.Name))
}

// microflowValidator holds state for validating a single microflow.
type microflowValidator struct {
	mfName string
	// docType is the Location.DocumentType violations carry: "microflow", or
	// "nanoflow" when ValidateNanoflow reuses the expression checks.
	docType       string
	returnType    *ast.MicroflowReturnType // nil = void
	violations    []linter.Violation
	loopDepth     int             // Track nesting depth inside loops
	loopVars      []string        // iterators of the enclosing loops, outermost first
	emptyListVars map[string]bool // List variables declared empty and never populated
	// varKinds maps in-scope variable names (params + declared) to their kind,
	// used to detect assigning a Decimal expression to an Integer/Long target.
	varKinds map[string]exprcheck.TypeKind
	// params is the microflow's parameter list. A parameter occupies the same
	// flat variable namespace as every activity output (MDL063).
	params []ast.MicroflowParam
	// excluded marks an @excluded document. mxbuild does not check one, so the
	// #893 rules stand down for it — see skipCEGapRules.
	excluded bool
	// fragment marks the body of an `alter microflow|nanoflow` operation: a
	// piece of a stored flow, whose notes `@annotation(existing: '…')` can
	// attach to (ako/mxcli#1695). Anything else states a whole flow.
	fragment bool
	// voids resolves which Java/JavaScript action calls target a void action;
	// such a call declares no variable (MDL063, #953). May be nil.
	voids *voidCodeActions
}

func (v *microflowValidator) addViolation(ruleID string, severity linter.Severity, message, suggestion string) {
	v.violations = append(v.violations, linter.Violation{
		RuleID:   ruleID,
		Severity: severity,
		Message:  message,
		Location: linter.Location{
			DocumentType: v.docType,
			DocumentName: v.mfName,
		},
		Suggestion: suggestion,
	})
}

// validate runs all checks on the microflow body.
func (v *microflowValidator) validate(body []ast.MicroflowStatement) {
	v.checkListOperationIterator(body)
	v.checkRetrieveLimitOneAsList(body)
	v.checkRetrieveListAsObject(body)
	v.checkShowMessageType(body)
	// MDL-TEMPLATE01: a message written as an expression with `with (…)`
	// (ako/mxcli#1683). Same function exec calls.
	v.checkMessageTemplates(body)
	// MDL-TEMPLATE02: a template text whose braces mxbuild refuses
	// (ako/mxcli#1397). Same function exec calls.
	v.checkTemplateGrammar(body)
	v.checkRetrieveOverSystemAssociation(body)
	v.checkSetOnObjectVariable(v.params, body)
	v.checkSystemMemberChange(body)
	v.checkListOperationSource(body)
	v.checkMergeJoinLabels(body)
	v.checkAnnotationLabels(body)
	v.checkAssociationPathValues(body)

	// Walk the body for per-statement checks (validation feedback, return value checks)
	v.emptyListVars = make(map[string]bool)
	v.walkBody(body)

	// Check 5: missing RETURN on non-void microflow paths.
	//
	// `RETURNS T AS $Var` is exempt: buildFlowGraph sets the final EndEvent's
	// ReturnValue to "$"+Var whenever the AS clause is present, so the return is
	// synthesized whether or not the body spells one out — the whole point of the
	// clause. Demanding an explicit RETURN on top of it flagged the documented
	// idiom as broken, and once `exec` began refusing scripts whose checks report
	// an error, that false positive blocked seven shipped examples from running.
	// Verified on mxbuild 11.13.0: such a microflow builds with 0 errors.
	if v.returnType != nil && v.returnType.Type.Kind != ast.TypeVoid && v.returnType.Variable == "" {
		if !bodyReturns(body) {
			v.addViolation("MDL003", linter.SeverityError,
				fmt.Sprintf("microflow returns %s but not all code paths have a return statement",
					returnTypeString(v.returnType)),
				"Add return statements to all code paths")
		}
	}

	// Check 3: variable scope — detect variables declared inside branches but used after
	v.checkBranchScoping(body)

	// Duplicate loop iterator names — a Mendix loop variable is scoped to the whole
	// microflow, so reusing a name across loops is CE0111 at build time.
	v.checkDuplicateLoopVariables(body)

	// The other half of that rule: names are unique flow-wide, but a loop's
	// variables are only VISIBLE inside its body, so using one after the loop
	// is CE0108.
	v.checkLoopScoping(body)

	// #893: three constructs that passed check and exec and were then rejected
	// by the build. See validate_microflow_ce_gaps.go for the measurements.
	v.checkReturnInLoop(body)
	v.checkDuplicateVariableNames(v.params, body)
	// ako/mxcli#962: the other half of #953's void-call finding — the name is
	// inert both ways, so reading it is CE0109. See validate_void_call_output.go.
	v.checkVoidCallOutputUse(v.params, body)

	// mendixlabs/mxcli#1030: an error event is legal only on an error-handling
	// flow. See validate_microflow_raise_error.go.
	v.checkRaiseErrorOutsideHandler(body)

	// #895: the commit default changed to match Studio Pro. One informational
	// note per microflow, not per statement — see validate_commit_events.go.
	v.checkBareCommitEvents(body)

	// upstream mendixlabs/mxcli#1186: `lint` has known commit-in-a-loop as
	// CONV011 all along, but only after the write and only project-wide. See
	// validate_commit_in_loop.go.
	v.checkCommitInLoop(body)

	// mendixlabs/mxcli#1491: a statement after one that ends the path is
	// written anyway and refused by the build. See validate_microflow_unreachable.go.
	v.checkUnreachableStatements(body)
}

// checkDuplicateLoopVariables flags a loop iterator name used by more than one
// loop in the same microflow. A Mendix loop variable is scoped to the WHOLE
// microflow (not to its loop), so two `loop $R in …` — even sequential ones, or a
// nested loop reusing an outer name — build as CE0111 "Duplicate variable name".
// (ledger finding #64). Fix for the user: give each loop a distinct iterator.
func (v *microflowValidator) checkDuplicateLoopVariables(body []ast.MicroflowStatement) {
	seen := map[string]bool{}
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.LoopStmt:
				if name := st.LoopVariable; name != "" {
					if seen[name] {
						v.addViolation("MDL052", linter.SeverityError,
							fmt.Sprintf("loop iterator '$%s' is reused by another loop in this microflow; "+
								"a Mendix loop variable is scoped to the whole microflow, so this builds as "+
								"CE0111 \"Duplicate variable name\"", name),
							fmt.Sprintf("Give each loop a distinct iterator name (e.g. rename one '$%s' to '$%s2')", name, name))
					}
					seen[name] = true
				}
				walk(st.Body)
			case *ast.IfStmt:
				walk(st.ThenBody)
				walk(st.ElseBody)
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			}
		}
	}
	walk(body)
}

// walkBody recursively walks microflow body statements looking for per-statement issues.
func (v *microflowValidator) walkBody(body []ast.MicroflowStatement) {
	for _, s := range body {
		v.checkUnknownAnnotations(s)
		v.checkErrorHandlingContinueSupported(s)
		v.checkStmtExprFunctions(s)
		// An annotation inside `on error begin … end error` is honoured like one
		// outside it, so a malformed one there must be refused too — this walk
		// never entered a handler body (mendixlabs/mxcli#991).
		if eh := stmtErrorHandling(s); eh != nil {
			v.walkAnnotations(eh.Body)
		}
		switch stmt := s.(type) {
		case *ast.LockWorkflowStmt:
			if stmt.WorkflowVariable == "" && stmt.Workflow == "" {
				v.refuseWorkflowAll("lock", "pause", "a Lock")
			}
		case *ast.UnlockWorkflowStmt:
			if stmt.WorkflowVariable == "" && stmt.Workflow == "" {
				v.refuseWorkflowAll("unlock", "unpause", "an Unlock")
			}
		case *ast.NotifyWorkflowStmt:
			// MDL-WF16. A notify reaches one named element of the workflow, and the
			// build refuses one that names none: CE0166 "The 'Target' property is
			// required" on the 11.10 and 11.13 mxbuilds ("'Activity'" on 11.6). The
			// target used to be neither writable nor described, so every notify
			// mxcli wrote failed the build.
			switch {
			case stmt.Target == "":
				v.addViolation("MDL-WF16", linter.SeverityError,
					fmt.Sprintf("notify workflow $%s names no target — Mendix needs the element it notifies; the build fails CE0166", stmt.WorkflowVariable),
					"Add `target Module.Workflow.Name`, naming a notification start, notification activity, notification boundary event or wait for notification.")
			case strings.Count(stmt.Target, ".") < 2:
				v.addViolation("MDL-WF16", linter.SeverityError,
					fmt.Sprintf("notify workflow target %q must name the element inside its workflow", stmt.Target),
					"Write the target as Module.Workflow.Name.")
			}
		case *ast.ValidationFeedbackStmt:
			if isEmptyMessage(stmt.Message) {
				v.addViolation("MDL007", linter.SeverityWarning,
					"validation feedback has empty message template. "+
						"Mendix requires a non-empty feedback message (CE0091).",
					"Add a message template to the validation feedback action")
			}
		case *ast.ReturnStmt:
			v.checkReturn(stmt)
			v.checkQualifiedCallInExpression("return", stmt.Value)
			v.checkDivisionSlash("return", stmt.Value)
			v.checkDateTimeLiterals("return", stmt.Value)
		case *ast.IfStmt:
			v.checkDivisionSlash("if condition", stmt.Condition)
			v.checkDateTimeLiterals("if condition", stmt.Condition)
			v.walkBody(stmt.ThenBody)
			v.walkBody(stmt.ElseBody)
		case *ast.EnumSplitStmt:
			// A Mendix enumeration split is an exclusive split that needs an
			// outgoing flow for every enum value AND for (empty); a default flow
			// is not offered. Verified on mxbuild 11.6.6.
			if len(stmt.ElseBody) > 0 {
				v.addViolation("MDL008", linter.SeverityError,
					fmt.Sprintf("case statement on '$%s' has an else branch; "+
						"Mendix enumeration splits do not support a default case. "+
						"Add an explicit when branch for every enum value instead.",
						stmt.Variable),
					"Add an explicit when branch for every enum value instead of using else")
			}
			// MDL009 used to error here on a branch listing more than one value,
			// claiming Mendix required exactly one per branch. That was wrong —
			// `when Open, Pending then` covering every value builds with 0 errors,
			// and the write-microflows skill documents that form — so the rule
			// rejected valid MDL. It is retired rather than repurposed; MDL056
			// below checks what actually fails the build.
			v.checkEnumSplitEmptyBranch(stmt)
			for _, c := range stmt.Cases {
				v.walkBody(c.Body)
			}
			v.walkBody(stmt.ElseBody)
		case *ast.InheritanceSplitStmt:
			v.checkInheritanceSplitSpelling(stmt)
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
			// A bare `Module.X` declare type parses as TypeEnumeration with EnumRef
			// (the documented entity/enum ambiguity); an explicit `Enumeration(...)`
			// sets ExplicitEnum. Treat the ambiguous form — and a resolved
			// TypeEntity — as an object declare.
			if stmt.Type.Kind == ast.TypeEntity ||
				(stmt.Type.Kind == ast.TypeEnumeration && stmt.Type.EnumRef != nil && !stmt.Type.ExplicitEnum) {
				// Same restriction as lists (MDL040): a `declare` maps to a Create
				// Variable activity, which only holds primitive types. An object type
				// is rejected by Studio Pro/mxbuild with CE0053 ("Selected type is
				// not allowed") — bare or initialized — plus CE0038 ("Value
				// required") and CE7247 on any following `set`. Object variables must
				// come from a microflow parameter, a retrieve, a create object, or a
				// loop iterator; there is no Create Variable form for them. mxcli
				// check previously passed this, so the invalid microflow surfaced
				// only later in mxbuild.
				typeName := stmt.Variable
				switch {
				case stmt.Type.EntityRef != nil:
					typeName = stmt.Type.EntityRef.String()
				case stmt.Type.EnumRef != nil:
					typeName = stmt.Type.EnumRef.String()
				}
				// With a project (or a script that creates it), the bare name may be
				// known to be an ENUMERATION: then this is no object declare, exec
				// writes it as the enumeration variable and it builds (measured
				// 11.15.0, mendixlabs/mxcli#1516). Say so, as a warning naming
				// describe's spelling, instead of refusing it as an object.
				if stmt.Type.Kind == ast.TypeEnumeration &&
					v.voids.declaredTypeKindOf(typeName) == declKindEnumeration {
					v.addViolation("MDL043", linter.SeverityWarning,
						fmt.Sprintf("declare '$%s' names the enumeration %s bare, which reads as an object type "+
							"wherever the project is not at hand — without -p, check refuses it as an object "+
							"declare (CE0053). Write `declare $%s enum %s = empty`, the spelling describe prints "+
							"(Enumeration(%s) is accepted too).",
							stmt.Variable, typeName, stmt.Variable, typeName, typeName),
						fmt.Sprintf("Write `declare $%s enum %s = empty;`", stmt.Variable, typeName))
				} else {
					v.addViolation("MDL043", linter.SeverityError,
						fmt.Sprintf("declare '$%s' creates an object variable of type %s, but Mendix does not allow the "+
							"Create Variable activity to hold an object (CE0053). "+
							"Get the object from a microflow parameter, a retrieve, or a create object instead — do not declare an object variable. "+
							"(If %s is an enumeration, write it as `enum %s`.)",
							stmt.Variable, typeName, typeName, typeName),
						"Accept the object as a parameter, use retrieve, or use create object — do not declare an object variable")
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
			// #893 item 1: a Create Variable activity requires a value (CE0038).
			v.checkDeclareHasValue(stmt)
			v.checkQualifiedCallInExpression(fmt.Sprintf("declare '$%s'", stmt.Variable), stmt.InitialValue)
			v.checkDivisionSlash(fmt.Sprintf("declare '$%s'", stmt.Variable), stmt.InitialValue)
			v.checkDateTimeLiterals(fmt.Sprintf("declare '$%s'", stmt.Variable), stmt.InitialValue)
		case *ast.MfSetStmt:
			// SET on a plain variable target (not $var/Member = …, which is a
			// member change). Flag a Decimal value assigned to an Integer/Long var.
			if !strings.Contains(stmt.Target, "/") {
				if k, ok := v.varKinds[stmt.Target]; ok {
					v.checkNumericAssignment("$"+stmt.Target, k, stmt.Value)
				}
			}
			v.checkQualifiedCallInExpression(fmt.Sprintf("set '%s'", stmt.Target), stmt.Value)
			v.checkDivisionSlash(fmt.Sprintf("set '%s'", stmt.Target), stmt.Value)
			v.checkDateTimeLiterals(fmt.Sprintf("set '%s'", stmt.Target), stmt.Value)
		case *ast.RetrieveStmt:
			// An ASSOCIATION retrieve names an association, not an entity, and its
			// source is unqualified by construction — do not read it as a bare
			// entity reference.
			if stmt.StartVariable == "" {
				v.checkQualifiedEntityRef("retrieve", stmt.Source)
			}
			// RETRIEVE populates a list variable — remove from empty tracking
			delete(v.emptyListVars, stmt.Variable)
			if stmt.Where != nil {
				xp := expressionToXPath(stmt.Where)
				v.checkXPathAssociationEmpty(stmt.Variable, xp)
				v.checkXPathFunctionNames(stmt.Variable, xp)
				v.checkXPathFunctionArgumentOperators(stmt.Variable, stmt.Where)
				v.checkXPathIdConstraint(stmt.Variable, xp)
				v.checkXPathVariableTraversal(stmt.Variable, xp)
			}
		case *ast.SynchronizeStmt:
			v.checkSynchronizeIsNanoflowOnly()
		case *ast.SendEmailStmt:
			v.checkSendEmail(stmt)
		case *ast.ListOperationStmt:
			v.checkRangeHasABound(stmt)
		case *ast.WhileStmt:
			// A while condition is a plain boolean expression — Mendix has no rule
			// split for a loop, so unlike an IF condition a qualified call here is
			// never legal. The body was not walked at all before, so nothing inside
			// a while was checked.
			v.checkQualifiedCallInExpression("while condition", stmt.Condition)
			// A while loop is the same Microflows$LoopedActivity as a for-each loop,
			// with a WhileLoopCondition instead of an iterator, so a @caption on it is
			// dropped exactly the same way.
			v.checkCaptionOnLoop(stmt.Annotations, "a while loop")
			v.walkBody(stmt.Body)
		case *ast.CallMicroflowStmt:
			v.checkAssociationObjectArgs("microflow "+stmt.MicroflowName.String(), stmt.Arguments)
		case *ast.CallNanoflowStmt:
			v.checkAssociationObjectArgs("nanoflow "+stmt.NanoflowName.String(), stmt.Arguments)
		case *ast.RestCallStmt:
			// #922: `returns Module.Entity` must name a FileDocument specialization.
			v.checkRestFileDocumentResult(stmt)
			// ako/mxcli#1444: an expression URL or body with `with (…)`.
			// Same function exec calls.
			v.checkRestCallTemplates(stmt)
		case *ast.CallJavaActionStmt:
			// ako/mxcli#1451: an expression argument with `with (…)`. Same
			// function exec calls.
			v.checkJavaActionTemplateArgs(stmt)
		case *ast.CallWebServiceStmt:
			// MDL-SOAP01: a call stores ONE request body, so arguments and a send
			// mapping are alternatives. Same function exec calls.
			v.checkWebServiceRequestBody(stmt)
		case *ast.LoopStmt:
			v.checkCaptionOnLoop(stmt.Annotations, "a loop")
			// Check: nested loop anti-pattern. This is a heuristic — a nested loop is
			// only wasteful when the inner loop is a key LOOKUP (find one matching
			// item). Intentional aggregation that must visit every element (group ×
			// category × month totals) is O(N*M) by nature and correct as written, so
			// the message flags the lookup case without asserting the loop is wrong.
			//
			// It fires only on the lookup's shape: a condition inside the inner loop
			// that compares the inner iterator with an outer one. Every nested loop
			// used to qualify, so an intentional region x plan iteration drew the
			// hint on each pass of every script.
			if v.loopDepth > 0 && matchesOuterIterator(stmt, v.loopVars) {
				v.addViolation("MDL001", linter.SeverityWarning,
					"nested loop detected (loop inside a loop). If the inner loop is a "+
						"key LOOKUP (finding one matching item), replace it with "+
						"FIND($List, <condition>) for an in-memory match (O(N) vs O(N^2)). "+
						"If it is intentional aggregation that must visit every element "+
						"(e.g. totals over group x category x month), this is correct — ignore this hint.",
					"For a lookup: $Match = FIND($List, key = $item/key). For genuine aggregation over all elements, no change is needed (a plain retrieve ... where cannot filter a list variable).")
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
			v.loopVars = append(v.loopVars, stmt.LoopVariable)
			v.walkBody(stmt.Body)
			v.loopVars = v.loopVars[:len(v.loopVars)-1]
			v.loopDepth--
		case *ast.CreateListStmt:
			v.checkQualifiedEntityRef("create list of", stmt.EntityType)
		case *ast.CreateObjectStmt:
			v.checkQualifiedEntityRef("create", stmt.EntityType)
			for _, ch := range stmt.Changes {
				v.checkQualifiedCallInExpression(fmt.Sprintf("create %s attribute '%s'", stmt.EntityType.String(), ch.Attribute), ch.Value)
			}
		case *ast.ChangeObjectStmt:
			for _, ch := range stmt.Changes {
				v.checkQualifiedCallInExpression(fmt.Sprintf("change '%s' attribute '%s'", stmt.Variable, ch.Attribute), ch.Value)
			}
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
	// Flag a Decimal-typed value assigned to an Integer/Long target: a raw
	// arithmetic Decimal (e.g. `$a div $b`) or a Decimal-returning built-in such
	// as random() / secondsBetween(...). Rounding functions (round/floor/ceil/
	// trunc) are excluded — Mendix accepts their whole-number result. (findings #2)
	if !exprcheck.SourceRejectedForIntegerTarget(src, v.varKinds) {
		return
	}
	target := "Integer"
	if targetKind == exprcheck.KindLong {
		target = "Long"
	}
	v.addViolation("MDL041", linter.SeverityError,
		fmt.Sprintf("assigning a Decimal expression to %s variable '%s' — Mendix rejects this with CE0117. "+
			"Integer division ('div') and functions like random()/secondsBetween() yield a Decimal.", target, targetLabel),
		fmt.Sprintf("Declare '%s' as Decimal, or round the value (e.g. round(%s) or floor(%s)).", targetLabel, src, src))
}

// mendixAggregateFuncs are the list aggregates that Mendix exposes as
// activities, not expression functions. Used inside an expression they fail
// CE0117; the fix is to assign the aggregate to a variable first.
var mendixAggregateFuncs = map[string]bool{
	"count": true, "sum": true, "average": true, "minimum": true, "maximum": true,
}

// checkStmtExprFunctions runs MDL044 over every expression one statement
// carries. It is the single list of expression sites, shared by walkBody and by
// ValidateNanoflow — two lists were how nanoflows came to have none
// (mendixlabs/mxcli#1033).
func (v *microflowValidator) checkStmtExprFunctions(s ast.MicroflowStatement) {
	switch stmt := s.(type) {
	case *ast.ReturnStmt:
		v.checkExprFunctions("return", stmt.Value)
	case *ast.IfStmt:
		v.checkExprFunctions("if condition", stmt.Condition)
	case *ast.DeclareStmt:
		v.checkExprFunctions(fmt.Sprintf("declare '$%s'", stmt.Variable), stmt.InitialValue)
	case *ast.MfSetStmt:
		v.checkExprFunctions(fmt.Sprintf("set '%s'", stmt.Target), stmt.Value)
	case *ast.CreateObjectStmt:
		// Attribute values in a `create` are expressions too — an aggregate
		// (sum/count/…) or an unknown function here fails the build with CE0117,
		// but check previously only inspected return/if/declare/set (FINDINGS #17).
		for _, ch := range stmt.Changes {
			v.checkExprFunctions(fmt.Sprintf("create %s attribute '%s'", stmt.EntityType.String(), ch.Attribute), ch.Value)
		}
	case *ast.ChangeObjectStmt:
		for _, ch := range stmt.Changes {
			v.checkExprFunctions(fmt.Sprintf("change '%s' attribute '%s'", stmt.Variable, ch.Attribute), ch.Value)
		}
	case *ast.LogStmt:
		// The #1033 repro put the unknown call in a log message, which was never
		// walked — in a microflow either.
		v.checkExprFunctions("log node", stmt.Node)
		v.checkExprFunctions("log message", stmt.Message)
		for _, tp := range stmt.Template {
			v.checkExprFunctions(fmt.Sprintf("log template {%d}", tp.Index), tp.Value)
		}
	}
}

// checkExprFunctions flags calls to names that are not Mendix expression
// functions (e.g. a hallucinated randomInt()) — these parse and pass a naive
// check but fail the build with CE0117. label describes where the expression
// appears (e.g. "declare '$r'"). (findings #1)
func (v *microflowValidator) checkExprFunctions(label string, expr ast.Expression) {
	// The one per-expression hook shared by microflows and nanoflows, so the
	// JSON-number check rides along rather than keeping a third list of sites.
	v.checkLocaleNumberInJSON(label, expr)
	src := microflowExprSource(expr)
	if src == "" {
		return
	}
	for _, u := range exprcheck.UnknownFunctionCalls(src) {
		var suggestion string
		if mendixAggregateFuncs[strings.ToLower(u.Name)] {
			// count/sum/average/minimum/maximum are aggregate ACTIVITIES, not
			// expression functions — a did-you-mean against an unrelated math
			// function (e.g. count→round) sends the author the wrong way. Tell them
			// to assign the aggregate to a variable first, then use the variable.
			suggestion = fmt.Sprintf(
				"'%s' is an aggregate activity, not an expression function. Assign it to a variable first: $n = %s($List); then use $n in the expression.",
				u.Name, u.Name)
		} else if u.Token != "" {
			suggestion = fmt.Sprintf("Use the %s token instead of '%s()'.", u.Token, u.Name)
		} else {
			suggestion = "Use a built-in Mendix expression function (see 'mxcli syntax microflow')."
			if u.Suggestion != "" {
				suggestion = fmt.Sprintf("Did you mean '%s()'? ", u.Suggestion) + suggestion
			}
		}
		v.addViolation("MDL044", linter.SeverityError,
			fmt.Sprintf("%s calls '%s()', which is not a Mendix expression function — "+
				"the build fails CE0117 \"Error(s) in expression\"", label, u.Name),
			suggestion)
	}
	// E006/E005: a built-in called with the wrong number of arguments, or with
	// an argument of the wrong kind. Both ran only on the `-p --references`
	// tier, so `formatDecimal($d, 2)` passed a plain check and failed the build
	// with CE0117 (mendixlabs/mxcli#1515). The kinds a script states — literals,
	// parameters, declared variables — need no project. The codes are
	// exprcheck's own, as on the catalog tier; that tier only runs on a script
	// this one passed, so the two never report the same call.
	for _, h := range exprcheck.BuiltinCallHints(src, v.varKinds) {
		v.addViolation(h.Code, linter.SeverityError, label+": "+h.Problem, h.Fix)
	}
	v.checkRegexTrailingBackslash(label, src)
}

// checkDivisionSlash flags `/` used as an arithmetic division operator, which
// Mendix rejects with CE0117 — in a Mendix expression `/` is only the
// member/association separator (`$obj/Attr`); integer/decimal division is `div`.
// `$Dec / 2` parses to a BinaryExpr whose operator is literally `/`; the walk
// finds it wherever it appears (nested in functions, if-then-else, etc.).
// (The `$a / $b` form degrades to a member-access path and is caught by
// `check --references` as an unresolvable attribute, so it is not re-flagged here.)
// (ledger finding #17)
func (v *microflowValidator) checkDivisionSlash(label string, expr ast.Expression) {
	// Two forms of `/`-as-division: a `BinaryExpr` with operator `/` (right operand
	// is a literal or parenthesized expr, e.g. `$Dec / 2`), and the variable/
	// variable form (`$Dec / $Dec2`) which parses as a member-access path — the
	// visitor preserves its raw source precisely because the `$` on the right
	// marks it as division, not navigation. The `$a / $b` form is detected from
	// the preserved source, so it is caught even when EMBEDDED in a larger
	// expression (`$a / $b + 1`, `round($a / $b)`) where the division degrades to
	// a member-path AttributePathExpr nested under a BinaryExpr/FunctionCallExpr.
	if exprHasSlashDivision(expr) || exprHasSlashDollarDivision(expr) {
		v.addViolation("MDL045", linter.SeverityError,
			fmt.Sprintf("%s uses '/' as a division operator, which Mendix rejects "+
				"(CE0117 \"Error(s) in expression\") — '/' navigates associations, it does not divide", label),
			"Use 'div' for division: `$a div $b` (integer/decimal division is always Decimal — wrap in round()/trunc() for an integer result).")
	}
}

// exprHasSlashDollarDivision reports the `$a / $b` division-misuse form: a
// source-preserved expression whose raw source uses `/` (optionally spaced)
// immediately before a `$` variable, OUTSIDE any string literal. A real
// member/association path never writes `/$` — a path segment is a bare or
// qualified name — so this is an unambiguous division misuse. Scanning the
// preserved source (rather than requiring the SourceExpr to wrap an
// AttributePathExpr directly) catches the division even when it is nested in a
// larger expression: `$a / $b + 1`, `round($a / $b)`, `$a / $b * 100`. The
// string-literal-aware scan avoids a false positive on a `/$` inside a quoted
// literal (e.g. `'path/$x'`).
func exprHasSlashDollarDivision(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.SourceExpr:
		if sourceHasSlashDollarDivision(e.Source) {
			return true
		}
		return exprHasSlashDollarDivision(e.Expression)
	case *ast.BinaryExpr:
		return exprHasSlashDollarDivision(e.Left) || exprHasSlashDollarDivision(e.Right)
	case *ast.UnaryExpr:
		return exprHasSlashDollarDivision(e.Operand)
	case *ast.ParenExpr:
		return exprHasSlashDollarDivision(e.Inner)
	case *ast.FunctionCallExpr:
		for _, arg := range e.Arguments {
			if exprHasSlashDollarDivision(arg) {
				return true
			}
		}
	case *ast.IfThenElseExpr:
		return exprHasSlashDollarDivision(e.Condition) ||
			exprHasSlashDollarDivision(e.ThenExpr) ||
			exprHasSlashDollarDivision(e.ElseExpr)
	}
	return false
}

// sourceHasSlashDollarDivision scans a raw expression source for a `/` used as
// division with a variable divisor (`… / $x …`) outside any single-quoted
// string literal. Doubled ” inside a string is a Mendix-escaped quote.
func sourceHasSlashDollarDivision(source string) bool {
	inStr := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\'' {
			if inStr && i+1 < len(source) && source[i+1] == '\'' {
				i++ // skip the escaped quote pair
				continue
			}
			inStr = !inStr
			continue
		}
		if inStr || c != '/' {
			continue
		}
		j := i + 1
		for j < len(source) && (source[j] == ' ' || source[j] == '\t') {
			j++
		}
		if j < len(source) && source[j] == '$' {
			return true
		}
	}
	return false
}

// exprHasSlashDivision reports whether the expression tree contains a BinaryExpr
// whose operator is a literal `/` (an arithmetic-division misuse).
func exprHasSlashDivision(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if strings.TrimSpace(e.Operator) == "/" {
			// A `/` whose RIGHT operand is a bare member name is association/member
			// navigation, not division. The MDL grammar parses `div`/`*`/`/` at one
			// precedence level, so `$a div $obj/Attr` mis-nests as `($a div $obj) / Attr`
			// with `Attr` a bare IdentifierExpr. Mendix has no `/` division operator, so
			// it re-parses the raw `$obj/Attr` as a path and the expression builds clean
			// (verified with mxbuild). Only a numeric/parenthesized/variable divisor is a
			// real division misuse — those are caught here (right operand is not an
			// IdentifierExpr) or by the source `/ $var` scan. (FINDINGS #52)
			if _, isMemberName := e.Right.(*ast.IdentifierExpr); !isMemberName {
				return true
			}
		}
		return exprHasSlashDivision(e.Left) || exprHasSlashDivision(e.Right)
	case *ast.UnaryExpr:
		return exprHasSlashDivision(e.Operand)
	case *ast.ParenExpr:
		return exprHasSlashDivision(e.Inner)
	case *ast.FunctionCallExpr:
		for _, arg := range e.Arguments {
			if exprHasSlashDivision(arg) {
				return true
			}
		}
	case *ast.IfThenElseExpr:
		return exprHasSlashDivision(e.Condition) || exprHasSlashDivision(e.ThenExpr) || exprHasSlashDivision(e.ElseExpr)
	case *ast.SourceExpr:
		return exprHasSlashDivision(e.Expression)
	}
	return false
}

// xpathAssocEmptyRe matches a module-qualified association compared directly to
// `empty` in an XPath constraint (`Ledger.Transaction_Category = empty`, or
// `!= empty`). The leading boundary class excludes a `/` (so an
// attribute-over-association path like `Assoc/Ledger.Category = empty` is NOT
// matched — that is a valid attribute nullability test) and a `.`/word char (so
// it captures the whole qualified name, not the tail of a 3-part enum literal).
var xpathAssocEmptyRe = regexp.MustCompile(`(^|[^\w./])([A-Za-z_]\w*\.[A-Za-z_]\w*)\s*(!=|=)\s*empty\b`)

// xpathAssocEmptyHit is one association compared directly to `empty`.
type xpathAssocEmptyHit struct {
	Assoc string // Module.Association
	Op    string // "=" or "!="
}

// test renders the comparison as written, for the message.
func (h xpathAssocEmptyHit) test() string { return h.Assoc + " " + h.Op + " empty" }

// fix is the spelling Mendix XPath accepts: a path to the associated object
// tests its presence, `not(…)` around it its absence.
func (h xpathAssocEmptyHit) fix() string {
	if h.Op == "!=" {
		return fmt.Sprintf("Test for the presence of the associated object with a path: `[%s/<Module.TargetEntity>]`.", h.Assoc)
	}
	return fmt.Sprintf("Test for the absence of the associated object with negation: `[not(%s/<Module.TargetEntity>)]`.", h.Assoc)
}

// xpathAssociationEmptyMatches returns the module-qualified associations an
// XPath constraint compares directly to `empty`. Shared by the
// microflow-retrieve check (MDL047) and the page/widget datasource check.
// Empty result → nothing to flag.
//
// `!= empty` was missed: only `=` was matched, and measured on mxbuild 11.13.0
// `[Mod.Order_Customer != empty]` is CE0161 as well — in a retrieve and in a
// list view's database source — while `[Mod.Order_Customer/Mod.Customer]`
// builds clean (mendixlabs/mxcli#1213).
func xpathAssociationEmptyMatches(xpath string) []xpathAssocEmptyHit {
	var out []xpathAssocEmptyHit
	for _, m := range xpathAssocEmptyRe.FindAllStringSubmatch(blankXPathLiterals(xpath), -1) {
		out = append(out, xpathAssocEmptyHit{Assoc: m[2], Op: m[3]})
	}
	return out
}

// checkXPathAssociationEmpty flags `[Module.Association = empty]` (and `!=`) in a
// retrieve constraint. Mendix XPath has no `empty` test for an association — it
// fails the build with CE0161; the test is a path to the associated object,
// negated with `not(…)` for absence. A bare attribute (`Name = empty`) is valid
// and is not module-qualified, so it never matches. (ledger finding #25)
func (v *microflowValidator) checkXPathAssociationEmpty(variable, xpath string) {
	for _, h := range xpathAssociationEmptyMatches(xpath) {
		v.addViolation("MDL047", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constraint tests association `%s`, which Mendix XPath does not support "+
				"(CE0161 \"Error(s) in XPath constraint\") — `empty` compares attributes, not associations", variable, h.test()),
			h.fix())
	}
}

// xpathExpressionOnlyFunctions maps the Mendix EXPRESSION functions an XPath
// constraint does not have to the XPath function that does the same thing. The
// two languages sit side by side in a microflow and are easy to mix: measured on
// mxbuild 11.13.0, `startsWith(Title, 'X')` and `endsWith(Title, 'X')` in a
// retrieve constraint are CE0161 "Error(s) in XPath constraint" while
// `starts-with(Title, 'X')` builds clean (mendixlabs/mxcli#1213). The list is
// the measured pairs, not every function the two languages spell differently.
//
// getKey has no XPath counterpart because XPath needs none: measured on mxbuild
// 11.14.0, `[St = getKey($S)]` (St an enumeration attribute, $S an enumeration
// variable) is CE0161 while `[St = $S]` builds clean. Its entry is therefore
// empty, and the fix is in xpathExpressionOnlyFunctionFixes.
var xpathExpressionOnlyFunctions = map[string]string{
	"startsWith": "starts-with",
	"endsWith":   "ends-with",
	"getKey":     "",
}

// xpathExpressionOnlyFunctionFixes is the hint for an expression-only function
// XPath has no counterpart for.
var xpathExpressionOnlyFunctionFixes = map[string]string{
	"getKey": "XPath compares an enumeration attribute with an enumeration value directly — " +
		"drop the getKey(): `[Status = $EnumVariable]`, or a literal `[Status = 'Key']`.",
}

// xpathFunctionCallRe matches a function call and the character before it. A
// `-` or word character before the name is part of another name.
var xpathFunctionCallRe = regexp.MustCompile(`(^|[^\w.-])([A-Za-z_][\w-]*)\s*\(`)

// xpathExpressionFunctionHits returns the expression-only functions an XPath
// constraint calls, outside string literals.
func xpathExpressionFunctionHits(xpath string) []string {
	var out []string
	for _, m := range xpathFunctionCallRe.FindAllStringSubmatch(blankXPathLiterals(xpath), -1) {
		if _, ok := xpathExpressionOnlyFunctions[m[2]]; ok {
			out = append(out, m[2])
		}
	}
	return out
}

// checkXPathFunctionNames flags (MDL091) an expression-language function in a
// retrieve constraint.
func (v *microflowValidator) checkXPathFunctionNames(variable, xpath string) {
	for _, fn := range xpathExpressionFunctionHits(xpath) {
		fix := xpathExpressionOnlyFunctionFixes[fn]
		if xp := xpathExpressionOnlyFunctions[fn]; xp != "" {
			fix = fmt.Sprintf("Use the XPath function `%s()` in a retrieve constraint.", xp)
		}
		v.addViolation("MDL091", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constraint calls `%s()`, which is a Mendix expression function — XPath "+
				"does not have it, and mxbuild reports CE0161 \"Error(s) in XPath constraint\"", variable, fn),
			fix)
	}
}

// xpathArithmeticOperators are the operators an XPath function argument cannot
// carry. Measured on mxbuild 11.14.0 (mendixlabs/mxcli#1326):
//
//	starts-with(Name, 'MS-' + $Key)          CE0161 "Error(s) in XPath constraint"
//	not(contains(Name, $Key + $Key))         CE0161
//	contains(Name, $Key - 'a')               CE0161
//	Name = 'X-' + $Key                       clean — the same operator at top level
//	year-from-dateTime(Due) = $N - 1         clean — an operator beside a call, not in it
//	starts-with(Name, $P)                    clean — the value computed beforehand
//
// `*`, `div` and `mod` are left out on purpose: the only argument they could
// appear in is numeric, and `contains(Name, $N)` with an Integer $N is already
// CE0161 with no operator at all, so no measurement isolates the operator. This
// rule blocks exec, so it covers only what was shown to fail.
var xpathArithmeticOperators = map[string]bool{"+": true, "-": true}

// xpathFunctionArgumentOperators returns, for each XPath function call in a
// retrieve constraint whose argument is itself a `+` or `-` operation,
// the function name and the operator. Only an argument that IS the operation
// counts: `not(Price > $A + 1)` has a comparison for an argument and is not
// flagged — that shape was not measured.
func xpathFunctionArgumentOperators(expr ast.Expression) (hits [][2]string) {
	var walk func(ast.Expression)
	walk = func(e ast.Expression) {
		switch n := e.(type) {
		case *ast.FunctionCallExpr:
			for _, arg := range n.Arguments {
				inner := unwrapXPathArgument(arg)
				if b, ok := inner.(*ast.BinaryExpr); ok && xpathArithmeticOperators[strings.ToLower(b.Operator)] {
					hits = append(hits, [2]string{mendixFunctionName(n.Name), b.Operator})
				}
				walk(arg)
			}
		case *ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case *ast.UnaryExpr:
			walk(n.Operand)
		case *ast.ParenExpr:
			walk(n.Inner)
		case *ast.SourceExpr:
			walk(n.Expression)
		case *ast.XPathPathExpr:
			for _, s := range n.Steps {
				walk(s.Expr)
				for _, pred := range s.Predicates {
					walk(pred)
				}
			}
		}
	}
	walk(expr)
	return hits
}

// unwrapXPathArgument strips the parentheses and source wrappers around an
// argument, down to the expression that is its value.
func unwrapXPathArgument(e ast.Expression) ast.Expression {
	for {
		switch n := e.(type) {
		case *ast.ParenExpr:
			e = n.Inner
		case *ast.SourceExpr:
			e = n.Expression
		default:
			return e
		}
	}
}

// checkXPathFunctionArgumentOperators flags (MDL091) an operator inside an XPath
// function argument — the other way an expression construct gets into a
// retrieve constraint and out as CE0161 (mendixlabs/mxcli#1326).
func (v *microflowValidator) checkXPathFunctionArgumentOperators(variable string, where ast.Expression) {
	for _, h := range xpathFunctionArgumentOperators(where) {
		v.addViolation("MDL091", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constraint computes `%s` inside an argument of `%s()` — XPath does not "+
				"evaluate an operator in a function argument, and mxbuild reports CE0161 \"Error(s) in XPath constraint\"",
				variable, h[1], h[0]),
			fmt.Sprintf("Compute the value into a variable first, then pass the variable: "+
				"`declare $P String = <the argument>;` and `%s(<attribute>, $P)`. An operator at the top "+
				"level of the constraint (`Name = 'X-' + $Key`) is fine.", h[0]))
	}
}

// xpathVarTraversalRe matches a path rooted at a $variable with TWO OR MORE
// segments (`$P/Mod.Assoc/Name`). One segment is deliberately not matched: both
// `$P/Code` (the parameter's own attribute) and `$P/Mod.Assoc` (one hop to the
// associated object) are valid XPath. The boundary is the hop count, not whether
// a segment is module-qualified — see checkXPathVariableTraversal.
var xpathVarTraversalRe = regexp.MustCompile(`\$(\w+)((?:/[A-Za-z_][\w.]*){2,})`)

// checkXPathVariableTraversal flags a retrieve constraint that traverses an
// association FROM a variable (`[Name = $RefProduct/Mod.Product_Category/Name]`).
// Mendix XPath reaches at most one hop off a variable, so this fails the build
// with CE0161 while mxcli accepted it silently (issue #831).
//
// Verified against mxbuild 11.6.6 — the boundary is narrower than it looks:
//
//	$Var/Attr             VALID   a parameter's own attribute
//	$Var/Mod.Assoc        VALID   one hop, the associated object
//	$Var/Mod.Assoc/Attr   CE0161  two or more hops
//
// There is no valid serialization of the two-hop form, which is why this is a
// rejection rather than a writer fix: the constraint has to be restructured, and
// only the author knows which of the two shapes they meant.
func (v *microflowValidator) checkXPathVariableTraversal(variable, xpath string) {
	for _, t := range xpathVariableTraversals(xpath) {
		v.addViolation("MDL055", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constraint traverses an association from a variable (`%s`), which Mendix XPath "+
				"does not support (CE0161 \"Error(s) in XPath constraint\") — a constraint reaches at most one hop off a variable",
				variable, t.path),
			fmt.Sprintf("Retrieve the associated object first, then constrain on its own attribute: "+
				"`retrieve $Related from $%s/%s;` and use `[%s = $Related/%s]`. Or invert the constraint so the "+
				"traversal starts at the entity being retrieved: `[%s/<Module.Entity> = $%s]`. Both forms build clean.",
				t.root, t.firstHop, t.leaf, t.leaf, t.firstHop, t.root))
	}
}

// xpathVariableTraversal is one path of two or more hops off a $variable.
type xpathVariableTraversal struct {
	path     string // `$P/Mod.Assoc/Name`, as written
	root     string // `P`
	firstHop string // `Mod.Assoc`
	leaf     string // `Name`
}

// xpathVariableTraversals returns every path in an XPath constraint that reaches
// two or more hops off a $variable — the shape MDL055 refuses, in a microflow
// retrieve and in a widget's database datasource alike.
func xpathVariableTraversals(xpath string) []xpathVariableTraversal {
	var out []xpathVariableTraversal
	for _, m := range xpathVarTraversalRe.FindAllStringSubmatch(xpath, -1) {
		segs := strings.Split(strings.TrimPrefix(m[2], "/"), "/")
		out = append(out, xpathVariableTraversal{
			path: "$" + m[1] + m[2], root: m[1], firstHop: segs[0], leaf: segs[len(segs)-1],
		})
	}
	return out
}

// checkSynchronizeIsNanoflowOnly rejects SYNCHRONIZE in a microflow.
//
// Offline synchronization is a client-side operation, so Mendix allows the
// activity only in nanoflows. This validator runs over microflow bodies, so
// reaching it at all is the error.
//
// Verified against mxbuild 11.13.0: the same nanoflow builds with 0 errors,
// while a microflow containing it fails CE0009 "This action is not supported in
// microflows."
func (v *microflowValidator) checkSynchronizeIsNanoflowOnly() {
	v.addViolation("MDL057", linter.SeverityError,
		"`synchronize` is not supported in a microflow — offline synchronization runs on the client "+
			"(CE0009 \"This action is not supported in microflows.\")",
		"Move the statement into a nanoflow: `create nanoflow Module.NF_Sync () begin synchronize all; end;`. "+
			"A microflow can call that nanoflow only from client-side logic, so the caller must be a nanoflow too.")
}

// checkRangeHasABound rejects `range($List)` — a Range list operation with
// neither an offset nor an amount.
//
// Mendix requires at least one, and says so at build time:
//
//	[error] [CE6520] "Amount and offset are not specified. Either amount or
//	offset or both must be specified." at List operation activity 'Range'
//
// (mxbuild 11.13.0; the same project with a bound is 0 errors.) The grammar
// makes both arguments optional, so the unbuildable form parsed and checked
// clean — and that is precisely the shape DESCRIBE emitted for every paged
// range while the reader was dropping the bounds, which is how issue #966's
// truncated output passed `mxcli check`. The reader is fixed, so describe no
// longer produces it; this closes the hand-written path too, and keeps a check
// that would have caught #966 from the other side.
func (v *microflowValidator) checkRangeHasABound(stmt *ast.ListOperationStmt) {
	if stmt.Operation != ast.ListOpRange || stmt.OffsetExpr != nil || stmt.LimitExpr != nil {
		return
	}
	v.addViolation("MDL068", linter.SeverityError,
		fmt.Sprintf("`range($%s)` has neither an offset nor an amount, and Mendix requires at least one "+
			"(CE6520 \"Amount and offset are not specified. Either amount or offset or both must be specified.\")",
			stmt.InputVariable),
		fmt.Sprintf("Give it a bound: `range($%s, $Offset, $Amount)` for a page, `range($%s, 0, $Amount)` for the "+
			"first N, or `range($%s, $Offset)` to skip. To use the whole list unchanged, drop the range activity "+
			"and use `$%s` directly.", stmt.InputVariable, stmt.InputVariable, stmt.InputVariable, stmt.InputVariable))
}

// xpathIdConstraintRe matches a constraint comparing the object id against a VALUE
// (`id = $strVar`, `id = '123'`, `id != 5`). It captures the right-hand operand.
// Comparing `id` against an OBJECT variable (`[id != $ExistingOrder]` — the valid
// "exclude self" pattern) is intentionally NOT matched here: the operand filter in
// checkXPathIdConstraint requires a primitive value, and an object variable is not
// one. `id` is a Mendix reserved member, so a bare `id` in identifier position is
// always the system id.
var xpathIdConstraintRe = regexp.MustCompile(`(?:^|[^\w./])(?i:id)\s*(?:=|!=|<|>)\s*(\$\w+|'[^']*'|-?\d+)`)

// checkXPathIdConstraint flags a retrieve that constrains the object id against a
// stored id VALUE (`where [id = $Id]` with `$Id` a String/Long, or a literal).
// Mendix XPath has no id operator reachable from a microflow expression, so this
// fails the build with CE0161. Comparing `id` to an OBJECT variable is a valid
// identity test and is left alone (its operand is not a primitive). (ledger #42)
func (v *microflowValidator) checkXPathIdConstraint(variable, xpath string) {
	for _, m := range xpathIdConstraintRe.FindAllStringSubmatch(xpath, -1) {
		operand := m[1]
		// `[id = '[%CurrentUser%]']` (and other `'[%…%]'` server tokens) is the
		// standard, build-clean Mendix idiom for the signed-in user's id — Mendix's
		// XPath engine resolves the token to a GUID, so it IS a valid id operand.
		// Only a STORED id value (String/Long variable or a plain literal) is the
		// unsupported case this rule targets. (FINDINGS #53)
		if strings.HasPrefix(operand, "'[%") && strings.HasSuffix(operand, "%]'") {
			continue
		}
		if strings.HasPrefix(operand, "$") {
			// A $-variable operand: flag only when it is a primitive VALUE (a stored
			// id — String/Long/Integer). An object variable is not in varKinds, so an
			// identity comparison (`id != $obj`) is correctly not flagged.
			if _, isPrimitive := v.varKinds[operand[1:]]; !isPrimitive {
				continue
			}
		}
		v.addViolation("MDL048", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constrains the object id against a value (`[id = %s]`), which Mendix XPath does not support "+
				"(CE0161 \"Error(s) in XPath constraint\") — there is no id operator reachable from a microflow expression", variable, operand),
			"Retrieve by GUID with a marketplace action (NanoflowCommons GetObjectByGuid / CommunityCommons), "+
				"or expose the id as a String on a view entity (`cast(id as string) as ObjectId`) and constrain on that String column. "+
				"(Comparing id against an OBJECT variable — `[id != $obj]` — IS valid and is not flagged.)")
		return
	}
}

// checkAssociationObjectArgs flags a call argument bound to a bare association
// path (`$obj/Module.Assoc`). Mendix rejects that as a value with CE0117 — the
// object has to be named. An attribute value over an association
// (`$obj/Module.Assoc/Attr`) is a legal value and is NOT flagged. (ledger #43/#44)
//
// Naming the object is what Studio Pro itself does: it writes the target entity
// after the association, `$obj/Module.Assoc/Module.Target`, and that is an
// object value mxbuild accepts. Measured on mxbuild 10.24.15 and 11.13.0, one
// microflow per argument into a fresh project:
//
//	$A/M.A_B                    CE0117 "Error(s) in expression."
//	$A/M.A_B/M.B                0 errors
//	$A/M.A_Bs/M.B  (ref set)    0 errors into a List parameter
//	$A/M.A_B/M.B/M.B_C/M.C      0 errors
//	$A/M.A_B/M.B/M.B_C          0 errors
//
// The rule used to fire on any path ending in a qualified segment, so all of
// those were refused — including 42 untouched microflows in Evora Factory
// Management, whose describe output exec then refused to re-apply. Only the
// one-segment shape is reported now. Paths that fail for a TYPE reason (a
// reverse traversal, a reference set into an object parameter) are CE0117 too,
// but telling them apart needs the domain model, which this check does not see.
func (v *microflowValidator) checkAssociationObjectArgs(callee string, args []ast.CallArgument) {
	for _, a := range args {
		if exprIsAssociationObjectPath(a.Value) {
			v.addViolation("MDL049", linter.SeverityError,
				fmt.Sprintf("call %s: argument '%s' passes a bare association path, which Mendix rejects "+
					"as a value (CE0117 \"Error(s) in expression\")", callee, a.Name),
				fmt.Sprintf("Name the associated object's entity after the association — `%s/<Module.TargetEntity>`, "+
					"the form Studio Pro writes — or retrieve it first: `retrieve $%s from %s;` then `%s = $%s`.",
					microflowExprSource(a.Value), a.Name, microflowExprSource(a.Value), a.Name, a.Name))
		}
	}
}

// exprIsAssociationObjectPath reports whether an expression is a bare
// association path: `$obj/Module.Assoc`, one module-qualified segment and
// nothing after it. `$obj/Module.Assoc/Module.Target` (the entity step that
// names the object) and `$obj/Module.Assoc/Attr` (an attribute value) are not.
func exprIsAssociationObjectPath(expr ast.Expression) bool {
	if se, ok := expr.(*ast.SourceExpr); ok {
		expr = se.Expression
	}
	ap, ok := expr.(*ast.AttributePathExpr)
	if !ok || len(ap.Path) != 1 {
		return false
	}
	return strings.Contains(ap.Path[0], ".")
}

// dateTimeLiteralFuncs are the date-construction functions whose arguments
// Mendix requires to be literal numeric constants — a variable or computed
// argument fails the build with CE0117.
var dateTimeLiteralFuncs = map[string]bool{"datetime": true, "datetimeutc": true}

// checkDateTimeLiterals flags a dateTime()/dateTimeUTC() call with a non-literal
// argument. Mendix builds these from hardcoded numeric constants only; a
// variable or expression (`dateTime(2026, $Month, $Day)`) is CE0117. (ledger #21)
func (v *microflowValidator) checkDateTimeLiterals(label string, expr ast.Expression) {
	if exprHasNonLiteralDateTime(expr) {
		v.addViolation("MDL046", linter.SeverityError,
			fmt.Sprintf("%s calls dateTime()/dateTimeUTC() with a non-literal argument, which Mendix rejects "+
				"(CE0117 \"Error(s) in expression\") — these functions accept only hardcoded numeric constants", label),
			"Step off a literal anchor date instead: `addDays(addMonths(dateTime(2026,1,1), $Month - 1), $Day - 1)` (addDays/addMonths take variables).")
	}
}

// exprHasNonLiteralDateTime reports whether the tree contains a dateTime/
// dateTimeUTC call any of whose arguments is not a plain numeric literal.
func exprHasNonLiteralDateTime(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.FunctionCallExpr:
		if dateTimeLiteralFuncs[strings.ToLower(e.Name)] {
			for _, arg := range e.Arguments {
				if _, ok := arg.(*ast.LiteralExpr); !ok {
					return true
				}
			}
		}
		for _, arg := range e.Arguments {
			if exprHasNonLiteralDateTime(arg) {
				return true
			}
		}
	case *ast.BinaryExpr:
		return exprHasNonLiteralDateTime(e.Left) || exprHasNonLiteralDateTime(e.Right)
	case *ast.UnaryExpr:
		return exprHasNonLiteralDateTime(e.Operand)
	case *ast.ParenExpr:
		return exprHasNonLiteralDateTime(e.Inner)
	case *ast.IfThenElseExpr:
		return exprHasNonLiteralDateTime(e.Condition) || exprHasNonLiteralDateTime(e.ThenExpr) || exprHasNonLiteralDateTime(e.ElseExpr)
	case *ast.SourceExpr:
		return exprHasNonLiteralDateTime(e.Expression)
	}
	return false
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
//
// The table lives in mdl/exprcheck/adapters because the catalog-backed checker
// needs the same answer; a second copy here is how the two would drift apart.
func astKindToExprKind(k ast.DataTypeKind) (exprcheck.TypeKind, bool) {
	return adapters.DataTypeKind(k)
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
		v.addViolation("MDL004", linter.SeverityError, voidReturnValueMessage,
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
		case *ast.MergeStmt:
			// See the break below: past a merge, branch-declared variables
			// are the joins' business, not this linear check's.
			branchVars = make(map[string]string)
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
				// Past a `merge`, what is in scope comes from the paths that
				// join it, not from the text above it — a describe of an
				// irreducible graph puts each shared region in a section after
				// the branches that declare its variables. This linear check
				// cannot see joins, so it stops rather than guess.
				if _, isMerge := subsequent.(*ast.MergeStmt); isMerge {
					break
				}
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
	case *ast.ClearListStmt:
		refs = append(refs, s.List)
	case *ast.LogStmt:
		refs = append(refs, exprVarRefs(s.Node)...)
		refs = append(refs, exprVarRefs(s.Message)...)
	case *ast.EnumSplitStmt:
		if s.Condition != nil {
			refs = append(refs, exprVarRefs(s.Condition)...)
		} else {
			refs = append(refs, extractVarName(s.Variable))
		}
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
//
// The table lives in mdl/exprcheck/adapters so the expression checker's walk and
// this one cannot disagree about which statements carry a handler: a statement
// missing from one copy is silently skipped by whichever walk holds it.
func stmtErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	return adapters.StatementErrorHandling(stmt)
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

// checkEnumSplitEmptyBranch (MDL056) flags an enumeration split with no
// `(empty)` branch. A Mendix enum split needs an outgoing flow for every value
// AND for the unset case; without one the build fails with
//
//	CE0079 "The '(empty)' condition value should be configured in properties
//	        for an outgoing flow."
//
// Verified on mxbuild 11.6.6, and the requirement is universal — it holds even
// when the split is on a `not null` enum attribute, so no nullability analysis
// is needed and the check works from the statement alone.
//
// This replaces the retired MDL009, which asserted the opposite of what Mendix
// does (see the EnumSplitStmt arm). A new ID was used rather than repurposing
// MDL009 so that anything referring to the old number still refers to the old,
// wrong meaning.
//
// Value coverage — every enum member having a branch, the other half of CE0079 —
// is deliberately NOT checked here: it needs the enumeration's member list,
// which means resolving the split variable's type against the script or the
// project. ValidateMicroflow sees only one statement. Worth adding where that
// context exists; guessing it here would trade one false positive for another.
// enumSplitSubject names what a case statement decides on, for messages:
// `$Var` / `$Obj/Attr`, or the rule it calls (mendixlabs/mxcli#1414).
func enumSplitSubject(stmt *ast.EnumSplitStmt) string {
	if call := unwrapParenCall(stmt.Condition); call != nil {
		return call.Name + "(…)"
	}
	return "$" + stmt.Variable
}

func (v *microflowValidator) checkEnumSplitEmptyBranch(stmt *ast.EnumSplitStmt) {
	for _, c := range stmt.Cases {
		for _, val := range c.Values {
			if strings.EqualFold(strings.TrimSpace(val), "(empty)") {
				return
			}
		}
	}
	v.addViolation("MDL056", linter.SeverityError,
		fmt.Sprintf("case statement on '%s' has no `(empty)` branch; a Mendix enumeration split needs an "+
			"outgoing flow for the unset value too, so this builds as CE0079 \"The '(empty)' condition value "+
			"should be configured in properties for an outgoing flow\"", enumSplitSubject(stmt)),
		"Add a `when (empty) then …` branch. It is required even when the attribute is `not null`. "+
			"A branch may list several values (`when Open, (empty) then …`) if they share a path.")
}

// checkInheritanceSplitSpelling warns on the pre-#913 spelling of a type split.
//
// `case X <body>` used `case` to introduce a BRANCH, while the enumeration
// split (`case $x when V then`) and the caseExpression in MDLSettings.g4 use it
// to introduce the SUBJECT. Two of the three agreed; the type split was the
// outlier, so the word meant two things depending on the statement.
//
// `else` is the worse half. It is not a default branch: it is Mendix's
// `(empty)` outgoing flow, taken when the object is NULL. Measured on mxbuild
// 11.13.0, a split with one `case` and an `else` still fails CE0090 demanding a
// flow for every other subtype AND for the base entity — so the `else`
// contributes nothing to type coverage, which is exactly what its name promises.
//
// A warning, not an error: both spellings build the identical flow, and scripts
// in the wild use the old one. Nothing downstream of this function may branch on
// the spelling flags.
func (v *microflowValidator) checkInheritanceSplitSpelling(stmt *ast.InheritanceSplitStmt) {
	if stmt.LegacyCaseKeyword {
		v.addViolation("MDL065", linter.SeverityWarning,
			fmt.Sprintf("split type '$%s' uses the legacy `case <Entity>` branch spelling; "+
				"`case` introduces the subject in every other MDL statement (`case $x when V then`), "+
				"not a branch", stmt.Variable),
			"Write `when Module.Entity then …` instead. Both build the identical flow.")
	}
	if stmt.LegacyElseKeyword {
		v.addViolation("MDL065", linter.SeverityWarning,
			fmt.Sprintf("split type '$%s' uses `else`, which reads as a default branch and is not one: "+
				"it is Mendix's `(empty)` flow, taken when the object is null. Mendix still requires an "+
				"outgoing flow for every subtype and for the base entity (CE0090)", stmt.Variable),
			"Write `when (empty) then …` instead — same flow, accurate name. "+
				"To handle unmatched types, add a `when <BaseEntity> then …` branch.")
	}
}

// knownActivityAnnotations is the set the visitor implements. It is the visitor's
// own switch arms, restated: the two are pinned together by
// TestKnownAnnotationsMatchTheVisitor, because a name added to one and not the
// other either rejects a valid annotation or silently drops an invalid one.
var knownActivityAnnotations = map[string]bool{
	"position":   true,
	"caption":    true,
	"color":      true,
	"annotation": true,
	"excluded":   true,
	"anchor":     true,
	"curve":      true,
	"merge":      true,
	"start":      true,
}

// invalidAnchorRule refuses an @anchor parameter the visitor could not use.
const invalidAnchorRule = "MDL092"

// checkCaseAnchors refuses an @anchor outcome key that names no outcome: on a
// CASE, a key that is none of its WHEN values, or `true:`/`false:`, which only
// an IF has; anywhere else, any value key. Each parsed and left its edge on the
// default sides, which is what made `@anchor(from: right)` on a CASE look
// accepted while it did nothing (mendixlabs/mxcli#1587).
func (v *microflowValidator) checkCaseAnchors(s ast.MicroflowStatement, ann *ast.ActivityAnnotations) {
	const hint = "A CASE names each outcome's edge by its WHEN value: " +
		"`@anchor(A: (from: right), (empty): (from: bottom, to: top))`. An IF's branches are " +
		"`true: (…)` and `false: (…)`."
	cs, isCase := s.(*ast.EnumSplitStmt)
	if !isCase {
		for _, ca := range ann.CaseAnchors {
			v.addViolation(invalidAnchorRule, linter.SeverityError,
				fmt.Sprintf("`@anchor` parameter `%s: (…)` names a CASE outcome, but it is not on a CASE, "+
					"so the edge it names keeps its default sides", ca.Value), hint)
		}
		return
	}
	for _, key := range []struct {
		name string
		set  bool
	}{{"true", ann.TrueBranchAnchor != nil}, {"false", ann.FalseBranchAnchor != nil}} {
		if key.set {
			v.addViolation(invalidAnchorRule, linter.SeverityError,
				fmt.Sprintf("`@anchor` parameter `%s: (…)` names an IF branch; a CASE has none, "+
					"so the edge it names keeps its default sides", key.name), hint)
		}
	}
	for _, ca := range ann.CaseAnchors {
		known := false
		for _, c := range cs.Cases {
			for _, value := range enumSplitCaseValues(c) {
				if sameCaseValue(value, ca.Value) {
					known = true
				}
			}
		}
		if !known {
			v.addViolation(invalidAnchorRule, linter.SeverityError,
				fmt.Sprintf("`@anchor` parameter `%s: (…)` names no WHEN value of this CASE, "+
					"so it does nothing", ca.Value), hint)
		}
	}
}

// checkUnknownAnnotations rejects an @annotation name the visitor does not
// implement.
//
// The grammar accepts any @name, and the visitor's switch had no default, so an
// unrecognised one was dropped in silence. That is benign for an annotation mxcli
// does not implement — @size(600, 300), which the #884 reporter found parsing
// without effect — and NOT benign for a typo of one it does: `@postion(10, 20)`
// passed `check` and discarded the layout the author asked for, on a workflow
// whose entire point is scripted canvas layout.
//
// An error rather than a warning: the statement's meaning silently differs from
// what was written, and warnings on a generated script are not read. (#884)
func (v *microflowValidator) checkUnknownAnnotations(s ast.MicroflowStatement) {
	ann := ast.StatementAnnotations(s)
	if ann == nil {
		return
	}
	for _, bad := range ann.InvalidCurves {
		v.addViolation("MDL060", linter.SeverityError,
			fmt.Sprintf("`@curve` parameter `%s` is not a whole-number (x, y) pair", bad),
			"A sequence flow's shape is two bezier control vectors, each a pixel offset from its end "+
				"of the line: `@curve(from: (40, -90), to: (-40, 90))`. Only `from:` and `to:` are accepted.")
	}
	for _, bad := range ann.InvalidAnchors {
		v.addViolation(invalidAnchorRule, linter.SeverityError,
			fmt.Sprintf("`@anchor` parameter `%s` is not one mxcli understands, so the edge it "+
				"names keeps its default sides", bad),
			"A side is top, right, bottom or left. The flow leaving a statement is "+
				"`@anchor(from: right, to: left)`; an IF's branches are `@anchor(true: (from: …, to: …), "+
				"false: (…))`, a CASE's outcomes `@anchor(Open: (from: right), (empty): (…))`, a loop's "+
				"`@anchor(iterator: (…), tail: (…))` — either side of a pair may be left out (mendixlabs/mxcli#992).")
	}
	v.checkCaseAnchors(s, ann)
	v.checkExcludedOnNonActivity(s, ann)
	for _, name := range ann.UnknownNames {
		v.addViolation("MDL059", linter.SeverityError,
			fmt.Sprintf("unknown annotation `@%s` — it parses but does nothing, so whatever it was "+
				"meant to express is silently lost", name),
			fmt.Sprintf("mxcli implements @position(x, y), @start(x, y), @caption, @color, @annotation, "+
				"@excluded, @anchor, @curve and @merge on a microflow statement. If `@%s` is a typo of "+
				"one of those, correct it; container size is not authorable (upstream #884).", name))
	}
	for _, bad := range ann.InvalidNotes {
		v.addViolation("MDL079", linter.SeverityError,
			fmt.Sprintf("`@annotation` parameter `%s` is not one mxcli understands, so the note it "+
				"belongs to is not written at all", bad),
			"A note is either `@annotation 'text'`, or the long form "+
				"`@annotation(id: n1, text: 'text', position: (x, y), size: (w, h))` where every parameter "+
				"except one of `id:`/`text:` is optional. `id:` names a note so a later "+
				"`@annotation(id: n1)` attaches THAT note to another activity instead of creating a "+
				"second one (mendixlabs/mxcli#1077). In an `alter` fragment, `@annotation(existing: 'text')` "+
				"attaches a note already stored in the flow (ako/mxcli#1695).")
	}
	v.checkExistingNotes(ann)
}

// checkExistingNotes refuses an `@annotation(existing: '…')` that cannot
// attach a stored note: one beside a parameter that describes a note of its
// own, and one in a statement that states a whole flow, where there is no
// stored flow to attach from. The builder refuses both at exec with the same
// words; whether the text names exactly one stored note is the plan's to say,
// against the project (resolveStoredNotes, ako/mxcli#1695).
func (v *microflowValidator) checkExistingNotes(ann *ast.ActivityAnnotations) {
	for _, note := range append(append([]ast.MicroflowAnnotation{}, ann.Notes...), ann.FreeNotes...) {
		if note.Existing == "" {
			continue
		}
		if msg := existingNoteMisuse(note); msg != "" {
			v.addViolation("MDL079", linter.SeverityError, msg,
				"Write `@annotation(existing: 'text')` alone to attach the stored note, or a note of the "+
					"statement's own without `existing:`.")
			continue
		}
		if !v.fragment {
			v.addViolation("MDL079", linter.SeverityError, existingNoteOutsideAlter(note.Existing),
				"Name a stored note by its text in `alter microflow M.F { insert after … begin "+
					"@annotation(existing: 'text') … end; }`.")
		}
	}
}

// checkAnnotationLabels refuses an `@annotation(id: …)` that never gets a text.
//
// This is a whole-body check rather than a per-statement one because a label is
// declared on one statement and referenced on another — the point of having
// labels at all. The flow builder refuses the same thing at exec time, but its
// errors do not escape a loop body, and `check` is what people run.
func (v *microflowValidator) checkAnnotationLabels(body []ast.MicroflowStatement) {
	// One pass, in statement order, because that is what the flow builder does:
	// a two-pass check would accept a reference written above its declaration
	// and exec would then refuse it. DESCRIBE never emits that shape either — it
	// declares a note at its first mention in traversal order — so agreeing with
	// the builder costs nothing and keeps `check` honest.
	declared := map[string]bool{}
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			if ann := ast.StatementAnnotations(s); ann != nil {
				for _, note := range append(append([]ast.MicroflowAnnotation{}, ann.Notes...), ann.FreeNotes...) {
					switch {
					case note.Label == "" || note.Existing != "":
						// `existing:` beside `id:` is checkExistingNotes' to refuse.
					case note.Text != "":
						declared[note.Label] = true
					case !declared[note.Label]:
						hint := fmt.Sprintf("The FIRST mention of a note carries its text: "+
							"`@annotation(id: %s, text: '…')`. Later mentions attach that same note to "+
							"another activity with `@annotation(id: %s)`.", note.Label, note.Label)
						if v.fragment {
							// The labels describe prints are not stored, so a
							// fragment cannot see the stored flow's (ako/mxcli#1695).
							hint += " A note already stored in the flow is attached by its text: " +
								"`@annotation(existing: '…')`."
						}
						v.addViolation("MDL079", linter.SeverityError,
							fmt.Sprintf("`@annotation(id: %s)` refers to a note that has not been declared above it", note.Label),
							hint)
					}
				}
			}
			for _, nested := range ast.StatementBodies(s) {
				walk(nested)
			}
		}
	}
	walk(body)
}

// excludedNonActivityRule is `@excluded` on a statement that builds no activity.
const excludedNonActivityRule = "MDL095"

// checkExcludedOnNonActivity raises MDL095 for `@excluded` on a statement that
// does not build an action activity. On an activity it is Studio Pro's "Disable
// activity" (ActionActivity.Disabled); the Model SDK declares that property on
// ActionActivity and nowhere else, so on a decision, a loop or an event the
// builder has nowhere to put it and drops it — and the logic the author meant
// to switch off runs (mendixlabs/mxcli#1139).
func (v *microflowValidator) checkExcludedOnNonActivity(s ast.MicroflowStatement, ann *ast.ActivityAnnotations) {
	if !ann.Excluded {
		return
	}
	var what string
	switch s.(type) {
	case *ast.IfStmt, *ast.EnumSplitStmt, *ast.InheritanceSplitStmt:
		what = "a decision"
	case *ast.LoopStmt, *ast.WhileStmt:
		what = "a loop"
	case *ast.ReturnStmt:
		what = "an end event"
	case *ast.RaiseErrorStmt:
		what = "an error event"
	case *ast.BreakStmt, *ast.ContinueStmt:
		what = "a break or continue event"
	case *ast.MergeStmt, *ast.JoinStmt:
		what = "a merge"
	default:
		return
	}
	v.addViolation(excludedNonActivityRule, linter.SeverityWarning,
		"@excluded on "+what+" has no effect — only an activity can be disabled (Disabled is "+
			"a property of the action activity alone), so the annotation is dropped and nothing is disabled.",
		"Put @excluded on the activities themselves, e.g. each activity inside the branch or loop body")
}

// checkCaptionOnLoop raises MDL042 for a @caption on a loop or a while loop. Both
// build a Microflows$LoopedActivity, which has no Caption property -- Studio Pro
// labels a for-each loop from its iterator and a while loop from its condition --
// so the caption is dropped on write. The supported way to label either is an
// annotation note, which round-trips through DESCRIBE.
func (v *microflowValidator) checkCaptionOnLoop(ann *ast.ActivityAnnotations, what string) {
	if ann == nil || ann.Caption == "" {
		return
	}
	v.addViolation("MDL042", linter.SeverityWarning,
		"@caption on "+what+" has no effect — Mendix loops have no caption "+
			"(the loop activity has no Caption property, so it is dropped). "+
			"Use @annotation to attach a note to the loop instead.",
		"Replace @caption with @annotation to label the loop")
}

// lockWorkflowAllRule refuses `lock workflow all` / `unlock workflow all`.
const lockWorkflowAllRule = "MDL-WF17"

// refuseWorkflowAll reports a lock or unlock that names no workflow. A Lock
// workflow activity always targets one definition; PauseAllWorkflows is the
// "Pause instances" option ON that definition, not "every workflow", and the
// metamodel has no all-definitions selection. The bare form built as CE1825
// "The 'Workflow' property is required" (mendixlabs/mxcli#870).
func (v *microflowValidator) refuseWorkflowAll(verb, flag, activity string) {
	label := strings.ToUpper(flag[:1]) + flag[1:]
	v.addViolation(lockWorkflowAllRule, linter.SeverityError,
		fmt.Sprintf("`%s workflow all` names no workflow — %s workflow activity always targets one "+
			"workflow definition, and the build fails CE1825 \"The 'Workflow' property is required\"", verb, activity),
		fmt.Sprintf("Name the workflow: `%s workflow $WorkflowDefinition;` or `%s workflow Module.Workflow;`. "+
			"To %s the running instances of that workflow as well (Studio Pro's \"%s instances\"), add `%s all`: "+
			"`%s workflow $WorkflowDefinition %s all;`.", verb, verb, flag, label, flag, verb, flag))
}

// matchesOuterIterator reports whether an inner loop's body holds an IF whose
// condition reads both the inner iterator and one of the enclosing loops'
// iterators — the key lookup MDL001 is about ($p/Key = $r/Key).
func matchesOuterIterator(inner *ast.LoopStmt, outer []string) bool {
	trim := func(n string) string { return strings.TrimPrefix(n, "$") }
	isOuter := map[string]bool{}
	for _, o := range outer {
		isOuter[trim(o)] = true
	}
	innerVar := trim(inner.LoopVariable)
	found := false
	forEachMicroflowStatement(inner.Body, func(s ast.MicroflowStatement) {
		ifs, ok := s.(*ast.IfStmt)
		if !ok || found {
			return
		}
		readsInner, readsOuter := false, false
		for _, v := range predicatePathVariables(ifs.Condition) {
			v = trim(v)
			readsInner = readsInner || v == innerVar
			readsOuter = readsOuter || isOuter[v]
		}
		found = readsInner && readsOuter
	})
	return found
}
