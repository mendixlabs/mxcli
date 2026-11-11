// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// CREATE / DROP / DESCRIBE / SHOW MESSAGE DEFINITION COLLECTION, and the two
// ALTER families (ako/mxcli#272).
//
// The executor's job is resolution: turn what the author wrote into the
// properties Mendix stores. Almost all of them are derived — see the codec
// writer for the measured table — and the two that need the domain model are
// here:
//
//   - an ATTRIBUTE resolves to the entity that DECLARES it, following the
//     generalization chain. 398 of 3,697 exposed attributes in the corpus are
//     inherited (10.8%), and qualifying one against the entity that merely uses
//     it is CE1613.
//   - an ASSOCIATION's MaxOccurs comes from the DIRECTION of traversal. See
//     resolveAssociationCardinality.

// execCreateMessageDefinitionCollection handles CREATE [OR MODIFY].
func execCreateMessageDefinitionCollection(ctx *ExecContext, s *ast.CreateMessageDefinitionCollectionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}
	existing := findMessageCollection(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("message definition collection", s.Name.String())
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}
	containerID := module.ID
	if s.Folder != "" {
		folderID, ferr := resolveFolder(ctx, module.ID, s.Folder)
		if ferr != nil {
			return mdlerrors.NewBackend("resolve folder "+s.Folder, ferr)
		}
		containerID = folderID
	} else if existing != nil {
		// Without a folder clause an existing document stays where it is.
		containerID = existing.ContainerID
	}

	c := &model.MessageDefinitionCollection{
		ContainerID: containerID,
		Name:        s.Name.Name,
	}
	if existing != nil {
		// Carry what the statement does not restate.
		c.ID = existing.ID
		c.Documentation = existing.Documentation
		c.Excluded = existing.Excluded
		c.ExportLevel = existing.ExportLevel
	}

	for _, def := range s.Definitions {
		built, berr := buildMessageDefinition(ctx, def, s.Name.String())
		if berr != nil {
			return berr
		}
		c.Definitions = append(c.Definitions, built)
	}

	if existing != nil {
		if err := ctx.Backend.UpdateMessageDefinitionCollection(c); err != nil {
			return mdlerrors.NewBackend("update message definition collection", err)
		}
		if _, err := applyDocumentFolder(ctx, c.ID, existing.ContainerID, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "message definition collection: %s", s.Name.String())
		return nil
	}
	if err := ctx.Backend.CreateMessageDefinitionCollection(c); err != nil {
		return mdlerrors.NewBackend("create message definition collection", err)
	}
	// The document — and any folder resolveFolder just created for it — is not
	// in the cached hierarchy, so a later statement looking this collection up
	// by module would not find it and would create a DUPLICATE (CE0122). The
	// update branch gets this for free from applyDocumentFolder; the create
	// branch has to say it.
	invalidateHierarchy(ctx)
	ctx.ReportMutation("Created", "message definition collection: %s", s.Name.String())
	return nil
}

// findMessageCollection looks a collection up by module and name.
func findMessageCollection(ctx *ExecContext, moduleName, name string) *model.MessageDefinitionCollection {
	all, err := ctx.Backend.ListMessageDefinitionCollections()
	if err != nil {
		return nil
	}
	h, herr := getHierarchy(ctx)
	for _, c := range all {
		if c == nil || c.Name != name {
			continue
		}
		if herr != nil {
			return c
		}
		if h.GetModuleName(h.FindModuleID(c.ContainerID)) == moduleName {
			return c
		}
	}
	return nil
}

// lookupAssociation finds an association by qualified name, in the module the
// name gives — scanning every domain model would pick up a same-named
// association from elsewhere.
func lookupAssociation(ctx *ExecContext, assocQN string) (*domainmodel.Association, bool) {
	parts := strings.SplitN(assocQN, ".", 2)
	if len(parts) != 2 {
		return nil, false
	}
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return nil, false
	}
	mod, err := b.GetModuleByName(parts[0])
	if err != nil || mod == nil {
		return nil, false
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return nil, false
	}
	for _, a := range dm.Associations {
		if a != nil && a.Name == parts[1] {
			return a, true
		}
	}
	// A cross-module association is stored on the module of its FROM entity, so
	// it may not be in the module its name suggests.
	for _, a := range dm.CrossAssociations {
		if a != nil && a.Name == parts[1] {
			return nil, false // shape differs; handled as unresolvable for now
		}
	}
	return nil, false
}

