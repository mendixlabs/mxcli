// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// entityAccessCallRule is the check-time counterpart of MxBuild's CE0114
// (mendixlabs/mxcli#1500):
//
//	"A microflow that does not apply entity access can only call microflows
//	 that also do not apply entity access."
//	"A microflow that does not apply entity access can only call rules that
//	 also do not apply entity access."
//
// Measured with mxbuild 11.13.0 and 11.14.0 on fresh projects, one caller per
// shape:
//
//	microflow off -> call microflow on            -> CE0114 ("microflows")
//	microflow off -> decision on a rule that is on -> CE0114 ("rules")
//	microflow on  -> microflow off, on -> on, off -> off  -> nothing
//	nanoflow (no such setting) -> microflow on     -> nothing
//	EXCLUDED microflow off -> microflow on         -> nothing
//
// at security level Off and Prototype alike: unlike CE0106 it is not a
// security-level check, so it is always an error.
//
// It is reported by `check -p` only, which predicts the build; exec does not
// refuse on it, so no script changes meaning and there is nothing to gate on the
// language version — the same call MDL-SEC21 made for CE0106.
const entityAccessCallRule = "MDL-SEC22"

// eaFlow is one microflow's or rule's simulated "apply entity access" state.
type eaFlow struct {
	kind     string // "microflow" or "rule"
	name     string
	exists   bool
	excluded bool
	ea       bool
	// origin says, for the message, where the setting came from.
	origin string
	calls  []eaCall
}

// eaCall is a call from a microflow body: a call-microflow activity or a
// decision on a rule.
type eaCall struct {
	kind string // "microflow" or "rule"
	name string
}

// CheckEntityAccessCalls reports MDL-SEC22 for the script.
func (e *Executor) CheckEntityAccessCalls(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	return CheckEntityAccessCalls(e.newExecContext(context.Background()), prog)
}

// CheckEntityAccessCalls simulates the script's microflows and rules and reports
// each call the script leaves from a microflow that does not apply entity access
// to a microflow or rule that does. Only what the script CAUSES is reported: a
// call already in that state before the script is the project's existing
// problem. A callee neither the project nor the script holds is not judged.
func CheckEntityAccessCalls(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || prog == nil || !ctx.Connected() || !scriptTouchesEntityAccessCalls(prog) {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return nil
	}
	sim := &eaSim{flows: map[string]*eaFlow{}}
	for _, typ := range []string{"Microflows$Microflow", "Microflows$Rule"} {
		units, err := ctx.Backend.ListRawUnitsByType(typ)
		if err != nil {
			continue
		}
		kind := "microflow"
		if typ == "Microflows$Rule" {
			kind = "rule"
		}
		for _, u := range units {
			if u.Type != typ || len(u.Contents) == 0 {
				continue
			}
			st, ok := storedEAFlow(u.Contents, kind == "microflow")
			if !ok {
				continue
			}
			qn := h.GetQualifiedName(u.ContainerID, st.name)
			if prev, ok := sim.flows[qn]; ok && !prev.excluded {
				continue // a live namesake wins over excluded ones (#914)
			}
			f := &eaFlow{kind: kind, name: qn, exists: true, excluded: st.excluded, ea: st.ea, calls: st.calls}
			sim.flows[qn] = f
		}
	}

	before := sim.violating()
	for _, stmt := range prog.Statements {
		sim.apply(stmt)
	}
	after := sim.violating()

	var keys []string
	for k := range after {
		if _, had := before[k]; !had {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var out []linter.Violation
	for _, k := range keys {
		edge := after[k]
		caller, callee := sim.flows[edge.caller], sim.flows[edge.callee]
		qn := splitQualifiedName(caller.name)
		out = append(out, linter.Violation{
			RuleID:   entityAccessCallRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"microflow %s does not apply entity access (%s) but calls %s %s, which does (%s) — MxBuild reports CE0114 \"A microflow that does not apply entity access can only call %ss that also do not apply entity access.\"",
				caller.name, eaOrigin(caller), callee.kind, callee.name, eaOrigin(callee), callee.kind),
			Suggestion: fmt.Sprintf(
				"apply entity access in the caller (`@applyentityaccess` before `create or modify microflow %s`), or call a %s that does not apply it — turning it off in %s (`@applyentityaccess(false)`) widens what it may read and write for every caller",
				caller.name, callee.kind, callee.name),
			Location: linter.Location{Module: qn.Module, DocumentType: "microflow", DocumentName: qn.Name},
		})
	}
	return out
}

func eaOrigin(f *eaFlow) string {
	v := "@applyentityaccess(false)"
	if f.ea {
		v = "@applyentityaccess(true)"
	}
	if f.origin == "" {
		return v + " as stored"
	}
	return v + " " + f.origin
}

// scriptTouchesEntityAccessCalls is the cheap gate: only a script that writes a
// microflow or rule, or alters a microflow, can add a call or change a setting.
func scriptTouchesEntityAccessCalls(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt, *ast.CreateRuleStmt:
			return true
		case *ast.AlterFlowStmt:
			if !s.Nanoflow {
				return true
			}
		}
	}
	return false
}

