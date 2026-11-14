// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// A documentNameSpace is a group of element kinds that share one name space per
// module: two of them may not have the same name, whatever their kinds, their
// folders, or the casing of the name (ako/mxcli#793).
//
// The groups are measured, not assumed — mx check 11.14.0 over copies of
// TestApp, every pair of fourteen creatable kinds plus one of each kind the
// project already held. Only these three clash. A microflow shares a name with
// a page, constant, enumeration, java action, workflow or entity without
// complaint, a javascript action with a nanoflow, a page with a building block
// or page template. Each group is one abstract base type in the metamodel
// (MicroflowBase, FormBase, and the domain-model members plus enumerations),
// which is why the rule is per group rather than per module.
type documentNameSpace struct {
	kinds []string // doc-type keys, as stmtCreateKind returns them
	ce    string   // the consistency error Mendix reports
	what  string   // the kinds, for the message
}

var documentNameSpaces = []documentNameSpace{
	// [CE0122] "Duplicate document name 'M.X'." at Microflow 'M.X', Nanoflow 'M.X'
	{kinds: []string{"microflow", "nanoflow", "rule"}, ce: "CE0122", what: "microflows, nanoflows and rules"},
	// [CE0122] … at Page 'M.X', Snippet 'M.X' / Page 'M.X', Layout 'M.X'
	{kinds: []string{"page", "snippet", "layout"}, ce: "CE0122", what: "pages, snippets and layouts"},
	// [CE0065] "Duplicate name 'X' in module 'M'. Entities, associations and
	// enumerations cannot share names." — cross-module associations included.
	{kinds: []string{"entity", "association", "enumeration"}, ce: "CE0065", what: "entities, associations and enumerations"},
}

// nameSpaceOf returns the shared name space docType belongs to, or nil.
func nameSpaceOf(docType string) *documentNameSpace {
	for i := range documentNameSpaces {
		for _, k := range documentNameSpaces[i].kinds {
			if k == docType {
				return &documentNameSpaces[i]
			}
		}
	}
	return nil
}

// caseSensitiveCreateKinds are the kinds whose create handler looks the stored
// element up by exact name, so `create microflow M.act_login` next to
// M.ACT_Login — plain or `or modify` — writes a second microflow, and Mendix
// rejects the pair: [CE0122] "Duplicate document name 'M.ACT_Login'." (and
// CE0065 for the domain-model kinds). Measured with mx check 11.13.0 on a
// PedApp copy for microflow, nanoflow, page, entity and enumeration; the rest
// are the handlers that compare with == (ako/mxcli#806). The other kinds'
// handlers already fold the name — a constant, queue or REST client finds
// M.C_One for M.c_one and refuses or modifies it — so a case-only create of
// one of those is the handler's to answer, and `or modify` keeps working.
var caseSensitiveCreateKinds = map[string]bool{
	"microflow": true, "nanoflow": true, "rule": true,
	"page": true, "snippet": true, "layout": true,
	"entity": true, "association": true, "enumeration": true,
	"javaaction": true, "javascriptaction": true, "workflow": true,
}

// nameSpaceKinds is every kind whose names docType's names must not collide
// with in a module: its shared name space, or the kind alone — two constants
// whose names differ only in case are CE0122 as well. nil when the kind has no
// listing here, and so is not checked.
func nameSpaceKinds(docType string) []string {
	if ns := nameSpaceOf(docType); ns != nil {
		return ns.kinds
	}
	switch docType {
	case "constant", "javaaction", "javascriptaction", "workflow":
		return []string{docType}
	}
	return nil
}

// nameRule is the rule a clash breaks, for the message.
func nameRule(docType string) string {
	if ns := nameSpaceOf(docType); ns != nil {
		return fmt.Sprintf("%s share one name space per module, whatever their folder, "+
			"and names compare case-insensitively (Mendix %s)", ns.what, ns.ce)
	}
	return fmt.Sprintf("%s names are unique per module, whatever their folder, "+
		"and compare case-insensitively (Mendix CE0122)", friendlyDocType(docType))
}

