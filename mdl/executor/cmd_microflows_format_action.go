// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow activity and action formatting as MDL statements.
package executor

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// formatActivity formats a single microflow activity as an MDL statement.

// commitModifier renders the Commit flag of a create/change activity as the MDL
// modifier, or "" for Mendix's default (No) so the common case stays unwritten.
// The leading space is included, since the caller appends it directly after the
// member list.
//
// Before #779 this was never emitted: DESCRIBE could not distinguish a committing
// create from a non-committing one, and re-executing its output cleared the flag.
func commitModifier(c microflows.CommitType) string {
	switch c {
	case microflows.CommitTypeYes:
		return " commit"
	case microflows.CommitTypeYesWithoutEvents:
		return " commit without events"
	default:
		return ""
	}
}

// associationMemberChangeStatement renders a Change object activity whose only
// member is an Add or Remove on an association as `add $X to $Obj/Assoc` or
// `remove $X from $Obj/Assoc` — the statements that build it. ok is false when
// the activity has another shape, or a Remove whose value is not a bare
// variable (the remove form takes a variable, as the list form does).
func associationMemberChangeStatement(ctx *ExecContext, varName string, a *microflows.ChangeObjectAction) (string, bool) {
	if len(a.Changes) != 1 {
		return "", false
	}
	m := a.Changes[0]
	if m.AssociationQualifiedName == "" {
		return "", false
	}
	value := escapeExpressionValue(ctx, describeExpr(ctx, m.Value))
	modifiers := commitModifier(a.Commit) + refreshModifier(a.RefreshInClient)
	switch m.Type {
	case microflows.MemberChangeTypeAdd:
		return fmt.Sprintf("add %s to $%s/%s%s;", value, varName, m.AssociationQualifiedName, modifiers), true
	case microflows.MemberChangeTypeRemove:
		if !isBareVariable(value) {
			return "", false
		}
		return fmt.Sprintf("remove %s from $%s/%s%s;", value, varName, m.AssociationQualifiedName, modifiers), true
	}
	return "", false
}

// memberChangeOperator is the CHANGE member operator for a stored MemberChange
// Type: Add and Remove are `+=` and `-=`, so an activity mixing them with other
// members re-executes to the same members (mendixlabs/mxcli#1473). Written as
// `=` they would come back as set assignments that replace the reference set.
func memberChangeOperator(t microflows.MemberChangeType) string {
	switch t {
	case microflows.MemberChangeTypeAdd:
		return "+="
	case microflows.MemberChangeTypeRemove:
		return "-="
	}
	return "="
}

