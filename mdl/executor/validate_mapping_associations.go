// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// CE0295 "Association '…' is not allowed." — an object mapping element, in an
// import or an export mapping, whose association does not connect the entity it
// sits under to its own entity. Measured on MxBuild 11.16.1, both mapping kinds:
//
//	the association touches neither entity            CE0295
//	touches the parent, not the element's entity      CE0295
//	touches the element's entity, not the parent      CE0295
//	connects the two, in either direction             clean
//	an end is a generalization of the parent, of the
//	element's entity, or of both                      clean
//
// `check` and `exec` accepted every rejected shape and wrote the mapping. (A
// Reference whose cardinality does not fit the element — a JSON object over a
// to-many association — is CE0295 too, but is a different rule and is not
// covered here.)
//
// The association is looked up in the script first, then the project; the
// generalization chain of each entity the same way. An association that cannot
// be found, or a chain that cannot be read to its root, is never refused: the
// question is then unanswered, not answered "no".
const mappingAssociationRule = "MDL100"

// mappingNode is the part of an import or export element this rule reads.
type mappingNode struct {
	entity, assoc, name string
	children            []mappingNode
}

func exportMappingNode(d *ast.ExportMappingElementDef) mappingNode {
	n := mappingNode{entity: d.Entity, assoc: d.Association, name: d.JsonName}
	for _, c := range d.Children {
		if c != nil {
			n.children = append(n.children, exportMappingNode(c))
		}
	}
	return n
}

func importMappingNode(d *ast.ImportMappingElementDef) mappingNode {
	n := mappingNode{entity: d.Entity, assoc: d.Association, name: d.JsonName}
	for _, c := range d.Children {
		if c != nil {
			n.children = append(n.children, importMappingNode(c))
		}
	}
	return n
}

// ValidateMappingAssociations reports each mapping element whose association
// connects neither the entity above it nor its own entity (CE0295). open, which
// may be nil, supplies the project for what the script does not declare.
func ValidateMappingAssociations(prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	if prog == nil {
		return nil
	}
	r := newMappingAssocResolver(prog, open)
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateExportMappingStmt:
			if s.RootElement != nil {
				out = append(out, r.walk("export mapping", s.Name, exportMappingNode(s.RootElement))...)
			}
		case *ast.CreateImportMappingStmt:
			if s.RootElement != nil {
				out = append(out, r.walk("import mapping", s.Name, importMappingNode(s.RootElement))...)
			}
		}
	}
	return out
}

type mappingAssocResolver struct {
	scriptEntities map[string]string    // lower-cased entity QN -> lower-cased generalization QN ("" = none)
	scriptAssocs   map[string][2]string // lower-cased association QN -> lower-cased FROM, TO entity QNs
	open           func() backend.FullBackend
	opened         bool
	project        entityLookupBackend
	catalog        associationCatalog
}

func newMappingAssocResolver(prog *ast.Program, open func() backend.FullBackend) *mappingAssocResolver {
	r := &mappingAssocResolver{
		scriptEntities: map[string]string{}, scriptAssocs: map[string][2]string{}, open: open,
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt:
			gen := ""
			if s.Generalization != nil {
				gen = qualifyIn(s.Name.Module, s.Generalization.String())
			}
			r.scriptEntities[strings.ToLower(s.Name.String())] = gen
		case *ast.CreateAssociationStmt:
			from := qualifyIn(s.Name.Module, s.Parent.String())
			to := qualifyIn(s.Name.Module, s.Child.String())
			r.scriptAssocs[strings.ToLower(s.Name.String())] = [2]string{strings.ToLower(from), strings.ToLower(to)}
		}
	}
	return r
}

// qualifyIn gives a bare name the module it is written in, as the builders do.
func qualifyIn(module, name string) string {
	if name == "" || strings.Contains(name, ".") || module == "" {
		return name
	}
	return module + "." + name
}

func (r *mappingAssocResolver) projectBackend() entityLookupBackend {
	if !r.opened {
		r.opened = true
		if r.open != nil {
			if b := r.open(); b != nil {
				if lb, ok := b.(entityLookupBackend); ok {
					r.project = lb
					r.catalog = associationCatalogFor(lb)
				}
			}
		}
	}
	return r.project
}

// ends returns the FROM and TO entities of an association, lower-cased.
func (r *mappingAssocResolver) ends(assocQN string) (from, to string, ok bool) {
	key := strings.ToLower(assocQN)
	if e, found := r.scriptAssocs[key]; found {
		return e[0], e[1], true
	}
	if r.projectBackend() == nil {
		return "", "", false
	}
	if sh, found := r.catalog.shapes[key]; found {
		return strings.ToLower(sh.from), strings.ToLower(sh.to), true
	}
	return "", "", false
}

// chain is the entity followed by its generalizations, keyed lower-cased, and
// whether the walk reached the root. The project is asked with the spelling the
// names were written in; only the comparison ignores case.
func (r *mappingAssocResolver) chain(entityQN string) (map[string]bool, bool) {
	set := map[string]bool{}
	for cur := entityQN; cur != ""; {
		key := strings.ToLower(cur)
		if set[key] {
			return set, true // a cycle: Mendix rejects it elsewhere
		}
		set[key] = true
		if gen, ok := r.scriptEntities[key]; ok {
			cur = gen
			continue
		}
		b := r.projectBackend()
		if b == nil {
			return set, false
		}
		ent, found := findEntityByQN(b, cur)
		if !found {
			return set, false
		}
		cur = ent.GeneralizationRef
	}
	return set, true
}

func (r *mappingAssocResolver) walk(kind string, name ast.QualifiedName, root mappingNode) []linter.Violation {
	var out []linter.Violation
	var visit func(n mappingNode, parentEntity string)
	visit = func(n mappingNode, parentEntity string) {
		for _, c := range n.children {
			childEntity := qualifyIn(name.Module, c.entity)
			if c.entity != "" && c.assoc != "" && parentEntity != "" {
				if v, bad := r.check(kind, name, c, qualifyIn(name.Module, c.assoc), parentEntity, childEntity); bad {
					out = append(out, v)
				}
			}
			next := parentEntity
			if c.entity != "" {
				next = childEntity
			}
			visit(c, next)
		}
	}
	visit(root, qualifyIn(name.Module, root.entity))
	return out
}

func (r *mappingAssocResolver) check(kind string, name ast.QualifiedName, el mappingNode, assocQN, parentEntity, entity string) (linter.Violation, bool) {
	from, to, ok := r.ends(assocQN)
	if !ok {
		return linter.Violation{}, false
	}
	parent, parentDone := r.chain(parentEntity)
	own, ownDone := r.chain(entity)
	if (parent[from] && own[to]) || (parent[to] && own[from]) {
		return linter.Violation{}, false
	}
	if !parentDone || !ownDone {
		return linter.Violation{}, false
	}
	return linter.Violation{
		RuleID:   mappingAssociationRule,
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s %s: element %q maps %s through %s, but that association runs between %s and %s, "+
				"not between %s and %s — mxbuild reports CE0295 \"Association '%s' is not allowed\"",
			kind, name.String(), el.name, entity, assocQN, from, to, parentEntity, entity, assocQN),
		Location: linter.Location{DocumentType: kind, DocumentName: name.String()},
		Suggestion: fmt.Sprintf(
			"Use an association that connects %s and %s (or entities they extend), or map the element under the entity that association does connect.",
			parentEntity, entity),
	}, true
}