// entityQNByID resolves an entity ID to its qualified name.
func entityQNByID(ctx *ExecContext, id model.ID) string {
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return ""
	}
	dms, err := b.ListDomainModels()
	if err != nil {
		return ""
	}
	h, herr := getHierarchy(ctx)
	for _, dm := range dms {
		for _, e := range dm.Entities {
			if e != nil && e.ID == id {
				if herr != nil {
					return e.Name
				}
				return h.GetModuleName(h.FindModuleID(dm.ContainerID)) + "." + e.Name
			}
		}
	}
	return ""
}

// buildMessageDefinition resolves one `definition <Name> for <Entity>` block.
func buildMessageDefinition(ctx *ExecContext, def *ast.MessageDefinitionDef, collection string) (*model.MessageDefinition, error) {
	return buildMessageDefinitionAs(ctx, def, collection,
		fmt.Sprintf("message definition collection %s: definition %s", collection, def.Name))
}

// buildMessageDefinitionAs is buildMessageDefinition for either storage: owner
// qualifies the definition in member errors (the collection, or the module of
// an 11.15 document) and label opens the entity error.
func buildMessageDefinitionAs(ctx *ExecContext, def *ast.MessageDefinitionDef, owner, label string) (*model.MessageDefinition, error) {
	collection := owner
	entityQN := def.Entity.String()
	if _, ok := lookupEntity(ctx, entityQN); !ok {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"%s names the entity %s, which does not exist", label, entityQN))
	}

	root := &model.MessageDefinitionElement{
		Kind:         "Entity",
		Entity:       entityQN,
		OriginalName: shortEntityName(entityQN),
		// A definition's root always repeats — 56 of 56 in the corpus — so it
		// always carries an item name, and that name is the entity's own.
		MaxOccurs:       -1,
		ExposedItemName: shortEntityName(entityQN),
	}
	root.ExposedName = def.ExposedName
	if root.ExposedName == "" {
		// Studio Pro pluralises here. mxcli does not guess English: it defaults
		// to the entity's own name and lets `as 'Orders'` say otherwise, the
		// same conclusion reached for array-item naming in ako/mxcli#272.
		root.ExposedName = root.OriginalName
	}

	for _, m := range def.Members {
		child, err := buildMessageMember(ctx, m, entityQN, collection, def.Name)
		if err != nil {
			return nil, err
		}
		root.Children = append(root.Children, child)
	}
	return &model.MessageDefinition{Name: def.Name, Root: root}, nil
}

// buildMessageMember resolves one member against the entity that holds it.
func buildMessageMember(ctx *ExecContext, m *ast.MessageMemberDef, holderQN, collection, definition string) (*model.MessageDefinitionElement, error) {
	where := fmt.Sprintf("message definition %s.%s", collection, definition)

	if !m.IsAssociation() {
		ref, ok := declaringAttributeRef(ctx, holderQN, m.Attribute)
		if !ok {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"%s: %s has no attribute %q%s", where, holderQN, m.Attribute,
				availableAttributes(ctx, holderQN)))
		}
		e := &model.MessageDefinitionElement{
			Kind:          "Attribute",
			Attribute:     ref,
			OriginalName:  m.Attribute,
			ExposedName:   m.ExposedName,
			Example:       m.Example,
			MaxOccurs:     1,
			PrimitiveType: resolveMessageMemberType(ctx, holderQN, m.Attribute),
		}
		if e.ExposedName == "" {
			e.ExposedName = m.Attribute
		}
		return e, nil
	}

	assocQN := m.Association.String()
	targetQN := m.Entity.String()
	maxOccurs, err := resolveAssociationCardinality(ctx, assocQN, holderQN, targetQN, where)
	if err != nil {
		return nil, err
	}

	e := &model.MessageDefinitionElement{
		Kind:         "Entity",
		Association:  assocQN,
		Entity:       targetQN,
		OriginalName: shortEntityName(targetQN),
		MaxOccurs:    maxOccurs,
		ExposedName:  m.ExposedName,
	}
	if e.ExposedName == "" {
		e.ExposedName = e.OriginalName
	}
	// ExposedItemName is set exactly when the element repeats — 461 of 461 — and
	// its value is the target entity's own name.
	if maxOccurs == -1 {
		e.ExposedItemName = e.OriginalName
	}

	for _, sub := range m.Members {
		child, cerr := buildMessageMember(ctx, sub, targetQN, collection, definition)
		if cerr != nil {
			return nil, cerr
		}
		e.Children = append(e.Children, child)
	}
	return e, nil
}

