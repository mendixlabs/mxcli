// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// singleEntityContract is what the contract makes of one hand-written
// `create external entity`: the entity type and set it maps, and the stated
// attributes as the contract maps them.
type singleEntityContract struct {
	et        *types.EdmEntityType
	entitySet *types.EdmEntitySet
	attrs     []*domainmodel.Attribute
	keyParts  []*domainmodel.RemoteKeyPart
}

// resolveSingleExternalEntity maps a hand-written external entity's attributes
// onto the service's cached contract (ako/mxcli#2497). The statement used to
// store each attribute as a local value with no key, and mx check 11.12.6
// reported, on a contract the bulk import builds at 0 errors:
//
//	CE6010 "Key cannot be empty"
//	CE6612 "Attribute 'X' of external entity 'E' is not supported." — every attribute
//	CE6630 "'E' is marked supports $top=True in the OData service, but False in the app." (and $skip)
//
// Each stated attribute is now the attribute the bulk import makes of the
// property of that name — mapped value, remote type, capabilities — so the
// two statements cannot disagree. What cannot build is refused, with nothing
// written: a name the contract does not have, a type it does not publish, a
// key left out, and a capability the contract contradicts.
//
// stored is the entity a rewrite replaces, nil on create. Its attributes are
// matched by their mapping as well as by name: an older import named a
// property differently (a key `_Id` where today's import says `DeviceId`), and
// describe prints the stored name.
func resolveSingleExternalEntity(ctx *ExecContext, s *ast.CreateExternalEntityStmt, moduleID model.ID, entitySetName string, stored *domainmodel.Entity) (*singleEntityContract, error) {
	qn := s.Name.String()
	doc, _, err := parseServiceContract(ctx, s.ServiceRef)
	if err != nil {
		return nil, err
	}
	var entitySet *types.EdmEntitySet
	var setNames []string
	for _, es := range doc.EntitySets {
		setNames = append(setNames, es.Name)
		if es.Name == entitySetName {
			entitySet = es
		}
	}
	if entitySet == nil {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"external entity %s: %s has no entity set %q; its entity sets are %s",
			qn, s.ServiceRef, entitySetName, strings.Join(setNames, ", ")))
	}
	typeByQualified := make(map[string]*types.EdmEntityType)
	for _, schema := range doc.Schemas {
		for _, et := range schema.EntityTypes {
			typeByQualified[schema.Namespace+"."+et.Name] = et
		}
	}
	et := typeByQualified[entitySet.EntityType]
	if et == nil {
		et = doc.FindEntityType(entitySet.EntityType)
	}
	if et == nil {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"external entity %s: the contract does not declare %s, the type of entity set %s", qn, entitySet.EntityType, entitySetName))
	}
	if s.RemoteName != nil && *s.RemoteName != "" && *s.RemoteName != et.Name && *s.RemoteName != entitySet.EntityType {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"external entity %s: RemoteName %q is not the type of entity set %s, which is %s",
			qn, *s.RemoteName, entitySetName, et.Name))
	}

	enums := &odataEnumerations{byRemote: map[string]string{}, deferred: true}
	parts := contractEntityAttributes(ctx, doc, et, typeByQualified, entitySet, true,
		s.Name.Name, moduleID, s.Name.Module, s.ServiceRef.String(), enums)
	byName := map[string]*domainmodel.Attribute{}
	byRemote := map[string]*domainmodel.Attribute{}
	var available []string
	for _, a := range parts.attrs {
		byName[a.Name] = a
		byRemote[a.RemoteName] = a
		available = append(available, a.Name+": "+getAttributeTypeName(a.Type))
	}
	sort.Strings(available)

	var problems []string
	var kept []*domainmodel.Attribute
	stated := map[string]bool{}
	// No attribute list: every property the contract has, as the bulk
	// import makes the entity.
	if len(s.Attributes) == 0 {
		kept = parts.attrs
		for _, a := range parts.attrs {
			stated[a.RemoteName] = true
		}
	}
	for _, a := range s.Attributes {
		c := byName[a.Name]
		if c == nil && stored != nil {
			for _, old := range stored.Attributes {
				if old != nil && old.Name == a.Name && old.RemoteName != "" && byRemote[old.RemoteName] != nil {
					renamed := *byRemote[old.RemoteName]
					renamed.Name = a.Name
					c = &renamed
				}
			}
		}
		if c != nil {
			stated[c.RemoteName] = true
		}
		if c == nil {
			why := "is not a property of " + et.Name
			for _, d := range parts.dropped {
				if strings.Contains(d, "."+a.Name+" ") {
					why = "cannot be imported: " + d
				}
			}
			problems = append(problems, fmt.Sprintf("%s %s", a.Name, why))
			continue
		}
		declared := convertDataType(a.Type)
		want := getAttributeTypeName(c.Type)
		if declared == nil || declared.GetTypeName() != c.Type.GetTypeName() ||
			(isEnumType(declared) && getAttributeTypeName(declared) != want) {
			problems = append(problems, fmt.Sprintf("%s is declared %s, but the service publishes it as %s, which is %s",
				a.Name, getAttributeTypeName(declared), c.RemoteType, want))
			continue
		}
		kept = append(kept, c)
	}
	// stated is keyed on the remote property, so a key kept under another
	// local name counts.
	for _, k := range parts.keyParts {
		if !stated[k.RemoteName] {
			problems = append(problems, fmt.Sprintf("%s is the key (%s) and must be listed: an external entity without it is CE6010 \"Key cannot be empty\"",
				k.Name, k.RemoteName))
		}
	}
	// The capabilities are the contract's; a stated one that differs is CE6630.
	for _, c := range []struct {
		name   string
		stated *bool
		want   bool
	}{
		{"Countable", s.Countable, entitySet.Countable == nil || *entitySet.Countable},
		{"Creatable", s.Creatable, entitySet.Insertable != nil && *entitySet.Insertable},
		{"Deletable", s.Deletable, entitySet.Deletable != nil && *entitySet.Deletable},
	} {
		if c.stated != nil && *c.stated != c.want {
			problems = append(problems, fmt.Sprintf("%s: %v, but the service says %v for entity set %s, which mx check reports as CE6630; leave it out to follow the service",
				c.name, *c.stated, c.want, entitySetName))
		}
	}
	if len(problems) > 0 {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"external entity %s: its attributes come from %s's contract, and this statement does not match it:\n  - %s\n"+
				"Nothing was written. The contract's attributes for %s are: %s. "+
				"`create or modify external entities from %s entities (%s)` imports the entity with all of them.",
			qn, s.ServiceRef, strings.Join(problems, "\n  - "), et.Name, strings.Join(available, ", "), s.ServiceRef, et.Name))
	}
	for _, a := range kept {
		if e, ok := a.Type.(*domainmodel.EnumerationAttributeType); ok {
			if err := enums.createPending(ctx, e.EnumerationRef); err != nil {
				return nil, mdlerrors.NewBackend("create enumeration", err)
			}
		}
	}
	// A key part is named as the attribute it keys (EntityKeyPartName).
	localName := map[string]string{}
	for _, a := range kept {
		localName[a.RemoteName] = a.Name
	}
	var keyParts []*domainmodel.RemoteKeyPart
	for _, k := range parts.keyParts {
		kp := *k
		if n := localName[k.RemoteName]; n != "" {
			kp.Name = n
		}
		keyParts = append(keyParts, &kp)
	}
	return &singleEntityContract{et: et, entitySet: entitySet, attrs: kept, keyParts: keyParts}, nil
}

func isEnumType(t domainmodel.AttributeType) bool {
	_, ok := t.(*domainmodel.EnumerationAttributeType)
	return ok
}
