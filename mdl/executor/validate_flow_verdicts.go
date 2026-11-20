// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// FlowRebuildRule is the rule check reports a `create or modify` of a stored
// flow under when its change cannot be spliced in: an error under mdl 1, where
// exec refuses the statement, and a warning under mdl 0, where exec rebuilds.
const FlowRebuildRule = "MDL-V1-REBUILD"

// alterFlowRefusedRule is check's report of an `alter microflow|nanoflow` exec
// would refuse, under every language version: alter is a patch and never
// falls back to a rebuild.
const alterFlowRefusedRule = "MDL090"

// CheckFlowVerdicts reports, for every `create or modify` and `alter` of a
// microflow or nanoflow the connected project holds, what exec would refuse
// (ako/mxcli#876). It runs the verdict exec acts on (decideFlowModify, and
// alterFlowContext.plan for an alter), so check, diff and exec cannot disagree:
//
//   - a change the splice cannot make is an MDL-V1-REBUILD error with exec's
//     own message under mdl 1, and the MDL-V1-REBUILD warning exec prints under
//     mdl 0, where exec rebuilds the flow instead;
//   - an alter exec would refuse is an MDL090 error, under every version.
//
// The prediction is made against the project as stored, overlaid with the
// flows earlier statements of the script declare, so a statement is only
// predicted where that is what exec will see when it gets there:
//
//   - an earlier statement of the script that creates, alters, renames or
//     moves the same flow leaves it unpredicted, and so does one that renames
//     its module or moves a folder out of or into it;
//   - the declared flow (or each fragment of an alter) must build. A call to a
//     flow an earlier `create [or modify] microflow|nanoflow` declares resolves
//     to that declaration (the stored project does not hold it yet); one that
//     still does not build names something else an earlier statement creates,
//     which exec builds after that statement has run.
//
// Neither is silent: each unpredicted change is in the report's Unpredicted,
// with what it depends on, so check does not claim more than it looked at.
//
// It writes nothing: the patch is applied in memory and discarded.
func (e *Executor) CheckFlowVerdicts(prog *ast.Program) []linter.Violation {
	return e.CheckFlowVerdictsReport(prog).Violations
}

// FlowVerdictReport is CheckFlowVerdicts' answer: what exec would refuse, and
// which flow changes could not be predicted.
type FlowVerdictReport struct {
	Violations  []linter.Violation
	Unpredicted []UnpredictedFlow
}

// UnpredictedFlow is a `create or modify` or `alter` of a stored flow whose
// verdict check cannot work out before the script runs; exec decides it when
// it gets there.
type UnpredictedFlow struct {
	Kind   string // "microflow" or "nanoflow"
	Name   string // Module.Name
	Reason string // what it depends on
}

// CheckFlowVerdictsReport is CheckFlowVerdicts with the unpredicted changes.
func (e *Executor) CheckFlowVerdictsReport(prog *ast.Program) FlowVerdictReport {
	if e == nil || prog == nil {
		return FlowVerdictReport{}
	}
	defer e.enterLanguage(prog.LanguageVersion)()
	defer e.CacheUnitReads()()
	return checkFlowVerdicts(e.newExecContext(context.Background()), prog)
}

// CheckFlowVerdicts is the ExecContext-level entry point. A disconnected
// context returns nothing: the verdict is about what the project holds.
func CheckFlowVerdicts(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	return checkFlowVerdicts(ctx, prog).Violations
}

