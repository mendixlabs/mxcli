// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation of definition order within one script.
//
// The executor resolves most cross-document references at the moment it writes
// the referring document, so a statement naming something a LATER statement
// creates fails partway through `exec` — after earlier statements have already
// been written, because `exec` is not transactional. `ValidateProgram` (the
// --references tier) cannot see this: it collects every name in the script up
// front, so a forward reference is indistinguishable from a backward one and
// the script is reported clean.
//
// `exec` already computes the diagnosis — annotateForwardRef appends
// "… is defined later in this script — move its create statement before this
// one" — but only once a statement has failed, which is one write too late.
// This pass says the same thing before anything is written. (issue #955)
//
// ValidateScriptPageOrder is the same idea for a widget's page reference and
// came first; this covers the other reference kinds. The two are kept separate
// because a page reference is found by walking a widget tree rather than a
// statement's own fields, and MDL-PAGE01 is already an established code.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Document kinds a forward reference can denote. These are namespaces for the
// purposes of this rule, not Mendix metamodel types.
const (
	defEntity      = "entity"
	defEnumeration = "enumeration"
	defMicroflow   = "microflow"
	defNanoflow    = "nanoflow"
	defPage        = "page"
	defQueue       = "task queue"
	// defImportMapping is the target of `import from mapping`, which the flow
	// builder resolves when it writes the activity — it needs the mapping's
	// root entity for the result type (mendixlabs/mxcli#1418).
	defImportMapping = "import mapping"
)

// defKey identifies a document by kind and qualified name. Kind is part of the
// key because Mendix namespaces documents by type — an entity and a microflow
// may share a qualified name without colliding.
type defKey struct{ kind, name string }

// defRef is one reference the executor resolves EAGERLY, i.e. at the moment it
// writes the referring document.
//
// kinds lists the document kinds the name could denote. It is usually one, but
// a bare `Module.Name` written as a type is genuinely ambiguous: the MDL visitor
// records it as TypeEnumeration with EnumRef set whether the author meant an
// entity or an enumeration (see "TypeEnumeration vs TypeEntity Ambiguity" in
// CLAUDE.md), so both are accepted and whichever the script defines is the one
// that matters.
type defRef struct {
	name  string
	kinds []string
	site  string // where the reference sits, for the message
	// flowCall marks a CALL MICROFLOW / CALL NANOFLOW target. The call is
	// stored by name, so a run that rolls back on failure resolves it against
	// the whole script (mendixlabs/mxcli#1405); see flowCallRefs.
	flowCall bool
	// byName marks a reference stored by name that is only resolved by looking
	// it up when the statement runs: a navigation profile's or menu document's
	// page, microflow or nanoflow. Like a flow call, one the script creates
	// further down is accepted by a run that rolls back (forwardFlowCalls) and
	// refused by one that cannot; unlike a call, it is not a call, so
	// forwardByNameRefusal words the refusal.
	byName bool
}

// forwardByNameRefusal is why a run that cannot roll back refuses a navigation
// target the script creates further down.
const forwardByNameRefusal = "the target is stored by name and looked up when the statement runs, and this run cannot be rolled back (--continue-on-error, the REPL, -c), so a later failure would leave it dangling. " +
	"Create the target first; plain `mxcli exec`, which rolls back, accepts the script as it is"

// flowCallRefs marks refs as flow-call targets.
func flowCallRefs(refs []defRef) []defRef {
	for i := range refs {
		refs[i].flowCall = true
	}
	return refs
}

// forwardFlowCallRefusal is why a run that cannot roll back refuses a call to
// a flow created further down the script.
const forwardFlowCallRefusal = "a call to a flow the script creates further down is written before its callee, " +
	"and this run cannot be rolled back (--continue-on-error, the REPL, -c), so a later failure would leave the call dangling. " +
	"Create the callee first, or create a stub of it first and `create or modify` it after the caller; " +
	"plain `mxcli exec`, which rolls back, accepts the script as it is"