// isBareVariable reports whether s is a single `$Name` reference.
func isBareVariable(s string) bool {
	if len(s) < 2 || s[0] != '$' {
		return false
	}
	for _, r := range s[1:] {
		if !(r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// refreshModifier renders the REFRESH modifier shared by create, change,
// commit, delete and rollback. Absent means Refresh in client = No, which is
// Mendix's default on every one of them.
//
// It is a helper rather than five inline conditionals because the five drifted:
// delete and create had no REFRESH in the grammar and none in the describer, so
// a Studio Pro activity with the flag set round-tripped back to No — the same
// defect #407 fixed for change alone.
func refreshModifier(refresh bool) string {
	if refresh {
		return " refresh"
	}
	return ""
}

// describeExpr renders a stored Mendix expression for describe output.
//
// Studio Pro stores an expression exactly as typed, and a trailing newline (or
// space) left in the expression editor is common in real projects. Emitted
// verbatim it puts the closing `)` or `;` of the MDL statement on a line of its
// own. Leading and trailing whitespace is never part of an expression's meaning
// — an expression cannot end inside a string literal — so it is dropped here,
// in the describer, and never in the stored model. Interior newlines are the
// author's formatting of a multi-line expression and are kept.
//
// Every describe site that interpolates a stored microflow expression goes
// through this helper; the ad-hoc TrimSuffix/TrimRight calls it replaced each
// covered one slot and let the neighbouring ones drift.
//
// It also writes the expression's string literals in the language describe
// writes (describeLanguage). Under mdl 1 a Mendix expression is MDL as it
// stands. Under mdl 0 a backslash in a string literal escapes, and the reader
// takes every literal of a microflow expression for its unescaped value, so a
// stored backslash is written `\\` (mdl0ExpressionStrings): written as it is,
// `'C:\temp'` would read back as a tab (ako/mxcli#810, #820). A control
// character is left as it is — the mdl 0 reader accepts it raw — except in a
// create or change member (escapeExpressionValue).
//
// Widget and workflow expressions are not microflow expressions: they are
// stored as written in both languages, so their describers only trim.
// describeCallArgument renders a call microflow / call nanoflow / call external
// action argument. A
// blank stored argument — the mapping is present, its expression empty, which
// is what Studio Pro writes for an argument field left empty — is `nothing`.
// Printing it through describeExpr gave `Param = ,` / `Param = )`, which does
// not parse; `empty` would re-store the Mendix value "empty" and omitting the
// argument would drop the mapping, so neither round-trips.
func describeCallArgument(ctx *ExecContext, v string) string {
	if strings.TrimSpace(v) == "" {
		return "nothing"
	}
	return describeExpr(ctx, v)
}

func describeExpr(ctx *ExecContext, v string) string {
	return describeExprText(ctx, strings.TrimSpace(v))
}

// describeExprText is describeExpr keeping the stored text's surrounding
// whitespace, for a field whose round trip carries it (a retrieve's limit).
func describeExprText(ctx *ExecContext, v string) string {
	if describeLanguage(ctx) >= langver.V1 {
		return v
	}
	return mdl0ExpressionStrings(v)
}

// mdl0ExpressionStrings doubles every backslash inside the string literals of
// a stored Mendix expression, where a doubled apostrophe is the only escape,
// so that the mdl 0 reader reads each literal back as the stored value.
func mdl0ExpressionStrings(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 8)
	inString := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\'' && inString && i+1 < len(v) && v[i+1] == '\'':
			b.WriteString("''")
			i++
		case c == '\'':
			inString = !inString
			b.WriteByte(c)
		case c == '\\' && inString:
			b.WriteString(`\\`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// describeXPath writes a stored XPath constraint for describe output in the
// language describe writes. A string in a stored constraint is spelled the
// Mendix way, a backslash as itself, and a bracketed XPath in a script reads
// its strings by the script's rule and stores their values (ako/mxcli#825):
// under mdl 1 the constraint is written as stored, under mdl 0 with every
// backslash in a string doubled (mdl0ExpressionStrings) so that it reads back
// as the stored value. Every describer of a bracketed XPath — retrieve, access
// rule, page datasource, workflow targeting, navigation sync — goes through
// here.
func describeXPath(ctx *ExecContext, x string) string {
	return describeExprText(ctx, x)
}

// escapeExpressionValue writes a stored Mendix expression for describe output
// in the language describe writes (describeLanguage).
//
// Under mdl 1 a Mendix expression is MDL as it stands — a string literal's
// only escape is `”` in both — so it is written as stored, a line break in a
// literal included; the reader stores such an expression as written, which is
// the stored expression (ako/mxcli#804). Under mdl 0 a control character
// inside a string literal is written as its backslash escape (`\n`, `\r`,
// `\t`), which the mdl 0 reader turns back into the character; characters
// outside string literals (structural whitespace) are preserved as-is.
func escapeExpressionValue(ctx *ExecContext, v string) string {
	if describeLanguage(ctx) >= langver.V1 || !strings.ContainsAny(v, "\n\r\t") {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 32)
	inString := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\'' {
			// Check for escaped quote ('') inside string
			if inString && i+1 < len(v) && v[i+1] == '\'' {
				b.WriteString("''")
				i++
				continue
			}
			inString = !inString
			b.WriteByte(c)
			continue
		}
		if inString {
			switch c {
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				b.WriteByte(c)
			}
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}
func formatActivity(
	ctx *ExecContext,
	obj microflows.MicroflowObject,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) string {
	switch activity := obj.(type) {
	case *microflows.StartEvent:
		return "" // Skip start events

	case *microflows.EndEvent:
		if activity.ReturnValue != "" {
			returnVal := describeExpr(ctx, activity.ReturnValue)
			// A legacy writer stored a returned variable without its `$`; add it
			// back for exactly that shape — a bare name, optionally with an
			// attribute path — and leave every other expression as stored. A
			// blacklist of characters here turned `[%CurrentUser%]` into
			// `$[%CurrentUser%]`, which does not parse (ako/mxcli#950).
			if isBareReturnVariable(returnVal) {
				returnVal = "$" + returnVal
			}
			return fmt.Sprintf("return %s;", returnVal)
		}
		if ctx != nil && ctx.DescribingMicroflowHasReturnValue {
			return ""
		}
		// Without render context, default to the void-flow form.
		return "return;"

	case *microflows.ActionActivity:
		return formatAction(ctx, activity.Action, entityNames, microflowNames)

	case *microflows.ExclusiveSplit:
		condition := formatSplitCondition(ctx, activity.SplitCondition)
		return fmt.Sprintf("if %s then", condition)

	case *microflows.InheritanceSplit:
		varName := activity.VariableName
		if !strings.HasPrefix(varName, "$") {
			varName = "$" + varName
		}
		return fmt.Sprintf("split type %s;", varName)

	case *microflows.ExclusiveMerge:
		return "end if;"

	case *microflows.LoopedActivity:
		switch ls := activity.LoopSource.(type) {
		case *microflows.WhileLoopCondition:
			return fmt.Sprintf("while %s", describeExpr(ctx, ls.WhileExpression))
		case *microflows.IterableList:
			iterVar := "Item"
			listVar := "List"
			if ls.VariableName != "" {
				iterVar = ls.VariableName
			}
			if ls.ListVariableName != "" {
				listVar = ls.ListVariableName
			}
			return fmt.Sprintf("loop $%s in $%s", iterVar, listVar)
		default:
			return "loop $Item in $List"
		}

	case *microflows.BreakEvent:
		return "break;"

	case *microflows.ContinueEvent:
		return "continue;"

	case *microflows.ErrorEvent:
		return "raise error;"

	case *microflows.Annotation:
		return "" // Annotations are emitted separately via @annotation

	default:
		return fmt.Sprintf("-- Unknown activity type: %T", obj)
	}
}

// formatAction formats a microflow action as an MDL statement.
func formatAction(
	ctx *ExecContext,
	action microflows.MicroflowAction,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) string {
	if action == nil {
		return "-- Empty action"
	}

	switch a := action.(type) {
	case *microflows.SynchronizeAction:
		// Emit the mode explicitly in every case, including the All default, so
		// the statement is self-describing and a describe→exec round trip cannot
		// turn a Specific-mode action into an All-mode one by omission.
		switch a.SyncType {
		case microflows.SynchronizationTypeSpecific:
			vars := make([]string, 0, len(a.VariableNames))
			for _, v := range a.VariableNames {
				vars = append(vars, "$"+strings.TrimPrefix(v, "$"))
			}
			return fmt.Sprintf("synchronize %s;", strings.Join(vars, ", "))
		case microflows.SynchronizationTypeUnsynchronized:
			return "synchronize unsynchronized;"
		default:
			return "synchronize all;"
		}

	case *microflows.UnsupportedAction:
		// The reader had no mapping for this action's $Type. Name it: the old
		// anonymous "-- Empty action" was indistinguishable from an activity that
		// genuinely has no action, so a describe→edit→exec cycle deleted the
		// activity with nothing in the output to suggest anything was lost (#863).
		if a.StorageType == "" {
			return "-- Empty action"
		}
		return fmt.Sprintf("-- Unsupported action: %s", a.StorageType)

	case *microflows.CastAction:
		outputVar := a.OutputVariable
		if outputVar != "" && !strings.HasPrefix(outputVar, "$") {
			outputVar = "$" + outputVar
		}
		objectVar := a.ObjectVariable
		if objectVar != "" && !strings.HasPrefix(objectVar, "$") {
			objectVar = "$" + objectVar
		}
		if objectVar == "" {
			return fmt.Sprintf("cast %s;", outputVar)
		}
		if outputVar == "" {
			return fmt.Sprintf("cast %s;", objectVar)
		}
		return fmt.Sprintf("%s = cast %s;", outputVar, objectVar)

	case *microflows.CreateVariableAction:
		varType := "Object"
		if a.DataType != nil {
			varType = formatMicroflowDataType(ctx, a.DataType, entityNames)
		}
		initialValue := describeExpr(ctx, a.InitialValue)
		if initialValue == "" {
			initialValue = "empty"
		}
		return fmt.Sprintf("declare $%s %s = %s;", a.VariableName, varType, initialValue)

	case *microflows.ChangeVariableAction:
		varName := a.VariableName
		// Check if this is an XPath attribute access (contains /)
		if strings.Contains(varName, "/") {
			// XPath like "$Product/Price" or "$Order/Module.Assoc/Attr"
			// This is actually changing an entity attribute, display as CHANGE statement
			parts := strings.SplitN(varName, "/", 2)
			objectName := parts[0]
			attrPath := parts[1]
			// Extract just the attribute name (last part after any association navigation)
			attrParts := strings.Split(attrPath, "/")
			attrName := attrParts[len(attrParts)-1]
			// objectName might already have $ prefix
			if !strings.HasPrefix(objectName, "$") {
				objectName = "$" + objectName
			}
			return fmt.Sprintf("change %s (%s = %s);", objectName, attrName, describeExpr(ctx, a.Value))
		}
		// Simple variable change
		if strings.HasPrefix(varName, "$") {
			return fmt.Sprintf("set %s = %s;", varName, describeExpr(ctx, a.Value))
		}
		return fmt.Sprintf("set $%s = %s;", varName, describeExpr(ctx, a.Value))

	case *microflows.CreateObjectAction:
		// Use EntityQualifiedName (BY_NAME_REFERENCE) or fall back to EntityID lookup
		entityName := a.EntityQualifiedName
		if entityName == "" {
			entityName = entityNames[a.EntityID]
		}
		if entityName == "" {
			entityName = "Entity"
		}
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "NewObject"
		}

		if len(a.InitialMembers) > 0 {
			var members []string
			// entityModule is the module of the create target (e.g. "TargetModule"
			// for `create TargetModule.Entity`). Associations in other modules
			// must keep their module prefix so the re-exec resolver finds them.
			entityModule := ""
			if parts := strings.SplitN(entityName, ".", 2); len(parts) == 2 {
				entityModule = parts[0]
			}
			for _, m := range a.InitialMembers {
				var memberName string
				// Check if this is an association change or an attribute change
				if m.AssociationQualifiedName != "" {
					// Association: keep the module prefix when the association
					// is defined in a different module than the create target.
					// The authored form `Module.Assoc` is preserved so re-exec
					// resolves against the association's owning module rather
					// than defaulting to the create target's module, which
					// would cause Studio Pro CE1613 "attribute no longer exists"
					// at re-open.
					memberName = m.AssociationQualifiedName
					if parts := strings.SplitN(memberName, ".", 2); len(parts) == 2 {
						if parts[0] == entityModule {
							memberName = parts[1]
						}
					}
				} else {
					// Attribute: extract just the attribute name
					memberName = m.AttributeQualifiedName
					if memberName == "" {
						memberName = string(m.AttributeID)
					}
					// Extract just the attribute name from qualified name (Module.Entity.Attr -> Attr)
					if parts := strings.Split(memberName, "."); len(parts) > 0 {
						memberName = parts[len(parts)-1]
					}
				}
				members = append(members, fmt.Sprintf("%s = %s", memberName, escapeExpressionValue(ctx, describeExpr(ctx, m.Value))))
			}
			return fmt.Sprintf("$%s = create %s (%s)%s%s;", outputVar, entityName, strings.Join(members, ", "), commitModifier(a.Commit), refreshModifier(a.RefreshInClient))
		}
		return fmt.Sprintf("$%s = create %s%s%s;", outputVar, entityName, commitModifier(a.Commit), refreshModifier(a.RefreshInClient))

	case *microflows.ChangeObjectAction:
		varName := a.ChangeVariable
		if varName == "" {
			varName = "Object"
		}
		if stmt, ok := associationMemberChangeStatement(ctx, varName, a); ok {
			return stmt
		}
		if len(a.Changes) > 0 {
			var members []string
			var listQualified []string
			for _, m := range a.Changes {
				if isListQualifiedMember(m) {
					listQualified = append(listQualified, m.AttributeQualifiedName+m.AssociationQualifiedName)
				}
				var memberName string
				// Check if this is an association change or an attribute change
				if m.AssociationQualifiedName != "" {
					// Association: use fully qualified name (Module.AssociationName)
					memberName = m.AssociationQualifiedName
				} else {
					// Attribute: extract just the attribute name
					memberName = m.AttributeQualifiedName
					if memberName == "" {
						memberName = string(m.AttributeID)
					}
					// Extract just the attribute name from qualified name (Module.Entity.Attr -> Attr)
					if parts := strings.Split(memberName, "."); len(parts) > 0 {
						memberName = parts[len(parts)-1]
					}
				}
				members = append(members, fmt.Sprintf("%s %s %s", memberName, memberChangeOperator(m.Type), escapeExpressionValue(ctx, describeExpr(ctx, m.Value))))
			}
			stmt := fmt.Sprintf("change $%s (%s)%s%s;", varName, strings.Join(members, ", "), commitModifier(a.Commit), refreshModifier(a.RefreshInClient))
			if len(listQualified) > 0 {
				// Written over a list before mendixlabs/mxcli#1422: the bare
				// name above is not what is stored, and mxbuild reports CE1613.
				stmt += fmt.Sprintf(" -- WARNING: stored member %s is qualified through a list type (CE1613); "+
					"re-create this flow with $%s retrieved as an object (`first`)",
					strings.Join(listQualified, ", "), varName)
			}
			return stmt
		}
		return fmt.Sprintf("change $%s%s%s;", varName, commitModifier(a.Commit), refreshModifier(a.RefreshInClient))

	case *microflows.CommitObjectsAction:
		varName := a.CommitVariable
		if varName == "" {
			varName = "Object"
		}
		// Events ON is the default and is left unwritten; events OFF is the
		// deviation and must be spelled out, or a describe → exec round trip
		// silently turns the handlers back on (#895).
		suffix := ""
		if !a.WithEvents {
			suffix += " without events"
		}
		suffix += refreshModifier(a.RefreshInClient)
		return fmt.Sprintf("commit $%s%s;", varName, suffix)

	case *microflows.DeleteObjectAction:
		return fmt.Sprintf("delete $%s%s;", a.DeleteVariable, refreshModifier(a.RefreshInClient))

	case *microflows.RollbackObjectAction:
		return fmt.Sprintf("rollback $%s%s;", a.RollbackVariable, refreshModifier(a.RefreshInClient))

	case *microflows.CreateListAction:
		// Use EntityQualifiedName (BY_NAME_REFERENCE) or fall back to EntityID lookup
		entityName := a.EntityQualifiedName
		if entityName == "" {
			entityName = entityNames[a.EntityID]
		}
		if entityName == "" {
			entityName = "Entity"
		}
		return fmt.Sprintf("$%s = create list of %s;", a.OutputVariable, entityName)

	case *microflows.ChangeListAction:
		varName := a.ChangeVariable
		switch a.Type {
		case microflows.ChangeListTypeAdd:
			return fmt.Sprintf("add %s to $%s;", describeExpr(ctx, a.Value), varName)
		case microflows.ChangeListTypeRemove:
			return fmt.Sprintf("remove %s from $%s;", describeExpr(ctx, a.Value), varName)
		case microflows.ChangeListTypeClear:
			return fmt.Sprintf("clear $%s;", varName)
		case microflows.ChangeListTypeSet:
			return fmt.Sprintf("set $%s = %s;", varName, describeExpr(ctx, a.Value))
		default:
			return fmt.Sprintf("change list $%s (%s);", varName, a.Type)
		}

	case *microflows.ListOperationAction:
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "Result"
		}
		return formatListOperation(ctx, a.Operation, outputVar)

	case *microflows.AggregateListAction:
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "Result"
		}
		fn, known := mdlAggregateKeyword(a.Function)
		if !known {
			// A function this build has no MDL keyword for. Rendering it by
			// lowercasing the stored name is what produced unreadable MDL for
			// Reduce/All/Any (#1004), so say so instead of emitting a lie.
			return fmt.Sprintf("// unsupported aggregate function %q on $%s — mxcli cannot render this activity as MDL;", a.Function, outputVar)
		}
		// Extract attribute name (use last part of qualified name for readability)
		attrName := a.AttributeQualifiedName
		if attrName != "" {
			// Qualified name is like "Module.Entity.Attribute" - extract just attribute name
			parts := strings.Split(attrName, ".")
			if len(parts) > 0 {
				attrName = parts[len(parts)-1]
			}
		}
		// The statement that mirrors the activity, in every language version:
		// the call form `$n = count($L)` is deprecated (MDL-DEPR004), and
		// describe never emits a deprecated spelling (ako/mxcli#840). The
		// statement form reads the same under mdl 0 and mdl 1.
		return formatAggregateActivity(ctx, a, fn, attrName, outputVar, entityNames)

	case *microflows.RetrieveAction:
		outputVar := a.OutputVariable
		if outputVar == "" {
			outputVar = "Result"
		}

		if dbSource, ok := a.Source.(*microflows.DatabaseRetrieveSource); ok {
			// Try EntityID lookup first, fall back to EntityQualifiedName
			entityName := entityNames[dbSource.EntityID]
			if entityName == "" && dbSource.EntityQualifiedName != "" {
				entityName = dbSource.EntityQualifiedName
			}
			if entityName == "" {
				entityName = "Entity"
			}

			// A DatabaseRetrieveSource always renders in database form
			// (`from Entity where …`). It must NOT be collapsed to the
			// association shorthand `from $var/Assoc` even when its XPath is a
			// reverse-association constraint — that shorthand is the in-memory
			// retrieve-by-association form, and rendering a database retrieve
			// that way makes the two indistinguishable and breaks round-trip
			// (issue #726).
			stmt := fmt.Sprintf("retrieve $%s from %s", outputVar, entityName)

			if dbSource.XPathConstraint != "" {
				// A stored constraint may be broken across lines — mxcli formats a
				// long one that way so it can be read in Studio Pro's editor
				// (upstream #979), and that layout is described on one line: the
				// executor re-derives it on write. A layout a person gave it is
				// described as stored, which is what re-executing it stores
				// (mendixlabs/mxcli#1542).
				constraint := visitor.DescribeXPathConstraintLayout(strings.TrimSpace(dbSource.XPathConstraint))
				// Enrich string literals for enum attributes to qualified names
				// (e.g. Status = 'Open' → Status = Module.OrderStatus.Open) when
				// the entity is known and we are connected to a project.
				constraint = enrichXPathConstraintForDescribe(ctx, entityName, constraint)
				// XPath may hold several sibling predicate groups — [a][b], or
				// separated by newlines. Emit each on its own line so the parser
				// sees separate xpathConstraint nodes. The splitter is nesting- and
				// quote-aware: the previous "][" split mangled a group containing a
				// nested bracket or a literal with a ']' in it (#772).
				if groups := visitor.SplitXPathPredicateGroups(constraint); len(groups) > 0 {
					if len(groups) > 1 {
						constraint = strings.Join(groups, "\n    ")
					} else if inner := strings.TrimSuffix(strings.TrimPrefix(groups[0], "["), "]"); !xpathHasNestedPredicate(inner) {
						// A lone group renders without its outer brackets, matching
						// the `where <expr>` form the MDL grammar expects.
						constraint = inner
					}
					// A group holding a nested predicate (`A/B[Name = empty]`)
					// keeps its outer brackets: the bare `where <expr>` form is
					// read as an expression, which has no `[`, so its own parser
					// rejected the description and create or modify could not
					// compare the stored flow at all (ako/mxcli#859).
				}
				// A string in the constraint is stored as its value, as in an
				// expression; under mdl 0 its backslashes are escaped so that it
				// reads back as that value.
				constraint = describeXPath(ctx, constraint)
				stmt += fmt.Sprintf("\n    where %s", constraint)
			}

			// Output SORT BY clause if present
			if len(dbSource.Sorting) > 0 {
				var sortParts []string
				for _, sortItem := range dbSource.Sorting {
					// A sort that navigates associations is emitted with its hops,
					// one `/` per step. Emitting the attribute alone is lossy in the
					// way that hides longest: the replay has to guess which
					// association was meant, and where two reach the same entity it
					// can pick the other one — a model that builds cleanly and sorts
					// by the wrong thing (mendixlabs/mxcli#1152).
					attrName := sortItem.AttributeQualifiedName
					if len(sortItem.EntityRefSteps) == 0 {
						attrName = shortSortAttribute(ctx, entityName, attrName)
					}
					for i := len(sortItem.EntityRefSteps) - 1; i >= 0; i-- {
						if assoc := sortItem.EntityRefSteps[i].Association; assoc != "" {
							attrName = assoc + "/" + attrName
						}
					}
					order := "asc"
					if sortItem.Direction == microflows.SortDirectionDescending {
						order = "desc"
					}
					sortParts = append(sortParts, attrName+" "+order)
				}
				stmt += fmt.Sprintf("\n    sort by %s", strings.Join(sortParts, ", "))
			}

			if dbSource.Range != nil {
				switch dbSource.Range.RangeType {
				case microflows.RangeTypeFirst:
					// `first`, not `limit 1`: under mdl 1 `limit 1` is a list of
					// one, and `first` means the object in every version, so the
					// output reads back to this range whatever the reader's
					// header (ako/mxcli#734).
					stmt += "\n    first"
				case microflows.RangeTypeCustom:
					// Limit and offset are Mendix expressions, spelled for the
					// describe language like any other (ako/mxcli#825).
					if dbSource.Range.Limit != "" {
						stmt += fmt.Sprintf("\n    limit %s", describeExprText(ctx, dbSource.Range.Limit))
					}
					if dbSource.Range.Offset != "" {
						stmt += fmt.Sprintf("\n    offset %s", describeExprText(ctx, dbSource.Range.Offset))
					}
				}
			}

			return stmt + ";"
		}

		if assocSource, ok := a.Source.(*microflows.AssociationRetrieveSource); ok {
			startVar := assocSource.StartVariable
			if startVar == "" {
				startVar = "Object"
			}
			// Use AssociationQualifiedName (BY_NAME_REFERENCE) if available
			assocName := assocSource.AssociationQualifiedName
			if assocName == "" {
				assocName = "..."
			}
			return fmt.Sprintf("retrieve $%s from $%s/%s;", outputVar, startVar, assocName)
		}

		return fmt.Sprintf("retrieve $%s from ...;", outputVar)

	case *microflows.LogMessageAction:
		level := string(a.LogLevel)
		if level == "" {
			level = "info"
		}
		// Node is an expression in Mendix (e.g., 'TEST' or $variable or 'Prefix' + $var)
		// Output it as-is since it's already stored as an expression
		node := describeExpr(ctx, a.LogNodeName)
		if node == "" {
			node = defaultLogNodeExpression
		}
		message := "'Message'"
		if text := pickTextTranslation(a.MessageTemplate, describeDefaultLanguage(ctx)); text != "" {
			message = mdlQuote(ctx, text)
		}

		// Build WITH clause if there are template parameters
		withClause := ""
		if len(a.TemplateParameters) > 0 {
			var params []string
			for i, expr := range a.TemplateParameters {
				params = append(params, fmt.Sprintf("{%d} = %s", i+1, describeExpr(ctx, expr)))
			}
			withClause = fmt.Sprintf(" with (%s)", strings.Join(params, ", "))
		}
		// "Include latest stack trace" (mendixlabs/mxcli#1543). Unticked is what
		// an absent clause builds, so only a ticked box is printed.
		if a.IncludeLastStackTrace {
			withClause += " with stacktrace"
		}

		// Level info and node 'Application' are what an unstated level and node
		// build (addLogMessageAction), so neither is printed (R12, #748).
		head := "log"
		if lvl := strings.ToLower(level); lvl != "info" {
			head += " " + lvl
		}
		if node != defaultLogNodeExpression {
			head += " node " + node
		}
		return fmt.Sprintf("%s %s%s;", head, message, withClause)

	case *microflows.MicroflowCallAction:
		mfName := ""
		if a.MicroflowCall != nil && a.MicroflowCall.Microflow != "" {
			mfName = a.MicroflowCall.Microflow
		} else {
			mfName = "Microflow"
		}

		var params []string
		if a.MicroflowCall != nil {
			for _, pm := range a.MicroflowCall.ParameterMappings {
				// Extract just the parameter name from qualified name (Module.Microflow.Param -> Param)
				paramName := pm.Parameter
				if idx := strings.LastIndex(paramName, "."); idx != -1 {
					paramName = paramName[idx+1:]
				}
				params = append(params, fmt.Sprintf("%s = %s", paramName, describeCallArgument(ctx, pm.Argument)))
			}
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		queue := ""
		if a.MicroflowCall != nil {
			queue = queueClauseMDL(a.MicroflowCall.QueueSettings)
		}
		if a.UseReturnVariable && a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = call microflow %s(%s)%s;", a.ResultVariableName, mfName, paramStr, queue)
		}
		return fmt.Sprintf("call microflow %s(%s)%s;", mfName, paramStr, queue)

	case *microflows.NanoflowCallAction:
		nfName := ""
		if a.NanoflowCall != nil && a.NanoflowCall.Nanoflow != "" {
			nfName = a.NanoflowCall.Nanoflow
		} else {
			nfName = "Nanoflow"
		}

		var params []string
		if a.NanoflowCall != nil {
			for _, pm := range a.NanoflowCall.ParameterMappings {
				paramName := pm.Parameter
				if idx := strings.LastIndex(paramName, "."); idx != -1 {
					paramName = paramName[idx+1:]
				}
				params = append(params, fmt.Sprintf("%s = %s", paramName, describeCallArgument(ctx, pm.Argument)))
			}
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.UseReturnVariable && a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = call nanoflow %s(%s);", a.OutputVariableName, nfName, paramStr)
		}
		return fmt.Sprintf("call nanoflow %s(%s);", nfName, paramStr)

	case *microflows.JavaActionCallAction:
		javaActionName := a.JavaAction
		if javaActionName == "" {
			javaActionName = "JavaAction"
		}

		var params []string
		for _, pm := range a.ParameterMappings {
			// Extract just the parameter name from qualified name
			paramName := pm.Parameter
			if idx := strings.LastIndex(paramName, "."); idx != -1 {
				paramName = paramName[idx+1:]
			}
			// Get the value based on parameter value type
			valueStr := ""
			switch v := pm.Value.(type) {
			case *microflows.StringTemplateParameterValue:
				valueStr = describeTypedTemplate(ctx, v.TypedTemplate)
			case *microflows.CustomBlobDocumentCodeActionParameterValue:
				if v.CustomDocument != "" {
					valueStr = mdlQuote(ctx, v.CustomDocument)
				} else {
					valueStr = "empty"
				}
			case *microflows.ExpressionBasedCodeActionParameterValue:
				valueStr = describeExpr(ctx, v.Expression)
			case *microflows.BasicCodeActionParameterValue:
				if v.Argument == "" {
					valueStr = "empty"
				} else {
					valueStr = describeExpr(ctx, v.Argument)
				}
			case *microflows.MicroflowParameterValue:
				if v.Microflow != "" {
					valueStr = mdlQuote(ctx, v.Microflow)
				} else {
					valueStr = "empty"
				}
			case *microflows.EntityTypeCodeActionParameterValue:
				if v.Entity != "" {
					valueStr = mdlQuote(ctx, v.Entity)
				}
			}
			if valueStr != "" {
				params = append(params, fmt.Sprintf("%s = %s", paramName, valueStr))
			}
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		queue := queueClauseMDL(a.QueueSettings)
		if a.UseReturnVariable && a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = call java action %s(%s)%s;", a.ResultVariableName, javaActionName, paramStr, queue)
		}
		return fmt.Sprintf("call java action %s(%s)%s;", javaActionName, paramStr, queue)

	case *microflows.CallExternalAction:
		serviceName := a.ConsumedODataService
		if serviceName == "" {
			serviceName = "ODataService"
		}
		actionName := a.Name
		if actionName == "" {
			actionName = "Action"
		}

		var params []string
		for _, pm := range a.ParameterMappings {
			// describeCallArgument, not describeExpr: Studio Pro 11.16 stores an
			// optional parameter left blank as `Argument: ""`, which printed as
			// `name = )` and did not parse (ako/mxcli#1915). `nothing` is the
			// blank; `empty` (11.14's spelling of the same field) stays a value.
			params = append(params, fmt.Sprintf("%s = %s", pm.ParameterName, describeCallArgument(ctx, pm.Argument)))
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.UseReturnVariable && a.ResultVariableName != "" {
			return fmt.Sprintf("$%s = call external action %s.%s(%s);", a.ResultVariableName, serviceName, actionName, paramStr)
		}
		return fmt.Sprintf("call external action %s.%s(%s);", serviceName, actionName, paramStr)

	case *microflows.ShowPageAction:
		// Get page name from action (PageName is BY_NAME_REFERENCE, PageID is legacy BY_ID_REFERENCE)
		pageName := a.PageName
		if pageName == "" && a.PageID != "" && ctx.Connected() {
			// Fall back to looking up by ID (legacy format)
			pages, _ := ctx.Backend.ListPages()
			for _, p := range pages {
				if p.ID == a.PageID {
					h, _ := getHierarchy(ctx)
					if h != nil {
						pageName = h.GetQualifiedName(p.ContainerID, p.Name)
					}
					break
				}
			}
		}
		if pageName == "" {
			pageName = "UnknownPage"
		}

		// Build parameter list
		var params []string
		for _, pm := range a.PageParameterMappings {
			// Extract just the parameter name from the qualified name
			parts := strings.Split(pm.Parameter, ".")
			paramName := parts[len(parts)-1]
			params = append(params, fmt.Sprintf("%s = %s", paramName, describeExpr(ctx, pm.Argument)))
		}

		// Build the statement
		paramStr := ""
		if len(params) > 0 {
			paramStr = "(" + strings.Join(params, ", ") + ")"
		}
		// The page title override. Left out, describe -> exec dropped it, and
		// taking it out of a create or modify compared as no change
		// (ako/mxcli#869). An override without text has no statement form:
		// `with title = ''` builds none.
		var settings []string
		if title := pickTextTranslation(a.OverridePageTitle, describeDefaultLanguage(ctx)); title != "" {
			settings = append(settings, "title = "+mdlQuote(ctx, title))
		}
		// "Number of pages to close". Left out, an action that closes the
		// current page read exactly like one that closes none
		// (mendixlabs/mxcli#1617). "" is Studio Pro's default: none.
		if n := strings.TrimSpace(a.NumberOfPagesToClose); n != "" {
			settings = append(settings, "NumberOfPagesToClose = "+describeExpr(ctx, n))
		}
		withStr := ""
		if len(settings) > 0 {
			withStr = " with " + strings.Join(settings, ", ")
		}
		return fmt.Sprintf("show page %s%s%s;", pageName, paramStr, withStr)

	case *microflows.ClosePageAction:
		if a.NumberOfPages > 1 {
			return fmt.Sprintf("close page %d;", a.NumberOfPages)
		}
		return "close page;"

	case *microflows.ShowHomePageAction:
		return "show home page;"

	case *microflows.ShowMessageAction:
		msgType := string(a.Type)
		if msgType == "" {
			msgType = "Information"
		}
		message := "'...'"
		if text := pickTextTranslation(a.Template, describeDefaultLanguage(ctx)); text != "" {
			message = mdlQuote(ctx, text)
		}
		result := fmt.Sprintf("show message %s type %s", message, msgType)
		if len(a.TemplateParameters) > 0 {
			result += templateArgsClause(ctx, a.TemplateParameters)
		}
		// Without this, a describe -> exec round trip turned a BLOCKING message
		// box into a non-blocking one. The model carried Blocking on both
		// engines all along; MDL simply had no word for it, which is why the
		// loss happened in the middle of a path where every other layer was
		// correct.
		if a.Blocking {
			result += " blocking"
		}
		return result + ";"

	case *microflows.DownloadFileAction:
		fileDocument := a.FileDocument
		if fileDocument != "" && !strings.HasPrefix(fileDocument, "$") {
			fileDocument = "$" + fileDocument
		}
		result := fmt.Sprintf("download file %s", fileDocument)
		if a.ShowInBrowser {
			result += " show in browser"
		}
		return result + ";"

	case *microflows.ValidationFeedbackAction:
		msgText := "'...'"
		if text := pickTextTranslation(a.Template, describeDefaultLanguage(ctx)); text != "" {
			msgText = mdlQuote(ctx, text)
		}
		// Build attribute path from variable and attribute name
		// AttributeName format: Module.Entity.Attribute
		// Add $ prefix to variable name if not present
		varName := a.ObjectVariable
		if !strings.HasPrefix(varName, "$") {
			varName = "$" + varName
		}
		attrPath := varName
		if a.AttributeName != "" {
			// Extract just the attribute part from "Module.Entity.Attribute"
			parts := strings.Split(a.AttributeName, ".")
			if len(parts) >= 3 {
				attrPath = varName + "/" + parts[len(parts)-1]
			}
		} else if a.AssociationName != "" {
			attrPath = varName + "/" + a.AssociationName
		}
		return fmt.Sprintf("validation feedback %s message %s%s;", attrPath, msgText, templateArgsClause(ctx, a.TemplateParameters))

	case *microflows.RestCallAction:
		return formatRestCallAction(ctx, a)

	case *microflows.SendEmailAction:
		return formatSendEmailAction(ctx, a)

	case *microflows.RestOperationCallAction:
		return formatRestOperationCallAction(ctx, a)

	case *microflows.ExecuteDatabaseQueryAction:
		return formatExecuteDatabaseQueryAction(ctx, a)

	case *microflows.ImportXmlAction:
		return formatImportXmlAction(ctx, a, entityNames)

	case *microflows.ExportXmlAction:
		return formatExportXmlAction(ctx, a)

	case *microflows.TransformJsonAction:
		return formatTransformJsonAction(a)

	// Workflow microflow actions
	case *microflows.GetWorkflowDataAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = get workflow data $%s as %s;", a.OutputVariableName, a.WorkflowVariable, a.Workflow)
		}
		return fmt.Sprintf("get workflow data $%s as %s;", a.WorkflowVariable, a.Workflow)

	case *microflows.WorkflowCallAction:
		if a.UseReturnVariable && a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = call workflow %s ($%s);", a.OutputVariableName, a.Workflow, a.WorkflowContextVariable)
		}
		return fmt.Sprintf("call workflow %s ($%s);", a.Workflow, a.WorkflowContextVariable)

	case *microflows.GetWorkflowsAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = get workflows for $%s;", a.OutputVariableName, a.WorkflowContextVariableName)
		}
		return fmt.Sprintf("get workflows for $%s;", a.WorkflowContextVariableName)

	case *microflows.GetWorkflowActivityRecordsAction:
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = get workflow activity records $%s;", a.OutputVariableName, a.WorkflowVariable)
		}
		return fmt.Sprintf("get workflow activity records $%s;", a.WorkflowVariable)

	case *microflows.WorkflowOperationAction:
		return formatWorkflowOperationAction(ctx, a)

	case *microflows.SetTaskOutcomeAction:
		return fmt.Sprintf("set task outcome $%s %s;", a.WorkflowTaskVariable, mdlQuote(ctx, a.OutcomeValue))

	case *microflows.OpenUserTaskAction:
		return fmt.Sprintf("open user task $%s;", a.UserTaskVariable)

	case *microflows.NotifyWorkflowAction:
		// The target used to be left out, so a rewrite from describe output wrote a
		// notify the build refuses (CE0166).
		target := ""
		if a.Target != nil && a.Target.Name != "" {
			target = " target " + a.Target.Name
		} else if a.Activity != "" {
			target = " target " + a.Activity
		}
		if a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = notify workflow $%s%s;", a.OutputVariableName, a.WorkflowVariable, target)
		}
		return fmt.Sprintf("notify workflow $%s%s;", a.WorkflowVariable, target)

	case *microflows.OpenWorkflowAction:
		return fmt.Sprintf("open workflow $%s;", a.WorkflowVariable)

	case *microflows.LockWorkflowAction:
		return "lock workflow " + workflowSelectionMDL(a.Workflow, a.WorkflowVariable, a.PauseAllWorkflows, "pause all") + ";"

	case *microflows.UnlockWorkflowAction:
		return "unlock workflow " + workflowSelectionMDL(a.Workflow, a.WorkflowVariable, a.ResumeAllPausedWorkflows, "unpause all") + ";"

	case *microflows.JavaScriptActionCallAction:
		jsActionName := a.JavaScriptAction
		if jsActionName == "" {
			if n := len(a.ParameterMappings); n > 0 {
				label := "params"
				if n == 1 {
					label = "param"
				}
				return fmt.Sprintf("-- JavaScriptAction: missing action reference (%d %s)", n, label)
			}
			return "-- JavaScriptAction: missing action reference"
		}

		var params []string
		for _, pm := range a.ParameterMappings {
			paramName := pm.Parameter
			if idx := strings.LastIndex(paramName, "."); idx != -1 {
				paramName = paramName[idx+1:]
			}
			valueStr := ""
			if pm.Value != nil {
				switch v := pm.Value.(type) {
				case *microflows.StringTemplateParameterValue:
					if v.TypedTemplate != nil {
						valueStr = mdlQuote(ctx, v.TypedTemplate.Text)
					}
				case *microflows.ExpressionBasedCodeActionParameterValue:
					valueStr = describeExpr(ctx, v.Expression)
				case *microflows.BasicCodeActionParameterValue:
					valueStr = describeExpr(ctx, v.Argument)
				case *microflows.EntityTypeCodeActionParameterValue:
					valueStr = v.Entity
				}
			}
			if valueStr != "" {
				params = append(params, fmt.Sprintf("%s = %s", paramName, valueStr))
			}
		}

		paramStr := ""
		if len(params) > 0 {
			paramStr = strings.Join(params, ", ")
		}

		if a.UseReturnVariable && a.OutputVariableName != "" {
			return fmt.Sprintf("$%s = call javascript action %s(%s);", a.OutputVariableName, jsActionName, paramStr)
		}
		return fmt.Sprintf("call javascript action %s(%s);", jsActionName, paramStr)

	case *microflows.WebServiceCallAction:
		return formatWebServiceCallAction(ctx, a)

	case *microflows.UnknownAction:
		return fmt.Sprintf("-- Unsupported action type: %s", a.TypeName)

	default:
		return fmt.Sprintf("-- Unknown action: %T", action)
	}
}