func checkFlowVerdicts(ctx *ExecContext, prog *ast.Program) FlowVerdictReport {
	var r FlowVerdictReport
	if ctx == nil || prog == nil || !ctx.Connected() {
		return r
	}
	// The flows the statements so far declare, which the ones after them
	// call: exec runs those first (see ExecContext.scriptFlows).
	saved := ctx.scriptFlows
	ctx.scriptFlows = map[string]*selfFlow{}
	defer func() { ctx.scriptFlows = saved }()
	declare := func(nanoflow bool, qn ast.QualifiedName, rt *ast.MicroflowReturnType) {
		declareScriptFlow(ctx.scriptFlows, nanoflow, qn, rt)
	}
	// A run that can roll back resolves a call against every flow the script
	// declares, not only the earlier ones (mendixlabs/mxcli#1405).
	if ctx.forwardFlowCalls {
		for _, stmt := range prog.Statements {
			switch s := stmt.(type) {
			case *ast.CreateMicroflowStmt:
				declare(false, s.Name, s.ReturnType)
			case *ast.CreateNanoflowStmt:
				declare(true, s.Name, s.ReturnType)
			}
		}
	}
	// Each flow change predicted as spliced is applied to a simulation — held
	// in memory, read back, never written — so a later statement on the same
	// flow is predicted against what exec will see there, not given up on
	// (mendixlabs/mxcli#1517). Discarded when the check ends.
	simulating := false
	if sim, ok := ctx.Backend.(writeSimulator); ok {
		if discard, ok := sim.SimulateWrites(); ok {
			defer discard()
			simulating = true
		}
	}
	// applied says whether a predicted change reached the simulation, after
	// which the flow is predicted as it is there rather than as "touched".
	applied := func(vs []linter.Violation, dep string, apply func() bool) bool {
		return simulating && apply != nil && len(vs) == 0 && dep == "" && apply()
	}
	undeclare := func(qn ast.QualifiedName) {
		delete(ctx.scriptFlows, scriptFlowKey(false, qn.String()))
		delete(ctx.scriptFlows, scriptFlowKey(true, qn.String()))
	}

	// What an earlier statement did to a flow ("microflow:M.N" /
	// "nanoflow:M.N", lower-cased) or to a module (lower-cased).
	touched := map[string]string{}
	touchedModules := map[string]string{}
	key := func(nanoflow bool, qn ast.QualifiedName) string {
		return strings.ToLower(scriptFlowKey(nanoflow, qn.String()))
	}
	// stale says why the stored flow is not what exec will see, "" when it is.
	// A flow dropped (on its own or with its module) is created afresh, which
	// has no verdict to predict.
	stale := func(nanoflow bool, qn ast.QualifiedName) (why string, skip bool) {
		if w := touchedModules[strings.ToLower(qn.Module)]; w != "" {
			return w, w == touchDropped
		}
		if w := touched[key(nanoflow, qn)]; w != "" {
			return w, w == touchDropped
		}
		return "", false
	}
	unpredicted := func(nanoflow bool, qn ast.QualifiedName, reason string) {
		kind := "microflow"
		if nanoflow {
			kind = "nanoflow"
		}
		r.Unpredicted = append(r.Unpredicted, UnpredictedFlow{Kind: kind, Name: qn.String(), Reason: reason})
	}
	// modify predicts a `create or modify`, and reports whether its change
	// reached the simulation.
	modify := func(d *flowDecl, ruled bool) bool {
		why, skip := stale(d.nanoflow, d.name)
		switch {
		case skip:
		case why != "":
			if flowStored(ctx, d.nanoflow, d.name) {
				unpredicted(d.nanoflow, d.name, "an earlier statement of the script "+why)
			}
		case ruled:
			vs, dep, apply := flowModifyViolations(ctx, d)
			r.Violations = append(r.Violations, vs...)
			// A rebuild exec runs instead of the splice (mdl 0) would delete
			// what MDL cannot express; exec refuses it when it gets there,
			// so refuse the script now, before anything is written.
			for _, v := range vs {
				if v.RuleID == FlowRebuildRule && v.Severity != linter.SeverityError {
					if types := storedFlowUnsupportedActions(ctx, d.kind(), d.name.String()); len(types) > 0 {
						r.Violations = append(r.Violations, unsupportedRebuildViolation(d.kind(), d.name, types))
					}
					break
				}
			}
			if dep != "" {
				unpredicted(d.nanoflow, d.name, dep)
			}
			return applied(vs, dep, apply)
		}
		return false
	}

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			if !(s.CreateOrModify && modify(microflowDecl(s), validateMicroflowRules(s) == nil)) {
				touched[key(false, s.Name)] = "creates or changes it"
			}
			declare(false, s.Name, s.ReturnType)
		case *ast.CreateNanoflowStmt:
			if !(s.CreateOrModify && modify(nanoflowDecl(s), true)) {
				touched[key(true, s.Name)] = "creates or changes it"
			}
			declare(true, s.Name, s.ReturnType)
		case *ast.AlterFlowStmt:
			simulated := false
			if why, skip := stale(s.Nanoflow, s.Name); skip {
			} else if why != "" {
				if flowStored(ctx, s.Nanoflow, s.Name) {
					unpredicted(s.Nanoflow, s.Name, "an earlier statement of the script "+why)
				}
			} else {
				vs, dep, apply := alterFlowViolations(ctx, s)
				r.Violations = append(r.Violations, vs...)
				if dep != "" {
					unpredicted(s.Nanoflow, s.Name, dep)
				}
				simulated = applied(vs, dep, apply)
			}
			if !simulated {
				touched[key(s.Nanoflow, s.Name)] = "alters it"
			}
		case *ast.DropMicroflowStmt:
			touched[key(false, s.Name)] = touchDropped
			undeclare(s.Name)
		case *ast.DropNanoflowStmt:
			touched[key(true, s.Name)] = touchDropped
			undeclare(s.Name)
		case *ast.MoveStmt:
			touched[key(false, s.Name)] = "moves it"
			touched[key(true, s.Name)] = "moves it"
			if s.TargetModule != "" {
				moved := ast.QualifiedName{Module: s.TargetModule, Name: s.Name.Name}
				touched[key(false, moved)] = "moves it"
				touched[key(true, moved)] = "moves it"
			}
		case *ast.DropModuleStmt:
			// The module's flows go with it: exec creates the flow afterwards.
			touchedModules[strings.ToLower(s.Name)] = touchDropped
		case *ast.MoveFolderStmt:
			// The folder's flows go with it; which ones is the project's to
			// say, so neither module is predicted from here on.
			touchedModules[strings.ToLower(s.Name.Module)] = "moves a folder out of or into its module"
			if s.TargetModule != "" {
				touchedModules[strings.ToLower(s.TargetModule)] = "moves a folder out of or into its module"
			}
		case *ast.RenameStmt:
			switch strings.ToLower(s.ObjectType) {
			case "module":
				touchedModules[strings.ToLower(s.Name.Module)] = "renames its module"
				touchedModules[strings.ToLower(s.NewName)] = "renames its module"
			default:
				renamed := ast.QualifiedName{Module: s.Name.Module, Name: s.NewName}
				for _, nf := range []bool{false, true} {
					touched[key(nf, s.Name)] = "renames it"
					touched[key(nf, renamed)] = "renames it"
				}
			}
		}
	}
	return r
}