// ValidateScriptDefinitionOrder flags (MDL-ORDER01) a statement that references
// a document created LATER in the same script by a plain CREATE.
//
// The "plain CREATE" condition is what makes this sound without a project, and
// is the same argument ValidateScriptPageOrder rests on: a plain CREATE fails if
// the document already exists, so a script containing one asserts the document
// does not exist yet — the earlier reference therefore cannot resolve against
// the project either. `CREATE OR MODIFY` / `CREATE OR REPLACE` carry no such
// assertion (the document may well already be in the project, in which case the
// forward reference resolves fine), so those are left alone here.
//
// Only reference kinds MEASURED to fail at exec are included. Several plausible
// ones do NOT fail and are deliberately absent, because flagging them would
// reject scripts that work today:
//
//   - an entity's EXTENDS generalization
//   - CALL JAVA ACTION / CALL JAVASCRIPT ACTION
//   - RETRIEVE … FROM an entity
//   - CREATE <entity> inside a microflow body
//
// A CALL MICROFLOW / CALL NANOFLOW target is not judged here either: whether
// it may run ahead of its callee depends on whether the run can roll back
// (mendixlabs/mxcli#1405), which validateForwardDefRefs and exec know.
//
// The matrix behind that split is in mdl-examples/bug-tests/955-*.mdl.
func ValidateScriptDefinitionOrder(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	plain := plainCreateIndex(prog)
	if len(plain) == 0 {
		return nil
	}

	created := map[defKey]bool{}
	var out []linter.Violation
	for i, stmt := range prog.Statements {
		for _, ref := range eagerDefRefs(stmt) {
			// Whether a flow call may run ahead of its callee depends on how
			// the script runs, which this tier cannot know: the project tier
			// (validateForwardDefRefs) and exec decide (mendixlabs/mxcli#1405).
			if ref.flowCall || ref.byName {
				continue
			}
			kind, at, ok := laterPlainCreate(plain, created, ref)
			// at == i is a document referring to itself (a recursive microflow
			// call, an entity whose attribute names its own enumeration); the
			// executor has the document in hand by then, so it is not an
			// ordering problem.
			if !ok || at <= i {
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-ORDER01",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s references %s %s before it is created — the executor resolves this in statement order, so it fails partway through `exec` with earlier statements already written",
					ref.site, kind, ref.name),
				Suggestion: fmt.Sprintf(
					"Move the CREATE statement for %s above this one.", ref.name),
			})
		}
		markCreated(created, stmt)
	}
	return out
}

// plainCreateIndex maps each document a plain CREATE defines to the index of
// that statement. Only the FIRST plain CREATE of a name is recorded: a second
// one is a duplicate definition, which CheckScriptDuplicates reports.
func plainCreateIndex(prog *ast.Program) map[defKey]int {
	out := map[defKey]int{}
	for i, stmt := range prog.Statements {
		key, plain := definedBy(stmt)
		if key.name == "" || !plain {
			continue
		}
		if _, seen := out[key]; !seen {
			out[key] = i
		}
	}
	return out
}

// markCreated records a document as existing from this statement onward. Unlike
// plainCreateIndex this counts EVERY create form: a `CREATE OR MODIFY` earlier
// in the script has still put the document in the project by the time a later
// statement refers to it.
func markCreated(created map[defKey]bool, stmt ast.Statement) {
	if key, _ := definedBy(stmt); key.name != "" {
		created[key] = true
	}
}

// definedBy reports the document a statement creates and whether it does so
// with a plain CREATE (as opposed to CREATE OR MODIFY / OR REPLACE).
func definedBy(stmt ast.Statement) (key defKey, plain bool) {
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		return qualifiedKey(defEntity, s.Name), !s.CreateOrModify
	case *ast.CreateEnumerationStmt:
		return qualifiedKey(defEnumeration, s.Name), !s.CreateOrModify
	case *ast.CreateMicroflowStmt:
		return qualifiedKey(defMicroflow, s.Name), !s.CreateOrModify
	case *ast.CreateNanoflowStmt:
		return qualifiedKey(defNanoflow, s.Name), !s.CreateOrModify
	case *ast.CreatePageStmtV3:
		return qualifiedKey(defPage, s.Name), !s.IsModify && !s.IsReplace
	case *ast.CreateQueueStmt:
		return qualifiedKey(defQueue, s.Name), !s.CreateOrModify
	case *ast.CreateImportMappingStmt:
		return qualifiedKey(defImportMapping, s.Name), !s.CreateOrModify
	}
	return defKey{}, false
}

// qualifiedKey builds a key, or the zero key for an unqualified name (which
// this rule cannot resolve and so ignores).
func qualifiedKey(kind string, name ast.QualifiedName) defKey {
	if name.Module == "" {
		return defKey{}
	}
	return defKey{kind: kind, name: name.String()}
}