// resolveAssociationCardinality returns the MaxOccurs an exposed association
// stores: whether the element is a single object or a list.
//
// It is a function of the direction of traversal AND the association's type,
// and the second half was learned the hard way (ako/mxcli-rest FINDINGS #60).
//
//	                       forward (holder is FROM)   reverse (holder is TO)
//	Reference                        1                        -1
//	Reference, Owner Both            1                         1
//	ReferenceSet                    -1                        -1
//
// The reverse is always a list: many holders point at one target, so from the
// target's side the element repeats. The forward direction is where the type
// decides — a Reference gives one object per holder, a ReferenceSet gives many.
//
// Direction ALONE looked exceptionless because the demo corpus it was measured
// on contains no ReferenceSet: all 927 resolvable associations are `Reference`,
// yet 526 store 1 and 401 store -1, which pins the direction half and says
// nothing about the type half. ako/TestApp confirms the direction half in one
// document: Mappings.Order_Customer stores 1 reaching Customer from Order and
// -1 reaching Order from Customer.
//
// Unlike the direction half, getting the type half wrong DOES have a build
// error behind it — mxbuild reports CE6524 "The occurrence of '...' has
// changed" on the definition, plus CE0295 on any object mapping element bound
// to it. Measured on ako/mxcli-rest at 11.13.0 against a 0-error baseline.
//
// The one-to-one row is the same rule seen from the other side: Owner Both on a
// Reference means each target has at most one holder, so the reverse is single
// too. Storing -1 there is CE6524 on the definition (mendixlabs/mxcli#1386);
// measured on 11.12.1, where 1 is what clears it.
//
// An association that connects the two entities in NEITHER direction is refused
// rather than defaulted. A wrong cardinality is worse than a refusal: for a
// Reference it builds clean and exposes a list as a single object.
func resolveAssociationCardinality(ctx *ExecContext, assocQN, holderQN, targetQN, where string) (int, error) {
	assoc, ok := lookupAssociation(ctx, assocQN)
	if !ok {
		return 0, mdlerrors.NewValidation(fmt.Sprintf(
			"%s: the association %s does not exist", where, assocQN))
	}
	fromQN, toQN := associationEnds(ctx, assoc)
	switch {
	case fromQN == holderQN && toQN == targetQN:
		// Following the reference: one target per holder for a Reference, many
		// for a ReferenceSet.
		if assoc.Type == domainmodel.AssociationTypeReferenceSet {
			return -1, nil
		}
		return 1, nil
	case toQN == holderQN && fromQN == targetQN:
		// The reverse: many holders point at one target, so from the target's
		// side the element repeats — unless the Reference is owned by both
		// ends, which makes it one-to-one and single this way too.
		if assoc.Type == domainmodel.AssociationTypeReference && assoc.Owner == domainmodel.AssociationOwnerBoth {
			return 1, nil
		}
		return -1, nil
	}
	return 0, mdlerrors.NewValidation(fmt.Sprintf(
		"%s: the association %s does not connect %s to %s — it runs from %s to %s. "+
			"The direction decides whether the element is a single object or a list, "+
			"so mxcli refuses rather than guessing",
		where, assocQN, holderQN, targetQN, fromQN, toQN))
}

// associationEnds returns the association's FROM and TO entity qualified names.
//
// ParentID is the FROM entity (the one that owns the foreign key) and ChildID
// the TO entity — the inversion documented in CLAUDE.md, and the reason this is
// a named function rather than two field reads at the call site.
func associationEnds(ctx *ExecContext, assoc *domainmodel.Association) (string, string) {
	return entityQNByID(ctx, assoc.ParentID), entityQNByID(ctx, assoc.ChildID)
}

func shortEntityName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// availableAttributes lists what the holder does have, so a typo says what would
// have worked — the shape #882 established for mapping members.
func availableAttributes(ctx *ExecContext, entityQN string) string {
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return ""
	}
	var names []string
	for _, mem := range EntityMembersFor(b, entityQN) {
		names = append(names, mem.Name)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return "; available: " + strings.Join(names, ", ")
}

func declaringAttributeRef(ctx *ExecContext, entityQN, attr string) (string, bool) {
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return "", false
	}
	return DeclaringMemberRef(b, entityQN, attr)
}

// messagePrimitiveTypes maps a Mendix attribute type to the PrimitiveType a
// message definition stores. Measured across 3,372 exposed attributes in the
// demo corpus and ako/TestApp:
//
//	Decimal      -> Decimal    1104
//	Integer      -> Integer     658
//	DateTime     -> DateTime    567
//	String       -> String      566
//	Enumeration  -> String      263
//	Boolean      -> Boolean     198
//	Long         -> Integer      12
//	AutoNumber   -> Integer       4
//
// The three that are NOT identity are the point: passing the attribute's type
// straight through stores Long and AutoNumber where Mendix stores Integer, and
// Enumeration where it stores String — 279 elements, and the round trip against
// TestApp caught exactly that (ProductId is a Long).
var messagePrimitiveTypes = map[string]string{
	"Enumeration": "String",
	"Long":        "Integer",
	"AutoNumber":  "Integer",
}

