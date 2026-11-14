// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// # create or modify microflow|nanoflow as diff-then-patch (plan item 4.2g)
//
// ADR-0012 decision 3: `create or modify` on a flow that exists compares the
// declared definition with the stored document, derives the minimal patch and
// applies it with the splice engine `alter microflow` uses (mfmutator). An
// empty patch writes nothing.
//
// The stored side is the flow as `describe` prints it, parsed back: that is the
// one rendering of a stored flow MDL already guarantees to re-parse, and it
// puts both sides in the same AST, so "the same definition" is a structural
// comparison rather than a second renderer (the #997 lesson). Statements are
// matched with declaredMatches — the whole statement, so its signature and its
// output variable — by a longest common subsequence; each run of unmatched
// statements between two matched ones becomes one insert, replace or drop,
// aimed at the stored activity by its @position and applied through the same
// alterFlowContext an `alter` statement uses. An `if` that differs only inside
// its branches is diffed branch by branch.
//
// Before any statement is matched, the declared body is built and compared
// with the stored graph itself (builtAsStored, flow_built_match.go): the same
// graph is nothing to patch, whichever of MDL's spellings of it describe
// prints (ako/mxcli#859). The statement match, for a body that does change,
// runs on one canonical control-flow form of both sides (flow_canonical.go).
//
// The rest of what describe states is patched too (ako/mxcli#818): a changed
// header or document property is set on the stored document, a parameter is
// added, retyped or removed in place, and a stated @position or @start that
// differs from the stored one moves the stored node — its position only, its
// flows keep their ends and curves (cmd_flow_modify_patch.go).
//
// What the patch cannot express — a decision added or changed inside a loop
// body, a redrawn connector, a reordered parameter — is not rebuilt under
// `mdl 1`: the whole-document rebuild is what reset curves, dropped merges and
// moved element IDs on Studio Pro-authored flows (#721 class A). It is refused
// with the reason instead. Under mdl 0 the rebuild still runs, with the
// MDL-V1-REBUILD warning (ADR-0011: a new refusal applies only under the
// header that opts into it).

// flowRebuildRefused is the language change: the lossy fallback becomes a
// refusal under mdl 1. It is gated in the executor rather than the visitor
// because whether a statement needs the fallback depends on what is stored, so
// no parse of the script can tell.
var flowRebuildRefused = langver.Change{
	Code:  "MDL-V1-REBUILD",
	Since: langver.V1,
	Old: "`create or modify microflow|nanoflow` rebuilds the whole stored flow when the change cannot be " +
		"spliced in, which resets curves, drops merges and renumbers element IDs",
	New: "a refusal that names the change the splice cannot make, with nothing written",
}

// flowDecl is what diff-then-patch needs of a CREATE MICROFLOW or CREATE
// NANOFLOW statement.
type flowDecl struct {
	nanoflow bool
	// replace is a statement spelled `create or replace`, which splices as
	// `create or modify` does; messages name it as written (ako/mxcli#1681).
	replace bool
	name    ast.QualifiedName
	body    []ast.MicroflowStatement
	folder  string
	// returnVar is the `returns … as $Var` variable, which the end a body
	// falls through to returns; "" when there is none.
	returnVar string
	// params are the declared parameters, with the positions they state.
	params []ast.MicroflowParam
	// header returns the declared and the stored statement with body, folder,
	// name and parameter positions cleared, after the rules by which an absent
	// clause keeps what is stored have been applied, ready for declaredMatches.
	header func(stored ast.Statement) (declared, storedHeader any)
	// build builds the declared document the way `create` would, without
	// writing it: the header SetHeader copies (a *microflows.Microflow or
	// *microflows.Nanoflow), its parameters, and the entity each object or
	// list variable of the body holds as the builder resolved it.
	build func(ctx *ExecContext) (any, []*microflows.MicroflowParameter, map[string]string, error)
}

// buildOnce returns d.build memoised: the first call builds, later calls
// return the same result.
func (d *flowDecl) buildOnce(ctx *ExecContext) func() (any, []*microflows.MicroflowParameter, map[string]string, error) {
	var (
		done     bool
		built    any
		params   []*microflows.MicroflowParameter
		varTypes map[string]string
		err      error
	)
	return func() (any, []*microflows.MicroflowParameter, map[string]string, error) {
		if !done {
			built, params, varTypes, err = d.build(ctx)
			done = true
		}
		return built, params, varTypes, err
	}
}

// verb is the statement as written: `create or replace` or `create or modify`.
func (d *flowDecl) verb() string {
	if d.replace {
		return "create or replace"
	}
	return "create or modify"
}

func (d *flowDecl) kind() string {
	if d.nanoflow {
		return "nanoflow"
	}
	return "microflow"
}

// notSpliceable is the reason a declared change has to fall back.
type notSpliceable struct {
	reason string
	// loopHandle is the stored loop's handle (`loop $It in $Items`) when the
	// change is inside that loop's body: the refusal then advises replacing
	// the loop's body with alter, the one statement that can make the change
	// (ako/mxcli#1693).
	loopHandle string
}

func (e *notSpliceable) Error() string { return e.reason }

func cannotSplice(format string, args ...any) error {
	return &notSpliceable{reason: fmt.Sprintf(format, args...)}
}

// modifyFlowInPlace applies a `create or modify` of an existing flow as a
// patch. handled is false when the statement is not this path's to apply — no
// such flow yet, or a change the splice cannot make under mdl 0 — and the
// caller then runs the create / rebuild path.
func modifyFlowInPlace(ctx *ExecContext, d *flowDecl) (handled bool, err error) {
	// The verdict only reads; what it writes comes after, outside the cache.
	release := ctx.CacheUnitReads()
	v := decideFlowModify(ctx, d)
	release()
	switch {
	case v.why != nil:
		return fallBack(ctx, d, v)
	case v.err != nil:
		return true, v.err
	case v.plan == nil:
		return false, nil // a create
	}
	p := v.plan
	a, ops, moves, set, storedFolder := p.a, p.ops, p.moves, p.set, p.storedFolder
	if p.mut != nil {
		if err := p.mut.Save(); err != nil {
			return true, mdlerrors.NewBackend("save modified "+d.kind(), err)
		}
	}
	containerID := a.mf.ContainerID
	moved := movesFolder(d.folder, storedFolder)
	if moved {
		mod, err := findModule(ctx, d.name.Module)
		if err != nil {
			return true, err
		}
		to, err := resolveRequestedFolder(ctx, mod.ID, d.folder)
		if err != nil {
			return true, err
		}
		if _, err := applyDocumentFolder(ctx, a.mf.ID, a.mf.ContainerID, to); err != nil {
			return true, err
		}
		containerID = to
	}

	switch summary := patchSummary(ops, a.edited, moves, set); {
	case summary != "":
		ctx.ReportMutation("Modified", "%s: %s (%s)", d.kind(), d.name, summary)
		if ctx.Output != nil {
			for _, c := range a.carriedIncludes {
				fmt.Fprintln(ctx.Output, carriedIncludesWarning(d.name.String(), c))
			}
		}
	case moved:
		ctx.ReportMutation("Moved", "%s: %s", d.kind(), d.name)
	default:
		reportUnchanged(ctx, fmt.Sprintf("%s: %s", d.kind(), d.name))
	}

	returnEntity := extractEntityFromReturnType(a.mf.ReturnType)
	if d.nanoflow {
		ctx.trackCreatedNanoflow(d.name.Module, d.name.Name, a.mf.ID, containerID, returnEntity)
	} else {
		ctx.trackCreatedMicroflow(d.name.Module, d.name.Name, a.mf.ID, containerID, returnEntity)
	}
	// An edit in place changes no unit's container, so the hierarchy built
	// from them still holds. Rebuilding it after every statement listed every
	// unit of the project per `create or modify` (ako/mxcli#2160).
	if moved {
		invalidateHierarchy(ctx)
	}
	return true, nil
}

// movesFolder reports whether a declared folder clause moves a stored flow.
// No clause is not "the module root": it leaves the flow where it is, the rule
// every create-or-modify path follows (document_placement.go). The splice
// compared the two paths and moved on any difference, so a script that said
// nothing about folders unfiled every foldered flow it touched, and an organise
// step filing them again made each run write (ako/mxcli#887).
func movesFolder(declared, stored string) bool {
	return declared != "" && declared != stored
}

// flowPlan is a `create or modify` of a stored flow worked out as far as it
// goes without writing: the patch applied to the stored flow in memory, ready
// to save.
type flowPlan struct {
	a            *alterFlowContext
	ops          []*ast.AlterFlowOperation
	moves        []flowMove
	set          []string // the header properties the patch sets
	storedFolder string
	// mut holds the patched flow, unsaved; nil when there is nothing to
	// patch (the folder may still differ).
	mut backend.MicroflowMutator
}