// nameClashMessage is the one wording every tier reports: what the statement
// does, the element that already has the name, and the rule.
func nameClashMessage(docType, name, otherType, otherName string) string {
	return clashMessage(fmt.Sprintf("create %s %s", friendlyDocType(docType), name), docType, otherType, otherName)
}

func clashMessage(action, docType, otherType, otherName string) string {
	return fmt.Sprintf("cannot %s: %s %s already has that name — %s",
		action, friendlyDocType(otherType), otherName, nameRule(docType))
}

const nameClashHint = "rename one of the two; a different folder does not separate them"

// caseOnlyHint is the hint for a create whose own kind holds the name in
// another casing: the stored spelling is the one that modifies it.
func caseOnlyHint(stored string) string {
	return "to change the existing one, spell it " + stored + "; otherwise pick a name that differs in more than case"
}

// nameSet is one kind's qualified names, exact and folded.
type nameSet struct {
	exact map[string]bool
	fold  map[string]string // lower-cased → a stored spelling
}

func newNameSet(names map[string]bool) *nameSet {
	s := &nameSet{exact: map[string]bool{}, fold: map[string]string{}}
	for qn := range names {
		s.add(qn)
	}
	return s
}

func (s *nameSet) add(qn string) {
	s.exact[qn] = true
	s.fold[strings.ToLower(qn)] = qn
}

func (s *nameSet) remove(qn string) {
	delete(s.exact, qn)
	key := strings.ToLower(qn)
	if stored, ok := s.fold[key]; ok && strings.EqualFold(stored, qn) {
		delete(s.fold, key)
		// Another spelling of the same name may still be there (a clash an
		// older mxcli wrote); keep it findable.
		for other := range s.exact {
			if strings.EqualFold(other, qn) {
				s.fold[key] = other
				break
			}
		}
	}
}

// find returns the stored spelling of qn, preferring an exact match.
func (s *nameSet) find(qn string) (string, bool) {
	if s == nil {
		return "", false
	}
	if s.exact[qn] {
		return qn, true
	}
	stored, ok := s.fold[strings.ToLower(qn)]
	return stored, ok
}

// loadNameSet lists the project's elements of one kind. A list that cannot be
// read yields no names, as every other build*QualifiedNames does: the handler
// that would write the element reads the same list and reports the failure
// itself.
func loadNameSet(ctx *ExecContext, docType string) *nameSet {
	var names map[string]bool
	switch docType {
	case "microflow":
		names = buildMicroflowQualifiedNames(ctx)
	case "nanoflow":
		names = buildNanoflowQualifiedNames(ctx)
	case "rule":
		names = buildRuleQualifiedNames(ctx)
	case "page":
		names = buildPageQualifiedNames(ctx)
	case "snippet":
		names = buildSnippetQualifiedNames(ctx)
	case "layout":
		names = buildLayoutQualifiedNames(ctx)
	case "entity":
		names = buildEntityQualifiedNames(ctx)
	case "association":
		names = buildAssociationQualifiedNames(ctx)
	case "enumeration":
		names = buildEnumerationQualifiedNames(ctx)
	case "constant":
		names, _ = buildConstantQualifiedNames(ctx)
	case "javaaction":
		names = buildJavaActionQualifiedNames(ctx)
	case "javascriptaction":
		names = buildJavaScriptActionQualifiedNames(ctx)
	case "workflow":
		names = buildWorkflowQualifiedNames(ctx)
	}
	return newNameSet(names)
}