func resolveMessageMemberType(ctx *ExecContext, entityQN, attr string) string {
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return "String"
	}
	t := ResolveMemberType(b, entityQN, attr)
	if t == "" {
		return "String"
	}
	if mapped, ok := messagePrimitiveTypes[t]; ok {
		return mapped
	}
	return t
}

func lookupEntity(ctx *ExecContext, entityQN string) (*domainmodel.Entity, bool) {
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return nil, false
	}
	return findEntityByQN(b, entityQN)
}

// execDropMessageDefinitionCollection deletes a collection.
//
// A mapping bound to it would be left dangling — mxbuild reports CE1613 — so a
// collection still referenced is refused, naming the mappings. That is the same
// courtesy `drop json structure` owes and the reason the refusal lists them
// rather than saying "in use".
func execDropMessageDefinitionCollection(ctx *ExecContext, s *ast.DropMessageDefinitionCollectionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}
	c := findMessageCollection(ctx, s.Name.Module, s.Name.Name)
	if c == nil {
		return mdlerrors.NewNotFound("message definition collection", s.Name.String())
	}
	if users := mappingsUsingCollection(ctx, s.Name.String()); len(users) > 0 {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"message definition collection %s is still used by %s — dropping it would leave "+
				"them bound to nothing (CE1613)", s.Name.String(), strings.Join(users, ", ")))
	}
	if err := ctx.Backend.DeleteMessageDefinitionCollection(string(c.ID)); err != nil {
		return mdlerrors.NewBackend("drop message definition collection", err)
	}
	ctx.ReportMutation("Dropped", "message definition collection: %s", s.Name.String())
	return nil
}

// messageDefinitionsUsingAssociation returns the definitions that expose the
// named association, each as a ready ALTER statement that would remove the
// member.
//
// A message definition is a SELECTION over the domain model, so it holds the
// association by qualified name and nothing keeps the two in step: dropping the
// association leaves the definition pointing at nothing, and mxbuild rejects the
// project with CE1613 "The selected association … no longer exists". `describe`
// goes on emitting the member, so the break survives a describe -> exec round
// trip (ako/mxcli-rest FINDINGS #60).
//
// The whole element tree is walked, not the root's own children: a definition
// nests, and the association that breaks is usually not at the top.
//
// The remedy is spelled out rather than described because the two names in it
// differ and guessing wrong is the likely outcome: `drop member` matches the
// member's ORIGINAL name (the target entity's, e.g. RateSnapshot) while an `in`
// path segment matches the EXPOSED one (e.g. Snapshots). Told only "remove the
// member", an author reaches for the name they wrote in the definition, which is
// the exposed one, and gets "message definition member not found".
func messageDefinitionsUsingAssociation(ctx *ExecContext, assocQN string) []string {
	colls, err := ctx.Backend.ListMessageDefinitionCollections()
	if err != nil {
		colls = nil // the 11.15 documents below are still checked
	}
	h, herr := getHierarchy(ctx)

	var out []string
	for _, c := range colls {
		if c == nil {
			continue
		}
		collName := c.Name
		if herr == nil {
			if m := h.GetModuleName(h.FindModuleID(c.ContainerID)); m != "" {
				collName = m + "." + c.Name
			}
		}
		for _, def := range c.Definitions {
			if def == nil {
				continue
			}
			for _, hit := range findAssociationMembers(def.Root, assocQN, nil) {
				stmt := fmt.Sprintf("alter message definition %s.%s drop member %s",
					collName, def.Name, hit.member)
				if len(hit.path) > 0 {
					stmt += " in " + strings.Join(hit.path, "/")
				}
				out = append(out, stmt)
			}
		}
	}
	// 11.15 documents (ako/mxcli#987): addressed by their two-part name.
	if docs, derr := ctx.Backend.ListMessageDefinitionDocuments(); derr == nil {
		for _, d := range docs {
			if d == nil {
				continue
			}
			for _, hit := range findAssociationMembers(d.Root, assocQN, nil) {
				stmt := fmt.Sprintf("alter message definition %s drop member %s",
					messageDocumentQN(ctx, d), hit.member)
				if len(hit.path) > 0 {
					stmt += " in " + strings.Join(hit.path, "/")
				}
				out = append(out, stmt)
			}
		}
	}
	sort.Strings(out)
	return out
}

// associationMemberHit is one exposed member that reaches through the
// association: its original name, and the exposed-name path to its holder.
type associationMemberHit struct {
	member string
	path   []string
}