// formatWorkflowOperationAction formats a workflow operation action as MDL.
func formatWorkflowOperationAction(ctx *ExecContext, a *microflows.WorkflowOperationAction) string {
	if a.Operation == nil {
		return "workflow operation ...;"
	}
	switch op := a.Operation.(type) {
	case *microflows.AbortOperation:
		if op.Reason != "" {
			return fmt.Sprintf("workflow operation abort $%s reason %s;", op.WorkflowVariable, mdlQuote(ctx, op.Reason))
		}
		return fmt.Sprintf("workflow operation abort $%s;", op.WorkflowVariable)
	case *microflows.ContinueOperation:
		return fmt.Sprintf("workflow operation continue $%s;", op.WorkflowVariable)
	case *microflows.PauseOperation:
		return fmt.Sprintf("workflow operation pause $%s;", op.WorkflowVariable)
	case *microflows.RestartOperation:
		return fmt.Sprintf("workflow operation restart $%s;", op.WorkflowVariable)
	case *microflows.RetryOperation:
		return fmt.Sprintf("workflow operation retry $%s;", op.WorkflowVariable)
	case *microflows.UnpauseOperation:
		return fmt.Sprintf("workflow operation unpause $%s;", op.WorkflowVariable)
	default:
		return fmt.Sprintf("-- Unknown workflow operation: %T", a.Operation)
	}
}