// namedNameSet is the names of one kind that a lookup of target can match:
// the elements whose own name folds to target's. A statement looks up one
// qualified name, and names compare with case folded, so those are the whole
// set its lookups can find. loadNameSet builds, and folds, the qualified name
// of every element of the kind in the project for the same answer, once per
// kind per statement — a few milliseconds and the garbage of 5,000 names for
// each `create or modify microflow` in a project of that size
// (ako/mxcli#2160).
//
// The filter is strings.EqualFold where the set folds with strings.ToLower.
// Two names with the same ToLower are EqualFold, so nothing the set would
// have matched is left out.
//
// nil when it does not apply — a kind not listed here, or no target — and the
// caller lists.
func namedNameSet(ctx *ExecContext, kind, target string) *nameSet {
	i := strings.Index(target, ".")
	if i <= 0 {
		return nil
	}
	short := target[i+1:]
	names := map[string]bool{}
	add := func(h *ContainerHierarchy, containerID model.ID, name string) {
		if strings.EqualFold(name, short) {
			names[h.GetQualifiedName(containerID, name)] = true
		}
	}
	switch kind {
	case "microflow":
		h, err := getHierarchy(ctx)
		if err != nil {
			return newNameSet(nil)
		}
		mfs, err := microflowHeadersNamedFold(ctx, short)
		if err != nil {
			return newNameSet(nil)
		}
		for _, mf := range mfs {
			add(h, mf.ContainerID, mf.Name)
		}
		// As buildMicroflowQualifiedNames: the platform's own, only beside a
		// listing that has an answer. Whether it has one is only asked when
		// the target is one of them.
		for _, qn := range meta.SystemMicroflows {
			if !strings.EqualFold(qn, target) {
				continue
			}
			if all, err := microflowHeaders(ctx); err == nil && len(all) > 0 {
				names[qn] = true
			}
		}
	case "nanoflow":
		h, err := getHierarchy(ctx)
		if err != nil {
			return newNameSet(nil)
		}
		nfs, err := projectNanoflows(ctx)
		if err != nil {
			return newNameSet(nil)
		}
		for _, nf := range nfs {
			add(h, nf.ContainerID, nf.Name)
		}
	case "page":
		h, err := getHierarchy(ctx)
		if err != nil {
			return newNameSet(nil)
		}
		pgs, err := projectPages(ctx)
		if err != nil {
			return newNameSet(nil)
		}
		for _, p := range pgs {
			add(h, p.ContainerID, p.Name)
		}
	default:
		return nil
	}
	return newNameSet(names)
}

// microflowHeaderFilter is a backend that lists the microflow headers a name
// filter accepts without copying the header of every microflow.
type microflowHeaderFilter interface {
	ListMicroflowHeadersWhere(keep func(name string) bool) ([]types.DocumentHeader, error)
}

// microflowHeadersNamedFold is the headers of the microflows whose name
// folds to short.
func microflowHeadersNamedFold(ctx *ExecContext, short string) ([]types.DocumentHeader, error) {
	keep := func(name string) bool { return strings.EqualFold(name, short) }
	if l, ok := ctx.Backend.(microflowHeaderFilter); ok {
		return l.ListMicroflowHeadersWhere(keep)
	}
	all, err := microflowHeaders(ctx)
	if err != nil {
		return nil, err
	}
	var out []types.DocumentHeader
	for _, h := range all {
		if keep(h.Name) {
			out = append(out, h)
		}
	}
	return out, nil
}

// domainModelNameSet is the entity or association names of the modules called
// module, read from those modules' domain models alone. A name is only ever
// looked up qualified, and compared with case folded, so the names of every
// module whose name folds to module's are the whole set a lookup in module can
// match. loadNameSet lists and converts every domain model of the project for
// the same answer, once per kind per statement (ako/mxcli#2160).
//
// nil when it does not apply — another kind, no module, no module of that name
// yet, or a domain model that cannot be read alone — and the caller lists.
func domainModelNameSet(ctx *ExecContext, kind, module string) *nameSet {
	if module == "" || (kind != "entity" && kind != "association") {
		return nil
	}
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	found := false
	for _, m := range modules {
		if m == nil || !strings.EqualFold(m.Name, module) {
			continue
		}
		dm, err := ctx.Backend.GetDomainModel(m.ID)
		if err != nil || dm == nil {
			return nil
		}
		found = true
		if kind == "entity" {
			for _, ent := range dm.Entities {
				names[m.Name+"."+ent.Name] = true
			}
			continue
		}
		for _, assoc := range dm.Associations {
			names[m.Name+"."+assoc.Name] = true
		}
		for _, ca := range dm.CrossAssociations {
			names[m.Name+"."+ca.Name] = true
		}
	}
	if !found {
		return nil
	}
	return newNameSet(names)
}