// planFlowModify derives the patch for a `create or modify` of a stored flow
// and applies it in memory, writing nothing. It returns nil and no error when
// the statement is not a modify (no such module or flow: a create), and a
// *notSpliceable error for a change the splice cannot make — which exec
// refuses under mdl 1 and rebuilds under mdl 0 (fallBack), and diff reports
// (ako/mxcli#839: the two must reach the same verdict). Any other error is the
// statement's own.
func planFlowModify(ctx *ExecContext, d *flowDecl) (*flowPlan, error) {
	if _, err := findModule(ctx, d.name.Module); err != nil {
		return nil, nil // the create path makes the module
	}
	alter := &ast.AlterFlowStmt{Nanoflow: d.nanoflow, Name: d.name}
	a, err := loadAlterFlow(ctx, alter)
	if err != nil {
		var nf *mdlerrors.NotFoundError
		if errors.As(err, &nf) {
			return nil, nil // a create
		}
		return nil, cannotSplice("the stored %s cannot be read for splicing: %v", d.kind(), err)
	}
	// A stored import activity without its result entity (what mxcli wrote
	// before mendixlabs/mxcli#1418 for a mapping created further down the
	// script) makes the project unloadable, and describe prints it exactly as
	// the statement that would repair it, so the statement diff finds nothing
	// and reported "Unchanged" over a broken project. Treat it as a change the
	// splice cannot make: mdl 0 rebuilds the flow — resolving the mapping
	// afresh — and mdl 1 refuses with the drop-and-create advice, rather than
	// keeping it silently.
	if mapping := storedImportWithoutEntity(a.mf.ObjectCollection); mapping != "" {
		return nil, cannotSplice("its stored `import from mapping %s` activity has no result entity, which Mendix cannot load (mendixlabs/mxcli#1418)", mapping)
	}
	// The same shape for a change activity mxcli wrote over a list before
	// mendixlabs/mxcli#1422: the member is qualified through the list type
	// ("List of M.E.Attr", CE1613), describe prints it as the bare attribute,
	// and a splice that replaced only the retrieve kept it.
	if member := storedListQualifiedMember(a.mf.ObjectCollection); member != "" {
		return nil, cannotSplice("its stored change activity names the member %q, qualified through a list type, which mxbuild rejects with CE1613 (mendixlabs/mxcli#1422)", member)
	}

	stored, err := describedFlowStmt(ctx, d, a)
	if err != nil {
		return nil, cannotSplice("its description cannot be compared: %v", err)
	}
	decl, storedHeader := d.header(stored)
	headerChanged := !declaredMatches(decl, storedHeader)
	storedParams := a.mf.Parameters
	// Building the declared flow is the expensive step of a modify, and the
	// header diff, builtAsStored and the member spellings all need it: it is
	// built at most once (ako/mxcli#870).
	build := d.buildOnce(ctx)
	var declared any
	var varTypes map[string]string
	if headerChanged {
		if err := checkRemovedParameters(ctx, d, a); err != nil {
			return nil, asNotSpliceable(err)
		}
		var params []*microflows.MicroflowParameter
		if declared, params, varTypes, err = build(); err != nil {
			return nil, err
		}
		// The fragments are built and scope-checked against the parameters
		// the flow will have, not the ones it had, and the flow is tracked
		// with the return type it will have.
		mf := *a.mf
		mf.Parameters = params
		switch built := declared.(type) {
		case *microflows.Microflow:
			mf.ReturnType = built.ReturnType
		case *microflows.Nanoflow:
			mf.ReturnType = built.ReturnType
		}
		a.mf = &mf
	}

	var ops []*ast.AlterFlowOperation
	var targets []mfmutator.Candidate
	var moves []flowMove
	var freeNotes *[]ast.MicroflowAnnotation
	var bodyPins []bodyPin
	// A body that states what describe prints for the stored one — the
	// statement re-run unchanged, or a describe executed back — has nothing to
	// patch, and needs no build to say so (ako/mxcli#870): every statement
	// matches its stored one, so neither builtAsStored nor the statement diff
	// could find a change.
	sameBody := !headerChanged && declaredMatches(d.body, storedBody(stored))
	if !sameBody && !builtAsStored(ctx, a, build) {
		// Members are compared in the spelling describe prints them in, or a
		// script naming one another way never matches its own stored activity.
		// Which entity a variable holds is the builder's to say where the
		// statement declaring it does not name one (a call's result, a retrieve
		// over an association). A body that does not build leaves the spellings
		// to what the statements state; its errors are reported by the patch.
		if varTypes == nil {
			if _, _, vt, berr := build(); berr == nil {
				varTypes = vt
			}
		}
		body := describedMemberSpellings(ctx, d.params, d.body, varTypes)
		if ops, targets, moves, freeNotes, bodyPins, err = diffFlowBody(a, body, storedBody(stored), d.returnVar); err != nil {
			return nil, asNotSpliceable(err)
		}
	}
	moves = append(moves, parameterMoves(d, storedParams)...)

	// The fragments are built knowing what each variable holds in the declared
	// flow, as the full build does: the stored flow does not type a variable
	// bound by a retrieve over an association, a list operation or a loop, and
	// a member of one was written bare (ako/mxcli#885).
	a.declaredVarTypes = varTypes

	// A node the script draws at a stated position is pinned under mdl 1 only
	// (ADR-0011: a new refusal applies under the version that introduces it).
	// Under mdl 0 the refused splice would fall back to rebuilding the whole
	// flow, losing the merges, curves and $IDs the shift keeps, so a headerless
	// script makes room as it did (mendixlabs/mxcli#1598, case 6).
	var stated []model.Point
	if flowRebuildRefused.Applies(ctx.LanguageVersion) {
		stated = statedPoints(d)
	} else {
		bodyPins = nil
	}
	p := &flowPlan{a: a, ops: ops, moves: moves, storedFolder: storedFolderOf(stored)}
	if len(ops) > 0 || len(moves) > 0 || headerChanged || freeNotes != nil {
		if p.mut, err = patch(ctx, a, ops, targets, moves, stated, bodyPins); err != nil {
			return nil, cannotSplice("%v", err)
		}
		if headerChanged {
			if p.set, err = p.mut.SetHeader(declared); err != nil {
				return nil, cannotSplice("%v", err)
			}
		}
		if freeNotes != nil {
			if err := a.setFreeNotes(ctx, p.mut, *freeNotes); err != nil {
				return nil, cannotSplice("%v", err)
			}
			p.set = append(p.set, "free notes")
		}
	}
	return p, nil
}

// builtAsStored reports whether the declared body builds the graph that is
// stored (sameBuiltFlow), in which case there is nothing to patch in it,
// whatever describe prints for it (ako/mxcli#859). build is the declared
// flow's build, memoised by planFlowModify. A body that does not
// build, or a backend that cannot say how a flow reads back — including a
// flow the reader would not read back whole, where both sides would compare
// equal in what it drops — leaves it to the statement diff.
func builtAsStored(ctx *ExecContext, a *alterFlowContext, build func() (any, []*microflows.MicroflowParameter, map[string]string, error)) bool {
	built, _, _, err := build()
	if err != nil {
		return false
	}
	// Compared as it would read back once stored: what the writer defaults,
	// or has no property for, reads back as it does from the project.
	var oc *microflows.MicroflowObjectCollection
	switch f := built.(type) {
	case *microflows.Microflow:
		rb, err := ctx.Backend.ReadBackMicroflow(f)
		if err != nil || rb == nil {
			return false
		}
		oc = rb.ObjectCollection
	case *microflows.Nanoflow:
		rb, err := ctx.Backend.ReadBackNanoflow(f)
		if err != nil || rb == nil {
			return false
		}
		oc = rb.ObjectCollection
	}
	if oc == nil || a.mf.ObjectCollection == nil {
		return false
	}
	same, _ := sameBuiltFlow(oc, a.mf.ObjectCollection)
	return same
}

// asNotSpliceable marks err as a change the splice cannot make, keeping its
// text.
func asNotSpliceable(err error) error {
	var ns *notSpliceable
	if errors.As(err, &ns) {
		return err
	}
	return cannotSplice("%v", err)
}

// fallBack decides what a change the splice cannot make does: refused under
// mdl 1, the whole-document rebuild with a warning under mdl 0. The verdict
// and its wording are shared with diff and check (flow_verdict.go).
func fallBack(ctx *ExecContext, d *flowDecl, v flowVerdict) (bool, error) {
	if v.refused {
		return true, flowRefusal(d, v.why)
	}
	fmt.Fprintf(ctx.progress(), "Warning [%s]: %s\n", flowRebuildRefused.Code, flowRebuildWarning(ctx, d, v.why))
	return false, nil
}

// storedImportWithoutEntity returns the mapping of the first import-from-mapping
// activity in oc (loop bodies included) whose result is an object or list type
// naming no entity, or "". That is the shape Mendix cannot load
// (mendixlabs/mxcli#1418). An activity that stores no result type, or a
// DataTypes$VoidType, binds no result and loads at 0 errors (measured on
// mxbuild 11.13.0); refusing it as "no result entity" refused every statement
// over a flow importing into a parameter object (ako/mxcli#1681).
func storedImportWithoutEntity(oc *microflows.MicroflowObjectCollection) string {
	if oc == nil {
		return ""
	}
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if ix, ok := o.Action.(*microflows.ImportXmlAction); ok {
				if ix.ResultHandling == nil {
					return "(none)"
				}
				rt := ix.ResultHandling.ResultType
				if ix.ResultHandling.ResultEntityID == "" && (rt == "DataTypes$ObjectType" || rt == "DataTypes$ListType") {
					return string(ix.ResultHandling.MappingID)
				}
			}
		case *microflows.LoopedActivity:
			if m := storedImportWithoutEntity(o.ObjectCollection); m != "" {
				return m
			}
		}
	}
	return ""
}

// storedListQualifiedMember returns the first change-object member in oc (loop
// bodies included) qualified through a list type ("List of M.E.Attr"), or "".
func storedListQualifiedMember(oc *microflows.MicroflowObjectCollection) string {
	if oc == nil {
		return ""
	}
	for _, obj := range oc.Objects {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if ch, ok := o.Action.(*microflows.ChangeObjectAction); ok {
				for _, m := range ch.Changes {
					if isListQualifiedMember(m) {
						return m.AttributeQualifiedName + m.AssociationQualifiedName
					}
				}
			}
		case *microflows.LoopedActivity:
			if m := storedListQualifiedMember(o.ObjectCollection); m != "" {
				return m
			}
		}
	}
	return ""
}

// isListQualifiedMember reports a member change whose name was qualified
// through a variable's list type, which no domain model member can have.
func isListQualifiedMember(m *microflows.MemberChange) bool {
	return m != nil && (strings.HasPrefix(m.AttributeQualifiedName, "List of ") || strings.HasPrefix(m.AssociationQualifiedName, "List of "))
}

// reportUnchanged reports a statement that wrote nothing, collapsing into the
// run's summary like an elided write does.
func reportUnchanged(ctx *ExecContext, what string) {
	line := fmt.Sprintf("Unchanged %s\n", what)
	if ctx.tally.countUnchanged(line) {
		return
	}
	fmt.Fprint(ctx.Output, line)
}