func formatWebServiceCallAction(ctx *ExecContext, a *microflows.WebServiceCallAction) string {
	prefix := ""
	if a.OutputVariable != "" {
		prefix = fmt.Sprintf("$%s = ", a.OutputVariable)
	}
	if len(a.RawBSON) > 0 {
		raw := base64.StdEncoding.EncodeToString(canonicalRawBSON(a.RawBSON))
		return prefix + "call web service raw " + mdlQuote(ctx, raw) + ";"
	}

	// The service and the two mappings are BY_NAME_REFERENCE properties: what is
	// stored IS the qualified name, so there is nothing to resolve. See
	// TestFormatAction_WebServiceCallRendersStoredQualifiedNames for what the
	// three resolvers that used to stand here actually did.
	parts := []string{prefix + "call web service " + formatWebServiceReference(ctx, string(a.ServiceID))}
	if a.OperationName != "" {
		op := "operation " + formatWebServiceReference(ctx, a.OperationName)
		// The arguments carry the stored ParameterPath, but only its last
		// segment is spelled in MDL — the rest is rebuilt from the operation
		// document on the way back in.
		if args := formatWebServiceArguments(ctx, a.Arguments); args != "" {
			op += " (" + args + ")"
		}
		parts = append(parts, op)
	}
	if a.SendMappingID != "" {
		send := "send mapping " + formatWebServiceReference(ctx, string(a.SendMappingID))
		if a.SendMappingVariable != "" {
			send += " from $" + a.SendMappingVariable
		}
		parts = append(parts, send)
	}
	if a.ReceiveMappingID != "" {
		parts = append(parts, "receive mapping "+formatWebServiceReference(ctx, string(a.ReceiveMappingID)))
	}
	if a.TimeoutExpression != "" {
		parts = append(parts, "timeout "+describeExpr(ctx, a.TimeoutExpression))
	}
	return strings.Join(parts, "\n") + ";"
}

