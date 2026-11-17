// SPDX-License-Identifier: Apache-2.0

// Package marketplace implements drift detection for installed marketplace
// modules: capture what a module looks like now, capture what the published
// package looks like, and report which elements the user has changed.
//
// The comparison is on DESCRIBE output, not on BSON. A path-level BSON diff of
// an *untouched* module against its own published package differs in ~15,000
// paths (PROPOSAL_marketplace_module_upgrade.md §3), because the installed copy
// carries whole subtrees the package does not. DESCRIBE discards exactly those
// artefacts — $IDs, storage envelopes, widget-internal representation — so two
// elements that describe alike are the same element as far as an author is
// concerned.
package marketplace

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
)

// ElementKey identifies an element within a module. Name+type is a sound join
// key: measured against a blank project's Administration module and its own
// published .mpk, every one of 27 elements matched with zero orphans on either
// side (§3).
type ElementKey struct {
	// Type is the catalog `objects` view ObjectType, e.g. PAGE or MICROFLOW.
	Type string
	// Name is the element name, unqualified — the module name is not part of the
	// key because the two sides are the same module by construction.
	Name string
}

func (k ElementKey) String() string { return k.Type + " " + k.Name }

// Element is one described element of a module.
type Element struct {
	Key ElementKey
	// MDL is the DESCRIBE output, normalised for comparison. Empty when Err is set.
	MDL string
	// Err records why the element could not be described. An element with Err set
	// is reported as unknown and never as unchanged — treating an un-describable
	// element as clean is the one failure mode that would make this dangerous
	// rather than merely incomplete.
	Err string
}

// Describable reports whether the element could be read at all.
func (e Element) Describable() bool { return e.Err == "" }

// Conclusive reports whether this element's DESCRIBE output is good enough to
// carry a "you edited this" verdict.
//
// Describable() is not enough. Some types describe *successfully* into output
// that says nothing about the element: a snippet whose body comes out as `{ }`,
// a building block that renders under "-- Building blocks are read-only; they
// cannot be created via MDL." Two such renderings can still differ — a resolved
// name that resolves in one project and not the other is enough — and the
// difference is then an artefact of DESCRIBE, not an edit.
//
// Reporting those as Modified is worse than reporting nothing: on an untouched
// blank app, `diff` accused the user of editing an Atlas_Core snippet and an
// Atlas_Web_Content building block, and `--save-edits` wrote MDL that would have
// *emptied* the snippet if replayed (mxcli-chat FINDINGS §16). Unknown is the
// honest verdict — `verified:false` already tells the caller that "no
// modifications found" is not a conclusion.
//
// The two signals are deliberately about the *output*, not a list of types: a
// type list goes stale the moment a describe handler improves, while "this text
// contains nothing to compare" stays true by construction.
func (e Element) Conclusive() (bool, string) {
	if !e.Describable() {
		return false, e.Err
	}
	if declaredReadOnly(e.MDL) {
		return false, "DESCRIBE output is informational for this type, not re-executable MDL"
	}
	if emptyBody(e.MDL) {
		return false, "DESCRIBE produced no body, so a difference here is not evidence of an edit"
	}
	if sourceOmitted(e.MDL) {
		return false, "the action's source file was not on disk, so its body could not be compared"
	}
	return true, ""
}

// sourceOmitted spots the placeholder DESCRIBE prints for a JavaScript or Java
// action whose source file it could not read (jsSourceOmittedBody /
// javaSourceOmittedBody in mdl/executor). That body says nothing about the
// code, so a difference against it is a missing file, not an edit — the shape
// of mendixlabs/mxcli#1441, where a cached reference without sources made every
// action of an untouched module read as "changed locally". Matching the
// handlers' own shared wording keeps the two in step.
func sourceOmitted(mdl string) bool {
	return strings.Contains(mdl, "source not available from this project; body omitted by DESCRIBE.")
}

// declaredReadOnly spots the comment a describe handler emits when its output
// cannot be replayed. Matching the handler's own words keeps the two in step:
// a handler that stops saying it is read-only has become authorable.
func declaredReadOnly(mdl string) bool {
	return strings.Contains(mdl, "read-only; they cannot be created via MDL")
}

// emptyBody reports whether a describe rendered a statement with nothing in it —
// `… { }` or `… ( … );` with no inner lines. Such output is identical for an
// element with content and one without, so it cannot distinguish them.
func emptyBody(mdl string) bool {
	trimmed := strings.TrimSpace(mdl)
	if trimmed == "" {
		return true
	}
	// A body was rendered but holds nothing: "{ }" or "{" immediately followed
	// by "}".
	compact := strings.Join(strings.Fields(trimmed), " ")
	return strings.HasSuffix(compact, "{ }") || strings.HasSuffix(compact, "{}")
}

