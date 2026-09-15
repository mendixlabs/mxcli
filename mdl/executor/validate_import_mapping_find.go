// SPDX-License-Identifier: Apache-2.0

// Check-time validation for import mapping elements whose object handling is
// `find` or `find or create`. Mendix requires two things of such an element, and
// mxcli enforced neither: the statement was accepted, the document written, and
// the project only became unbuildable a build later (ako/mxcli#253).
//
//	CE0250  "Object element must have a key defined if object handling is set to
//	        'Search for an object'."
//	CE0251  "Searching for an object is not allowed for the entity 'X', because
//	        it is not persistable."
//
// Measured on mxbuild 11.13.0 against a base-project control, and the ordering
// matters for anyone re-measuring: mxbuild reports **one at a time**. A `find`
// over a non-persistent entity with no key is CE0250 only; CE0251 appears once a
// key exists. Fixing the key half alone therefore looks like it fixed the whole
// problem until someone adds a key. The positive control — persistable entity,
// one key, both `or error` and `or create` — is 0 errors.
//
// Both apply to nested elements, not just the root: the same mapping with the
// `find` one level down reports at "Object mapping element 'Pet'".
//
// A `custom` element (`find X by Module.MF(...)`) is exempt from both. The
// microflow IS the find, so there is no key to declare and no query to run.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ValidateImportMappingFind reports MDL-MAP02 (no key) and MDL-MAP03 (entity not
// persistable) for every `find` / `find or create` object element in the script.
//
// MDL-MAP02 needs no project — whether a member carries `key` is in the
// statement — so it fires under a plain `mxcli check`, which is the whole point:
// an agent generating MDL gets the signal while it can still fix the statement.
// MDL-MAP03 needs the domain model and skips itself without one.
func ValidateImportMappingFind(prog *ast.Program, projectPath string) []linter.Violation {
	if prog == nil {
		return nil
	}

	var out []linter.Violation
	var needEntities []findElement
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateImportMappingStmt)
		if !ok || s.RootElement == nil {
			continue
		}
		for _, fe := range findElements(s.RootElement, "") {
			fe = fe.withStmt(s)
			if !hasKeyMember(fe.def) {
				out = append(out, keyViolation(s, fe))
			}
			needEntities = append(needEntities, fe)
		}
	}
	if len(needEntities) == 0 {
		return out
	}
	return append(out, persistabilityViolations(prog, projectPath, needEntities)...)
}

// findElement is one object element that searches, with the label the diagnostic
// names it by.
type findElement struct {
	def *ast.ImportMappingElementDef
	// path is the element's position in the mapping, empty for the root. It is
	// how a diagnostic distinguishes two `find`s in one mapping the way
	// mxbuild's "at Object mapping element 'Pet'" does.
	path string
	stmt *ast.CreateImportMappingStmt
}

func (fe findElement) withStmt(s *ast.CreateImportMappingStmt) findElement {
	fe.stmt = s
	return fe
}

// label renders the element for a message: the mapping's name, plus the element
// path when the element is not the root.
// statement renders the element's handling the way the author wrote it, backup
// included. `find X or create` and the legacy `find or create X` both store
// ObjectHandling "Find" plus a Create backup, so quoting the handling alone
// would print `find` at an author who wrote something else.
func (fe findElement) statement() string {
	out := handlingKeyword(fe.def.ObjectHandling) + " " + fe.def.Entity
	if fe.def.Backup != "" {
		out += " or " + strings.ToLower(fe.def.Backup)
	}
	return out
}

func (fe findElement) label() string {
	if fe.path == "" {
		return fe.stmt.Name.String()
	}
	return fe.stmt.Name.String() + " element " + fe.path
}

