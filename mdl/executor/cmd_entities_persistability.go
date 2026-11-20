// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A specialization has no persistability of its own (ako/mxcli#2005,
// mendixlabs/mxcli#1772). Mendix stores Persistable on the NoGeneralization of
// the root of the generalization chain, and a DomainModels$Generalization holds
// only the parent's name, so `create persistent entity Child extends Base` with
// Base non-persistent wrote a non-persistent Child and said "Created entity".
// A keyword that matches the chain restates it, which is what describe prints;
// one that contradicts it is refused — check and exec alike — naming the root
// that decides. This is the system-member case of
// cmd_entities_system_members.go again: a value the specialization has no
// place to store.

// checkSpecializationPersistability refuses a stated persistability that
// contradicts the root of s's generalization chain. sc is the script's
// declarations, so a parent created earlier in the script counts; nil at exec,
// where that parent has been written already. An unresolved chain gives no
// verdict: the generalization checks own a missing parent.
func checkSpecializationPersistability(ctx *ExecContext, sc *scriptContext, s *ast.CreateEntityStmt) error {
	if s == nil || s.Generalization == nil || !s.KindStated {
		return nil
	}
	if s.Kind != ast.EntityPersistent && s.Kind != ast.EntityNonPersistent {
		return nil
	}
	root, persistable, known := persistabilityRoot(ctx, sc, s.Generalization.String())
	stated := s.Kind == ast.EntityPersistent
	if !known || persistable == stated {
		return nil
	}
	keyword, rootIs := "persistent", "non-persistent"
	if !stated {
		keyword, rootIs = "non-persistent", "persistent"
	}
	fix := ", or make " + root + " " + keyword + " instead"
	if strings.HasPrefix(root, "System.") {
		fix = "; System entities cannot be changed, so every specialization of " + root + " is " + rootIs
	}
	return mdlerrors.NewValidationf(
		"%s extends %s, but its persistability is inherited from the root of the generalization chain, and %s is %s: "+
			"Mendix stores no persistability on a specialization, so `%s` would be dropped and %s created %s. "+
			"Write `create %s entity %s …` or leave the keyword out%s",
		s.Name.String(), s.Generalization.String(), root, rootIs, keyword, s.Name.String(), rootIs,
		rootIs, s.Name.String(), fix)
}

// persistabilityRoot follows the chain from qn to its root and reports the
// root and whether it is persistable. The script's declarations come first,
// then the project, then the System module mxcli knows.
func persistabilityRoot(ctx *ExecContext, sc *scriptContext, qn string) (root string, persistable, known bool) {
	var index map[string]*domainmodel.Entity
	seen := map[string]bool{}
	for qn != "" && !seen[qn] {
		seen[qn] = true
		if sc != nil {
			if decl := sc.entityDecls[qn]; decl != nil {
				if decl.Generalization == nil {
					switch decl.Kind {
					case ast.EntityPersistent:
						return qn, true, true
					case ast.EntityNonPersistent:
						return qn, false, true
					}
					return qn, false, false
				}
				qn = decl.Generalization.String()
				continue
			}
		}
		if index == nil {
			index = buildEntityIndex(ctx)
		}
		if e := index[qn]; e != nil {
			if e.GeneralizationRef == "" {
				return qn, e.Persistable, true
			}
			qn = e.GeneralizationRef
			continue
		}
		if strings.HasPrefix(qn, "System.") {
			p, ok := meta.SystemEntityPersistable(qn)
			return systemChainRoot(qn), p, ok
		}
		return "", false, false
	}
	return "", false, false
}