// nameHolder finds the element, among kinds, that already holds target —
// skipping self, the element a rename or move is about (of kind docType).
// exactOwnIsHandlers leaves an exact same-kind match to the statement's own
// handler, which already refuses it with its own message.
func nameHolder(sets func(kind string) *nameSet, docType, target, self string, exactOwnIsHandlers bool) (string, string, bool) {
	for _, k := range nameSpaceKinds(docType) {
		stored, ok := sets(k).find(target)
		if !ok {
			continue
		}
		if k == docType {
			if self != "" && strings.EqualFold(stored, self) {
				continue // the element itself, renamed in case only
			}
			if exactOwnIsHandlers && stored == target {
				continue
			}
		}
		return k, stored, true
	}
	return "", "", false
}

// createClash is the clash a create would cause, decided the same way by exec
// and by check: nothing when its own kind already has the name exactly (the
// handler modifies or refuses it), nor when the own kind has it in another
// casing and the handler folds names; otherwise the element that holds it.
func createClash(sets func(kind string) *nameSet, docType, name string) (c nameClash, clash bool) {
	if nameSpaceKinds(docType) == nil {
		return c, false
	}
	action := fmt.Sprintf("create %s %s", friendlyDocType(docType), name)
	if stored, ok := sets(docType).find(name); ok {
		if stored == name || !caseSensitiveCreateKinds[docType] {
			return c, false
		}
		return nameClash{docType, stored, clashMessage(action, docType, docType, stored), caseOnlyHint(stored)}, true
	}
	if other, stored, ok := nameHolder(sets, docType, name, "", false); ok {
		return nameClash{other, stored, clashMessage(action, docType, other, stored), nameClashHint}, true
	}
	return c, false
}

// nameClash is the element that already holds a name, and how to report it.
type nameClash struct {
	kind, stored string
	msg, hint    string
}

// renameTarget is what a rename statement asks for: the kind, the element's
// current qualified name and the one it would get. ok is false for a rename
// the clash check does not cover (a module, a dry run, an untracked kind).
func renameTarget(s *ast.RenameStmt) (docType, self, target string, ok bool) {
	if s.DryRun {
		return "", "", "", false
	}
	dt := renameDocType(s.ObjectType)
	if dt == "" || nameSpaceKinds(dt) == nil {
		return "", "", "", false
	}
	return dt, s.Name.String(), s.Name.Module + "." + s.NewName, true
}

// moveDocType maps a MOVE statement's document type to the doc-type key.
func moveDocType(t ast.DocumentType) string {
	switch t {
	case ast.DocumentTypeMicroflow:
		return "microflow"
	case ast.DocumentTypeNanoflow:
		return "nanoflow"
	case ast.DocumentTypeRule:
		return "rule"
	case ast.DocumentTypePage:
		return "page"
	case ast.DocumentTypeSnippet:
		return "snippet"
	case ast.DocumentTypeLayout:
		return "layout"
	case ast.DocumentTypeEntity:
		return "entity"
	case ast.DocumentTypeEnumeration:
		return "enumeration"
	case ast.DocumentTypeConstant:
		return "constant"
	case ast.DocumentTypeJavaAction:
		return "javaaction"
	case ast.DocumentTypeJavaScriptAction:
		return "javascriptaction"
	case ast.DocumentTypeWorkflow:
		return "workflow"
	}
	return ""
}

// moveTarget is what a cross-module move asks for. A move within its module
// keeps the name, and a folder does not scope it, so it cannot clash.
func moveTarget(s *ast.MoveStmt) (docType, self, target string, ok bool) {
	if s.TargetModule == "" || strings.EqualFold(s.TargetModule, s.Name.Module) {
		return "", "", "", false
	}
	dt := moveDocType(s.DocumentType)
	if dt == "" || nameSpaceKinds(dt) == nil {
		return "", "", "", false
	}
	return dt, s.Name.String(), s.TargetModule + "." + s.Name.Name, true
}