// describedFlowStmt describes the stored flow in the script's own language
// and parses the description under that language's header, so the stored side
// and the declared side are read by the same rules and a value the two state
// alike compares as the same AST.
//
// Describing in any other language misreads one side. Under mdl 1 an mdl 0
// description spells a stored line break `\n`, which the mdl 1 reader takes
// as a backslash and an n, so a script stating exactly that reported Unchanged
// (#747); and a stored expression whose string holds a line break is
// re-rendered from `'…\n…'` but stored as written from the mdl 1 literal that
// spans lines, so an unchanged mdl 1 description rewrote the activity
// (ako/mxcli#804). See correctAmbiguousRanges for the one stored state mdl 0
// cannot state at all.
func describedFlowStmt(ctx *ExecContext, d *flowDecl, a *alterFlowContext) (ast.Statement, error) {
	var buf bytes.Buffer
	// Full layout: stored activities are located by the @position printed above
	// them, which the canonical describe leaves out when the engine derives it (#748).
	prevOut, prevIn, prevFull := ctx.Output, ctx.describeIn, ctx.describeFullLayout
	script := ctx.LanguageVersion
	ctx.Output, ctx.describeIn, ctx.describeFullLayout = &buf, &script, true
	var err error
	if d.nanoflow {
		err = describeNanoflow(ctx, d.name)
	} else {
		err = describeMicroflow(ctx, d.name)
	}
	src := describedSource(ctx, buf.String())
	ctx.Output, ctx.describeIn, ctx.describeFullLayout = prevOut, prevIn, prevFull
	if err != nil {
		return nil, err
	}
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return nil, fmt.Errorf("the description does not parse: %v", errs[0])
	}
	for _, st := range prog.Statements {
		switch st.(type) {
		case *ast.CreateMicroflowStmt:
			if d.nanoflow {
				continue
			}
		case *ast.CreateNanoflowStmt:
			if !d.nanoflow {
				continue
			}
		default:
			continue
		}
		correctAmbiguousRanges(st, a.mf.ObjectCollection)
		return st, nil
	}
	return nil, fmt.Errorf("the description has no create statement")
}

// correctAmbiguousRanges fixes the stored side where its mdl 0 description
// says something other than what is stored.
//
// A retrieve with a Custom range of `limit 1` and no offset — a list of one —
// is described as `limit 1`, which mdl 0 reads as the object range
// (ako/mxcli#734): mdl 0 has no spelling for that exact state. Parsed as is,
// the stored side would claim the object range, and a script stating the
// object range would look unchanged against a flow that holds a list; so the
// parsed statement is set back to the list it stands for. The statement is
// found by its output variable and its @position.
func correctAmbiguousRanges(st ast.Statement, oc *microflows.MicroflowObjectCollection) {
	type key struct {
		v string
		p model.Point
	}
	lists := map[key]bool{}
	var collect func(oc *microflows.MicroflowObjectCollection)
	collect = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, obj := range oc.Objects {
			if loop, ok := obj.(*microflows.LoopedActivity); ok {
				collect(loop.ObjectCollection)
				continue
			}
			act, ok := obj.(*microflows.ActionActivity)
			if !ok {
				continue
			}
			r, ok := act.Action.(*microflows.RetrieveAction)
			if !ok {
				continue
			}
			src, ok := r.Source.(*microflows.DatabaseRetrieveSource)
			if !ok || src.Range == nil || src.Range.RangeType != microflows.RangeTypeCustom ||
				src.Range.Limit != "1" || src.Range.Offset != "" {
				continue
			}
			lists[key{r.OutputVariable, act.Position}] = true
		}
	}
	collect(oc)
	if len(lists) == 0 {
		return
	}
	walkStatements(st, func(r *ast.RetrieveStmt) {
		if !r.First || r.Annotations == nil || r.Annotations.Position == nil {
			return
		}
		k := key{r.Variable, model.Point{X: r.Annotations.Position.X, Y: r.Annotations.Position.Y}}
		if lists[k] {
			r.First, r.Limit, r.Offset = false, "1", ""
		}
	})
}

// walkStatements calls fn for every retrieve statement in v, at any depth.
func walkStatements(v any, fn func(*ast.RetrieveStmt)) {
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			if r, ok := rv.Interface().(*ast.RetrieveStmt); ok {
				fn(r)
			}
			walk(rv.Elem())
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i))
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(v))
}

func storedBody(st ast.Statement) []ast.MicroflowStatement {
	switch s := st.(type) {
	case *ast.CreateMicroflowStmt:
		return s.Body
	case *ast.CreateNanoflowStmt:
		return s.Body
	}
	return nil
}

func storedFolderOf(st ast.Statement) string {
	switch s := st.(type) {
	case *ast.CreateMicroflowStmt:
		return s.Folder
	case *ast.CreateNanoflowStmt:
		return s.Folder
	}
	return ""
}

// microflowDecl adapts a CREATE MICROFLOW statement.
func microflowDecl(s *ast.CreateMicroflowStmt) *flowDecl {
	return &flowDecl{
		replace: s.CreateOrReplace,
		name:    s.Name, body: s.Body, folder: s.Folder, returnVar: returnVariable(s.ReturnType), params: s.Parameters,
		build: func(ctx *ExecContext) (any, []*microflows.MicroflowParameter, map[string]string, error) {
			built, err := buildMicroflowFromStmt(ctx, s, buildFlowOpts{Quiet: true})
			if err != nil {
				return nil, nil, nil, err
			}
			return built.Microflow, built.Microflow.Parameters, built.VarTypes, nil
		},
		header: func(stored ast.Statement) (any, any) {
			st, _ := stored.(*ast.CreateMicroflowStmt)
			if st == nil {
				return s, nil
			}
			dh, sh := *s, *st
			for _, h := range []*ast.CreateMicroflowStmt{&dh, &sh} {
				h.Body, h.Folder, h.Name, h.CreateOrModify = nil, "", ast.QualifiedName{}, false
				h.Parameters = withoutParameterPositions(h.Parameters)
			}
			// Absent means "keep what is stored" for these (see the field
			// comments on CreateMicroflowStmt), so absent is not a difference.
			if !dh.DocumentationSet {
				dh.Documentation = sh.Documentation
			}
			dh.DocumentationSet, sh.DocumentationSet = false, false
			if !dh.Excluded {
				dh.Excluded = sh.Excluded
			}
			if dh.ApplyEntityAccess == nil {
				dh.ApplyEntityAccess = sh.ApplyEntityAccess
			}
			if dh.Expose == nil {
				dh.Expose = sh.Expose
			}
			if dh.URL == nil {
				dh.URL = sh.URL
			}
			if dh.URLSearchParameters == nil {
				dh.URLSearchParameters = sh.URLSearchParameters
			}
			if dh.ExportLevel == nil {
				dh.ExportLevel = sh.ExportLevel
			}
			if dh.Concurrency == nil {
				dh.Concurrency = sh.Concurrency
			}
			return &dh, &sh
		},
	}
}

func returnVariable(rt *ast.MicroflowReturnType) string {
	if rt == nil {
		return ""
	}
	return rt.Variable
}

// nanoflowDecl adapts a CREATE NANOFLOW statement.
func nanoflowDecl(s *ast.CreateNanoflowStmt) *flowDecl {
	return &flowDecl{
		nanoflow: true, replace: s.CreateOrReplace,
		name: s.Name, body: s.Body, folder: s.Folder, returnVar: returnVariable(s.ReturnType), params: s.Parameters,
		build: func(ctx *ExecContext) (any, []*microflows.MicroflowParameter, map[string]string, error) {
			built, err := buildNanoflowFromStmt(ctx, s, buildFlowOpts{Quiet: true})
			if err != nil {
				return nil, nil, nil, err
			}
			return built.Nanoflow, built.Nanoflow.Parameters, built.VarTypes, nil
		},
		header: func(stored ast.Statement) (any, any) {
			st, _ := stored.(*ast.CreateNanoflowStmt)
			if st == nil {
				return s, nil
			}
			dh, sh := *s, *st
			for _, h := range []*ast.CreateNanoflowStmt{&dh, &sh} {
				h.Body, h.Folder, h.Name, h.CreateOrModify = nil, "", ast.QualifiedName{}, false
				h.Parameters = withoutParameterPositions(h.Parameters)
			}
			if !dh.DocumentationSet {
				dh.Documentation = sh.Documentation
			}
			dh.DocumentationSet, sh.DocumentationSet = false, false
			if !dh.Excluded {
				dh.Excluded = sh.Excluded
			}
			if dh.Expose == nil {
				dh.Expose = sh.Expose
			}
			if dh.ExportLevel == nil {
				dh.ExportLevel = sh.ExportLevel
			}
			return &dh, &sh
		},
	}
}

// diffFlowBody derives the splice operations that turn the stored top-level
// statements into the declared ones. Matched statements are left alone; each
// run of unmatched statements between two matched ones becomes one operation:
//
//   - declared statements where none are stored: insert after the stored
//     statement before the run (or before the one after it);
//   - stored statements where none are declared: drop each;
//   - both: replace the first stored one with the declared run, drop the rest.
//
// A statement that matches a stored one except where it is drawn is the stored
// node moved: a move, not an operation (ako/mxcli#818). So is a stated @start
// that is not where the start event is.
//
// Targets are the stored activities, located by the @position describe printed
// for each statement. A run whose stored end cannot be located, or that the
// splice cannot express, is a notSpliceable error.
func diffFlowBody(a *alterFlowContext, declared, stored []ast.MicroflowStatement, returnVar string) ([]*ast.AlterFlowOperation, []mfmutator.Candidate, []flowMove, *[]ast.MicroflowAnnotation, []bodyPin, error) {
	// Free annotations — notes wired to nothing — belong to the flow, not to
	// the statement describe happens to print them above (the first one), so
	// they are compared as a whole and kept out of the statement match: an
	// insert at the top would otherwise carry them onto a new statement and
	// write them a second time.
	declared, declaredFree := withoutFreeNotes(declared)
	stored, storedFree := withoutFreeNotes(stored)
	// Changed, they are replaced as a whole: a free note is no part of any
	// activity, so nothing else moves (mendixlabs/mxcli#1517). A note restated
	// above its own activity's @position reads as a free one; the stored flow
	// says it is attached, so that is refused with the spelling that works.
	var freeNotes *[]ast.MicroflowAnnotation
	if !declaredMatches(declaredFree, storedFree) {
		if hint := misplacedNote(declaredFree, storedFree, stored); hint != "" {
			return nil, nil, nil, nil, nil, cannotSplice("%s", hint)
		}
		for _, n := range declaredFree {
			if n.Label != "" {
				return nil, nil, nil, nil, nil, cannotSplice("the free annotations change (%s), and one of them has an id (%s); "+
					"the splice sets free notes that are attached to nothing", freeNoteChange(declaredFree, storedFree), n.Label)
			}
		}
		freeNotes = &declaredFree
	}

	// @start is where the start event is drawn, which describe prints on the
	// first statement: it belongs to the start event, not to the statement.
	declared, declaredStart := withoutStart(declared)
	stored, _ = withoutStart(stored)
	// One spelling of each control-flow shape on both sides, so a script
	// matches the flow built from it however describe prints it (#859).
	declared, stored = canonicalFlow(declared), canonicalFlow(stored)
	// The implicit end is stated against the canonical stored flow: a guard
	// directly before the final end is described as an if/else whose else
	// returns, which only the canonical form ends with that return
	// (ako/mxcli#888).
	declared = withImplicitEnd(declared, stored, returnVar)

	pd := &patchDiff{loc: newStoredLocator(a)}
	if err := pd.statements(declared, stored); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	pd.startEvent(a, declared, declaredStart)
	if err := pd.followingEnd(declared, stored); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	// Drops go last. Each operation's scope check runs against the flow as the
	// earlier operations left it, so a stored activity whose output only a
	// replaced statement read can be dropped once that statement is replaced,
	// and not before. The splice itself does not care about the order: every
	// target is a stored activity no other operation touches.
	var ops []*ast.AlterFlowOperation
	var targets []mfmutator.Candidate
	for _, drops := range []bool{false, true} {
		for i, op := range pd.ops {
			if (op.Op == ast.AlterFlowDrop) == drops {
				ops = append(ops, op)
				targets = append(targets, pd.targets[i])
			}
		}
	}
	return ops, targets, pd.moves, freeNotes, pd.bodyPins, nil
}