// declareScriptFlow records a flow a script declares, as a call to it resolves.
func declareScriptFlow(flows map[string]*selfFlow, nanoflow bool, qn ast.QualifiedName, rt *ast.MicroflowReturnType) {
	var t microflows.DataType = &microflows.VoidType{}
	if rt != nil {
		t = convertASTToMicroflowDataType(rt.Type, nil)
	}
	flows[scriptFlowKey(nanoflow, qn.String())] = &selfFlow{qualifiedName: qn.String(), nanoflow: nanoflow, returnType: t}
}

// touchDropped is what CheckFlowVerdicts records for a flow or module an
// earlier statement drops: a later `create or modify` of it is a create.
const touchDropped = "drops it"

// flowStored reports whether the project holds the flow, so that a statement
// about it is a modify (with a verdict) rather than a create.
func flowStored(ctx *ExecContext, nanoflow bool, qn ast.QualifiedName) bool {
	if _, err := findModule(ctx, qn.Module); err != nil {
		return false
	}
	_, err := loadAlterFlow(ctx, &ast.AlterFlowStmt{Nanoflow: nanoflow, Name: qn})
	var nf *mdlerrors.NotFoundError
	return !errors.As(err, &nf)
}

// writeSimulator is the optional backend capability check predicts a script's
// flow changes in order with (modelsdk Backend.SimulateWrites).
type writeSimulator interface {
	SimulateWrites() (discard func(), ok bool)
}

// flowModifyViolations is the check-time report of decideFlowModify. dep is
// non-empty when the stored flow's verdict cannot be predicted because the
// declared one does not build: it says what the body depends on. apply, when
// not nil, writes the spliced change the way exec would — into the
// simulation check holds open — and reports whether it did; a change that
// also moves the flow to another folder is not applied, as that is not a unit
// update.
func flowModifyViolations(ctx *ExecContext, d *flowDecl) (_ []linter.Violation, dep string, apply func() bool) {
	if _, _, _, err := d.build(ctx); err != nil {
		// Not predictable against the stored project (see CheckFlowVerdicts);
		// a flow that is not stored is a create, with no verdict to predict.
		if flowStored(ctx, d.nanoflow, d.name) {
			return nil, buildDependency(err.Error()), nil
		}
		return nil, "", nil
	}
	v := decideFlowModify(ctx, d)
	if v.why == nil {
		if p := v.plan; p != nil && v.err == nil && !movesFolder(d.folder, p.storedFolder) {
			apply = func() bool { return p.mut == nil || p.mut.Save() == nil }
		}
		return nil, "", apply
	}
	loc := linter.Location{Module: d.name.Module, DocumentType: d.kind(), DocumentName: d.name.Name}
	if v.refused {
		return []linter.Violation{{
			RuleID:   FlowRebuildRule,
			Severity: linter.SeverityError,
			Message:  "exec would refuse this statement: " + flowRefusal(d, v.why).Error(),
			Location: loc,
		}}, "", nil
	}
	return []linter.Violation{{
		RuleID:   FlowRebuildRule,
		Severity: linter.SeverityWarning,
		Message:  flowRebuildWarning(ctx, d, v.why),
		Location: loc,
	}}, "", nil
}