// formatWebServiceArguments renders the operation's argument list, or "" when
// there is nothing MDL can spell.
//
// An argument whose Name did not survive the read — a ParameterPath with no
// "|", which no reference document has — renders nothing at all, and the
// action falls back to the raw form rather than emitting a list that would
// write a different path back.
func formatWebServiceArguments(ctx *ExecContext, args []microflows.WebServiceArgument) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		if arg.Name == "" {
			return ""
		}
		parts = append(parts, arg.Name+" = "+describeExpr(ctx, arg.Expression))
	}
	return strings.Join(parts, ", ")
}

func formatWebServiceReference(ctx *ExecContext, ref string) string {
	if isBareQualifiedReference(ref) {
		return ref
	}
	return mdlQuote(ctx, ref)
}

func isBareQualifiedReference(ref string) bool {
	if ref == "" {
		return false
	}
	for _, part := range strings.Split(ref, ".") {
		if !isBareIdentifier(part) {
			return false
		}
	}
	return true
}

func isBareIdentifier(part string) bool {
	if part == "" {
		return false
	}
	for i, r := range part {
		if i == 0 {
			if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return false
			}
			continue
		}
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func rawUnitName(contents []byte) string {
	var raw map[string]any
	if err := bsoncompat.UnmarshalDocMap(contents, &raw); err != nil {
		return ""
	}
	name, _ := raw["Name"].(string)
	return name
}

// formatListOperation formats a list operation as MDL.
func formatListOperation(ctx *ExecContext, op microflows.ListOperation, outputVar string) string {
	if op == nil {
		return fmt.Sprintf("$%s = list operation ...;", outputVar)
	}
	// The statement that mirrors the activity, in every language version: the
	// call forms are deprecated (MDL-DEPR003) and, for find/filter, mean
	// something else under mdl 1 — describe never emits a deprecated spelling
	// (ako/mxcli#840). Only an activity the statement cannot express falls
	// back to the call form below.
	if stmt, ok := formatListActivity(ctx, op, outputVar); ok {
		return stmt
	}

	switch o := op.(type) {
	case *microflows.HeadOperation:
		return fmt.Sprintf("$%s = head($%s);", outputVar, o.ListVariable)
	case *microflows.TailOperation:
		return fmt.Sprintf("$%s = tail($%s);", outputVar, o.ListVariable)
	case *microflows.FindOperation:
		return fmt.Sprintf("$%s = find($%s, %s);", outputVar, o.ListVariable, describeExpr(ctx, o.Expression))
	case *microflows.FilterOperation:
		return fmt.Sprintf("$%s = filter($%s, %s);", outputVar, o.ListVariable, describeExpr(ctx, o.Expression))
	case *microflows.SortOperation:
		if len(o.Sorting) > 0 {
			var sortCols []string
			for _, s := range o.Sorting {
				dir := "asc"
				if s.Direction == microflows.SortDirectionDescending {
					dir = "desc"
				}
				// Extract attribute name (use last part of qualified name for readability)
				attrName := s.AttributeQualifiedName
				if attrName != "" {
					parts := strings.Split(attrName, ".")
					if len(parts) > 0 {
						attrName = parts[len(parts)-1]
					}
				}
				if attrName == "" {
					attrName = "..."
				}
				sortCols = append(sortCols, fmt.Sprintf("%s %s", mdlIdent(attrName), dir))
			}
			return fmt.Sprintf("$%s = sort($%s, %s);", outputVar, o.ListVariable, strings.Join(sortCols, ", "))
		}
		return fmt.Sprintf("$%s = sort($%s);", outputVar, o.ListVariable)
	case *microflows.UnionOperation:
		return fmt.Sprintf("$%s = union($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.IntersectOperation:
		return fmt.Sprintf("$%s = intersect($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.SubtractOperation:
		return fmt.Sprintf("$%s = subtract($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.ContainsOperation:
		return fmt.Sprintf("$%s = contains($%s, $%s);", outputVar, o.ListVariable, o.ObjectVariable)
	case *microflows.EqualsOperation:
		return fmt.Sprintf("$%s = equals($%s, $%s);", outputVar, o.ListVariable1, o.ListVariable2)
	case *microflows.FindByAttributeOperation:
		fieldName := extractFieldName(o.Attribute, o.Association)
		if fieldName != "" && o.Expression != "" {
			return fmt.Sprintf("$%s = find($%s, %s = %s);", outputVar, o.ListVariable, fieldName, describeExpr(ctx, o.Expression))
		} else if o.Expression != "" {
			return fmt.Sprintf("$%s = find($%s, %s);", outputVar, o.ListVariable, describeExpr(ctx, o.Expression))
		}
		return fmt.Sprintf("-- $%s = find($%s) — missing attribute/expression", outputVar, o.ListVariable)
	case *microflows.FilterByAttributeOperation:
		fieldName := extractFieldName(o.Attribute, o.Association)
		if fieldName != "" && o.Expression != "" {
			return fmt.Sprintf("$%s = filter($%s, %s = %s);", outputVar, o.ListVariable, fieldName, describeExpr(ctx, o.Expression))
		} else if o.Expression != "" {
			return fmt.Sprintf("$%s = filter($%s, %s);", outputVar, o.ListVariable, describeExpr(ctx, o.Expression))
		}
		return fmt.Sprintf("-- $%s = filter($%s) — missing attribute/expression", outputVar, o.ListVariable)
	case *microflows.ListRangeOperation:
		if o.OffsetExpression != "" && o.LimitExpression != "" {
			return fmt.Sprintf("$%s = range($%s, %s, %s);", outputVar, o.ListVariable, describeExpr(ctx, o.OffsetExpression), describeExpr(ctx, o.LimitExpression))
		} else if o.OffsetExpression != "" {
			return fmt.Sprintf("$%s = range($%s, %s);", outputVar, o.ListVariable, describeExpr(ctx, o.OffsetExpression))
		} else if o.LimitExpression != "" {
			return fmt.Sprintf("$%s = range($%s, 0, %s);", outputVar, o.ListVariable, describeExpr(ctx, o.LimitExpression))
		}
		return fmt.Sprintf("$%s = range($%s);", outputVar, o.ListVariable)
	default:
		return fmt.Sprintf("$%s = list operation %T;", outputVar, op)
	}
}

// extractFieldName returns the short field name from a qualified attribute
// or association reference (e.g. "MyModule.Entity.Status" → "Status",
// "MyModule.Order_Customer" → "Order_Customer"). Returns the association
// name if attribute is empty — qualified for a System association
// ("System.owner").
func extractFieldName(attribute, association string) string {
	ref := attribute
	if ref == "" {
		ref = association
		// A System association stays qualified. The builder resolves a bare
		// name in the list entity's own module, so `owner` came back as an
		// attribute of the entity and `User_Language` on an Administration.Account
		// list as Administration.Account.User_Language — CE1613 on the next
		// build (ako/mxcli#1629). The member speller spells it the same way.
		if strings.HasPrefix(ref, "System.") {
			return ref
		}
	}
	if ref == "" {
		return ""
	}
	parts := strings.Split(ref, ".")
	return parts[len(parts)-1]
}

// unsupportedRestResult renders a result handling mxcli cannot express as MDL.
//
// It deliberately produces something the parser REJECTS. A describe whose output
// silently means something else is worse than one that fails: the previous
// behaviour rendered any unrecognised handling as `returns String`, so a
// describe → exec round trip rewrote a FileDocument result into a String one and
// mxbuild still reported zero errors (#922). Anything that reaches this is a gap
// in the reader, and the text names it so the report says which.
func unsupportedRestResult(what string) string {
	return fmt.Sprintf("<<unsupported result handling: %s — please report this "+
		"with the microflow name at https://github.com/mendixlabs/mxcli/issues>>", what)
}

// formatRestCallAction formats a REST call action as MDL.
func formatRestCallAction(ctx *ExecContext, a *microflows.RestCallAction) string {
	var sb strings.Builder

	// Output variable assignment (may be on RestCallAction or ResultHandling)
	outputVar := a.OutputVariable
	if outputVar == "" && a.ResultHandling != nil {
		switch rh := a.ResultHandling.(type) {
		case *microflows.ResultHandlingString:
			outputVar = rh.VariableName
		case *microflows.ResultHandlingHttpResponse:
			outputVar = rh.VariableName
		case *microflows.ResultHandlingMapping:
			outputVar = rh.ResultVariable
		case *microflows.ResultHandlingFileDocument:
			outputVar = rh.VariableName
		}
	}
	if outputVar != "" {
		sb.WriteString("$")
		sb.WriteString(outputVar)
		sb.WriteString(" = ")
	}

	sb.WriteString("call rest service ")

	// HTTP method
	method := "get"
	if a.HttpConfiguration != nil {
		switch a.HttpConfiguration.HttpMethod {
		case microflows.HttpMethodGet:
			method = "get"
		case microflows.HttpMethodPost:
			method = "post"
		case microflows.HttpMethodPut:
			method = "put"
		case microflows.HttpMethodPatch:
			method = "patch"
		case microflows.HttpMethodDelete:
			method = "delete"
		}
	}
	sb.WriteString(method)
	sb.WriteString(" ")

	// URL
	url := "''"
	if a.HttpConfiguration != nil && a.HttpConfiguration.LocationTemplate != "" {
		url = mdlQuote(ctx, a.HttpConfiguration.LocationTemplate)
	}
	sb.WriteString(url)

	// URL parameters
	if a.HttpConfiguration != nil && len(a.HttpConfiguration.LocationParams) > 0 {
		sb.WriteString(" with (")
		for i, param := range a.HttpConfiguration.LocationParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("{%d} = %s", i+1, describeExpr(ctx, param)))
		}
		sb.WriteString(")")
	}

	// The dialog's settings: one property list after the URL (ADR-0013),
	// keyed as the consumed REST service names the same concepts.
	var settings []string

	if a.HttpConfiguration != nil && len(a.HttpConfiguration.CustomHeaders) > 0 {
		hdrs := make([]string, 0, len(a.HttpConfiguration.CustomHeaders))
		for _, h := range a.HttpConfiguration.CustomHeaders {
			hdrs = append(hdrs, mdlQuote(ctx, h.Name)+": "+describeExpr(ctx, h.Value))
		}
		settings = append(settings, "Headers: ("+strings.Join(hdrs, ", ")+")")
	}

	if a.HttpConfiguration != nil && a.HttpConfiguration.UseAuthentication {
		settings = append(settings, "Authentication: basic (Username: "+a.HttpConfiguration.Username+
			", Password: "+a.HttpConfiguration.Password+")")
	}

	if a.RequestHandling != nil {
		switch rh := a.RequestHandling.(type) {
		case *microflows.CustomRequestHandling:
			if rh.Template != "" {
				body := "Body: template " + mdlQuote(ctx, rh.Template)
				if len(rh.TemplateParams) > 0 {
					params := make([]string, 0, len(rh.TemplateParams))
					for i, param := range rh.TemplateParams {
						params = append(params, fmt.Sprintf("{%d} = %s", i+1, describeExpr(ctx, param)))
					}
					body += " with (" + strings.Join(params, ", ") + ")"
				}
				settings = append(settings, body)
			}
		case *microflows.BinaryRequestHandling:
			// The expression is stored as source text (`$Doc/Contents`), so it is
			// emitted verbatim — quoting it would make the round trip send the
			// path as a string literal.
			if rh.Expression != "" {
				settings = append(settings, "Body: binary "+rh.Expression)
			}
		case *microflows.MappingRequestHandling:
			if rh.MappingID != "" {
				body := "Body: mapping " + string(rh.MappingID)
				if rh.ParameterVariable != "" {
					body += " from $" + rh.ParameterVariable
				}
				settings = append(settings, body)
			}
		}
	}

	if a.TimeoutExpression != "" {
		settings = append(settings, "Timeout: "+a.TimeoutExpression)
	}

	if len(settings) > 0 {
		sb.WriteString(" (")
		for _, setting := range settings {
			sb.WriteString("\n    ")
			sb.WriteString(setting)
			sb.WriteString(",")
		}
		sb.WriteString("\n  )")
	}

	// Returns
	if len(settings) > 0 {
		sb.WriteString(" returns ")
	} else {
		sb.WriteString("\n    returns ")
	}
	if a.ResultHandling != nil {
		switch rh := a.ResultHandling.(type) {
		case *microflows.ResultHandlingString:
			sb.WriteString("String")
			_ = rh // used for type assertion only
		case *microflows.ResultHandlingHttpResponse:
			sb.WriteString("response")
		case *microflows.ResultHandlingMapping:
			sb.WriteString("mapping ")
			sb.WriteString(string(rh.MappingID))
			if rh.ResultEntityID != "" {
				// `as list of Entity` when the mapping yields a list,
				// otherwise `as Entity` for a single object. Studio Pro
				// keeps this on the ImportMappingCall (Range.SingleObject
				// + ForceSingleOccurrence); the parser collapses both into
				// SingleObject, so a list is `!SingleObject`.
				if rh.SingleObject {
					sb.WriteString(" as ")
				} else {
					sb.WriteString(" as list of ")
				}
				sb.WriteString(string(rh.ResultEntityID))
			}
		case *microflows.ResultHandlingFileDocument:
			// A file document result is always bound to a System.FileDocument
			// specialization, so the entity is the whole clause. Rendering this
			// as "String" was #922: the description round-tripped into a model
			// whose activity returned a string instead of a file, and mxbuild
			// reported nothing.
			sb.WriteString(rh.EntityRef)
		case *microflows.ResultHandlingNone:
			sb.WriteString("nothing")
		default:
			// Refuse rather than guess. The previous "String" fallback here and
			// below is what turned an unread result handling into a silent
			// retyping of the activity (#922); an unknown one is a gap in the
			// reader, and saying so is the only outcome that cannot corrupt a
			// round trip.
			sb.WriteString(unsupportedRestResult(fmt.Sprintf("%T", rh)))
		}
	} else {
		sb.WriteString(unsupportedRestResult("no result handling"))
	}

	// Note: Error handling suffix is added at the activity level, not here
	sb.WriteString(";")
	return sb.String()
}

// formatRestOperationCallAction formats a RestOperationCallAction as MDL.
func formatRestOperationCallAction(ctx *ExecContext, a *microflows.RestOperationCallAction) string {
	var sb strings.Builder

	if a.OutputVariable != nil && a.OutputVariable.VariableName != "" {
		sb.WriteString("$")
		sb.WriteString(a.OutputVariable.VariableName)
		sb.WriteString(" = ")
	}

	sb.WriteString("send rest request ")
	sb.WriteString(a.Operation)

	// WITH clause for parameter mappings
	allParams := make([]struct{ name, value string }, 0)
	for _, pm := range a.ParameterMappings {
		// Strip operation prefix from parameter name
		name := pm.Parameter
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		allParams = append(allParams, struct{ name, value string }{name, describeExpr(ctx, pm.Value)})
	}
	for _, qm := range a.QueryParameterMappings {
		name := qm.Parameter
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		allParams = append(allParams, struct{ name, value string }{name, describeExpr(ctx, qm.Value)})
	}
	if len(allParams) > 0 {
		sb.WriteString("\n    with (")
		for i, p := range allParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(p.name)
			sb.WriteString(" = ")
			sb.WriteString(p.value)
		}
		sb.WriteString(")")
	}

	if a.BodyVariable != nil && a.BodyVariable.VariableName != "" {
		sb.WriteString("\n    body $")
		sb.WriteString(a.BodyVariable.VariableName)
	}

	// RestOperationCallAction does not support custom error handling (CE6035)
	// so we don't emit ON ERROR clauses.

	sb.WriteString(";")
	return sb.String()
}

// formatExecuteDatabaseQueryAction formats a DatabaseConnector ExecuteDatabaseQueryAction as MDL.
func formatExecuteDatabaseQueryAction(ctx *ExecContext, a *microflows.ExecuteDatabaseQueryAction) string {
	var sb strings.Builder

	if a.OutputVariableName != "" {
		sb.WriteString(fmt.Sprintf("$%s = ", a.OutputVariableName))
	}

	sb.WriteString("execute database query ")
	sb.WriteString(a.Query)

	// Dynamic query override
	if a.DynamicQuery != "" {
		// A Mendix expression, spelled for the describe language like any other
		// (ako/mxcli#825): written as stored, `'C:\temp'` read back under
		// mdl 0 as a tab.
		sb.WriteString(fmt.Sprintf(" dynamic %s", describeExpr(ctx, a.DynamicQuery)))
	}

	// Parameter mappings
	if len(a.ParameterMappings) > 0 {
		sb.WriteString(" (")
		for i, pm := range a.ParameterMappings {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s = %s", pm.ParameterName, describeExpr(ctx, pm.Value)))
		}
		sb.WriteString(")")
	}

	// Connection parameter mappings (runtime connection override)
	if len(a.ConnectionParameterMappings) > 0 {
		sb.WriteString("\n    connection (")
		for i, cm := range a.ConnectionParameterMappings {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s = %s", cm.ParameterName, describeExpr(ctx, cm.Value)))
		}
		sb.WriteString(")")
	}

	sb.WriteString(";")
	return sb.String()
}

// isMendixKeyword returns true for Mendix expression keywords that must not be
// prefixed with "$" when serialized as a RETURN value.
func isMendixKeyword(s string) bool {
	switch s {
	case "empty", "true", "false", "null":
		return true
	}
	return false
}

// bareReturnVariable is a variable name without its `$`, optionally followed
// by an attribute path (`Order/Total`, `Order/Module.Assoc/Name`).
var bareReturnVariable = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(/[A-Za-z_][A-Za-z0-9_.]*)*$`)

// isBareReturnVariable reports whether a stored return value is a variable
// reference missing its `$` — the only shape describe may prefix.
func isBareReturnVariable(s string) bool {
	return bareReturnVariable.MatchString(s) && !isMendixKeyword(s)
}

// isQualifiedEnumLiteral returns true for qualified enum literals (e.g., "Module.Enum.Value")
// that must not be prefixed with "$" when serialized as a RETURN value.
func isQualifiedEnumLiteral(s string) bool {
	return strings.Count(s, ".") >= 2
}

// isNumericLiteral returns true for numeric literals (integers and decimals)
// that must not be prefixed with "$" when serialized as a RETURN value.
func isNumericLiteral(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	if s[0] == '-' {
		start = 1
		if len(s) == 1 {
			return false
		}
	}
	dotSeen := false
	hasDigit := false
	for i := start; i < len(s); i++ {
		if s[i] == '.' && !dotSeen {
			dotSeen = true
		} else if s[i] >= '0' && s[i] <= '9' {
			hasDigit = true
		} else {
			return false
		}
	}
	return hasDigit && s[len(s)-1] != '.'
}

// formatImportXmlAction formats an import mapping action as MDL.
// Syntax: [$Var =] IMPORT FROM MAPPING Module.IMM($SourceVar);
func formatImportXmlAction(ctx *ExecContext, a *microflows.ImportXmlAction, entityNames map[model.ID]string) string {
	var sb strings.Builder

	// Resolve mapping qualified name
	mappingName := ""
	resultVar := ""
	if a.ResultHandling != nil {
		mappingName = string(a.ResultHandling.MappingID)
		resultVar = a.ResultHandling.ResultVariable
	}

	// Optional assignment
	if resultVar != "" {
		sb.WriteString("$")
		sb.WriteString(resultVar)
		sb.WriteString(" = ")
	}

	sb.WriteString("import from mapping ")
	sb.WriteString(mappingName)
	sb.WriteString("($")
	sb.WriteString(a.XmlDocumentVariable)
	sb.WriteString(")")
	sb.WriteString(formatImportMappingSettings(a.ResultHandling))
	sb.WriteString(formatImportMappingRange(a.ResultHandling))
	sb.WriteString(";")

	return sb.String()
}

// formatImportMappingSettings renders the activity's settings list, `
// (Parameter: $Obj, Commit: No)`, or "" when it has neither. Unprinted, a
// rewrite dropped the input object and the commit mode (ako/mxcli#1681).
// YesWithoutEvents is what the writer stores for an unwritten Commit, so it is
// not printed.
func formatImportMappingSettings(h *microflows.ResultHandlingMapping) string {
	if h == nil {
		return ""
	}
	var parts []string
	if h.ParameterVariable != "" {
		parts = append(parts, "Parameter: $"+h.ParameterVariable)
	}
	if h.Commit != "" && h.Commit != "YesWithoutEvents" {
		parts = append(parts, "Commit: "+h.Commit)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// formatImportMappingRange renders the activity's Range — Studio Pro's
// All / First / Custom setting.
//
// Before #881 all three settings described identically, so the
// describe→edit→exec cycle silently rewrote the activity; each is now distinct.
//
// The one form left bare is All against an OBJECT variable — Studio Pro's
// default for an object-rooted mapping (the blank app's
// FeedbackModule.IMM_PostResponse). There `all` read as "returns a list"
// (upstream #1176). Bare is exact, not a shorthand: the builder writes a
// missing keyword as All explicitly and infers the object from the mapping.
// That equivalence is what makes omitting it safe — before the builder did
// so, silence stored First and the runtime threw on import. A list result
// keeps its `all`.
func formatImportMappingRange(h *microflows.ResultHandlingMapping) string {
	if h == nil {
		return ""
	}
	if h.LimitExpression != "" || h.OffsetExpression != "" {
		var sb strings.Builder
		if h.LimitExpression != "" {
			sb.WriteString(" limit ")
			sb.WriteString(h.LimitExpression)
		}
		if h.OffsetExpression != "" {
			sb.WriteString(" offset ")
			sb.WriteString(h.OffsetExpression)
		}
		return sb.String()
	}
	// The RANGE's own flag, not the result variable's cardinality: the two
	// disagree in Mendix's own models (ConstantRange{SingleObject:false} against
	// an ObjectType variable), and printing the variable's would describe
	// Studio Pro's "All" as `first`.
	if microflows.RangeSingleObjectOf(h) {
		return " first"
	}
	if h.SingleObject {
		return ""
	}
	return " all"
}

// formatExportXmlAction formats an export mapping action as MDL.
// Syntax: $Var = EXPORT TO MAPPING Module.EMM($SourceVar);
func formatExportXmlAction(ctx *ExecContext, a *microflows.ExportXmlAction) string {
	var sb strings.Builder

	// Output variable
	if a.OutputVariable != "" {
		sb.WriteString("$")
		sb.WriteString(a.OutputVariable)
		sb.WriteString(" = ")
	}

	sb.WriteString("export to mapping ")

	mappingName := ""
	paramVar := ""
	if a.RequestHandling != nil {
		mappingName = string(a.RequestHandling.MappingID)
		paramVar = a.RequestHandling.ParameterVariable
	}

	sb.WriteString(mappingName)
	if paramVar != "" {
		sb.WriteString("($")
		sb.WriteString(paramVar)
		sb.WriteString(")")
	}
	sb.WriteString(";")

	return sb.String()
}

// formatTransformJsonAction formats a TRANSFORM JSON action as MDL.
// Syntax: $Result = TRANSFORM $Input WITH Module.Transformer;
func formatTransformJsonAction(a *microflows.TransformJsonAction) string {
	var sb strings.Builder
	if a.OutputVariableName != "" {
		sb.WriteString("$")
		sb.WriteString(a.OutputVariableName)
		sb.WriteString(" = ")
	}
	sb.WriteString("transform $")
	sb.WriteString(a.InputVariableName)
	sb.WriteString(" with ")
	sb.WriteString(a.Transformation)
	sb.WriteString(";")
	return sb.String()
}

func parseReverseAssociationRetrieve(
	ctx *ExecContext,
	source *microflows.DatabaseRetrieveSource,
	entityName string,
) (string, string, bool) {
	if ctx == nil || ctx.Backend == nil || source == nil || entityName == "" {
		return "", "", false
	}
	if len(source.Sorting) > 0 || !isRangeAllOrNil(source.Range) {
		return "", "", false
	}

	assocName, startVar, ok := parseReverseAssociationXPath(source.XPathConstraint)
	if !ok || !databaseRetrieveMatchesAssociationTarget(ctx, entityName, assocName) {
		return "", "", false
	}
	return startVar, assocName, true
}

func isRangeAllOrNil(r *microflows.Range) bool {
	return r == nil || r.RangeType == "" || r.RangeType == microflows.RangeTypeAll
}

func parseReverseAssociationXPath(raw string) (string, string, bool) {
	parts, ok := splitTopLevelXPathPredicates(raw)
	if !ok || len(parts) != 1 {
		return "", "", false
	}

	condition := strings.TrimSpace(parts[0])
	if strings.ContainsAny(condition, "<>!") || strings.Count(condition, "=") != 1 {
		return "", "", false
	}

	sides := strings.SplitN(condition, "=", 2)
	assocName := strings.TrimSpace(sides[0])
	startVar := strings.TrimSpace(sides[1])
	if !isQualifiedAssociationName(assocName) || !strings.HasPrefix(startVar, "$") {
		return "", "", false
	}

	startVar = strings.TrimPrefix(startVar, "$")
	if !isSimpleMendixName(startVar) {
		return "", "", false
	}
	return assocName, startVar, true
}

func isQualifiedAssociationName(name string) bool {
	parts := strings.Split(name, ".")
	return len(parts) == 2 && isSimpleMendixName(parts[0]) && isSimpleMendixName(parts[1])
}

func isSimpleMendixName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
				continue
			}
			return false
		}
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func databaseRetrieveMatchesAssociationTarget(ctx *ExecContext, entityName, assocQualifiedName string) bool {
	moduleName, assocName, ok := strings.Cut(assocQualifiedName, ".")
	if !ok {
		return false
	}

	mod, err := ctx.Backend.GetModuleByName(moduleName)
	if err != nil || mod == nil {
		return false
	}
	dm, err := ctx.Backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return false
	}

	entityNames := make(map[model.ID]string, len(dm.Entities))
	for _, entity := range dm.Entities {
		entityNames[entity.ID] = moduleName + "." + entity.Name
	}
	for _, assoc := range dm.Associations {
		if assoc.Name == assocName {
			return entityNames[assoc.ParentID] == entityName
		}
	}
	return false
}