// renameOrMoveClash is the clash a rename or a cross-module move would cause.
func renameOrMoveClash(sets func(kind string) *nameSet, stmt ast.Statement, exactOwnIsHandlers bool) (msg string, clash bool) {
	switch s := stmt.(type) {
	case *ast.RenameStmt:
		dt, self, target, ok := renameTarget(s)
		if !ok {
			return "", false
		}
		if other, stored, ok := nameHolder(sets, dt, target, self, exactOwnIsHandlers); ok {
			return clashMessage(fmt.Sprintf("rename %s %s to %s", friendlyDocType(dt), self, target), dt, other, stored), true
		}
	case *ast.MoveStmt:
		dt, self, target, ok := moveTarget(s)
		if !ok {
			return "", false
		}
		// The moved element is in another module, so it cannot be its own
		// match; an exact same-kind holder is a clash like any other.
		if other, stored, ok := nameHolder(sets, dt, target, "", false); ok {
			return clashMessage(fmt.Sprintf("move %s %s to module %s", friendlyDocType(dt), self, s.TargetModule), dt, other, stored), true
		}
	}
	return "", false
}

// refuseNameClash is the exec-time refusal of a statement that would give an
// element a name its name space already holds in the module: a create that
// ADDS an element (ako/mxcli#793), one whose name differs from its own kind's
// only in case, and a rename or cross-module move onto a taken name
// (ako/mxcli#806). It runs in the dispatch, before the handler, so nothing is
// written.
//
// A create whose own kind already has the name adds nothing — the handler
// refuses it as "already exists" or modifies the stored element in place — so
// it is left alone, even in a project that already holds such a clash.
func refuseNameClash(ctx *ExecContext, stmt ast.Statement) error {
	if !ctx.Connected() {
		return nil
	}
	loaded := map[string]*nameSet{}
	// scope is the module a create names. Its entity and association names are
	// then read from that module's domain model alone (domainModelNameSet).
	scope := ""
	// target is the one qualified name this statement looks up: the name a
	// create gives, or the one a rename or move would give.
	target := ""
	sets := func(kind string) *nameSet {
		if s, ok := loaded[kind]; ok {
			return s
		}
		s := domainModelNameSet(ctx, kind, scope)
		if s == nil {
			s = namedNameSet(ctx, kind, target)
		}
		if s == nil {
			s = loadNameSet(ctx, kind)
		}
		loaded[kind] = s
		return s
	}
	switch st := stmt.(type) {
	case *ast.RenameStmt:
		_, _, target, _ = renameTarget(st)
	case *ast.MoveStmt:
		_, _, target, _ = moveTarget(st)
	}
	switch stmt.(type) {
	case *ast.RenameStmt, *ast.MoveStmt:
		if msg, ok := renameOrMoveClash(sets, stmt, true); ok {
			return mdlerrors.NewValidation(msg + " — pick another name, or rename the other element first")
		}
		return nil
	}
	docType, name, _ := stmtCreateKind(stmt)
	if i := strings.Index(name, "."); i > 0 {
		scope = name[:i]
	}
	target = name
	if c, ok := createClash(sets, docType, name); ok {
		return mdlerrors.NewAlreadyExistsMsg(c.kind, c.stored, c.msg+" — "+c.hint)
	}
	return nil
}

// checkScriptNameClash reports a create whose name another kind in its name
// space already holds, alive in the script at that point — or its own kind
// holds in another casing, where the handler compares exactly (MDL-DUPNAME).
// reg is CheckScriptDuplicates' registry, before this statement is added.
func checkScriptNameClash(reg *nameRegistry, docType, name string) *linter.Violation {
	if nameSpaceKinds(docType) == nil {
		return nil
	}
	if reg.isAlive(docType, name) {
		return nil // the same kind: MDL-DUPDEF's business, or a modify
	}
	if stored, idx, ok := reg.findFold(docType, name); ok {
		if !caseSensitiveCreateKinds[docType] {
			return nil
		}
		return &linter.Violation{
			RuleID:   "MDL-DUPNAME",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("%s (statement %d)",
				clashMessage(fmt.Sprintf("create %s %s", friendlyDocType(docType), name), docType, docType, stored), idx),
			Suggestion: caseOnlyHint(stored),
		}
	}
	for _, other := range nameSpaceKinds(docType) {
		if other == docType {
			continue
		}
		if stored, idx, ok := reg.findFold(other, name); ok {
			return &linter.Violation{
				RuleID:   "MDL-DUPNAME",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf("%s (statement %d)",
					nameClashMessage(docType, name, other, stored), idx),
				Suggestion: nameClashHint,
			}
		}
	}
	return nil
}

