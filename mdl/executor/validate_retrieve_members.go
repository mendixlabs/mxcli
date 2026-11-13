// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A retrieve constraint naming a member its entity does not have passed
// `check --references` and `exec`, and mxbuild then reported CE0161 "Error(s)
// in XPath constraint" (mendixlabs/mxcli#1213). The widget data source had this
// check since #1049; the microflow retrieve did not, and neither looked at an
// entity the same script declares — the ordinary shape, since the entity and
// the microflow querying it are usually one script.
//
// Measured on mxbuild 11.13.0 (entity with `CreatedDate: AutoCreatedDate`):
//
//	[NoSuchAttr = 'x']                        CE0161
//	[CreatedDate > '[%CurrentDateTime%]']     CE0161  (the spelling describe prints)
//	[createdDate > '[%CurrentDateTime%]']     clean
//
// The resolution is unresolvableXPathStepsWith, the widget check's walk, over the
// constraint exactly as the flow builder stores it (retrieveXPathConstraint), so
// what is checked is what is written. Silence where the entity cannot be
// established, as everywhere in that walk.

// validateRetrieveMembers reports the members of each database retrieve's
// constraint that resolve to nothing on the retrieved entity.
func validateRetrieveMembers(ctx *ExecContext, retrieves []retrieveConstraintRef, sc *scriptContext) []string {
	if len(retrieves) == 0 {
		return nil
	}
	m, resolve := scriptXPathResolver(ctx, sc)

	var errs []string
	for _, r := range retrieves {
		if r.entity == "" || r.stored == "" {
			continue
		}
		for _, bad := range unresolvableXPathStepsAndLeadingEntity(ctx, m, r.stored, r.entity, resolve) {
			if bad.kind == "entity" {
				errs = append(errs, fmt.Sprintf("retrieve from %s: %s",
					r.entity, leadingEntityStepMessage(ctx, r.stored, bad.name, r.entity)))
				continue
			}
			errs = append(errs, fmt.Sprintf(
				"retrieve from %s: the constraint names %q, which is neither an attribute nor an association of it — mxbuild rejects the constraint (CE0161 \"Error(s) in XPath constraint\")%s",
				r.entity, bad.name, systemMemberSpellingHint(bad.name)))
		}
	}
	return errs
}

// scriptXPathResolver is the model and bare-member lookup an XPath member walk
// (unresolvableXPathStepsWith) needs to judge a constraint against the project
// AND the entities and associations the script declares. Shared by the
// microflow retrieve check and the access-rule check (mendixlabs/mxcli#1472).
func scriptXPathResolver(ctx *ExecContext, sc *scriptContext) (*scriptXPathModel, func(entityQN, member string) memberResolution) {
	// exec validates a statement with no script context. It declares nothing,
	// so an empty one answers the same; the closure below used to read the nil
	// pointer and panic (ako/mxcli#2237).
	if sc == nil {
		sc = newScriptContext()
	}
	m := &scriptXPathModel{base: &execXPathModel{ctx: ctx}, ctx: ctx, sc: sc}
	var assocs map[string]string // unqualified association name -> qualified, loaded on demand
	resolve := func(entityQN, member string) memberResolution {
		// An entity the script alters may gain the member there: what the
		// ALTER leaves is not worked out, so its members go unjudged.
		if sc.alteredEntities[entityQN] {
			return memberUnknown
		}
		decl := sc.entityDecls[entityQN]
		if decl == nil {
			return resolveMemberOnEntity(ctx, entityQN, member)
		}
		// Declared by the script: its attribute list is what exec writes. An
		// inherited member cannot be judged from the declaration.
		if decl.Generalization != nil {
			return memberUnknown
		}
		for _, a := range decl.Attributes {
			if a.Name == member && !isAutoSystemMemberType(a.Type.Kind) {
				return memberFound
			}
		}
		// A bare association is a different mistake, with its own rule and fix
		// (MDL-XPATH01) — not "names nothing".
		if sc.associations[member] != "" || sc.ambiguousAssc[member] {
			return memberFound
		}
		if assocs == nil {
			assocs = buildAssociationIndex(ctx)
		}
		if assocs[member] != "" {
			return memberFound
		}
		return memberMissing
	}
	return m, resolve
}

// scriptXPathModel answers xpathrefs.Model from the project AND the script.
//
// The project-only model reported an association the SCRIPT creates as naming
// nothing whenever the retrieved entity was already in the project — the
// ordinary shape of a script that adds an association to an existing domain
// model and then queries over it: `retrieve … from M.Child where
// [M.Child_Parent = $P]` failed check while mxbuild built it clean (11.14.0).
type scriptXPathModel struct {
	base *execXPathModel
	ctx  *ExecContext
	sc   *scriptContext
}

func (m *scriptXPathModel) IsEntity(qn string) bool {
	if m.sc != nil && m.sc.entities[qn] {
		return true
	}
	return m.base.IsEntity(qn)
}

func (m *scriptXPathModel) AssociationTarget(qn, from string) (string, bool) {
	if m.sc != nil {
		if ends, ok := m.sc.associationEnds[strings.ToLower(qn)]; ok && from != "" {
			// Either end may be traversed, through the start entity's
			// generalization chain as for a stored association.
			chain := []string{from}
			if c, _ := generalizationChain(m.ctx, from); len(c) > 0 {
				chain = c
			}
			for _, e := range chain {
				switch e {
				case ends[0]:
					return ends[1], true
				case ends[1]:
					return ends[0], true
				}
			}
		}
	}
	return m.base.AssociationTarget(qn, from)
}

// isAutoSystemMemberType reports the attribute types that are not attributes in
// the model: the entity stores a flag, and XPath names the member in lower
// camel case (createdDate) whatever the declaration called it.
func isAutoSystemMemberType(k ast.DataTypeKind) bool {
	switch k {
	case ast.TypeAutoCreatedDate, ast.TypeAutoChangedDate, ast.TypeAutoOwner, ast.TypeAutoChangedBy:
		return true
	}
	return false
}

// systemMemberSpellingHint names the XPath spelling of a system member written
// another way: the way describe prints the attribute (`CreatedDate`), a bare
// association (`owner`, CE0161), or a mis-cased qualified one (`System.Owner`,
// CE1613).
func systemMemberSpellingHint(name string) string {
	bare := name
	if len(bare) > len("System.") && strings.EqualFold(bare[:len("System.")], "System.") {
		bare = bare[len("System."):]
	}
	for _, spelling := range xpathSystemMemberSpellings {
		if spelling == name {
			continue
		}
		if strings.EqualFold(strings.TrimPrefix(spelling, "System."), bare) {
			return fmt.Sprintf(". In XPath the system member is spelled `%s`", spelling)
		}
	}
	return ""
}

// xpathSystemMemberSpellings are the four system members as XPath accepts them.
// The dates are attributes, written bare; owner and changedBy are associations
// to System.User, written qualified.
var xpathSystemMemberSpellings = []string{"createdDate", "changedDate", "System.owner", "System.changedBy"}
