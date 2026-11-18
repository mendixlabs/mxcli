// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// publishedAttrTypeRule is the rule ID for "an alter that leaves a published
// attribute published as the wrong type".
const publishedAttrTypeRule = "MDL102"

// CE5016 "Attribute X has type T, but is published as Y." Measured on MxBuild
// 11.16.1: after a published OData service exposes an attribute, `alter entity
// … modify attribute` to a type with another EDM type leaves the published type
// stale, and `mx check` reports one CE5016 per attribute — String to Integer,
// Decimal to Integer, Boolean to String, DateTime to String, String to Decimal.
// `check` and `exec` accepted all of them. A change inside one EDM type
// (String(50) to String(80); Integer to Long, both Edm.Int64) is clean.
//
// The way out is `create or modify published odata service` AFTER the alter,
// which re-derives every published type (0 errors); the same statement BEFORE
// the alter is CE5016 again. So the order of the script decides, and this check
// follows it: an alter is reported unless a later statement in the script
// re-publishes (or alters, or drops) the service it leaves stale.
//
// Services are read from the script and from the project (open may be nil, in
// which case only the script's own are known). Anything whose published or new
// type cannot be worked out is not reported: an unanswered question is not "no".
func ValidatePublishedAttributeTypes(prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	if prog == nil {
		return nil
	}
	s := &pubTypeScan{
		open:    open,
		pubs:    map[pubAttrKey]map[string]string{},
		attrEdm: map[pubAttrKey]string{},
		display: map[string]string{},
	}
	for _, stmt := range prog.Statements {
		switch st := stmt.(type) {
		case *ast.CreateEntityStmt:
			for i := range st.Attributes {
				if edm, ok := astTypeToEdm(st.Attributes[i].Type); ok {
					s.attrEdm[pubAttrKey{lowerQN(st.Name.String()), strings.ToLower(st.Attributes[i].Name)}] = edm
				}
			}
		case *ast.CreateODataServiceStmt:
			s.republish(st)
		case *ast.AlterODataServiceStmt:
			s.forget(lowerQN(st.Name.String()))
		case *ast.DropODataServiceStmt:
			s.forget(lowerQN(st.Name.String()))
		case *ast.AlterEntityStmt:
			if st.Operation == ast.AlterEntityModifyAttribute && !st.Calculated {
				s.modify(st)
			}
		}
	}
	var out []linter.Violation
	for _, p := range s.pending {
		out = append(out, linter.Violation{
			RuleID:   publishedAttrTypeRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"attribute %s.%s is published by %s as %s, but this statement changes it to a type published as %s — "+
					"the published type is not updated, and mxbuild reports CE5016 \"Attribute '%s' has type …, but is published as %s\"",
				p.entity, p.attr, p.service, p.from, p.to, p.attr, p.from),
			Location: linter.Location{DocumentType: "entity", DocumentName: p.entity},
			Suggestion: fmt.Sprintf(
				"Re-publish the service after this statement (`create or modify published odata service %s …`), "+
					"or stop exposing %s.", p.service, p.attr),
		})
	}
	return out
}

type pubAttrKey struct{ entity, attr string }

type pendingStale struct {
	key                   pubAttrKey
	svc                   string // lower-cased service QN
	service, entity, attr string // as written
	from, to              string
}

type pubTypeScan struct {
	open    func() backend.FullBackend
	loaded  bool
	pubs    map[pubAttrKey]map[string]string // attribute -> lower-cased service QN -> published EDM type
	attrEdm map[pubAttrKey]string            // the script's current EDM type per attribute
	display map[string]string                // lower-cased service QN -> as written
	pending []pendingStale
}

func lowerQN(s string) string { return strings.ToLower(s) }

