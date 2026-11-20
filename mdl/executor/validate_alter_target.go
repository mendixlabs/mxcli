// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// An ALTER names a document that must already exist, and nothing checked that it
// did. `alter page RestLab."RestLab_Home"` — no such page — passed
// `mxcli check -p --references` and was then refused by `exec` with "page not
// found" (ako/mxcli-rest FINDINGS #60).
//
// Harmless in itself, and still worth closing, because it inverts the contract
// the check-syntax skill states: check is meant to be the strict gate and exec
// the thing that runs. Here exec was stricter, so a script could pass every
// pre-flight and then stop halfway through, having applied the statements before
// the typo and none after.
//
// The switch above this one already resolves the ALTER's MODULE. The document
// itself was the gap, which is why a misspelled module was reported and a
// misspelled document was not — a distinction with no meaning to the author.
//
// Scope. This covers the ALTER statements whose target is a document resolvable
// from a listing that already exists, and each one below was measured passing
// check and failing exec before it was added. It is not extended speculatively:
// a target this cannot resolve must be left alone rather than guessed at, since
// a false "not found" blocks a script that would have worked, which is worse
// than the silence it replaces.

// validateAlterTarget reports a statement whose target document does not
// exist, counting documents the script itself creates or lands under the name.
//
// It began with ALTER; drop, rename and move name an existing document the same
// way and passed check while exec refused them — 9 of the 11 forms that name
// one (ako/mxcli#1821). They resolve against the same listings, so each kind
// below means the same thing for every verb.
func validateAlterTarget(ctx *ExecContext, stmt ast.Statement, sc *scriptContext) error {
	if !ctx.Connected() {
		return nil
	}
	// DROP … IF EXISTS names something that may legitimately be absent.
	if g, ok := stmt.(ast.IfExistsDrop); ok && g.DropIfExists() {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.AlterPageStmt:
		// One statement type, three document kinds — the visitor sets
		// ContainerType from the keyword.
		switch strings.ToUpper(s.ContainerType) {
		case "SNIPPET":
			return checkTarget(ctx, sc, "alter", "snippet", s.PageName)
		case "LAYOUT":
			return checkTarget(ctx, sc, "alter", "layout", s.PageName)
		default:
			return checkTarget(ctx, sc, "alter", "page", s.PageName)
		}
	case *ast.AlterEntityStmt:
		return checkTarget(ctx, sc, "alter", "entity", s.Name)
	case *ast.AlterEnumerationStmt:
		return checkTarget(ctx, sc, "alter", "enumeration", s.Name)
	case *ast.AlterFlowStmt:
		return checkTarget(ctx, sc, "alter", s.Kind(), s.Name)
	case *ast.DropMicroflowStmt:
		return checkTarget(ctx, sc, "drop", "microflow", s.Name)
	case *ast.DropNanoflowStmt:
		return checkTarget(ctx, sc, "drop", "nanoflow", s.Name)
	case *ast.DropPageStmt:
		return checkTarget(ctx, sc, "drop", "page", s.Name)
	case *ast.DropSnippetStmt:
		return checkTarget(ctx, sc, "drop", "snippet", s.Name)
	case *ast.DropEntityStmt:
		return checkTarget(ctx, sc, "drop", "entity", s.Name)
	case *ast.DropEnumerationStmt:
		return checkTarget(ctx, sc, "drop", "enumeration", s.Name)
	case *ast.DropAssociationStmt:
		return checkTarget(ctx, sc, "drop", "association", s.Name)
	case *ast.DropConstantStmt:
		return checkTarget(ctx, sc, "drop", "constant", s.Name)
	case *ast.RenameStmt:
		if s.DryRun {
			return nil
		}
		return checkTarget(ctx, sc, "rename", s.ObjectType, s.Name)
	case *ast.MoveStmt:
		return checkTarget(ctx, sc, "move", moveTargetKind(s.DocumentType), s.Name)
	}
	return nil
}

// moveTargetKind is a move's document type as the kind name rename and the
// target check use; "" for one they do not resolve.
func moveTargetKind(t ast.DocumentType) string {
	switch t {
	case ast.DocumentTypePage:
		return "page"
	case ast.DocumentTypeMicroflow:
		return "microflow"
	case ast.DocumentTypeNanoflow:
		return "nanoflow"
	case ast.DocumentTypeSnippet:
		return "snippet"
	case ast.DocumentTypeEntity:
		return "entity"
	case ast.DocumentTypeEnumeration:
		return "enumeration"
	case ast.DocumentTypeConstant:
		return "constant"
	}
	return ""
}