// withImplicitEnd states the end a declared body falls through to as the
// `return` it stands for, when the stored flow's description spells its end
// that way (ako/mxcli#805).
//
// A body whose last statement does not end the flow ends at an end event the
// builder draws, returning the `returns … as $Var` variable if there is one.
// describe prints that end event as a trailing `@position(x, y) return;`
// whenever it keeps layout — which the stored side of the diff always does —
// so without this the stored end looks like a statement the script dropped,
// and an end event cannot be dropped. Stated, the two match as any statement
// does: a declared return without @position matches the stored end wherever it
// is drawn, so the end event stays where it is.
func withImplicitEnd(declared, stored []ast.MicroflowStatement, returnVar string) []ast.MicroflowStatement {
	if len(stored) == 0 || lastStmtIsReturn(declared) {
		return declared
	}
	if _, ok := stored[len(stored)-1].(*ast.ReturnStmt); !ok {
		return declared
	}
	end := &ast.ReturnStmt{}
	if returnVar != "" {
		end.Value = &ast.VariableExpr{Name: returnVar}
	}
	return append(append([]ast.MicroflowStatement(nil), declared...), end)
}

// patchDiff accumulates the operations of one diff.
type patchDiff struct {
	loc     *storedLocator
	ops     []*ast.AlterFlowOperation
	targets []mfmutator.Candidate
	moves   []flowMove
	// bodyPins are the points the declared loop bodies state their nodes at
	// (pinBody).
	bodyPins []bodyPin
}

func (pd *patchDiff) add(op ast.AlterFlowOpKind, c mfmutator.Candidate, body []ast.MicroflowStatement) {
	pd.ops = append(pd.ops, &ast.AlterFlowOperation{Op: op, Target: targetLabel(c), Body: body})
	pd.targets = append(pd.targets, c)
}

// statements diffs one statement list: the flow's top level, or a branch of
// an `if`. The activities in an `if` branch are top-level nodes of the stored
// graph (only a loop nests a collection), so a branch is spliced the same way.
func (pd *patchDiff) statements(declared, stored []ast.MicroflowStatement) error {
	pairs := lcsStatements(declared, stored)
	di, si := 0, 0
	for p := 0; p <= len(pairs); p++ {
		dEnd, sEnd := len(declared), len(stored)
		if p < len(pairs) {
			dEnd, sEnd = pairs[p][0], pairs[p][1]
		}
		if err := pd.gap(declared[di:dEnd], stored, si, sEnd); err != nil {
			return err
		}
		if p < len(pairs) {
			// A pair matched only once positions are ignored is the stored
			// statement drawn somewhere else; one matched by its shell is the
			// same if with a change inside, diffed as its own run.
			if d, st := declared[pairs[p][0]], stored[pairs[p][1]]; !declaredMatches(d, st) {
				if !sameExceptPositions(d, st) {
					if err := pd.gap([]ast.MicroflowStatement{d}, stored, pairs[p][1], pairs[p][1]+1); err != nil {
						return err
					}
				} else if err := pd.moved(d, st); err != nil {
					return err
				}
			}
			di, si = pairs[p][0]+1, pairs[p][1]+1
		}
	}
	return nil
}

// gap turns one run of unmatched statements — ins declared where stored[si:sEnd]
// is stored — into operations.
func (pd *patchDiff) gap(ins []ast.MicroflowStatement, stored []ast.MicroflowStatement, si, sEnd int) error {
	del := stored[si:sEnd]
	if len(ins) == 0 && len(del) == 0 {
		return nil
	}
	// A run that ends a path on both sides: the returns are the same end
	// event, whatever else changes before it (ako/mxcli#805).
	if len(ins) > 0 && len(del) > 0 {
		dr, ok1 := ins[len(ins)-1].(*ast.ReturnStmt)
		sr, ok2 := del[len(del)-1].(*ast.ReturnStmt)
		if ok1 && ok2 {
			if err := pd.endEvent(dr, sr); err != nil {
				return err
			}
			return pd.gap(ins[:len(ins)-1], stored, si, sEnd-1)
		}
	}
	// Any other return added or taken away moves where a path ends.
	for _, st := range del {
		if _, ok := st.(*ast.ReturnStmt); ok {
			return cannotSplice("the %s is taken out, so the path it ended would go on; where a path ends is the shape "+
				"of the flow, and the splice changes a return's value only", describeAt(st))
		}
	}
	for _, st := range ins {
		if _, ok := st.(*ast.ReturnStmt); ok {
			return cannotSplice("a return is added where the stored flow does not end a path; where a path ends is the " +
				"shape of the flow, and the splice changes a return's value only")
		}
	}
	if len(del) == 0 {
		op, c, err := insertAnchor(pd.loc, stored, si, sEnd)
		if err != nil {
			return err
		}
		pd.add(op, c, ins)
		return nil
	}
	if len(ins) == 1 && len(del) == 1 {
		if d, s, ok := ifConditionChanged(ins[0], del[0]); ok {
			return pd.conditionEdit(d, s)
		}
		if d, s, ok := sameIfShell(ins[0], del[0]); ok {
			// The same `if` with a change in a branch: splice the branches,
			// and move the split if the script draws it elsewhere.
			if err := pd.movedNode(d, s); err != nil {
				return err
			}
			if err := pd.statements(d.ThenBody, s.ThenBody); err != nil {
				return err
			}
			return pd.statements(d.ElseBody, s.ElseBody)
		}
		if d, s, ok := sameCaseShell(ins[0], del[0]); ok {
			// The same `case` with a change in a branch: splice each branch
			// like an if's, rather than replace the split — which the splice
			// cannot do for more than two paths (mendixlabs/mxcli#1778).
			if err := pd.movedNode(d, s); err != nil {
				return err
			}
			for i := range d.Cases {
				if err := pd.statements(d.Cases[i].Body, s.Cases[i].Body); err != nil {
					return err
				}
			}
			return pd.statements(d.ElseBody, s.ElseBody)
		}
		if sameHandlerShell(ins[0], del[0]) {
			return pd.handlerEdit(ins[0], del[0])
		}
		if sameLoopShell(ins[0], del[0]) {
			return pd.loopBody(ins[0], del[0])
		}
		if sameIgnoringLayout(ins[0], del[0]) {
			return redrawn(del[0])
		}
	}
	// A declared statement that is a stored one of the same run with its
	// connectors redrawn, whichever other statements change around it. As a
	// replace and a drop it would be written as a new node, and the geometry
	// the script states would be silently lost (ako/mxcli#805). A stored one
	// that is only moved has matched already (lcsStatements).
	for _, d := range ins {
		for _, st := range del {
			if sameIgnoringLayout(d, st) {
				return redrawn(st)
			}
		}
	}
	// A stored loop declared again with a new body, in a run with other
	// changes: replacing the run would rebuild the loop, so the run is split
	// at the loop, which is diffed in place like a loop on its own, and the
	// changes on either side of it are runs of their own (ako/mxcli#942,
	// ako/mxcli#1693).
	// A stored `case` declared again with a change in a branch, in a run with
	// other changes, is split out the same way: replacing the run would take
	// the split out, which the splice cannot do for more than two paths
	// (mendixlabs/mxcli#1778).
	if i, j, ok := casePair(ins, del); ok && (len(ins) > 1 || len(del) > 1) {
		if err := pd.gap(ins[:i], stored, si, si+j); err != nil {
			return err
		}
		if err := pd.gap(ins[i:i+1], stored, si+j, si+j+1); err != nil {
			return err
		}
		return pd.gap(ins[i+1:], stored, si+j+1, sEnd)
	}
	if i, j, ok := loopPair(ins, del); ok {
		if err := pd.gap(ins[:i], stored, si, si+j); err != nil {
			return err
		}
		if err := pd.gap(ins[i:i+1], stored, si+j, si+j+1); err != nil {
			return err
		}
		return pd.gap(ins[i+1:], stored, si+j+1, sEnd)
	}
	// A stored `if` declared again with its condition but neither as the same
	// shell nor with only its condition changed has its else or a branch's
	// return added or taken away: where its paths end or meet changes, which
	// a replace of the decision cannot express (it would take out both of its
	// paths).
	for _, st := range del {
		si, ok := st.(*ast.IfStmt)
		if !ok {
			continue
		}
		for _, d := range ins {
			if di, ok := d.(*ast.IfStmt); ok && declaredMatches(di.Condition, si.Condition) {
				return cannotSplice("the %s changes where its paths end or meet — a branch's return, or its else, is "+
					"added or taken away; that is the shape of the flow, and the splice changes a return's value only", describeAt(st))
			}
		}
	}
	// A run of activities each changed in its own place is that many changes
	// of one activity, so each can be edited in place (ako/mxcli#1761) rather
	// than the run being replaced as a whole.
	if pairwiseSameKind(ins, del) {
		for i := range ins {
			if err := pd.gap(ins[i:i+1], stored, si+i, si+i+1); err != nil {
				return err
			}
		}
		return nil
	}
	cands := make([]mfmutator.Candidate, len(del))
	for i, st := range del {
		c, err := pd.loc.locate(st)
		if err != nil {
			return err
		}
		cands[i] = c
	}
	rest, restStmts := cands, del
	if len(ins) > 0 {
		body, replaceNotes, err := keepStoredNotes(del[0], ins)
		if err != nil {
			return err
		}
		pd.add(ast.AlterFlowReplace, cands[0], body)
		pd.ops[len(pd.ops)-1].ReplaceNotes = replaceNotes
		rest, restStmts = cands[1:], del[1:]
	}
	for i, c := range rest {
		// A dropped activity's notes go with it: the declared statements
		// state every note they keep, and draw it (ako/mxcli#859). One shared
		// with another activity would go from there too.
		var notes []ast.MicroflowAnnotation
		if ann := statementAnnotations(restStmts[i]); ann != nil {
			notes = ann.Notes
		}
		for _, n := range notes {
			if n.Label != "" {
				return cannotSplice("the %s dropped at (%d, %d) carries an annotation shared with another activity (id: %s), "+
					"which dropping it would take from that activity too",
					statementKind(restStmts[i]), c.Object.GetPosition().X, c.Object.GetPosition().Y, n.Label)
			}
		}
		pd.add(ast.AlterFlowDrop, c, nil)
		pd.ops[len(pd.ops)-1].ReplaceNotes = len(notes) > 0
	}
	return nil
}