// Snapshot is the describable content of one module at one point in time.
type Snapshot struct {
	Module   string
	Elements map[ElementKey]Element
	// Dependents are the cross-module associations OTHER modules own that reach
	// into this one, keyed by qualified name (they are not this module's, so the
	// unqualified-name convention of Elements does not apply).
	//
	// They are captured because replacing the module destroys them: DROP MODULE
	// deletes this module's entities, and DeleteEntity cascades into every other
	// domain model to remove the cross-module associations that reach a deleted
	// entity by name. The association belongs to the user's module, so it is
	// nobody's "edit" to this one — which is how --save-edits came to save every
	// change the update would discard except the one it was certain to
	// (mendixlabs/mxcli#1502).
	Dependents []Element
}

// Keys returns the snapshot's element keys in a stable order.
func (s *Snapshot) Keys() []ElementKey {
	keys := make([]ElementKey, 0, len(s.Elements))
	for k := range s.Elements {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Type != keys[j].Type {
			return keys[i].Type < keys[j].Type
		}
		return keys[i].Name < keys[j].Name
	})
	return keys
}

// SnapshotModule connects to a project and captures DESCRIBE output for every
// element of one module.
//
// Elements are enumerated from the catalog's `objects` view, which indexes every
// describable top-level document type. That is deliberately the same source the
// DESCRIBE auto-detect uses, so the enumeration and the describe agree on what
// exists; an element the catalog does not index is invisible here, which is the
// coverage bound recorded in §7 of the proposal.
// newBackend supplies the engine to read with. It is injected rather than chosen
// here because the engine is a global CLI concern (--engine / MXCLI_ENGINE) that
// this package has no business deciding — and because comparing the two sides
// with the *same* engine is what makes the result meaningful.
func SnapshotModule(mprPath, moduleName string, newBackend func() backend.FullBackend) (*Snapshot, error) {
	var sink bytes.Buffer
	exec, err := openForSnapshot(mprPath, &sink, newBackend)
	if err != nil {
		return nil, err
	}
	defer exec.Close()

	rows, err := queryModuleObjects(exec, moduleName)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{Module: moduleName, Elements: make(map[ElementKey]Element, len(rows))}
	for _, row := range rows {
		key := ElementKey{Type: row.objectType, Name: row.name}
		// A module can legitimately hold two elements of different types with the
		// same name; the type is part of the key so they do not collide. Two of the
		// same type and name cannot exist, so a repeat means the enumeration
		// returned a duplicate — keep the first and move on rather than pick
		// arbitrarily.
		if _, seen := snap.Elements[key]; seen {
			continue
		}
		snap.Elements[key] = describeElement(exec, &sink, row)
	}

	if snap.Dependents, err = dependents(exec, &sink, moduleName); err != nil {
		return nil, err
	}
	return snap, nil
}

// SnapshotDependents captures only the associations other modules own that
// reach into moduleName. It needs nothing but the project, so `update
// --no-baseline` — which cannot build the baseline to find edits to the module
// — still names, saves and gates on what the update deletes outside it
// (ako/mxcli#2368).
func SnapshotDependents(mprPath, moduleName string, newBackend func() backend.FullBackend) ([]Element, error) {
	var sink bytes.Buffer
	exec, err := openForSnapshot(mprPath, &sink, newBackend)
	if err != nil {
		return nil, err
	}
	defer exec.Close()
	return dependents(exec, &sink, moduleName)
}

func dependents(exec *executor.Executor, sink *bytes.Buffer, moduleName string) ([]Element, error) {
	rows, err := queryDependentAssociations(exec, moduleName)
	if err != nil {
		return nil, err
	}
	var out []Element
	for _, row := range rows {
		out = append(out, describeElement(exec, sink, row))
	}
	return out, nil
}

// openForSnapshot connects an executor writing to sink and builds its catalog.
func openForSnapshot(mprPath string, sink *bytes.Buffer, newBackend func() backend.FullBackend) (*executor.Executor, error) {
	exec := executor.New(sink)
	if newBackend != nil {
		exec.SetBackendFactory(newBackend)
	}
	if err := exec.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		exec.Close()
		return nil, fmt.Errorf("connect %s: %w", mprPath, err)
	}
	// The catalog is built lazily by the statements that need it, and the
	// enumeration reads it directly rather than through a statement, so build
	// it explicitly. Fast mode is enough: the `objects` index it populates is
	// exactly what this needs, and full mode additionally parses every
	// activity and widget for cross-references nothing here uses.
	if err := exec.Execute(&ast.RefreshCatalogStmt{}); err != nil {
		exec.Close()
		return nil, fmt.Errorf("build catalog for %s: %w", mprPath, err)
	}
	return exec, nil
}