func splitTopLevelXPathPredicates(raw string) ([]string, bool) {
	var parts []string
	input := strings.TrimSpace(raw)
	if input == "" {
		return nil, false
	}

	i := 0
	for i < len(input) {
		for i < len(input) && (input[i] == ' ' || input[i] == '\t' || input[i] == '\r' || input[i] == '\n') {
			i++
		}
		if i >= len(input) {
			break
		}
		if input[i] != '[' {
			return nil, false
		}

		start := i + 1
		depth := 1
		var quote byte
		for i = start; i < len(input); i++ {
			ch := input[i]
			if quote != 0 {
				if ch == quote {
					quote = 0
				}
				continue
			}
			switch ch {
			case '\'', '"':
				quote = ch
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					part := strings.TrimSpace(input[start:i])
					parts = append(parts, part)
					i++
					goto nextPredicate
				}
			}
		}
		return nil, false

	nextPredicate:
	}

	if len(parts) == 0 {
		return nil, false
	}

	return parts, true
}

// --- Executor method wrappers for callers in unmigrated code and tests ---

func (e *Executor) formatActivity(obj microflows.MicroflowObject, entityNames map[model.ID]string, microflowNames map[model.ID]string) string {
	return formatActivity(e.newExecContext(context.Background()), obj, entityNames, microflowNames)
}