// laterPlainCreate reports the kind and statement index of a plain CREATE that
// defines ref, when the name has not already been created earlier in the script.
func laterPlainCreate(plain map[defKey]int, created map[defKey]bool, ref defRef) (kind string, at int, ok bool) {
	for _, k := range ref.kinds {
		key := defKey{kind: k, name: ref.name}
		if created[key] {
			return "", 0, false // exists by now; nothing to report for any kind
		}
	}
	for _, k := range ref.kinds {
		if idx, found := plain[defKey{kind: k, name: ref.name}]; found {
			return k, idx, true
		}
	}
	return "", 0, false
}

// eagerDefRefs returns the references a statement resolves at write time.
func eagerDefRefs(stmt ast.Statement) []defRef {
	switch s := stmt.(type) {
	case *ast.CreateMicroflowStmt:
		return flowDefRefs("microflow "+s.Name.String(), s.Parameters, s.ReturnType, s.Body)
	case *ast.CreateNanoflowStmt:
		return flowDefRefs("nanoflow "+s.Name.String(), s.Parameters, s.ReturnType, s.Body)
	case *ast.CreateRuleStmt:
		return flowDefRefs("rule "+s.Name.String(), s.Parameters, s.ReturnType, s.Body)

	case *ast.CreateEntityStmt:
		// Attribute types only. An entity's EXTENDS generalization is resolved
		// lazily and a forward one executes correctly (measured), so it is not
		// an ordering error.
		site := "entity " + s.Name.String()
		var out []defRef
		for _, attr := range s.Attributes {
			out = append(out, typeDefRefs(attr.Type, fmt.Sprintf("%s: attribute %q", site, attr.Name))...)
		}
		return out

	case *ast.CreateAssociationStmt:
		site := "association " + s.Name.String()
		var out []defRef
		for _, end := range []ast.QualifiedName{s.Parent, s.Child} {
			if end.Module != "" {
				out = append(out, defRef{name: end.String(), kinds: []string{defEntity}, site: site})
			}
		}
		return out

	case *ast.CreatePageStmtV3:
		if s.Excluded {
			// An excluded page may name a flow the project lacks; the builder
			// writes it by name (danglingRefOK), so the order does not fail.
			return nil
		}
		return widgetFlowDefRefs("page "+s.Name.String(), allPageWidgets(s))
	case *ast.CreateSnippetStmtV3:
		return widgetFlowDefRefs("snippet "+s.Name.String(), s.Widgets)

	case *ast.GrantEntityAccessStmt:
		return oneDefRef(s.Entity, defEntity, "grant on entity "+s.Entity.String())
	case *ast.GrantMicroflowAccessStmt:
		return oneDefRef(s.Microflow, defMicroflow, "grant execute on microflow "+s.Microflow.String())
	case *ast.GrantNanoflowAccessStmt:
		return oneDefRef(s.Nanoflow, defNanoflow, "grant execute on nanoflow "+s.Nanoflow.String())
	case *ast.GrantPageAccessStmt:
		return oneDefRef(s.Page, defPage, "grant view on page "+s.Page.String())

	case *ast.AlterNavigationStmt, *ast.CreateMenuStmt:
		// The home, login and not-found pages and every menu item's target,
		// which the handler resolves before it writes the profile or menu.
		return navigationDefRefs(stmt)
	}
	return nil
}

func oneDefRef(name ast.QualifiedName, kind, site string) []defRef {
	if name.Module == "" {
		return nil
	}
	return []defRef{{name: name.String(), kinds: []string{kind}, site: site}}
}