// moduleObject is one row of the enumeration.
type moduleObject struct {
	objectType    string
	name          string
	qualifiedName string
}

func queryModuleObjects(exec *executor.Executor, moduleName string) ([]moduleObject, error) {
	cat := exec.Catalog()
	if cat == nil {
		return nil, fmt.Errorf("no catalog available for %s", moduleName)
	}

	q := "SELECT ObjectType, Name, QualifiedName FROM objects WHERE ModuleName = '" +
		strings.ReplaceAll(moduleName, "'", "''") + "' ORDER BY ObjectType, Name"
	res, err := cat.Query(q)
	if err != nil {
		return nil, fmt.Errorf("enumerate module %s: %w", moduleName, err)
	}

	out := make([]moduleObject, 0, len(res.Rows))
	for _, row := range res.Rows {
		if len(row) < 3 {
			continue
		}
		out = append(out, moduleObject{
			objectType:    fmt.Sprintf("%v", row[0]),
			name:          fmt.Sprintf("%v", row[1]),
			qualifiedName: fmt.Sprintf("%v", row[2]),
		})
	}
	return out, nil
}

// queryDependentAssociations finds the associations owned by other modules that
// have an end in moduleName. In practice that is the TO end — a cross-module
// association is stored with its FROM entity — but both ends are matched so a
// shape the catalog reports differently is not silently missed.
//
// The prefix is compared with substr rather than LIKE: module names may contain
// `_`, which LIKE reads as a wildcard.
func queryDependentAssociations(exec *executor.Executor, moduleName string) ([]moduleObject, error) {
	cat := exec.Catalog()
	if cat == nil {
		return nil, fmt.Errorf("no catalog available for %s", moduleName)
	}
	quoted := "'" + strings.ReplaceAll(moduleName, "'", "''") + ".'"
	n := fmt.Sprint(len(moduleName) + 1)
	q := "SELECT QualifiedName FROM associations WHERE ModuleName <> '" +
		strings.ReplaceAll(moduleName, "'", "''") + "' AND (" +
		"substr(ToEntity, 1, " + n + ") = " + quoted + " OR substr(FromEntity, 1, " + n + ") = " + quoted +
		") ORDER BY QualifiedName"
	res, err := cat.Query(q)
	if err != nil {
		return nil, fmt.Errorf("find associations into module %s: %w", moduleName, err)
	}
	out := make([]moduleObject, 0, len(res.Rows))
	for _, row := range res.Rows {
		if len(row) < 1 {
			continue
		}
		qn := fmt.Sprintf("%v", row[0])
		out = append(out, moduleObject{objectType: "ASSOCIATION", name: qn, qualifiedName: qn})
	}
	return out, nil
}

// describeElement runs one DESCRIBE and captures its output.
func describeElement(exec *executor.Executor, sink *bytes.Buffer, obj moduleObject) Element {
	key := ElementKey{Type: obj.objectType, Name: obj.name}

	kind, ok := executor.DescribeKindFor(obj.objectType)
	if !ok {
		return Element{Key: key, Err: "no DESCRIBE support for " + obj.objectType}
	}

	qn, err := splitQualified(obj.qualifiedName)
	if err != nil {
		return Element{Key: key, Err: err.Error()}
	}

	sink.Reset()
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: kind, Name: qn}); err != nil {
		return Element{Key: key, Err: err.Error()}
	}
	out := normalizeMDL(sink.String())
	if out == "" {
		return Element{Key: key, Err: "DESCRIBE produced no output"}
	}
	return Element{Key: key, MDL: out}
}

// splitQualified splits Module.Name. Some catalog rows carry a dotted service
// prefix (Service.Action), so the split is on the first separator and the
// remainder is the name.
func splitQualified(qualified string) (ast.QualifiedName, error) {
	i := strings.Index(qualified, ".")
	if i <= 0 || i == len(qualified)-1 {
		return ast.QualifiedName{}, fmt.Errorf("not a qualified name: %q", qualified)
	}
	return ast.QualifiedName{Module: qualified[:i], Name: qualified[i+1:]}, nil
}

// normalizeMDL makes DESCRIBE output comparable across two projects.
//
// Only incidental formatting is removed. Nothing that could carry a real edit is
// touched: a normaliser that stripped, say, property values would report a
// modified element as clean, which is the failure this whole design exists to
// avoid.
func normalizeMDL(s string) string {
	lines := strings.Split(s, "\n")
	kept := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimRight(ln, " \t\r")
		if strings.TrimSpace(ln) == "" {
			continue
		}
		kept = append(kept, ln)
	}
	return strings.Join(kept, "\n")
}