// storedEAState is what the simulation reads of one stored microflow or rule.
type storedEAState struct {
	name         string
	excluded, ea bool
	calls        []eaCall
}

// eaCallMarkers are the $Type values collectRawEntityAccessCalls looks for. A
// document that holds neither string holds no call.
var eaCallMarkers = [][]byte{[]byte("Microflows$MicroflowCall"), []byte("Microflows$RuleCall")}

// eaDocumentsDecoded counts the stored documents decoded in full to read
// their calls. For tests.
var eaDocumentsDecoded atomic.Int64

// storedEAFlow reads one stored microflow or rule for the simulation. ok is
// false for contents that are not a document.
//
// The check runs once per script and reads every microflow of the project, so
// what it costs per document is what `check` costs per project unit before it
// has looked at a statement (ako/mxcli#2160, mendixlabs/mxcli#1842). The name
// and the two flags are top-level values, read where they lie. The calls are
// anywhere in the body, and finding them takes the whole document decoded:
// that is done only for a document whose bytes hold one of the call types'
// names, which a document with a call always does, since a $Type is stored as
// its name.
func storedEAFlow(contents []byte, withCalls bool) (storedEAState, bool) {
	raw := bson.Raw(contents)
	if raw.Validate() != nil {
		return storedEAState{}, false
	}
	var st storedEAState
	st.name, _ = raw.Lookup("Name").StringValueOK()
	st.excluded, _ = raw.Lookup("Excluded").BooleanOK()
	st.ea, _ = raw.Lookup("ApplyEntityAccess").BooleanOK()
	if !withCalls {
		return st, true
	}
	for _, marker := range eaCallMarkers {
		if bytes.Contains(contents, marker) {
			var doc bson.D
			if err := bson.Unmarshal(contents, &doc); err != nil {
				return storedEAState{}, false
			}
			eaDocumentsDecoded.Add(1)
			st.calls = collectRawEntityAccessCalls(doc)
			break
		}
	}
	return st, true
}

// storedEAFlowDecoded is storedEAFlow as it was: the whole document decoded,
// whatever it holds. It is what storedEAFlow is compared with.
func storedEAFlowDecoded(contents []byte, withCalls bool) (storedEAState, bool) {
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return storedEAState{}, false
	}
	var st storedEAState
	st.name, _ = bsonField(doc, "Name").(string)
	st.excluded, _ = bsonField(doc, "Excluded").(bool)
	st.ea, _ = bsonField(doc, "ApplyEntityAccess").(bool)
	if withCalls {
		st.calls = collectRawEntityAccessCalls(doc)
	}
	return st, true
}

// collectRawEntityAccessCalls reads the calls out of a stored microflow: the
// qualified name under `Microflow` in a Microflows$MicroflowCall (call
// activities, queued or not) or a Microflows$RuleCall (a rule split).
func collectRawEntityAccessCalls(doc bson.D) []eaCall {
	var out []eaCall
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case bson.D:
			typ, _ := bsonField(t, "$Type").(string)
			kind := ""
			switch typ {
			case "Microflows$MicroflowCall":
				kind = "microflow"
			case "Microflows$RuleCall":
				kind = "rule"
			}
			if kind != "" {
				if s, ok := bsonField(t, "Microflow").(string); ok && s != "" {
					out = append(out, eaCall{kind, s})
				}
			}
			for _, e := range t {
				walk(e.Value)
			}
		case bson.A:
			for _, x := range t {
				walk(x)
			}
		}
	}
	walk(doc)
	return out
}

type eaSim struct {
	flows map[string]*eaFlow
}

type eaEdge struct {
	caller, callee string
}