// flowDefRefs collects the eager references of a microflow, nanoflow or rule:
// its parameter types, its return type, and the flows its body calls.
func flowDefRefs(site string, params []ast.MicroflowParam, ret *ast.MicroflowReturnType, body []ast.MicroflowStatement) []defRef {
	var out []defRef
	for _, p := range params {
		out = append(out, typeDefRefs(p.Type, fmt.Sprintf("%s: parameter %q", site, p.Name))...)
	}
	if ret != nil {
		out = append(out, typeDefRefs(ret.Type, site+": return type")...)
	}
	for _, stmt := range flattenFlowBody(body) {
		switch c := stmt.(type) {
		case *ast.CallMicroflowStmt:
			out = append(out, flowCallRefs(oneDefRef(c.MicroflowName, defMicroflow, site))...)
			out = append(out, queueDefRef(c.Queue, site)...)
		case *ast.CallNanoflowStmt:
			out = append(out, flowCallRefs(oneDefRef(c.NanoflowName, defNanoflow, site))...)
		case *ast.CallJavaActionStmt:
			// The action itself resolves lazily (measured, #955); its IN QUEUE
			// does not — buildQueueSettings refuses a queue it cannot find.
			out = append(out, queueDefRef(c.Queue, site)...)
		case *ast.ImportFromMappingStmt:
			// Resolved for its root entity, which the activity stores as its
			// result type; exec refuses one it cannot find (#1418).
			out = append(out, oneDefRef(c.Mapping, defImportMapping, site+": import from mapping")...)
		}
	}
	return out
}

// queueDefRef is an `IN QUEUE` target, which the flow builder resolves when it
// writes the call (buildQueueSettings) — so a queue created further down the
// script fails the flow (mendixlabs/mxcli#1211).
func queueDefRef(q *ast.QualifiedName, site string) []defRef {
	if q == nil {
		return nil
	}
	return oneDefRef(*q, defQueue, site+": in queue")
}

// widgetFlowDefRefs is the microflows and nanoflows a page or snippet's widgets
// name — action targets and data sources. The page builder resolves each when
// it writes the page, so one created further down the script stops exec at the
// page, after everything before it was written (mendixlabs/mxcli#1212).
func widgetFlowDefRefs(site string, widgets []*ast.WidgetV3) []defRef {
	refs := &widgetRefCollector{}
	refs.collectFromWidgets(widgets)
	refs.dedupe()
	var out []defRef
	for _, mf := range refs.microflows {
		if mf = unquoteQualifiedName(mf); strings.Contains(mf, ".") {
			out = append(out, defRef{name: mf, kinds: []string{defMicroflow}, site: site})
		}
	}
	for _, nf := range refs.nanoflows {
		if nf = unquoteQualifiedName(nf); strings.Contains(nf, ".") {
			out = append(out, defRef{name: nf, kinds: []string{defNanoflow}, site: site})
		}
	}
	return out
}

// typeDefRefs turns a data type into the document it names, if any.
//
// Both EntityRef and EnumRef are read regardless of Kind: a bare qualified name
// lands in EnumRef even when it denotes an entity, and `List of Module.Entity`
// lands in EntityRef, so keying off Kind alone would miss one or the other.
func typeDefRefs(t ast.DataType, site string) []defRef {
	if t.EntityRef != nil && t.EntityRef.Module != "" {
		return []defRef{{name: t.EntityRef.String(), kinds: []string{defEntity}, site: site}}
	}
	if t.EnumRef != nil && t.EnumRef.Module != "" {
		// `Enumeration(Module.Name)` / `ENUM Module.Name` says which it is;
		// a bare `Module.Name` does not, so accept either.
		kinds := []string{defEnumeration, defEntity}
		if t.ExplicitEnum {
			kinds = []string{defEnumeration}
		}
		return []defRef{{name: t.EnumRef.String(), kinds: kinds, site: site}}
	}
	return nil
}

// flattenFlowBody returns every statement in a flow body, descending into the
// containers that hold nested bodies. A call inside a loop or a split is
// resolved exactly like a top-level one, so it has to be reached.
func flattenFlowBody(body []ast.MicroflowStatement) []ast.MicroflowStatement {
	var out []ast.MicroflowStatement
	for _, stmt := range body {
		out = append(out, stmt)
		switch s := stmt.(type) {
		case *ast.LoopStmt:
			out = append(out, flattenFlowBody(s.Body)...)
		case *ast.WhileStmt:
			out = append(out, flattenFlowBody(s.Body)...)
		case *ast.IfStmt:
			out = append(out, flattenFlowBody(s.ThenBody)...)
			out = append(out, flattenFlowBody(s.ElseBody)...)
		case *ast.EnumSplitStmt:
			out = append(out, flattenFlowBody(appendEnumBodies(s))...)
		case *ast.InheritanceSplitStmt:
			for _, branch := range inheritanceBranchBodies(s) {
				out = append(out, flattenFlowBody(branch)...)
			}
		}
		if h := errorHandlerBody(stmt); len(h) > 0 {
			out = append(out, flattenFlowBody(h)...)
		}
	}
	return out
}