// findFold looks name up case-insensitively among the alive names of docType.
func (r *nameRegistry) findFold(docType, name string) (string, int, bool) {
	for qn, idx := range r.alive[docType] {
		if strings.EqualFold(qn, name) {
			return qn, idx, true
		}
	}
	return "", 0, false
}

// nameClashCheckedKinds is every kind CheckProjectNameClashes tracks.
var nameClashCheckedKinds = []string{
	"microflow", "nanoflow", "rule", "page", "snippet", "layout",
	"entity", "association", "enumeration",
	"constant", "javaaction", "javascriptaction", "workflow",
}

// CheckProjectNameClashes walks prog in statement order and reports every
// statement that would give an element a name already taken in the project:
// a create over a name another kind in its name space holds (ako/mxcli#793),
// a create whose name differs from its own kind's only in case, and a rename
// or cross-module move onto a taken name (ako/mxcli#806). Unlike a same-kind
// conflict this is never an ordinary re-run — `create or modify` included — so
// exec's pre-flight refuses it too, before anything is written.
//
// Names the script itself creates are CheckScriptDuplicates' (MDL-DUPNAME); a
// project element the script drops, renames or moves first no longer holds its
// name, and the renamed or moved one holds its new name.
func CheckProjectNameClashes(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}
	// A kind is listed the first time a statement asks about it, and only if
	// some statement can ask: a create, rename or move reads the kinds of its
	// name space and nothing else. A drop, rename or move still updates a
	// kind nothing reads — into an empty set, since what it holds is never
	// consulted. Listing every kind up front cost a one-flow script ten
	// listings it did not need.
	checked := map[string]bool{}
	for _, k := range nameClashCheckedKinds {
		checked[k] = true
	}
	asked := map[string]bool{}
	for _, stmt := range prog.Statements {
		var dt string
		switch s := stmt.(type) {
		case *ast.RenameStmt:
			dt, _, _, _ = renameTarget(s)
		case *ast.MoveStmt:
			dt, _, _, _ = moveTarget(s)
		default:
			dt, _, _ = stmtCreateKind(stmt)
		}
		for _, k := range nameSpaceKinds(dt) {
			asked[k] = true
		}
	}
	project := map[string]*nameSet{}
	sets := func(kind string) *nameSet {
		if s, ok := project[kind]; ok {
			return s
		}
		var s *nameSet
		if checked[kind] && asked[kind] {
			s = loadNameSet(ctx, kind)
		} else {
			s = newNameSet(nil)
		}
		project[kind] = s
		return s
	}
	var errs []error
	for i, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.RenameStmt:
			if msg, ok := renameOrMoveClash(sets, s, false); ok {
				errs = append(errs, fmt.Errorf("statement %d: %s — pick another name, or rename the other element first", i+1, msg))
			}
			if dt, self, target, ok := renameTarget(s); ok {
				sets(dt).remove(self)
				sets(dt).add(target)
			} else if dt := renameDocType(s.ObjectType); dt != "" && !s.DryRun {
				sets(dt).remove(s.Name.String())
			}
			continue
		case *ast.MoveStmt:
			if msg, ok := renameOrMoveClash(sets, s, false); ok {
				errs = append(errs, fmt.Errorf("statement %d: %s — pick another name, or rename the other element first", i+1, msg))
			}
			if dt, self, target, ok := moveTarget(s); ok {
				sets(dt).remove(self)
				sets(dt).add(target)
			}
			continue
		}
		if dt, name := stmtDropInfo(stmt); dt != "" {
			sets(dt).remove(name)
			continue
		}
		dt, name, _ := stmtCreateKind(stmt)
		if c, ok := createClash(sets, dt, name); ok {
			errs = append(errs, fmt.Errorf("statement %d: %s — %s", i+1, c.msg, c.hint))
		}
	}
	return errs
}