func (e *Executor) formatAction(action microflows.MicroflowAction, entityNames map[model.ID]string, microflowNames map[model.ID]string) string {
	return formatAction(e.newExecContext(context.Background()), action, entityNames, microflowNames)
}

func (e *Executor) formatListOperation(op microflows.ListOperation, outputVar string) string {
	return formatListOperation(e.newExecContext(context.Background()), op, outputVar)
}

// formatSendEmailAction renders a Send Email activity as `send email ( … )`:
// the dialog's settings as one property list (ADR-0013), one per line with a
// trailing comma, in the order the visitor documents them. Platform defaults
// (SecurityType tls, ConnectionTimeout 20000, CheckServerIdentity false, no
// authentication) and empty optional settings are omitted, so a statement that
// does not state them builds the same action (R12).
func formatSendEmailAction(ctx *ExecContext, a *microflows.SendEmailAction) string {
	var settings []string
	set := func(key, value string) { settings = append(settings, key+": "+value) }
	expr := func(e string) string {
		if d := describeExpr(ctx, e); d != "" {
			return d
		}
		return "''"
	}
	params := func(t microflows.EmailTemplate) string {
		if len(t.Parameters) == 0 {
			return ""
		}
		ps := make([]string, 0, len(t.Parameters))
		for i, p := range t.Parameters {
			ps = append(ps, fmt.Sprintf("{%d} = %s", i+1, expr(p)))
		}
		return " with (" + strings.Join(ps, ", ") + ")"
	}
	present := func(t microflows.EmailTemplate) bool { return t.Text != "" || len(t.Parameters) > 0 }

	set("From", expr(a.From))
	if a.To != "" {
		set("To", describeExpr(ctx, a.To))
	}
	if a.Cc != "" {
		set("Cc", describeExpr(ctx, a.Cc))
	}
	if a.Bcc != "" {
		set("Bcc", describeExpr(ctx, a.Bcc))
	}
	if present(a.Subject) {
		set("Subject", mdlQuote(ctx, a.Subject.Text)+params(a.Subject))
	}
	if present(a.BodyPlainText) {
		set("Body", "template "+mdlQuote(ctx, a.BodyPlainText.Text)+params(a.BodyPlainText))
	}
	if present(a.BodyHTML) {
		set("HtmlBody", "template "+mdlQuote(ctx, a.BodyHTML.Text)+params(a.BodyHTML))
	}
	if len(a.CustomHeaders) > 0 {
		hdrs := make([]string, 0, len(a.CustomHeaders))
		for _, h := range a.CustomHeaders {
			hdrs = append(hdrs, mdlQuote(ctx, h.Name)+": "+mdlQuote(ctx, h.Value))
		}
		set("Headers", "("+strings.Join(hdrs, ", ")+")")
	}
	if a.Attachment != "" {
		set("Attachment", "$"+strings.TrimPrefix(a.Attachment, "$"))
	}
	set("Host", expr(a.Host))
	set("Port", expr(a.Port))
	if a.SecurityType != "" && a.SecurityType != microflows.EmailSecurityTLS {
		set("SecurityType", strings.ToLower(string(a.SecurityType)))
	}
	if a.CheckServerIdentity {
		set("CheckServerIdentity", "true")
	}
	if a.ConnectionTimeout != 0 && a.ConnectionTimeout != microflows.DefaultEmailConnectionTimeout {
		set("ConnectionTimeout", fmt.Sprintf("%d", a.ConnectionTimeout))
	}
	if a.UseAuthentication {
		set("Authentication", "basic (Username: "+expr(a.Username)+", Password: "+expr(a.Password)+")")
	}

	var sb strings.Builder
	sb.WriteString("send email (")
	for _, s := range settings {
		sb.WriteString("\n    ")
		sb.WriteString(s)
		sb.WriteString(",")
	}
	sb.WriteString("\n  );")
	return sb.String()
}