// errorHandlerBody returns the custom ON ERROR body of a call activity, which
// is a nested body like any other.
func errorHandlerBody(stmt ast.MicroflowStatement) []ast.MicroflowStatement {
	switch s := stmt.(type) {
	case *ast.CallMicroflowStmt:
		if s.ErrorHandling != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallNanoflowStmt:
		if s.ErrorHandling != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallJavaActionStmt:
		if s.ErrorHandling != nil {
			return s.ErrorHandling.Body
		}
	}
	return nil
}

// validateForwardDefRefs is MDL-ORDER01's project-tier half. MDL-ORDER01 needs
// no project because it only judges a reference whose later definition is a
// PLAIN create — which asserts the document is not in the project yet. A later
// `create or modify` asserts nothing, so whether the earlier reference resolves
// depends on the project: when it has the document, exec resolves against it;
// when it does not, exec fails at the referring statement with the statements
// before it already written, while the whole-script reference pass, which
// counts every name the script defines anywhere, reported it clean.
//
// The references are eagerDefRefs — the same index MDL-ORDER01 walks — so the
// two tiers cannot disagree about what is resolved eagerly.
func validateForwardDefRefs(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	plain := plainCreateIndex(prog)
	anyCreate := map[defKey]int{}
	for i, stmt := range prog.Statements {
		if key, _ := definedBy(stmt); key.name != "" {
			if _, seen := anyCreate[key]; !seen {
				anyCreate[key] = i
			}
		}
	}
	if len(anyCreate) == 0 {
		return nil
	}
	stored := map[string]map[string]bool{}
	storedOf := func(kind string) map[string]bool {
		if m, ok := stored[kind]; ok {
			return m
		}
		var m map[string]bool
		switch kind {
		case defEntity:
			m = buildEntityQualifiedNames(ctx)
		case defEnumeration:
			m = buildEnumerationQualifiedNames(ctx)
		case defMicroflow:
			m = buildMicroflowQualifiedNames(ctx)
		case defNanoflow:
			m = buildNanoflowQualifiedNames(ctx)
		case defPage:
			m = buildPageQualifiedNames(ctx)
		case defQueue:
			m = buildQueueQualifiedNames(ctx)
		case defImportMapping:
			m = buildImportMappingQualifiedNames(ctx)
		}
		stored[kind] = m
		return m
	}
	inProject := func(kind, name string) bool {
		m := storedOf(kind)
		// An empty listing is a backend that could not answer, as often as a
		// project without any — silence, not a finding (cf. checkAlterTarget).
		// Queues and import mappings are the exception: a project with none
		// is ordinary.
		if len(m) == 0 && kind != defQueue && kind != defImportMapping {
			return true
		}
		if kind == defQueue {
			return m[strings.ToLower(name)]
		}
		return m[name]
	}

	created := map[defKey]bool{}
	var errs []error
	for i, stmt := range prog.Statements {
		for _, ref := range eagerDefRefs(stmt) {
			resolved := false
			for _, k := range ref.kinds {
				if created[defKey{kind: k, name: ref.name}] || inProject(k, ref.name) {
					resolved = true
					break
				}
			}
			if resolved || ((ref.flowCall || ref.byName) && ctx.forwardFlowCalls) {
				continue
			}
			for _, k := range ref.kinds {
				key := defKey{kind: k, name: ref.name}
				at, later := anyCreate[key]
				if !later || at <= i {
					continue
				}
				if ref.flowCall {
					errs = append(errs, fmt.Errorf("statement %d: %s calls %s %s before it is created (statement %d): %s",
						i+1, ref.site, k, ref.name, at+1, forwardFlowCallRefusal))
					break
				}
				if ref.byName {
					errs = append(errs, fmt.Errorf("statement %d: %s references %s %s before it is created (statement %d): %s",
						i+1, ref.site, k, ref.name, at+1, forwardByNameRefusal))
					break
				}
				if _, isPlain := plain[key]; isPlain {
					break // MDL-ORDER01 reports it, without a project
				}
				errs = append(errs, fmt.Errorf(
					"statement %d: %s references %s %s before it is created (statement %d), and the project does not have it — exec resolves this in statement order, so it fails here with the statements before it already written. Move the create statement for %s above this one",
					i+1, ref.site, k, ref.name, at+1, ref.name))
				break
			}
		}
		markCreated(created, stmt)
	}
	return errs
}