// findElements collects every object element in the tree whose handling
// searches for an object.
//
// `custom` is excluded: a CustomHandler makes the executor store ObjectHandling
// "Custom" regardless of the keyword written, so `find X by MF(...)` is not a
// search as far as Mendix is concerned, and neither requirement applies.
func findElements(root *ast.ImportMappingElementDef, path string) []findElement {
	if root == nil {
		return nil
	}
	var out []findElement
	if root.Entity != "" && root.CustomHandler == nil && isFindHandling(root.ObjectHandling) {
		out = append(out, findElement{def: root, path: path})
	}
	for _, child := range root.Children {
		if child == nil || child.Entity == "" {
			continue
		}
		childPath := child.JsonName
		if childPath == "" {
			childPath = child.Entity
		}
		if path != "" {
			childPath = path + "/" + childPath
		}
		out = append(out, findElements(child, childPath)...)
	}
	return out
}

// isFindHandling reports whether the handling searches. The visitor spells the
// two searching forms "Find" and "FindOrCreate"; "Create" and "" do not search.
func isFindHandling(h string) bool {
	return h == "Find" || h == "FindOrCreate"
}

// hasKeyMember reports whether any DIRECT value member of the element is marked
// `key`. Direct is the right scope: CE0250 is raised per object element, and a
// nested element's key belongs to that element's own search.
func hasKeyMember(def *ast.ImportMappingElementDef) bool {
	for _, child := range def.Children {
		if child != nil && child.Entity == "" && child.IsKey {
			return true
		}
	}
	return false
}

func keyViolation(s *ast.CreateImportMappingStmt, fe findElement) linter.Violation {
	member := firstValueMember(fe.def)
	suggestion := "Mark the member that identifies the object with `key`"
	if member != "" {
		suggestion = "Mark the identifying member with `key`, e.g. " + member + " = ... key"
	}
	return linter.Violation{
		RuleID:   "MDL-MAP02",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"import mapping %s: `%s` searches for an object but no member is marked `key`, "+
				"so Mendix has nothing to search on (CE0250)",
			fe.label(), fe.statement()),
		Location: linter.Location{
			Module:       s.Name.Module,
			DocumentType: "import mapping",
			DocumentName: s.Name.Name,
		},
		Suggestion: suggestion,
	}
}

// firstValueMember names a member the author could mark, so the suggestion is
// about their mapping rather than a generic instruction.
func firstValueMember(def *ast.ImportMappingElementDef) string {
	for _, child := range def.Children {
		if child != nil && child.Entity == "" && child.Attribute != "" {
			return child.Attribute
		}
	}
	return ""
}

// persistabilityViolations reports MDL-MAP03 for elements searching a
// non-persistable entity.
//
// It resolves through the GENERALIZATION CHAIN, not the entity's own flag, and
// that distinction is load-bearing: an entity created with plain `create entity`
// that extends a non-persistent parent stores Persistable=true, and mxbuild
// still rejects a search on it with CE0251. Measured — `NpSub extends NpBase`
// reads PERSISTENT in the catalog and is CE0251 in the build, while `Sub extends
// Keeper` (persistable parent) is accepted.
//
// An entity whose chain cannot be resolved is left ALONE rather than assumed
// non-persistable. A checker that is wrong in the confident direction teaches
// people to ignore it, and here it would fire on every mapping over a module the
// script has not created and the project does not yet hold.
func persistabilityViolations(prog *ast.Program, projectPath string, elems []findElement) []linter.Violation {
	res := newPersistabilityResolver(prog, projectPath)
	defer res.close()

	var out []linter.Violation
	for _, fe := range elems {
		entity := qualifyEntity(fe.def.Entity, fe.stmt.Name.Module)
		persistable, known := res.persistable(entity)
		if !known || persistable {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-MAP03",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"import mapping %s: `%s` searches for an object, but %s is not persistable — "+
					"there is no database to search (CE0251)",
				fe.label(), fe.statement(), entity),
			Location: linter.Location{
				Module:       fe.stmt.Name.Module,
				DocumentType: "import mapping",
				DocumentName: fe.stmt.Name.Name,
			},
			Suggestion: "Use `create " + fe.def.Entity + "` instead, or make " + entity + " persistable",
		})
	}
	return out
}