func findAssociationMembers(n *model.MessageDefinitionElement, assocQN string, path []string) []associationMemberHit {
	if n == nil {
		return nil
	}
	var out []associationMemberHit
	for _, c := range n.Children {
		if c == nil {
			continue
		}
		if c.Association == assocQN {
			out = append(out, associationMemberHit{member: c.OriginalName, path: append([]string(nil), path...)})
			// No recursion into a member that is itself going away.
			continue
		}
		out = append(out, findAssociationMembers(c, assocQN, append(path, c.ExposedName))...)
	}
	return out
}

// mappingsUsingCollection returns the mappings whose source is a definition in
// this collection. A mapping's reference is three parts
// (Module.Collection.Definition), so the collection is its prefix.
func mappingsUsingCollection(ctx *ExecContext, collectionQN string) []string {
	prefix := collectionQN + "."
	var out []string
	if ims, err := ctx.Backend.ListImportMappings(); err == nil {
		for _, im := range ims {
			if im != nil && strings.HasPrefix(im.MessageDefinitionSource(), prefix) {
				out = append(out, "import mapping "+im.Name)
			}
		}
	}
	if ems, err := ctx.Backend.ListExportMappings(); err == nil {
		for _, em := range ems {
			if em != nil && strings.HasPrefix(em.MessageDefinitionSource(), prefix) {
				out = append(out, "export mapping "+em.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// execDescribeMessageDefinitionCollection prints re-executable MDL.
func execDescribeMessageDefinitionCollection(ctx *ExecContext, name ast.QualifiedName) error {
	c := findMessageCollection(ctx, name.Module, name.Name)
	if c == nil && projectStoresMessageDocuments(ctx) {
		return messageCollectionsRemovedError(ctx, "describe message definition collection")
	}
	if c == nil {
		return mdlerrors.NewNotFound("message definition collection", name.String())
	}
	fmt.Fprintf(ctx.Output, "create or modify message definition collection %s\n", name.String())
	if h, err := getHierarchy(ctx); err == nil {
		if folder := h.BuildFolderPath(c.ContainerID); folder != "" {
			fmt.Fprintf(ctx.Output, "  folder %s\n", mdlQuote(ctx, folder))
		}
	}
	fmt.Fprintln(ctx.Output, "{")
	for _, def := range c.Definitions {
		describeMessageDefinition(ctx, def)
	}
	fmt.Fprintln(ctx.Output, "};")
	return nil
}

// describeMessageDefinition prints one definition. Definitions and member
// trees are children, so they are in { } (R2); a definition ends in `}`, so the
// definitions need no separator.
func describeMessageDefinition(ctx *ExecContext, def *model.MessageDefinition) {
	if def == nil || def.Root == nil {
		return
	}
	fmt.Fprintf(ctx.Output, "  definition %s for %s%s {\n",
		def.Name, def.Root.Entity, exposedClause(def.Root, shortEntityName(def.Root.Entity)))
	describeMessageMembers(ctx, def.Root.Children, "    ")
	fmt.Fprintln(ctx.Output, "  }")
}

func describeMessageMembers(ctx *ExecContext, members []*model.MessageDefinitionElement, indent string) {
	for i, m := range members {
		sep := ","
		if i == len(members)-1 {
			sep = ""
		}
		switch {
		case m.Kind == "Attribute":
			name := memberAttributeName(m)
			fmt.Fprintf(ctx.Output, "%s%s%s%s%s\n", indent, name,
				exposedClause(m, name), exampleClause(m), sep)
		case len(m.Children) == 0:
			fmt.Fprintf(ctx.Output, "%s%s/%s%s { }%s\n", indent, m.Association, m.Entity,
				exposedClause(m, shortEntityName(m.Entity)), sep)
		default:
			fmt.Fprintf(ctx.Output, "%s%s/%s%s {\n", indent, m.Association, m.Entity,
				exposedClause(m, shortEntityName(m.Entity)))
			describeMessageMembers(ctx, m.Children, indent+"  ")
			fmt.Fprintf(ctx.Output, "%s}%s\n", indent, sep)
		}
	}
}

// memberAttributeName is the attribute an Attribute member is printed by: the
// last segment of the stored attribute reference, which is what create resolves
// the member against. OriginalName is the element's own stored name and lags a
// rename — `rename attribute` updates the reference and leaves the element name
// for Studio Pro's refresh, which is CE6524 until then (mendixlabs/mxcli#1733).
// Printing OriginalName made describe emit an attribute that no longer exists;
// printing the reference with the stored ExposedName as `as '…'` round-trips
// and keeps the exposed name, i.e. the message contract, unchanged.
func memberAttributeName(m *model.MessageDefinitionElement) string {
	if m.Attribute == "" {
		return m.OriginalName
	}
	if i := strings.LastIndex(m.Attribute, "."); i >= 0 {
		return m.Attribute[i+1:]
	}
	return m.Attribute
}

// exampleClause emits `example '...'`, the one authored field besides the name.
// Rare — 1 of 4,707 elements — but describe dropping it silently is what makes
// describe -> exec lossy, so it is emitted whenever set.
func exampleClause(e *model.MessageDefinitionElement) string {
	if e.Example == "" {
		return ""
	}
	return fmt.Sprintf(" example '%s'", strings.ReplaceAll(e.Example, "'", "''"))
}

// exposedClause emits `as '<Name>'` only when the exposed name differs from what
// the executor would derive, so a describe does not restate every default.
func exposedClause(e *model.MessageDefinitionElement, derived string) string {
	if e.ExposedName == "" || e.ExposedName == derived {
		return ""
	}
	return fmt.Sprintf(" as '%s'", e.ExposedName)
}

// execAlterMessageDefinitionCollection adds, drops or renames a DEFINITION.
//
// It edits the stored collection rather than rebuilding it from a statement, so
// the definitions the statement does not mention are never round-tripped through
// the describer — the argument that made ALTER LAYOUT a capability rather than a
// convenience.
func execAlterMessageDefinitionCollection(ctx *ExecContext, s *ast.AlterMessageDefinitionCollectionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}
	c := findMessageCollection(ctx, s.Name.Module, s.Name.Name)
	if c == nil {
		return mdlerrors.NewNotFound("message definition collection", s.Name.String())
	}

	switch s.Op {
	case "ADD":
		if idx := indexOfDefinition(c, s.Definition.Name); idx >= 0 {
			if s.IfNotExist {
				ctx.ReportMutation("Unchanged", "message definition collection: %s (definition %s already exists)",
					s.Name.String(), s.Definition.Name)
				return nil
			}
			return mdlerrors.NewAlreadyExists("message definition", s.Name.String()+"."+s.Definition.Name)
		}
		built, err := buildMessageDefinition(ctx, s.Definition, s.Name.String())
		if err != nil {
			return err
		}
		c.Definitions = append(c.Definitions, built)
	case "DROP":
		idx := indexOfDefinition(c, s.Target)
		if idx < 0 {
			if s.IfExists {
				ctx.ReportMutation("Unchanged", "message definition collection: %s (no definition %s)",
					s.Name.String(), s.Target)
				return nil
			}
			return mdlerrors.NewNotFound("message definition", s.Name.String()+"."+s.Target)
		}
		if users := mappingsUsingDefinition(ctx, s.Name.String()+"."+s.Target); len(users) > 0 {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"message definition %s.%s is still used by %s — dropping it would leave them "+
					"bound to nothing (CE1613)", s.Name.String(), s.Target, strings.Join(users, ", ")))
		}
		c.Definitions = append(c.Definitions[:idx], c.Definitions[idx+1:]...)
	case "RENAME":
		idx := indexOfDefinition(c, s.Target)
		if idx < 0 {
			return mdlerrors.NewNotFound("message definition", s.Name.String()+"."+s.Target)
		}
		if indexOfDefinition(c, s.NewName) >= 0 {
			return mdlerrors.NewAlreadyExists("message definition", s.Name.String()+"."+s.NewName)
		}
		// A mapping names the definition, so renaming one behind its back leaves
		// the mapping dangling. Refuse rather than rename half the model.
		if users := mappingsUsingDefinition(ctx, s.Name.String()+"."+s.Target); len(users) > 0 {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"message definition %s.%s is used by %s, which names it — renaming it here would "+
					"leave them bound to nothing (CE1613)",
				s.Name.String(), s.Target, strings.Join(users, ", ")))
		}
		c.Definitions[idx].Name = s.NewName
	default:
		return mdlerrors.NewValidation("unsupported ALTER MESSAGE DEFINITION COLLECTION operation")
	}

	if err := ctx.Backend.UpdateMessageDefinitionCollection(c); err != nil {
		return mdlerrors.NewBackend("update message definition collection", err)
	}
	ctx.ReportMutation("Modified", "message definition collection: %s", s.Name.String())
	return nil
}