// violating returns the calls that currently break CE0114, keyed by
// caller -> callee.
func (s *eaSim) violating() map[string]eaEdge {
	out := map[string]eaEdge{}
	for _, f := range s.flows {
		if f.kind != "microflow" || !f.exists || f.excluded || f.ea {
			continue
		}
		for _, c := range f.calls {
			t, ok := s.flows[c.name]
			// An unknown, dropped or excluded callee, or a name of the other
			// kind, cannot be judged here.
			if !ok || !t.exists || t.excluded || t.kind != c.kind || !t.ea {
				continue
			}
			out[f.name+"->"+c.kind+":"+c.name] = eaEdge{f.name, c.name}
		}
	}
	return out
}

func (s *eaSim) write(kind string, name ast.QualifiedName, ifNotExists, excluded bool, stated *bool, body []ast.MicroflowStatement) {
	qn := name.String()
	f, ok := s.flows[qn]
	if ok && f.exists && ifNotExists {
		return
	}
	if !ok || !f.exists {
		// A new document, or one this script dropped: nothing is carried.
		f = &eaFlow{kind: kind, name: qn, origin: "by default, as a new " + kind}
		s.flows[qn] = f
	} else {
		// create or modify (or a plain create exec refuses): exclusion and an
		// unstated setting are carried (#914, carriedApplyEntityAccess).
		excluded = excluded || f.excluded
	}
	f.kind = kind
	f.exists = true
	f.excluded = excluded
	f.ea = carriedApplyEntityAccess(stated, f.ea)
	if stated != nil {
		f.origin = "in this script"
	}
	f.calls = nil
	if kind == "microflow" {
		f.calls = s.collect(body, nil)
	}
}

// collect reads the calls from a script body. A decision is a rule call only
// when the name is a rule at this point of the script — exec asks IsRule when
// it builds the flow, so a rule created later makes it an expression split.
func (s *eaSim) collect(stmts []ast.MicroflowStatement, out []eaCall) []eaCall {
	rule := func(cond ast.Expression) {
		if cond == nil {
			return
		}
		call := unwrapParenCall(cond)
		if call == nil || !strings.Contains(call.Name, ".") {
			return
		}
		if t, ok := s.flows[call.Name]; ok && t.exists && t.kind == "rule" {
			out = append(out, eaCall{"rule", call.Name})
		}
	}
	for _, stmt := range stmts {
		switch st := stmt.(type) {
		case *ast.CallMicroflowStmt:
			if st.MicroflowName.Module != "" {
				out = append(out, eaCall{"microflow", st.MicroflowName.String()})
			}
		case *ast.IfStmt:
			rule(st.Condition)
			out = s.collect(st.ThenBody, out)
			out = s.collect(st.ElseBody, out)
		case *ast.EnumSplitStmt:
			rule(st.Condition)
			for _, c := range st.Cases {
				out = s.collect(c.Body, out)
			}
			out = s.collect(st.ElseBody, out)
		case *ast.InheritanceSplitStmt:
			for _, c := range st.Cases {
				out = s.collect(c.Body, out)
			}
			out = s.collect(st.ElseBody, out)
		case *ast.LoopStmt:
			out = s.collect(st.Body, out)
		case *ast.WhileStmt:
			out = s.collect(st.Body, out)
		}
		if eh := getErrorHandlerBody(stmt); eh != nil {
			out = s.collect(eh, out)
		}
	}
	return out
}

func (s *eaSim) drop(qn string) {
	if f, ok := s.flows[qn]; ok {
		f.exists = false
		f.calls = nil
	}
}

func (s *eaSim) apply(stmt ast.Statement) {
	switch st := stmt.(type) {
	case *ast.CreateMicroflowStmt:
		s.write("microflow", st.Name, st.IfNotExists, st.Excluded, st.ApplyEntityAccess, st.Body)
	case *ast.CreateRuleStmt:
		s.write("rule", st.Name, st.IfNotExists, st.Excluded, st.ApplyEntityAccess, nil)
	case *ast.DropMicroflowStmt:
		s.drop(st.Name.String())
	case *ast.DropRuleStmt:
		s.drop(st.Name.String())
	case *ast.AlterFlowStmt:
		if st.Nanoflow {
			return
		}
		// Only additions are modelled: an inserted or replacing fragment adds
		// its calls. A call it removes was already counted before the script,
		// so it cannot produce a report of its own.
		f, ok := s.flows[st.Name.String()]
		if !ok || !f.exists || f.kind != "microflow" {
			return
		}
		for _, op := range st.Operations {
			f.calls = s.collect(op.Body, f.calls)
		}
	}
}