// targetListing is, per kind, the project's documents and the script's own.
// A kind missing here is not resolved: a false "not found" blocks a script
// that would have run, which is worse than the silence it replaces.
func targetListing(ctx *ExecContext, sc *scriptContext, kind string) (stored func() map[string]bool, inScript func(string) bool) {
	switch kind {
	case "page":
		return func() map[string]bool { return buildPageQualifiedNames(ctx) }, func(qn string) bool { return sc.pages[qn] }
	case "snippet":
		return func() map[string]bool { return buildSnippetQualifiedNames(ctx) }, func(qn string) bool { return sc.snippets[qn] }
	case "layout":
		return func() map[string]bool { return buildLayoutQualifiedNames(ctx) }, func(qn string) bool { return sc.layouts[qn] }
	case "microflow":
		return func() map[string]bool { return buildMicroflowQualifiedNames(ctx) }, func(qn string) bool { return sc.microflows[qn] }
	case "nanoflow":
		return func() map[string]bool { return buildNanoflowQualifiedNames(ctx) }, func(qn string) bool { return sc.nanoflows[qn] }
	case "entity":
		// A MOVE or RENAME earlier in the script lands an entity under a name
		// no CREATE declares (mendixlabs/mxcli#1601).
		return func() map[string]bool { return buildEntityQualifiedNames(ctx) },
			func(qn string) bool { return sc.entities[qn] || sc.indirectEntities[qn] }
	case "enumeration":
		return func() map[string]bool { return buildEnumerationQualifiedNames(ctx) }, func(qn string) bool { return sc.enumerations[qn] }
	case "association":
		return func() map[string]bool { return buildAssociationQualifiedNames(ctx) },
			func(qn string) bool {
				_, name, _ := strings.Cut(qn, ".")
				return sc.associations[name] == qn || sc.ambiguousAssc[name]
			}
	case "constant":
		return func() map[string]bool {
				names, ok := buildConstantQualifiedNames(ctx)
				if !ok {
					return nil
				}
				return names
			},
			func(qn string) bool { return sc.constants[qn] }
	}
	return nil, nil
}

// checkTarget resolves one target, and on a miss reports the near names so
// the author can see the typo rather than only that they made one.
func checkTarget(ctx *ExecContext, sc *scriptContext, verb, kind string, name ast.QualifiedName) error {
	stored, inScript := targetListing(ctx, sc, kind)
	if stored == nil {
		return nil
	}
	qn := name.String()
	// An unqualified name is a different error, already reported elsewhere; a
	// module the script creates or renames has no listing to resolve against
	// yet; and a module something fills without naming its documents cannot
	// be judged from the statements.
	if name.Module == "" || sc.modules[name.Module] || sc.renamedModules[name.Module] ||
		sc.opaqueScript || sc.opaqueModules[name.Module] ||
		(kind == "entity" && sc.externalEntityModules[name.Module]) {
		return nil
	}
	if inScript(qn) || sc.indirectDocs[kind+":"+qn] {
		return nil
	}
	known := stored()
	if known[qn] {
		return nil
	}
	// An empty listing means the backend could not answer, not that the project
	// has no pages. Reporting "not found" from it would fail every script.
	if len(known) == 0 {
		return nil
	}
	return mdlerrors.NewNotFoundMsg(kind, qn, fmt.Sprintf(
		"%s not found: %s (referenced by %s %s)%s", kind, qn, verb, kind,
		nearNamesIn(known, name.Module)))
}

// nearNamesIn lists what the module does have, capped so a large module does not
// bury the error. The same shape as availableAttributes.
func nearNamesIn(stored map[string]bool, module string) string {
	const max = 8
	var in []string
	for qn := range stored {
		if strings.HasPrefix(qn, module+".") {
			in = append(in, strings.TrimPrefix(qn, module+"."))
		}
	}
	if len(in) == 0 {
		return ""
	}
	sort.Strings(in)
	if len(in) > max {
		return fmt.Sprintf(" — %s has %s and %d more", module,
			strings.Join(in[:max], ", "), len(in)-max)
	}
	return fmt.Sprintf(" — %s has %s", module, strings.Join(in, ", "))
}

// buildLayoutQualifiedNames returns every layout qualified name in the project.
func buildLayoutQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	layouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return result
	}
	for _, l := range layouts {
		if l == nil {
			continue
		}
		result[h.GetQualifiedName(l.ContainerID, l.Name)] = true
	}
	return result
}