// execAlterMessageDefinition adds, drops or renames a MEMBER within one
// definition.
func execAlterMessageDefinition(ctx *ExecContext, s *ast.AlterMessageDefinitionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	target, err := resolveAlterMessageDefinitionTarget(ctx, s)
	if err != nil {
		return err
	}
	def := target.def
	label := target.label
	owner, defName := target.owner, target.name

	holder, err := resolveMemberPath(def.Root, s.Path, label)
	if err != nil {
		return err
	}

	switch s.Op {
	case "ADD":
		name := memberName(s.Member)
		if findChildByName(holder, name) != nil {
			if s.IfNotExist {
				ctx.ReportMutation("Unchanged", "message definition: %s (member %s already exists)",
					label, name)
				return nil
			}
			return mdlerrors.NewAlreadyExists("message definition member", name)
		}
		built, berr := buildMessageMember(ctx, s.Member, holder.Entity, owner, defName)
		if berr != nil {
			return berr
		}
		holder.Children = append(holder.Children, built)
	case "DROP":
		child := findChildByName(holder, s.Target)
		if child == nil {
			if s.IfExists {
				ctx.ReportMutation("Unchanged", "message definition: %s (no member %s)",
					label, s.Target)
				return nil
			}
			return mdlerrors.NewNotFound("message definition member", s.Target)
		}
		holder.Children = removeChild(holder.Children, child)
	case "SET":
		child := findChildByName(holder, s.Target)
		if child == nil {
			return mdlerrors.NewNotFound("message definition member", s.Target)
		}
		// SET changes the ExposedName and nothing else. It is not a model
		// rename: the underlying attribute or association is untouched, which is
		// why the keyword is SET rather than RENAME.
		child.ExposedName = s.ExposedName
	default:
		return mdlerrors.NewValidation("unsupported ALTER MESSAGE DEFINITION operation")
	}

	if err := target.save(); err != nil {
		return err
	}
	ctx.ReportMutation("Modified", "message definition: %s", label)
	return nil
}