// qualifyEntity gives a bare entity name the mapping's own module, which is how
// the executor resolves it too.
func qualifyEntity(entity, module string) string {
	if strings.Contains(entity, ".") || module == "" {
		return entity
	}
	return module + "." + entity
}

// persistabilityResolver answers "is this entity persistable" from the script
// first and the project second, so an entity the script creates is resolved
// before it exists on disk.
type persistabilityResolver struct {
	script  map[string]entityFacts
	project map[string]entityFacts
	reader  backend.FullBackend
}

// entityFacts is what deciding persistability needs: the entity's own flag and
// the parent it inherits from.
type entityFacts struct {
	persistable    bool
	generalization string
}

func newPersistabilityResolver(prog *ast.Program, projectPath string) *persistabilityResolver {
	r := &persistabilityResolver{script: map[string]entityFacts{}}
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateEntityStmt)
		if !ok {
			continue
		}
		facts := entityFacts{persistable: s.Kind == ast.EntityPersistent}
		if s.Generalization != nil {
			facts.generalization = s.Generalization.String()
		}
		r.script[s.Name.String()] = facts
	}
	if projectPath != "" {
		r.project, r.reader = projectEntityFacts(projectPath)
	}
	return r
}

func (r *persistabilityResolver) close() {
	if r.reader != nil {
		_ = r.reader.Disconnect()
	}
}

// persistable walks the generalization chain and reports the answer, plus
// whether it could be established at all.
//
// The visited set is not defensive tidiness: a model can carry a generalization
// cycle (Studio Pro rejects one, an mxcli-written model need not), and without
// it a check would hang rather than report.
func (r *persistabilityResolver) persistable(entity string) (bool, bool) {
	visited := map[string]bool{}
	for entity != "" {
		if visited[entity] {
			return false, false
		}
		visited[entity] = true

		facts, ok := r.lookup(entity)
		if !ok {
			return false, false
		}
		if facts.generalization == "" {
			return facts.persistable, true
		}
		// A specialization takes its parent's persistability; its own stored
		// flag is not what Mendix reads (measured — see the doc comment).
		entity = facts.generalization
	}
	return false, false
}

func (r *persistabilityResolver) lookup(entity string) (entityFacts, bool) {
	if facts, ok := r.script[entity]; ok {
		return facts, true
	}
	facts, ok := r.project[entity]
	return facts, ok
}

// projectEntityFacts reads every entity's persistability and parent from the
// project. A project that cannot be opened yields none, which silences MDL-MAP03
// rather than failing the check on something it could not inspect — the same
// fail-open as offlineProfilesIn.
func projectEntityFacts(projectPath string) (map[string]entityFacts, backend.FullBackend) {
	reader := openProjectForValidation(projectPath)
	if reader == nil {
		return nil, nil
	}
	dms, err := reader.ListDomainModels()
	if err != nil {
		_ = reader.Disconnect()
		return nil, nil
	}
	modules, err := reader.ListModules()
	if err != nil {
		_ = reader.Disconnect()
		return nil, nil
	}
	moduleName := map[string]string{}
	for _, m := range modules {
		if m != nil {
			moduleName[string(m.ID)] = m.Name
		}
	}

	out := map[string]entityFacts{}
	for _, dm := range dms {
		name := moduleName[string(dm.ContainerID)]
		if name == "" {
			continue
		}
		for _, e := range dm.Entities {
			if e == nil {
				continue
			}
			out[name+"."+e.Name] = entityFactsOf(e)
		}
	}
	return out, reader
}

func entityFactsOf(e *domainmodel.Entity) entityFacts {
	return entityFacts{
		persistable:    e.Persistable,
		generalization: e.GeneralizationRef,
	}
}