// endEvent diffs a declared return against the stored one it stands for: the
// same end event, so a changed value is set in place — its $ID, the flows into
// it and the notes on it stay. A moved end event is moved; one with other notes
// is refused like any re-annotated node.
func (pd *patchDiff) endEvent(declared, stored *ast.ReturnStmt) error {
	if declaredMatches(declared, stored) {
		return nil
	}
	c, err := pd.loc.locate(stored)
	if err != nil {
		return err
	}
	if _, ok := c.Object.(*microflows.EndEvent); !ok {
		return cannotSplice("the stored %s is not drawn as an end event", describeAt(stored))
	}
	// The notes are set on the end event in place, like its value
	// (mendixlabs/mxcli#1517); anything else about it is its geometry.
	shell, sShell := *declared, *stored
	shell.Value = stored.Value
	shell.Annotations, sShell.Annotations = withoutNotes(declared.Annotations), withoutNotes(stored.Annotations)
	if !sameIgnoringLayout(&shell, &sShell) {
		return cannotSplice("the annotations on the %s change; the splice changes a return's value and notes only", describeAt(stored))
	}
	if !declaredMatches(&shell, &sShell) {
		if err := pd.moved(&shell, &sShell); err != nil {
			return err
		}
	}
	if !declaredMatches(declared.Value, stored.Value) {
		pd.add(ast.AlterFlowReplace, c, []ast.MicroflowStatement{&ast.ReturnStmt{Value: declared.Value}})
	}
	return pd.nodeNotes(c, declared, stored)
}

// nodeNotesChanged reports whether the declared statement states other notes
// than the stored one: one added, reworded or taken off.
func nodeNotesChanged(declared, stored ast.MicroflowStatement) bool {
	return !declaredMatches(statementNotes(declared), statementNotes(stored))
}

// statementNotes are the notes attached to a statement.
func statementNotes(st ast.MicroflowStatement) []ast.MicroflowAnnotation {
	if ann := statementAnnotations(st); ann != nil {
		return ann.Notes
	}
	return nil
}

// withoutNotes is a copy of ann with no notes, nil when nothing else is
// stated.
func withoutNotes(ann *ast.ActivityAnnotations) *ast.ActivityAnnotations {
	if ann == nil {
		return nil
	}
	c := *ann
	c.Notes = nil
	if declaredMatches(&c, &ast.ActivityAnnotations{}) {
		return nil
	}
	return &c
}

// nodeNotes adds the operation that gives the stored node c — a decision or
// an end event, changed in place — the declared statement's notes, when they
// differ from the stored ones. A note shared with another activity is refused:
// changing it here would change it there too.
func (pd *patchDiff) nodeNotes(c mfmutator.Candidate, declared, stored ast.MicroflowStatement) error {
	if !nodeNotesChanged(declared, stored) {
		return nil
	}
	for _, n := range append(append([]ast.MicroflowAnnotation(nil), statementNotes(stored)...), statementNotes(declared)...) {
		if n.Label != "" {
			return cannotSplice("the annotations on the %s change, and one of them is shared with another activity "+
				"(id: %s); the splice changes the notes of one node only", describeAt(stored), n.Label)
		}
	}
	pd.add(ast.AlterFlowReplace, c, []ast.MicroflowStatement{declared})
	pd.ops[len(pd.ops)-1].SetNotes = true
	return nil
}

// sameIfShell reports whether two statements are the same `if` — condition,
// annotations, whether it has an else — differing at most inside its branches
// and in where the split is drawn.
func sameIfShell(declared, stored ast.MicroflowStatement) (*ast.IfStmt, *ast.IfStmt, bool) {
	d, ok1 := declared.(*ast.IfStmt)
	s, ok2 := stored.(*ast.IfStmt)
	if !ok1 || !ok2 {
		return nil, nil, false
	}
	dShell, sShell := *d, *s
	dShell.ThenBody, dShell.ElseBody, sShell.ThenBody, sShell.ElseBody = nil, nil, nil, nil
	if !sameExceptPositions(&dShell, &sShell) {
		return nil, nil, false
	}
	return d, s, true
}

// sameCaseShell reports whether two statements are the same `case` — the
// same subject, annotations and branches, each branch deciding on the same
// values in the same order, and an else on both or neither — differing at most
// inside its branches and in where the split is drawn. A branch's activities
// are top-level nodes of the stored graph, as an if's are, so each branch is
// spliced like one.
//
// The branches are paired by position, not by value: describe prints them in
// the order the stored flows leave the split, and a script restating the flow
// keeps that order. A reordered or regrouped branch is a change of the split
// itself, which is still replaced (and refused for more than two paths).
func sameCaseShell(declared, stored ast.MicroflowStatement) (*ast.EnumSplitStmt, *ast.EnumSplitStmt, bool) {
	d, ok1 := declared.(*ast.EnumSplitStmt)
	s, ok2 := stored.(*ast.EnumSplitStmt)
	if !ok1 || !ok2 || len(d.Cases) != len(s.Cases) || (len(d.ElseBody) == 0) != (len(s.ElseBody) == 0) {
		return nil, nil, false
	}
	dShell, sShell := *d, *s
	dShell.Cases, sShell.Cases = make([]ast.EnumSplitCase, len(d.Cases)), make([]ast.EnumSplitCase, len(s.Cases))
	for i := range d.Cases {
		dShell.Cases[i] = ast.EnumSplitCase{Value: d.Cases[i].Value, Values: d.Cases[i].Values}
		sShell.Cases[i] = ast.EnumSplitCase{Value: s.Cases[i].Value, Values: s.Cases[i].Values}
	}
	dShell.ElseBody, sShell.ElseBody = nil, nil
	if !sameExceptPositions(&dShell, &sShell) {
		return nil, nil, false
	}
	return d, s, true
}

// ifConditionChanged reports whether two statements are the same `if` but for
// its condition or its caption — the same notes, the same else — differing
// besides at most inside its branches and in where the split is drawn
// (ako/mxcli#888). Both are properties of the decision, set on it in place; a
// caption change was refused as a change of where the paths end or meet
// (mendixlabs/mxcli#1610).
func ifConditionChanged(declared, stored ast.MicroflowStatement) (*ast.IfStmt, *ast.IfStmt, bool) {
	d, ok1 := declared.(*ast.IfStmt)
	s, ok2 := stored.(*ast.IfStmt)
	if !ok1 || !ok2 {
		return nil, nil, false
	}
	if declaredMatches(d.Condition, s.Condition) && !decisionCaptionChanged(d, s) && !nodeNotesChanged(d, s) {
		return nil, nil, false
	}
	dShell, sShell := *d, *s
	dShell.ThenBody, dShell.ElseBody, sShell.ThenBody, sShell.ElseBody = nil, nil, nil, nil
	dShell.Condition = s.Condition
	dShell.Annotations, sShell.Annotations = withoutCaption(d.Annotations), withoutCaption(s.Annotations)
	if !sameExceptPositions(&dShell, &sShell) {
		return nil, nil, false
	}
	return d, s, true
}

// decisionCaptionChanged reports whether the declared `if` states another
// caption than the stored one: one added, reworded or taken off.
func decisionCaptionChanged(declared, stored *ast.IfStmt) bool {
	dc, ds := decisionCaption(declared)
	sc, ss := decisionCaption(stored)
	return dc != sc || ds != ss
}

// decisionCaption is the @caption an `if` states, and whether it states one.
func decisionCaption(st *ast.IfStmt) (string, bool) {
	if st.Annotations == nil || !st.Annotations.CaptionSet {
		return "", false
	}
	return st.Annotations.Caption, true
}

// withoutCaption is a copy of ann with no @caption and no notes, nil when
// nothing else is stated, so two shells that differ only in those compare
// equal: both are set on the stored decision in place.
func withoutCaption(ann *ast.ActivityAnnotations) *ast.ActivityAnnotations {
	if ann == nil {
		return nil
	}
	c := *ann
	c.Caption, c.CaptionSet, c.Notes = "", false, nil
	if declaredMatches(&c, &ast.ActivityAnnotations{}) {
		return nil
	}
	return &c
}

// conditionEdit diffs an `if` whose condition changes against the stored
// one: the decision is the same node, so the new condition is set on it in
// place — its $ID, its flows and both of its paths stay — and its branches
// are diffed like any other statement list. Replacing the decision instead
// would take out what both of its paths hold, which the splice refuses.
func (pd *patchDiff) conditionEdit(declared, stored *ast.IfStmt) error {
	c, err := pd.loc.locate(stored)
	if err != nil {
		return err
	}
	if _, ok := c.Object.(*microflows.ExclusiveSplit); !ok {
		return cannotSplice("the stored %s is not drawn as a decision", describeAt(stored))
	}
	if !declaredMatches(declared.Condition, stored.Condition) || decisionCaptionChanged(declared, stored) {
		// A caption the script changes is stated on the operation's `if`;
		// otherwise the stored caption is kept, or follows the condition.
		edit := &ast.IfStmt{Condition: declared.Condition}
		if decisionCaptionChanged(declared, stored) {
			caption, set := decisionCaption(declared)
			edit.Annotations = &ast.ActivityAnnotations{Caption: caption, CaptionSet: set}
		}
		pd.add(ast.AlterFlowReplace, c, []ast.MicroflowStatement{edit})
		pd.ops[len(pd.ops)-1].SetCondition = true
	}
	if err := pd.nodeNotes(c, declared, stored); err != nil {
		return err
	}
	if err := pd.movedNode(declared, stored); err != nil {
		return err
	}
	if err := pd.statements(declared.ThenBody, stored.ThenBody); err != nil {
		return err
	}
	return pd.statements(declared.ElseBody, stored.ElseBody)
}