// alterMessageDefinitionTarget is the definition an ALTER MESSAGE DEFINITION
// edits, wherever it is stored, and how to write it back.
type alterMessageDefinitionTarget struct {
	def         *model.MessageDefinition
	label       string // the reference as written back in messages
	owner, name string // buildMessageMember's qualifiers
	save        func() error
}

// resolveAlterMessageDefinitionTarget finds the definition an ALTER names:
// Module.Name is an 11.15 document; Module.Collection.Definition is an entry of
// a collection, or — on an 11.15 project — the document `mx convert` made of
// that entry (ako/mxcli#987).
func resolveAlterMessageDefinitionTarget(ctx *ExecContext, s *ast.AlterMessageDefinitionStmt) (*alterMessageDefinitionTarget, error) {
	docTarget := func(d *model.MessageDefinitionDocument) *alterMessageDefinitionTarget {
		qn := messageDocumentQN(ctx, d)
		module := qn
		if i := strings.Index(qn, "."); i >= 0 {
			module = qn[:i]
		}
		return &alterMessageDefinitionTarget{
			def: d.Definition(), label: qn, owner: module, name: d.Name,
			save: func() error {
				if err := ctx.Backend.UpdateMessageDefinitionDocument(d); err != nil {
					return mdlerrors.NewBackend("update message definition", err)
				}
				return nil
			},
		}
	}
	if s.Document.Name != "" {
		d := findMessageDefinitionDocument(ctx, s.Document.Module, s.Document.Name)
		if d == nil {
			if err := requireMessageDefinitionDocuments(ctx, "alter message definition "+s.Document.String()); err != nil {
				return nil, err
			}
			return nil, mdlerrors.NewNotFound("message definition", s.Document.String())
		}
		return docTarget(d), nil
	}

	label := s.Collection.String() + "." + s.Definition
	c := findMessageCollection(ctx, s.Collection.Module, s.Collection.Name)
	if c == nil {
		if projectStoresMessageDocuments(ctx) {
			if d := findConvertedMessageDefinitionDocument(ctx, s.Collection.Module, s.Collection.Name, s.Definition); d != nil {
				return docTarget(d), nil
			}
		}
		return nil, mdlerrors.NewNotFound("message definition collection", s.Collection.String())
	}
	idx := indexOfDefinition(c, s.Definition)
	if idx < 0 {
		return nil, mdlerrors.NewNotFound("message definition", label)
	}
	return &alterMessageDefinitionTarget{
		def: c.Definitions[idx], label: label, owner: s.Collection.String(), name: s.Definition,
		save: func() error {
			if err := ctx.Backend.UpdateMessageDefinitionCollection(c); err != nil {
				return mdlerrors.NewBackend("update message definition collection", err)
			}
			return nil
		},
	}, nil
}

// resolveMemberPath walks `in a/b` down the definition's tree, in exposed names.
//
// Members nest to depth 7 in the corpus, so reaching one is ordinary rather than
// an edge case, and a path that reaches nothing is an error naming what is
// there — the shape MDL-JSON01 established.
func resolveMemberPath(root *model.MessageDefinitionElement, path []string, where string) (*model.MessageDefinitionElement, error) {
	node := root
	for _, seg := range path {
		child := findChildByExposedName(node, seg)
		if child == nil {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"%s: no member %q under %q%s", where, seg, node.ExposedName, availableChildren(node)))
		}
		if child.Kind != "Entity" {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"%s: %q is a value, so it has no members to reach into", where, seg))
		}
		node = child
	}
	return node, nil
}