func (e *Executor) formatRestCallAction(a *microflows.RestCallAction) string {
	return formatRestCallAction(e.newExecContext(context.Background()), a)
}

// formatSplitCondition renders an ExclusiveSplit's condition as an MDL expression.
// ExpressionSplitCondition is emitted verbatim. RuleSplitCondition is rendered as
// a rule call expression using the rule's qualified name — Mendix expressions
// allow calling a rule the same way as a microflow, so this is re-parseable.
// Unknown or nil conditions fall back to "true" so the describer still produces
// valid MDL; callers should rely on the original Caption (emitted via @caption)
// to preserve human-readable intent.
func formatSplitCondition(ctx *ExecContext, cond microflows.SplitCondition) string {
	switch c := cond.(type) {
	case *microflows.ExpressionSplitCondition:
		expr := describeExpr(ctx, c.Expression)
		if expr == "" {
			return "true"
		}
		return expr
	case *microflows.RuleSplitCondition:
		name := c.RuleQualifiedName
		if name == "" {
			return "true"
		}
		args := make([]string, 0, len(c.ParameterMappings))
		for _, pm := range c.ParameterMappings {
			paramName := pm.ParameterName
			if idx := strings.LastIndex(paramName, "."); idx >= 0 {
				paramName = paramName[idx+1:]
			}
			arg := describeExpr(ctx, pm.Argument)
			if paramName != "" {
				args = append(args, fmt.Sprintf("%s = %s", paramName, arg))
			} else {
				args = append(args, arg)
			}
		}
		return fmt.Sprintf("%s(%s)", name, strings.Join(args, ", "))
	default:
		return "true"
	}
}

func canonicalRawBSON(raw []byte) []byte {
	// RawBSON is preserved byte-for-byte when written back to disk. This
	// canonical form only stabilizes the MDL text emitted by describe, so BSON
	// documents with equivalent key/value content do not drift due to map or
	// parser ordering.
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	canonical, err := bson.Marshal(canonicalBSONValue(doc))
	if err != nil {
		return raw
	}
	return canonical
}

func canonicalBSONValue(value any) any {
	switch v := value.(type) {
	case bson.D:
		return canonicalBSONDocument(v)
	case bson.A:
		out := make(bson.A, len(v))
		for i, item := range v {
			out[i] = canonicalBSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = canonicalBSONValue(item)
		}
		return out
	case bson.M:
		return canonicalBSONMap(map[string]any(v))
	case map[string]any:
		return canonicalBSONMap(v)
	default:
		return value
	}
}

func canonicalBSONDocument(doc bson.D) bson.D {
	out := make(bson.D, len(doc))
	copy(out, doc)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	for i := range out {
		out[i].Value = canonicalBSONValue(out[i].Value)
	}
	return out
}

func canonicalBSONMap(m map[string]any) bson.D {
	doc := make(bson.D, 0, len(m))
	for k, v := range m {
		doc = append(doc, bson.E{Key: k, Value: canonicalBSONValue(v)})
	}
	return canonicalBSONDocument(doc)
}

// shortSortAttribute returns the bare attribute name for a sort on the
// retrieved entity's own (or inherited) attribute, and the qualified name
// otherwise.
//
// A fully qualified `sort by M.Order.Name` is the default spelling of what
// `sort by Name` already means (R12, #748): the builder qualifies a bare name
// with the entity that DECLARES it, walking the generalization chain. So the
// short form is printed exactly when that same walk, from the retrieved
// entity, lands on the stored reference — then re-executing it writes the
// same AttributeQualifiedName. Anything the walk does not confirm (no project,
// a system member, an attribute of another entity) keeps the qualified name.
func shortSortAttribute(ctx *ExecContext, entityQN, attrQN string) string {
	if !ctx.Connected() || entityQN == "" {
		return attrQN
	}
	dot := strings.LastIndex(attrQN, ".")
	if dot <= 0 || strings.Count(attrQN, ".") != 2 {
		return attrQN
	}
	bare := attrQN[dot+1:]
	fb := &flowBuilder{backend: ctx.Backend}
	if declared, ok := fb.resolveAttributeInEntityHierarchy(entityQN, bare); ok && declared == attrQN {
		return bare
	}
	return attrQN
}

// xpathHasNestedPredicate reports whether an XPath constraint, without its
// outer brackets, holds a `[` outside a string literal — a predicate on a path
// step, which only the bracketed `where [ … ]` form can state.
func xpathHasNestedPredicate(xpath string) bool {
	for i := 0; i < len(xpath); i++ {
		switch xpath[i] {
		case '\'':
			// Skip the literal; a doubled quote is an escaped one.
			for i++; i < len(xpath); i++ {
				if xpath[i] == '\'' {
					if i+1 < len(xpath) && xpath[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
		case '[':
			return true
		}
	}
	return false
}

// enrichXPathConstraintForDescribe enriches the raw BSON XPathConstraint string for
// DESCRIBE output. String-literal comparisons against enum attributes are replaced with
// qualified enum value references (e.g. Status = 'Open' → Status = Module.OrderStatus.Open).
// Falls back to the original string on any parse failure.
func enrichXPathConstraintForDescribe(ctx *ExecContext, entityQN, constraint string) string {
	if entityQN == "" || constraint == "" {
		return constraint
	}
	enumAttrs := buildEntityEnumAttrMap(ctx, entityQN)
	if len(enumAttrs) == 0 {
		return constraint
	}
	return enrichXPathGroups(constraint, enumAttrs)
}

// enrichXPathGroups applies enum enrichment to every top-level predicate group of a
// stored constraint.
//
// Mendix stores sibling groups concatenated (`[a][b][c]`), but the grammar rule
// matches one group. Enriching the whole string at once parsed only the first and
// re-rendered just that, silently discarding the rest — describe then showed a
// materially less restrictive query than the project actually contains
// (mendixlabs/mxcli#772). ParseXPathConstraint now refuses that partial parse, which
// stops the data loss on its own; splitting first is what keeps enrichment working
// for every group rather than only the first.
func enrichXPathGroups(constraint string, enumAttrs map[string]string) string {
	groups := visitor.SplitXPathPredicateGroups(constraint)
	if len(groups) == 0 {
		return constraint
	}
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, enrichXPathGroup(g, enumAttrs))
	}
	return strings.Join(out, "")
}

// enrichXPathGroup enriches one bracket group, returning it unchanged when it does
// not parse — a group mxcli cannot read is passed through verbatim rather than
// dropped or guessed at.
func enrichXPathGroup(group string, enumAttrs map[string]string) string {
	expr, ok := visitor.ParseXPathConstraint(group)
	if !ok || expr == nil {
		return group
	}
	return "[" + xpathExprToMDLString(enrichXPathExprWithEnums(expr, enumAttrs)) + "]"
}

// queueClauseMDL renders the `IN QUEUE Module.Name` clause of a queued call, or
// "" for an unqueued one. A DESCRIBE that omitted it would emit a script whose
// re-execution silently unqueues the call.
func queueClauseMDL(qs *microflows.QueueSettings) string {
	if qs == nil || qs.Queue == "" {
		return ""
	}
	return " in queue " + qs.Queue
}

// mdlAggregateKeyword maps a Mendix aggregate function to the MDL keyword that
// parses back to it, reporting false for one this build cannot express.
//
// DESCRIBE used to render an aggregate by lowercasing whatever was stored,
// which silently assumed every value of Mendix's enumeration was also an MDL
// keyword. It was not: Reduce, All and Any described as MDL that the grammar
// then rejected (#1004). Mapping explicitly means a function added to Mendix
// later fails TestAggregateKeywordsCoverEveryFunction rather than a user's
// script.
func mdlAggregateKeyword(fn microflows.AggregateFunction) (string, bool) {
	switch fn {
	case microflows.AggregateFunctionCount, "":
		// Count is also the fallback for an activity with no function stored,
		// which is what Mendix defaults a fresh aggregate to.
		return "count", true
	case microflows.AggregateFunctionSum:
		return "sum", true
	case microflows.AggregateFunctionAverage:
		return "average", true
	case microflows.AggregateFunctionMin:
		return "minimum", true
	case microflows.AggregateFunctionMax:
		return "maximum", true
	case microflows.AggregateFunctionReduce:
		return "reduce", true
	case microflows.AggregateFunctionAll:
		return "all", true
	case microflows.AggregateFunctionAny:
		return "any", true
	default:
		return "", false
	}
}

// describeTypedTemplate renders a Java action's text-template argument
// (ako/mxcli#1451): the text, with its arguments as ` with ({1} = …)`. The
// template '{1}' holding one expression is that expression — what the builder
// writes for one — unless the expression is itself a string literal, which
// would read back as the template's text.
func describeTypedTemplate(ctx *ExecContext, tt *microflows.TypedTemplate) string {
	if tt == nil {
		return "''"
	}
	args := make([]string, 0, len(tt.Arguments))
	for _, a := range tt.Arguments {
		if a != nil {
			args = append(args, a.Expression)
		}
	}
	if tt.Text == "{1}" && len(args) == 1 {
		if _, isText := singleStringLiteral(args[0]); !isText {
			return describeExpr(ctx, args[0])
		}
	}
	return mdlQuote(ctx, tt.Text) + templateArgsClause(ctx, args)
}

// templateArgsClause renders a text template's arguments as ` with ({1} = a,
// {2} = b)` — R4's one text-template form (ako/mxcli#751) — or "" when there
// are none.
func templateArgsClause(ctx *ExecContext, args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprintf("{%d} = %s", i+1, describeExpr(ctx, a))
	}
	return " with (" + strings.Join(parts, ", ") + ")"
}

// workflowSelectionMDL spells a lock/unlock's workflow and its instances flag.
// The flag is Studio Pro's "Pause instances" / "Unpause instances" on the
// selected workflow, and was printed as a bare `all` in place of the workflow
// — a statement that dropped the selection and rebuilt as CE1825
// (mendixlabs/mxcli#870). A stored activity with the flag and no selection
// still prints as `all`, which check and exec refuse: it has no buildable form.
func workflowSelectionMDL(workflow, variable string, flag bool, flagWords string) string {
	var sel string
	switch {
	case workflow != "":
		sel = workflow
	case variable != "":
		sel = "$" + variable
	default:
		return "all"
	}
	if flag {
		sel += " " + flagWords
	}
	return sel
}