// sameBlockShell reports whether two statements are the same `if`, differing
// inside its branches. Such a pair is matched as a pair (lcsStatements) and
// diffed branch by branch, rather than left to merge with the changes around
// it into one run: a change before a guard and one inside its then-branch were
// one replace whose fragment held the guard's return, which the splice cannot
// insert (#859). A loop is paired the same way (lcsStatements), and its body
// diffed inside it (inLoopBody, ako/mxcli#1693).
func sameBlockShell(declared, stored ast.MicroflowStatement) bool {
	if _, _, ok := sameIfShell(declared, stored); ok {
		return true
	}
	// The same condition with another caption or other notes is the restated
	// decision too (mendixlabs/mxcli#1610, #1517), not a condition change.
	d, s, ok := ifConditionChanged(declared, stored)
	return ok && declaredMatches(d.Condition, s.Condition)
}

// conditionChanged reports whether two statements are the same `if` but for
// its condition (ifConditionChanged). Such a pair is matched as a pair too
// (ako/mxcli#888): the condition is set on the stored decision, and a change
// around it must not merge with it into a replace of the decision.
func conditionChanged(declared, stored ast.MicroflowStatement) bool {
	d, s, ok := ifConditionChanged(declared, stored)
	return ok && !declaredMatches(d.Condition, s.Condition)
}

// sameLoopShell reports whether two statements are the same loop — the same
// kind and iteration — differing at most in its body and its geometry. (A loop
// moved as well as edited inside is still a change inside the loop.)
func sameLoopShell(declared, stored ast.MicroflowStatement) bool {
	switch d := declared.(type) {
	case *ast.LoopStmt:
		s, ok := stored.(*ast.LoopStmt)
		if !ok {
			return false
		}
		dShell, sShell := *d, *s
		dShell.Body, sShell.Body = nil, nil
		return sameIgnoringLayout(&dShell, &sShell)
	case *ast.WhileStmt:
		s, ok := stored.(*ast.WhileStmt)
		if !ok {
			return false
		}
		dShell, sShell := *d, *s
		dShell.Body, sShell.Body = nil, nil
		return sameIgnoringLayout(&dShell, &sShell)
	}
	return false
}

// loopBody diffs a stored loop declared again with a change inside its body
// (ako/mxcli#1106, ako/mxcli#1693). The loop itself stays — its $ID, where it
// is drawn, its iterator and list — and its body is diffed as a statement list
// of its own (inLoopBody).
func (pd *patchDiff) loopBody(declared, stored ast.MicroflowStatement) error {
	c, err := pd.loc.locate(stored)
	if err != nil {
		return err
	}
	loop, ok := c.Object.(*microflows.LoopedActivity)
	if !ok {
		return cannotSplice("the stored %s is not drawn as a loop", describeAt(stored))
	}
	if !sameExceptPositions(loopShell(declared), loopShell(stored)) {
		return redrawn(stored)
	}
	if err := pd.movedNode(declared, stored); err != nil {
		return err
	}
	return pd.inLoopBody(c, loop, stored, loopStatements(declared), loopStatements(stored))
}

// loopShell is a copy of a loop or while statement without its body.
func loopShell(st ast.MicroflowStatement) ast.MicroflowStatement {
	switch l := st.(type) {
	case *ast.LoopStmt:
		c := *l
		c.Body = nil
		return &c
	case *ast.WhileStmt:
		c := *l
		c.Body = nil
		return &c
	}
	return st
}