// load reads the project's published services, once.
func (s *pubTypeScan) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	if s.open == nil {
		return
	}
	b := s.open()
	if b == nil {
		return
	}
	modules, err := b.ListModules()
	if err != nil {
		return
	}
	names := map[string]string{}
	for _, m := range modules {
		if m != nil {
			names[string(m.ID)] = m.Name
		}
	}
	services, err := b.ListPublishedODataServices()
	if err != nil {
		return
	}
	for _, svc := range services {
		mod := names[string(svc.ContainerID)]
		if svc == nil || mod == "" {
			continue
		}
		qn := mod + "." + svc.Name
		for _, et := range svc.EntityTypes {
			for _, m := range et.Members {
				if m.Kind != "attribute" || m.EdmType == "" {
					continue
				}
				k := pubAttrKey{lowerQN(et.Entity), strings.ToLower(bareMemberName(m.Name))}
				if s.pubs[k] == nil {
					s.pubs[k] = map[string]string{}
				}
				s.pubs[k][lowerQN(qn)] = m.EdmType
				s.display[lowerQN(qn)] = qn
			}
		}
	}
}

// forget drops what is known of a service and any staleness reported against it:
// a later statement that rewrites, alters or drops it is something this check
// cannot prove did not fix the types.
func (s *pubTypeScan) forget(svc string) {
	s.load()
	for k, m := range s.pubs {
		delete(m, svc)
		if len(m) == 0 {
			delete(s.pubs, k)
		}
	}
	kept := s.pending[:0]
	for _, p := range s.pending {
		if p.svc != svc {
			kept = append(kept, p)
		}
	}
	s.pending = kept
}

// republish records a service the script creates: every exposed attribute is
// published as the type it has at that point in the script.
func (s *pubTypeScan) republish(st *ast.CreateODataServiceStmt) {
	svc := lowerQN(st.Name.String())
	s.forget(svc)
	s.display[svc] = st.Name.String()
	for _, e := range st.Entities {
		entity := lowerQN(e.Entity.String())
		for _, m := range e.Members {
			k := pubAttrKey{entity, strings.ToLower(m.Name)}
			edm, ok := s.attrEdm[k]
			if !ok {
				edm, ok = s.projectAttrEdm(e.Entity.String(), m.Name)
			}
			if !ok {
				continue // an association, or an attribute whose type is not known here
			}
			if s.pubs[k] == nil {
				s.pubs[k] = map[string]string{}
			}
			s.pubs[k][svc] = edm
		}
	}
}

// projectAttrEdm is the EDM type an attribute of a project entity publishes as,
// for what the script never declared. The type must be read from the project:
// `create or modify` derives every published type from the attributes as they
// are when it runs.
func (s *pubTypeScan) projectAttrEdm(entityQN, attr string) (string, bool) {
	if s.open == nil {
		return "", false
	}
	b, ok := s.open().(entityLookupBackend)
	if !ok || b == nil {
		return "", false
	}
	ent, found := findEntityByQN(b, entityQN)
	if !found {
		return "", false
	}
	for _, a := range ent.Attributes {
		if a != nil && strings.EqualFold(a.Name, attr) && a.Type != nil {
			if edm := mendixAttrTypeToEdm(a.Type); edm != "" {
				return edm, true
			}
		}
	}
	return "", false
}

// modify applies one `modify attribute`: every service publishing it keeps the
// old type, which is reported when the new type publishes differently.
func (s *pubTypeScan) modify(st *ast.AlterEntityStmt) {
	s.load()
	k := pubAttrKey{lowerQN(st.Name.String()), strings.ToLower(st.AttributeName)}
	to, ok := astTypeToEdm(st.DataType)
	if !ok {
		delete(s.attrEdm, k)
		return
	}
	for svc, from := range s.pubs[k] {
		if from != to {
			s.pending = append(s.pending, pendingStale{
				key: k, svc: svc, service: s.display[svc], entity: st.Name.String(), attr: st.AttributeName,
				from: from, to: to,
			})
		}
	}
	s.attrEdm[k] = to
}

// astTypeToEdm is mendixAttrTypeToEdm for a type written in the script. The
// published type of a kind it does not know is not guessed.
func astTypeToEdm(dt ast.DataType) (string, bool) {
	switch dt.Kind {
	case ast.TypeString, ast.TypeEnumeration:
		return "Edm.String", true
	case ast.TypeInteger, ast.TypeLong, ast.TypeAutoNumber:
		return "Edm.Int64", true
	case ast.TypeDecimal:
		return "Edm.Decimal", true
	case ast.TypeBoolean:
		return "Edm.Boolean", true
	case ast.TypeDateTime, ast.TypeDate:
		return "Edm.DateTimeOffset", true
	}
	return "", false
}