func availableChildren(n *model.MessageDefinitionElement) string {
	var names []string
	for _, c := range n.Children {
		names = append(names, c.ExposedName)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return "; available: " + strings.Join(names, ", ")
}

// findChildByName matches on the member's OWN name — the attribute's or the
// target entity's — which is what `add`/`drop`/`set member X` names. For an
// attribute that is the attribute it references, which is how describe prints
// it; the stored element name is accepted too, since it lags a rename until
// Studio Pro refreshes the definition (mendixlabs/mxcli#1733).
func findChildByName(n *model.MessageDefinitionElement, name string) *model.MessageDefinitionElement {
	for _, c := range n.Children {
		if c.OriginalName == name || (c.Kind == "Attribute" && memberAttributeName(c) == name) {
			return c
		}
	}
	return nil
}

// findChildByExposedName matches on the exposed name, which is what an `in`
// path segment names.
func findChildByExposedName(n *model.MessageDefinitionElement, name string) *model.MessageDefinitionElement {
	for _, c := range n.Children {
		if c.ExposedName == name {
			return c
		}
	}
	return nil
}

func removeChild(children []*model.MessageDefinitionElement, target *model.MessageDefinitionElement) []*model.MessageDefinitionElement {
	out := children[:0]
	for _, c := range children {
		if c != target {
			out = append(out, c)
		}
	}
	return out
}

func memberName(m *ast.MessageMemberDef) string {
	if m == nil {
		return ""
	}
	if m.IsAssociation() {
		return shortEntityName(m.Entity.String())
	}
	return m.Attribute
}

func indexOfDefinition(c *model.MessageDefinitionCollection, name string) int {
	for i, d := range c.Definitions {
		if d != nil && d.Name == name {
			return i
		}
	}
	return -1
}

// mappingsUsingDefinition returns the mappings bound to exactly this definition.
func mappingsUsingDefinition(ctx *ExecContext, definitionQN string) []string {
	var out []string
	if ims, err := ctx.Backend.ListImportMappings(); err == nil {
		for _, im := range ims {
			if im != nil && im.MessageDefinitionSource() == definitionQN {
				out = append(out, "import mapping "+im.Name)
			}
		}
	}
	if ems, err := ctx.Backend.ListExportMappings(); err == nil {
		for _, em := range ems {
			if em != nil && em.MessageDefinitionSource() == definitionQN {
				out = append(out, "export mapping "+em.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// listMessageDefinitionCollections handles SHOW MESSAGE DEFINITION COLLECTIONS.
//
// There was no listing at all before this: a mapping could be authored over a
// definition, but nothing told you which definitions existed.
func listMessageDefinitionCollections(ctx *ExecContext, inModule string) error {
	all, err := ctx.Backend.ListMessageDefinitionCollections()
	if err != nil {
		return mdlerrors.NewBackend("list message definition collections", err)
	}
	h, herr := getHierarchy(ctx)

	type row struct{ module, name, defs string }
	var rows []row
	for _, c := range all {
		if c == nil {
			continue
		}
		module := ""
		if herr == nil {
			module = h.GetModuleName(h.FindModuleID(c.ContainerID))
		}
		if inModule != "" && !strings.EqualFold(module, inModule) {
			continue
		}
		names := make([]string, 0, len(c.Definitions))
		for _, d := range c.Definitions {
			if d != nil {
				names = append(names, d.Name)
			}
		}
		rows = append(rows, row{module, c.Name, strings.Join(names, ", ")})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].module != rows[j].module {
			return rows[i].module < rows[j].module
		}
		return rows[i].name < rows[j].name
	})

	if len(rows) == 0 {
		if projectStoresMessageDocuments(ctx) {
			// Mendix 11.15 has no collections (ako/mxcli#987): say where the
			// definitions went instead of reporting an empty project.
			fmt.Fprintln(ctx.Output, "No message definition collections found (Mendix 11.15 stores "+
				"each message definition as a document: list message definitions)")
			return nil
		}
		fmt.Fprintln(ctx.Output, "No message definition collections found")
		return nil
	}
	fmt.Fprintln(ctx.Output, "| Module | Collection | Definitions |")
	fmt.Fprintln(ctx.Output, "|--------|------------|-------------|")
	for _, r := range rows {
		fmt.Fprintf(ctx.Output, "| %s | %s | %s |\n", r.module, r.name, r.defs)
	}
	fmt.Fprintf(ctx.Output, "\n(%d collection(s))\n", len(rows))
	return nil
}