// loopPair finds, in a run of unmatched statements, a declared loop that is a
// stored one restated (sameLoopShell): its index in ins and in del.
func loopPair(ins, del []ast.MicroflowStatement) (int, int, bool) {
	for i, d := range ins {
		for j, st := range del {
			if sameLoopShell(d, st) {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// casePair finds, in a run of unmatched statements, a declared `case` that is
// a stored one restated (sameCaseShell): its index in ins and in del.
func casePair(ins, del []ast.MicroflowStatement) (int, int, bool) {
	for i, d := range ins {
		for j, st := range del {
			if _, _, ok := sameCaseShell(d, st); ok {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// inLoopBody diffs the body of the stored loop lc (drawn as loop, described
// as at) the way the top level is diffed — statements matched by a longest
// common subsequence, each run between two matched ones an insert, replace or
// drop aimed at the stored body activity — but for plain activities only
// (ako/mxcli#1693). The mutator lays an inserted run out among the body's
// nodes, in the loop box's coordinates, and grows the box when the body no
// longer fits it. A nested loop restated with a new body is diffed the same
// way, inside its own box.
//
// What changes the body's shape — a decision, a loop, a return, break or
// continue, a raise error or an error handler added, changed or taken out —
// and a body node drawn elsewhere are refused (bodyRefusal), naming
// `replace body of` the loop. A stored body that is empty has no activity to
// insert next to: the declared one is put in it by replace body, which loses
// nothing there.
func (pd *patchDiff) inLoopBody(lc mfmutator.Candidate, loop *microflows.LoopedActivity, at ast.MicroflowStatement,
	declared, stored []ast.MicroflowStatement) error {
	outer := pd.loc
	pd.loc = outer.within(loop)
	defer func() { pd.loc = outer }()
	pd.pinBody(lc.ID, declared)
	if len(stored) == 0 {
		if len(declared) > 0 {
			pd.add(ast.AlterFlowReplaceBody, lc, declared)
		}
		return nil
	}
	pairs := lcsStatements(declared, stored)
	di, si := 0, 0
	for p := 0; p <= len(pairs); p++ {
		dEnd, sEnd := len(declared), len(stored)
		if p < len(pairs) {
			dEnd, sEnd = pairs[p][0], pairs[p][1]
		}
		if err := pd.bodyGap(lc, at, declared[di:dEnd], stored, si, sEnd); err != nil {
			return err
		}
		if p < len(pairs) {
			d, st := declared[pairs[p][0]], stored[pairs[p][1]]
			if !declaredMatches(d, st) {
				if sameExceptPositions(d, st) {
					return bodyRefusal(lc, at, fmt.Sprintf("the %s in it is drawn elsewhere, and the splice moves no node "+
						"inside a loop body", describeAt(st)))
				}
				if err := pd.bodyGap(lc, at, []ast.MicroflowStatement{d}, stored, pairs[p][1], pairs[p][1]+1); err != nil {
					return err
				}
			}
			di, si = pairs[p][0]+1, pairs[p][1]+1
		}
	}
	return nil
}

// bodyGap is gap for a run of unmatched statements inside the body of the
// stored loop lc: declared ins where stored[si:sEnd] is stored.
func (pd *patchDiff) bodyGap(lc mfmutator.Candidate, at ast.MicroflowStatement, ins, stored []ast.MicroflowStatement, si, sEnd int) error {
	del := stored[si:sEnd]
	if len(ins) == 0 && len(del) == 0 {
		return nil
	}
	if len(ins) == 1 && len(del) == 1 {
		if sameLoopShell(ins[0], del[0]) {
			// A nested loop restated with a change in its body: the same rules,
			// inside its own box. The loop is a body node, so it is not moved.
			c, err := pd.loc.locate(del[0])
			if err != nil {
				return err
			}
			inner, ok := c.Object.(*microflows.LoopedActivity)
			if !ok {
				return cannotSplice("the stored %s is not drawn as a loop", describeAt(del[0]))
			}
			if !declaredMatches(loopShell(ins[0]), loopShell(del[0])) {
				return bodyRefusal(lc, at, fmt.Sprintf("the %s in it is drawn elsewhere or its connectors are redrawn, and "+
					"the splice moves no node inside a loop body", describeAt(del[0])))
			}
			return pd.inLoopBody(c, inner, del[0], loopStatements(ins[0]), loopStatements(del[0]))
		}
		if replaceableInPlace(ins[0], del[0]) {
			// One activity changed: replaced in place, drawn where it is
			// (ako/mxcli#1106). The stored notes stay on the replacement.
			c, err := pd.loc.locate(del[0])
			if err != nil {
				return err
			}
			body, _, err := keepStoredNotes(del[0], ins)
			if err != nil {
				return err
			}
			pd.add(ast.AlterFlowReplace, c, body)
			return nil
		}
	}
	// A run of activities each changed in its own place: one at a time, as
	// at top level (ako/mxcli#1761).
	if pairwiseSameKind(ins, del) {
		for i := range ins {
			if err := pd.bodyGap(lc, at, ins[i:i+1], stored, si+i, si+i+1); err != nil {
				return err
			}
		}
		return nil
	}
	if i, j, ok := loopPair(ins, del); ok {
		if err := pd.bodyGap(lc, at, ins[:i], stored, si, si+j); err != nil {
			return err
		}
		if err := pd.bodyGap(lc, at, ins[i:i+1], stored, si+j, si+j+1); err != nil {
			return err
		}
		return pd.bodyGap(lc, at, ins[i+1:], stored, si+j+1, sEnd)
	}
	for _, st := range del {
		if what := notPlainInBody(st); what != "" {
			return bodyRefusal(lc, at, fmt.Sprintf("%s in it (the %s) is changed or taken out", what, describeAt(st)))
		}
	}
	for _, st := range ins {
		if what := notPlainInBody(st); what != "" {
			return bodyRefusal(lc, at, fmt.Sprintf("%s is added to it", what))
		}
	}
	cands := make([]mfmutator.Candidate, len(del))
	for i, st := range del {
		c, err := pd.loc.locate(st)
		if err != nil {
			return err
		}
		cands[i] = c
	}
	if len(del) == 0 {
		op, c, err := insertAnchor(pd.loc, stored, si, sEnd)
		if err != nil {
			return err
		}
		pd.add(op, c, ins)
		return nil
	}
	rest, restStmts := cands, del
	if len(ins) > 0 {
		body, replaceNotes, err := keepStoredNotes(del[0], ins)
		if err != nil {
			return err
		}
		pd.add(ast.AlterFlowReplace, cands[0], body)
		pd.ops[len(pd.ops)-1].ReplaceNotes = replaceNotes
		rest, restStmts = cands[1:], del[1:]
	}
	for i, c := range rest {
		var notes []ast.MicroflowAnnotation
		if ann := statementAnnotations(restStmts[i]); ann != nil {
			notes = ann.Notes
		}
		for _, n := range notes {
			if n.Label != "" {
				return cannotSplice("the %s dropped at (%d, %d) carries an annotation shared with another activity (id: %s), "+
					"which dropping it would take from that activity too",
					statementKind(restStmts[i]), c.Object.GetPosition().X, c.Object.GetPosition().Y, n.Label)
			}
		}
		pd.add(ast.AlterFlowDrop, c, nil)
		pd.ops[len(pd.ops)-1].ReplaceNotes = len(notes) > 0
	}
	return nil
}

// notPlainInBody says what a statement is when it is not a plain activity —
// one the splice inserts, drops or replaces inside a loop body — and "" when
// it is one. Each of the others changes where the body's paths split, end or
// rejoin.
func notPlainInBody(st ast.MicroflowStatement) string {
	switch st.(type) {
	case *ast.IfStmt, *ast.EnumSplitStmt, *ast.InheritanceSplitStmt:
		return "a decision"
	case *ast.LoopStmt, *ast.WhileStmt:
		return "a loop"
	case *ast.ReturnStmt:
		return "a return"
	case *ast.RaiseErrorStmt:
		return "a raise error"
	case *ast.BreakStmt:
		return "a break"
	case *ast.ContinueStmt:
		return "a continue"
	case *ast.MergeStmt, *ast.JoinStmt:
		return "a merge"
	}
	if customErrorHandler(st) {
		return "an activity with an error handler"
	}
	return ""
}

// plainInBodyNote is the part of every in-body refusal that says what the
// splice does make there.
const plainInBodyNote = "inside a loop body the splice inserts, drops and replaces plain activities only — not a " +
	"decision, a loop, a return, break or continue, a raise error, or an activity with an error handler"

// bodyRefusal refuses a change inside the body of the stored loop lc,
// described as at, saying why; the refusal names `replace body of` the loop,
// which makes any change of a body (ako/mxcli#1693).
func bodyRefusal(lc mfmutator.Candidate, at ast.MicroflowStatement, why string) error {
	return &notSpliceable{
		reason:     fmt.Sprintf("the %s changes inside its body: %s; %s", describeAt(at), why, plainInBodyNote),
		loopHandle: lc.Statement,
	}
}

// pinBody records the points the declared body of the stored loop lc states
// its nodes are drawn at, in the loop box's coordinates: each statement's
// @position and its notes' positions. Making room in that body then moves no
// node away from where the script draws it — under mdl 1, as at top level
// (statedPoints).
func (pd *patchDiff) pinBody(loop model.ID, declared []ast.MicroflowStatement) {
	for _, st := range declared {
		ann := statementAnnotations(st)
		if ann == nil {
			continue
		}
		if ann.Position != nil {
			pd.bodyPins = append(pd.bodyPins, bodyPin{loop: loop, at: model.Point{X: ann.Position.X, Y: ann.Position.Y}})
		}
		for _, n := range ann.Notes {
			if n.Position != nil {
				pd.bodyPins = append(pd.bodyPins, bodyPin{loop: loop, at: model.Point{X: n.Position.X, Y: n.Position.Y}})
			}
		}
	}
}

// bodyPin is a point a declared loop body states one of its nodes is drawn
// at, in the box of the stored loop.
type bodyPin struct {
	loop model.ID
	at   model.Point
}

// replaceableInPlace reports whether a declared statement is the stored one
// changed so that it can be swapped in place inside a loop body: the same kind
// of plain activity — not a decision, a return, a break or continue, nor one
// with a custom error handler — whose annotations (where it is drawn, its
// connectors, caption, colour and notes) state nothing the stored one does
// not have (ako/mxcli#1106).
func replaceableInPlace(declared, stored ast.MicroflowStatement) bool {
	if statementKind(declared) != statementKind(stored) || sameExceptPositions(declared, stored) {
		return false
	}
	if notPlainInBody(declared) != "" || notPlainInBody(stored) != "" {
		return false
	}
	if da := statementAnnotations(declared); da != nil {
		sa := statementAnnotations(stored)
		if sa == nil {
			sa = &ast.ActivityAnnotations{}
		}
		if !declaredMatches(da, sa) {
			return false
		}
	}
	return true
}

// pairwiseSameKind reports whether a run of more than one changed statement
// is each declared statement standing where the stored one of the same kind
// stood: as many of each, every pair the same kind of plain activity (not a
// decision, a loop, a return, a break or continue, nor one with an error
// handler). Each pair is then a change of one activity, which the splice
// edits in place when the activity stays the same kind (ako/mxcli#1761).
func pairwiseSameKind(ins, del []ast.MicroflowStatement) bool {
	if len(ins) < 2 || len(ins) != len(del) {
		return false
	}
	for i := range ins {
		if statementKind(ins[i]) != statementKind(del[i]) || notPlainInBody(ins[i]) != "" || notPlainInBody(del[i]) != "" {
			return false
		}
	}
	return true
}

// customErrorHandler reports whether a statement has an `on error` with a
// body: a handler path of its own, drawn in the body beside it.
func customErrorHandler(st ast.MicroflowStatement) bool {
	v := reflectElem(st)
	if !v.IsValid() {
		return false
	}
	f := v.FieldByName("ErrorHandling")
	if !f.IsValid() {
		return false
	}
	eh, _ := f.Interface().(*ast.ErrorHandlingClause)
	return eh != nil && len(eh.Body) > 0
}

// loopStatements is the body of a loop or while statement.
func loopStatements(st ast.MicroflowStatement) []ast.MicroflowStatement {
	switch l := st.(type) {
	case *ast.LoopStmt:
		return l.Body
	case *ast.WhileStmt:
		return l.Body
	}
	return nil
}

// describeAt names a stored statement and where it is drawn, for a message.
func describeAt(st ast.MicroflowStatement) string {
	if p := statementAnnotations(st); p != nil && p.Position != nil {
		return fmt.Sprintf("%s at (%d, %d)", statementKind(st), p.Position.X, p.Position.Y)
	}
	return statementKind(st)
}

// keepStoredNotes prepares the declared statements that replace a stored one.
// When the declared statement carries the stored activity's notes, the splice
// keeps the stored notes and attaches them to the replacement's first
// activity, so they are taken off the declared statement — or the builder
// would draw each one a second time.
//
// When it carries other notes — one added, reworded or taken off — the
// declared statement keeps them, the builder draws them, and replaceNotes says
// the stored notes go out with the activity (ako/mxcli#859): the statement
// states the activity's notes, like the rest of it. A note the stored flow
// shares with another activity (describe gives it an id) is refused: changing
// it here would change it there too.
func keepStoredNotes(stored ast.MicroflowStatement, declared []ast.MicroflowStatement) (out []ast.MicroflowStatement, replaceNotes bool, err error) {
	var storedNotes []ast.MicroflowAnnotation
	if ann := statementAnnotations(stored); ann != nil {
		storedNotes = ann.Notes
	}
	if len(storedNotes) == 0 {
		return declared, false, nil
	}
	var declaredNotes []ast.MicroflowAnnotation
	if ann := statementAnnotations(declared[0]); ann != nil {
		declaredNotes = ann.Notes
	}
	if !declaredMatches(declaredNotes, storedNotes) {
		for _, n := range append(append([]ast.MicroflowAnnotation(nil), storedNotes...), declaredNotes...) {
			if n.Label != "" {
				return nil, false, cannotSplice("the annotations on the replaced %s change, and one of them is shared "+
					"with another activity (id: %s); the splice changes the notes of one activity only",
					statementKind(stored), n.Label)
			}
		}
		return declared, true, nil
	}
	out = append([]ast.MicroflowStatement(nil), declared...)
	out[0] = withAnnotations(declared[0], func(a *ast.ActivityAnnotations) { a.Notes = nil })
	return out, false, nil
}

// freeNoteChange names the first free annotation that differs between the
// declared and the stored flow, so a refusal says which note to look at
// (mendixlabs/mxcli#1505). The text is quoted with escapes, which shows
// a difference in invisible characters for what it is.
func freeNoteChange(declared, stored []ast.MicroflowAnnotation) string {
	quote := func(text string) string {
		if r := []rune(text); len(r) > 40 {
			text = string(r[:40]) + "…"
		}
		return strconv.Quote(text)
	}
	for i := 0; i < len(declared) || i < len(stored); i++ {
		switch {
		case i >= len(stored):
			return "note " + quote(declared[i].Text) + " is not in the stored flow"
		case i >= len(declared):
			return "note " + quote(stored[i].Text) + " is not in the script"
		case !declaredMatches(declared[i], stored[i]):
			if declared[i].Text != stored[i].Text {
				return "note " + quote(stored[i].Text) + " is now " + quote(declared[i].Text)
			}
			return "note " + quote(stored[i].Text) + " is drawn elsewhere"
		}
	}
	return "their order differs"
}

// withoutFreeNotes returns the statements with their free annotations taken
// off (as copies; the script's own statements are not modified, since the
// rebuild may still need them), and the free annotations in order.
// misplacedNote explains a free note the script adds that is a note the
// stored flow attaches to an activity: an @annotation written before a
// statement's @position (or @caption, @color, @anchor, @excluded) is a free
// note, attached to nothing, because that is where describe prints one — so a
// note restated above its activity's @position reads as a new note on the
// canvas and the activity's own note as taken off (mendixlabs/mxcli#1517).
// stored is the stored body with its free notes still on it; "" when no added
// free note is attached in the stored flow.
func misplacedNote(declared, stored []ast.MicroflowAnnotation, storedBody []ast.MicroflowStatement) string {
	isFree := map[string]bool{}
	for _, n := range stored {
		isFree[n.Text] = true
	}
	attached := map[string]ast.MicroflowStatement{}
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, st := range stmts {
			if ann := statementAnnotations(st); ann != nil {
				for _, n := range ann.Notes {
					if _, seen := attached[n.Text]; !seen && n.Text != "" {
						attached[n.Text] = st
					}
				}
			}
			if ifs, ok := st.(*ast.IfStmt); ok {
				walk(ifs.ThenBody)
				walk(ifs.ElseBody)
			}
			if eh := errorHandling(st); eh != nil {
				walk(eh.Body)
			}
			walk(loopStatements(st))
		}
	}
	walk(storedBody)
	for _, n := range declared {
		if isFree[n.Text] {
			continue
		}
		if st, ok := attached[n.Text]; ok {
			return fmt.Sprintf("note %s is written before @position on its statement, which makes it a free note on the "+
				"canvas, attached to nothing (where describe prints one); the stored note is attached to the %s — to keep "+
				"it attached, write it after @position", strconv.Quote(n.Text), describeAt(st))
		}
	}
	return ""
}

func withoutFreeNotes(stmts []ast.MicroflowStatement) ([]ast.MicroflowStatement, []ast.MicroflowAnnotation) {
	var free []ast.MicroflowAnnotation
	out := make([]ast.MicroflowStatement, len(stmts))
	for i, st := range stmts {
		out[i] = st
		if ann := statementAnnotations(st); ann != nil && len(ann.FreeNotes) > 0 {
			free = append(free, ann.FreeNotes...)
			out[i] = withAnnotations(st, func(a *ast.ActivityAnnotations) { a.FreeNotes = nil })
		}
	}
	return out, free
}

// withAnnotations returns a shallow copy of st whose annotations are a copy
// edited by edit. st itself is left as it was.
func withAnnotations(st ast.MicroflowStatement, edit func(*ast.ActivityAnnotations)) ast.MicroflowStatement {
	v := reflectElem(st)
	if !v.IsValid() {
		return st
	}
	cp := reflect.New(v.Type())
	cp.Elem().Set(v)
	f := cp.Elem().FieldByName("Annotations")
	ann, ok := f.Interface().(*ast.ActivityAnnotations)
	if !ok || ann == nil {
		return st
	}
	annCopy := *ann
	edit(&annCopy)
	f.Set(reflect.ValueOf(&annCopy))
	out, ok := cp.Interface().(ast.MicroflowStatement)
	if !ok {
		return st
	}
	return out
}

// insertAnchor chooses where a run of new statements goes: after the stored
// activity before it when that is an activity (one flow leaves it), else
// before the stored statement after it. gapStart is the index of the first
// stored statement after the run's predecessor; next is the index of the
// matched statement after the run.
func insertAnchor(loc *storedLocator, stored []ast.MicroflowStatement, gapStart, next int) (ast.AlterFlowOpKind, mfmutator.Candidate, error) {
	if gapStart > 0 {
		if c, err := loc.locate(stored[gapStart-1]); err == nil {
			if _, ok := c.Object.(*microflows.ActionActivity); ok {
				return ast.AlterFlowInsertAfter, c, nil
			}
		}
	}
	if next < len(stored) {
		c, err := loc.locate(stored[next])
		if err != nil {
			return "", mfmutator.Candidate{}, err
		}
		return ast.AlterFlowInsertBefore, c, nil
	}
	if gapStart > 0 {
		c, err := loc.locate(stored[gapStart-1])
		if err != nil {
			return "", mfmutator.Candidate{}, err
		}
		return ast.AlterFlowInsertAfter, c, nil
	}
	return "", mfmutator.Candidate{}, cannotSplice("the stored flow has no statement to insert next to")
}

// lcsStatements pairs declared and stored statements that match, as a longest
// common subsequence; each pair is {declared index, stored index}, ascending.
//
// A statement that matches a stored one except where it is drawn pairs with
// it too — it is that node, moved (ako/mxcli#818) — but an exact match weighs
// more, so of two alike statements the one the script left in place keeps its
// node.
//
// An `if` whose condition changes weighs least: of a new `if` and a restated
// one before the same stored decision, the restated one — the same condition
// — is that decision. Weighed alike, the tie went to the earlier pair: the new
// if took the stored decision over and the restated one was inserted as a new
// node, drawn on top of it (mendixlabs/mxcli#1598, case 6).
func lcsStatements(declared, stored []ast.MicroflowStatement) [][2]int {
	n, m := len(declared), len(stored)
	w := make([][]int, n)
	for i := range declared {
		w[i] = make([]int, m)
		for j := range stored {
			switch {
			case declaredMatches(declared[i], stored[j]):
				w[i][j] = 4
			case sameExceptPositions(declared[i], stored[j]):
				w[i][j] = 2
			case sameBlockShell(declared[i], stored[j]):
				w[i][j] = 2
			case sameHandlerShell(declared[i], stored[j]):
				// An activity with the same custom error handler kind is that
				// activity, changed itself or inside its handler
				// (mendixlabs/mxcli#1489): a change beside it must not merge
				// with it into a replace, which would take the handler out.
				w[i][j] = 2
			case sameLoopShell(declared[i], stored[j]):
				// A loop with a change in its body is that loop: a change
				// beside it must not merge with it into a replace, which would
				// rebuild it (ako/mxcli#1106, ako/mxcli#1693). Whether the
				// change can be made inside it is inLoopBody's to say.
				w[i][j] = 2
			case conditionChanged(declared[i], stored[j]):
				w[i][j] = 1
			}
		}
	}
	// l[i][j] is the best weight of declared[i:] and stored[j:].
	l := make([][]int, n+1)
	for i := range l {
		l[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			best := max(l[i+1][j], l[i][j+1])
			if w[i][j] > 0 {
				best = max(best, w[i][j]+l[i+1][j+1])
			}
			l[i][j] = best
		}
	}
	var pairs [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case w[i][j] > 0 && l[i][j] == w[i][j]+l[i+1][j+1]:
			pairs = append(pairs, [2]int{i, j})
			i++
			j++
		case l[i+1][j] >= l[i][j+1]:
			i++
		default:
			j++
		}
	}
	return pairs
}

// storedLocator finds the stored activity a statement of the stored flow's
// description stands for. describe prints each activity's @position, which is
// its RelativeMiddlePoint; among the top-level objects that is unique in any
// flow drawn so that nodes do not sit on top of each other.
type storedLocator struct {
	byPos map[model.Point][]mfmutator.Candidate
	cands []mfmutator.Candidate
}

func newStoredLocator(a *alterFlowContext) *storedLocator {
	return locatorOver(a.cands, a.mf.ObjectCollection)
}

// within is the locator of a stored loop's body: describe prints a body
// statement's @position relative to the loop box, unique among the body's
// own objects but not across the flow.
func (l *storedLocator) within(loop *microflows.LoopedActivity) *storedLocator {
	return locatorOver(l.cands, loop.ObjectCollection)
}

func locatorOver(cands []mfmutator.Candidate, oc *microflows.MicroflowObjectCollection) *storedLocator {
	top := map[model.ID]bool{}
	if oc != nil {
		for _, obj := range oc.Objects {
			top[obj.GetID()] = true
		}
	}
	loc := &storedLocator{byPos: map[model.Point][]mfmutator.Candidate{}, cands: cands}
	for _, c := range cands {
		if top[c.ID] && c.Object != nil {
			p := c.Object.GetPosition()
			loc.byPos[p] = append(loc.byPos[p], c)
		}
	}
	return loc
}

func (l *storedLocator) locate(st ast.MicroflowStatement) (mfmutator.Candidate, error) {
	ann := statementAnnotations(st)
	if ann == nil || ann.Position == nil {
		return mfmutator.Candidate{}, cannotSplice("the stored statement %q has no activity to address (a join, merge or end of a branch)", statementKind(st))
	}
	p := model.Point{X: ann.Position.X, Y: ann.Position.Y}
	switch cs := l.byPos[p]; len(cs) {
	case 1:
		return cs[0], nil
	case 0:
		return mfmutator.Candidate{}, cannotSplice("no top-level activity is drawn at (%d, %d), where the stored %s is", p.X, p.Y, statementKind(st))
	default:
		return mfmutator.Candidate{}, cannotSplice("%d activities are drawn at (%d, %d); cannot tell which one the stored %s is", len(cs), p.X, p.Y, statementKind(st))
	}
}

// statementAnnotations returns a statement's annotations: every microflow
// statement that has them carries them in a field named Annotations.
func statementAnnotations(st ast.MicroflowStatement) *ast.ActivityAnnotations {
	v := reflectElem(st)
	if !v.IsValid() {
		return nil
	}
	f := v.FieldByName("Annotations")
	if !f.IsValid() {
		return nil
	}
	ann, _ := f.Interface().(*ast.ActivityAnnotations)
	return ann
}

func statementKind(st ast.MicroflowStatement) string {
	return strings.TrimSuffix(strings.TrimPrefix(fmt.Sprintf("%T", st), "*ast."), "Stmt")
}

// targetLabel is how an operation's target is named in a message: its alter
// handle when it has one, else its statement.
func targetLabel(c mfmutator.Candidate) string {
	if c.OutputVariable != "" {
		return "$" + c.OutputVariable
	}
	if c.Statement != "" {
		return c.Statement
	}
	return string(c.ID)
}

// patchSummary says what a patch did, e.g. "spliced: 1 inserted, 2 moved;
// set: ExportLevel", or "" when it did nothing. A replace that edited its
// stored node in place (edited) — which keeps its $ID, and so reads in Studio
// Pro's version control as that node changed — is told apart from one that
// put new elements in its place, which reads as one removed and another added
// (ako/mxcli#1761).
func patchSummary(ops []*ast.AlterFlowOperation, edited map[*ast.AlterFlowOperation]bool, moves []flowMove, set []string) string {
	var ins, edit, rep, drop int
	for _, op := range ops {
		switch op.Op {
		case ast.AlterFlowInsertAfter, ast.AlterFlowInsertBefore:
			ins++
		case ast.AlterFlowReplace, ast.AlterFlowReplaceBody:
			if edited[op] {
				edit++
			} else {
				rep++
			}
		case ast.AlterFlowDrop:
			drop++
		}
	}
	var parts []string
	for _, p := range []struct {
		n    int
		verb string
	}{{ins, "inserted"}, {edit, "edited in place"}, {rep, "replaced with new element IDs"}, {drop, "dropped"}, {len(moves), "moved"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.verb))
		}
	}
	var out []string
	if len(parts) > 0 {
		out = append(out, "spliced: "+strings.Join(parts, ", "))
	}
	if len(set) > 0 {
		out = append(out, "set: "+strings.Join(set, ", "))
	}
	return strings.Join(out, "; ")
}