// buildDependency condenses a build error ("microflow 'M.N' has validation
// errors:\n  - …") to the errors themselves, on one line.
func buildDependency(msg string) string {
	var parts []string
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, "has validation errors:") {
			continue
		}
		parts = append(parts, strings.TrimPrefix(line, "- "))
	}
	return strings.Join(parts, "; ")
}

// alterFlowViolations is the check-time report of an alter exec would refuse.
// apply, when not nil, writes the alter into the simulation check holds open,
// as flowModifyViolations' does.
func alterFlowViolations(ctx *ExecContext, s *ast.AlterFlowStmt) (_ []linter.Violation, dep string, apply func() bool) {
	a, err := loadAlterFlow(ctx, s)
	if err != nil {
		return nil, "", nil // no such flow: reference validation reports it
	}
	refusal := func(err error) []linter.Violation {
		return []linter.Violation{{
			RuleID:   alterFlowRefusedRule,
			Severity: linter.SeverityError,
			Message:  "exec would refuse this statement and write nothing: " + err.Error(),
			Location: linter.Location{Module: s.Name.Module, DocumentType: s.Kind(), DocumentName: s.Name.Name},
		}}
	}
	// A stored note named by a text that names none, or several, is a
	// refusal, not something the script could still declare: resolved before
	// a fragment is built, by the function exec's plan calls (ako/mxcli#1695).
	if err := a.resolveStoredNotes(); err != nil {
		return refusal(err), "", nil
	}
	// A fragment that does not build against the stored project is not
	// predictable (see CheckFlowVerdicts). Only the builder's own errors say
	// so; a fragment that builds and cannot be spliced is a refusal. Each is
	// built knowing what the operations before it declare, as exec builds it
	// (ako/mxcli#1399).
	for _, op := range s.Operations {
		if len(op.Body) == 0 {
			continue
		}
		body := op.Body
		if op.Op == ast.AlterFlowReplaceBody {
			// A loop's new body is built inside the loop restated, as exec
			// builds it (replaceBody): its iterator is typed from the list.
			// A target that is no stored loop is the plan's to refuse.
			c, err := mfmutator.ResolveText(a.cands, op.Target)
			if err != nil {
				continue
			}
			header, err := a.storedLoopStatement(ctx, c)
			if err != nil {
				continue
			}
			body = []ast.MicroflowStatement{withLoopBody(header, op.Body)}
		}
		fb := a.fragmentBuilder(ctx)
		oc := fb.buildFlowGraph(body, nil)
		if errs := fb.GetErrors(); len(errs) > 0 {
			return nil, buildDependency(strings.Join(errs, "\n")), nil
		}
		if oc != nil {
			a.opVars = a.opVars.with(declaredVarsOf(fb, oc.Objects))
		}
	}
	a.opVars = fragmentVars{} // the plan below starts from the stored flow, as exec's does
	mut, err := a.plan(ctx)
	if err == nil {
		return nil, "", func() bool { return mut.Save() == nil }
	}
	var nf *mdlerrors.NotFoundError
	if errors.As(err, &nf) {
		return nil, "", nil
	}
	return refusal(err), "", nil
}

// FormatUnpredictedFlows is the note check and exec's preflight print for
// flow changes whose verdict could not be predicted, one per line:
//
//	N flow change(s) not predicted — exec decides when it reaches them:
//	  microflow M.N — depends on: <what the body needs that is not stored yet>
//
// "" when there are none.
func FormatUnpredictedFlows(us []UnpredictedFlow) string {
	if len(us) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Note: %d flow change(s) not predicted — exec decides when it reaches them, after the statements before have been written:\n", len(us))
	for _, u := range us {
		fmt.Fprintf(&b, "  %s %s — depends on: %s\n", u.Kind, u.Name, u.Reason)
	}
	return b.String()
}
