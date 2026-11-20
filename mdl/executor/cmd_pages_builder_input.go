// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// unquoteIdentifier strips surrounding double-quotes or backticks from a quoted identifier.
func unquoteIdentifier(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// unquoteQualifiedName strips quotes from each segment of a dotted qualified name.
func unquoteQualifiedName(s string) string {
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = unquoteIdentifier(p)
	}
	return strings.Join(parts, ".")
}

// resolveAttributePath resolves a short attribute name to a fully qualified name
// using the current entity context. If the attribute already has dots or no entity
// context is available, the attribute is returned as-is.
func (pb *pageBuilder) resolveAttributePath(attr string) string {
	if attr == "" {
		return ""
	}
	attr = storedSystemMemberName(attr)
	// If the attribute already contains a dot, it's already qualified
	if strings.Contains(attr, ".") {
		return attr
	}
	// If we have an entity context, prefix the attribute with it — but with the
	// entity that actually DECLARES it, which for an inherited attribute is an
	// ancestor rather than the context entity itself.
	if pb.entityContext != "" {
		if declaring, ok := pb.declaringEntityFor(pb.entityContext, attr); ok {
			return declaring + "." + attr
		}
		return pb.entityContext + "." + attr
	}
	return attr
}

// declaringEntityFor returns the entity in entityQN's generalization chain that
// declares attrName — entityQN itself when the attribute is its own.
//
// Mendix stores a page's attribute reference against the declaring entity. A
// reference qualified with a specialization that merely inherits the attribute
// is dangling, and the build fails with
//
//	[CE1613] "The selected attribute 'Module.Sub.Attr' no longer exists."
//
// which reads as if the attribute had been deleted; it never existed there.
// Entity access rules resolve inherited members correctly, so this was the page
// layer alone. (mxcli-todo findings #12)
//
// ok is false when nothing in the chain declares the name — an unknown
// attribute, or a domain model we cannot read. The caller then keeps today's
// behaviour rather than inventing a qualification.
func (pb *pageBuilder) declaringEntityFor(entityQN, attrName string) (string, bool) {
	if entityQN == "" || attrName == "" {
		return "", false
	}
	// Resolution is best-effort: without a model to consult (no backend and
	// nothing cached — e.g. a unit test building widgets in isolation) keep the
	// caller's plain context qualification rather than panicking on the lookup.
	if pb.backend == nil && (pb.execCache == nil || pb.execCache.domainModels == nil) {
		return "", false
	}
	owners, parents, err := pb.entityAttributeOwners()
	if err != nil {
		return "", false
	}
	lower := strings.ToLower(attrName)
	seen := map[string]bool{}
	for cur := entityQN; cur != "" && !seen[cur]; cur = parents[cur] {
		seen[cur] = true
		if attrs, ok := owners[cur]; ok && attrs[lower] {
			return cur, true
		}
	}
	return "", false
}

// attributeIndex is entityAttributeOwners as the index misboundAttributeError
// consults; empty (every answer "unknown") when the model cannot be read.
func (pb *pageBuilder) attributeIndex() attributeIndex {
	if pb.backend == nil && (pb.execCache == nil || pb.execCache.domainModels == nil) {
		return attributeIndex{}
	}
	owners, parents, err := pb.entityAttributeOwners()
	if err != nil {
		return attributeIndex{}
	}
	return attributeIndex{owners: owners, parents: parents}
}

// entityAttributeOwners indexes, for every entity in the project, the set of
// attribute names it declares itself, plus each entity's direct parent. The
// system members an entity stores (createdDate, changedDate, owner,
// changedBy) count as its own: Studio Pro stores a binding of one that a
// specialization inherits against the entity that stores it (ako/mxcli#2106).
func (pb *pageBuilder) entityAttributeOwners() (owners map[string]map[string]bool, parents map[string]string, err error) {
	dms, err := pb.getDomainModels()
	if err != nil {
		return nil, nil, err
	}
	h, err := pb.getHierarchy()
	if err != nil {
		return nil, nil, err
	}
	owners = make(map[string]map[string]bool, len(dms)*8)
	parents = make(map[string]string)
	for _, dm := range dms {
		mod := h.GetModuleName(dm.ContainerID)
		for _, e := range dm.Entities {
			qn := mod + "." + e.Name
			attrs := make(map[string]bool, len(e.Attributes))
			for _, a := range e.Attributes {
				attrs[strings.ToLower(a.Name)] = true
			}
			for name, stored := range map[string]bool{
				"createddate": e.HasCreatedDate, "changeddate": e.HasChangedDate,
				"owner": e.HasOwner, "changedby": e.HasChangedBy,
			} {
				if stored {
					attrs[name] = true
				}
			}
			owners[qn] = attrs
			if e.GeneralizationRef != "" {
				parents[qn] = e.GeneralizationRef
			}
		}
	}
	return owners, parents, nil
}

// rejectAssociationAsAttribute refuses a widget property that is attribute-typed
// but was given the name of an ASSOCIATION — `column c (attribute: Order_Customer)`.
//
// The reference cannot be represented. mxcli qualified the name like an attribute
// and wrote `DomainModels$AttributeRef{Attribute: "Mod.Order.Order_Customer"}`, so
// the build failed CE1613 "The selected attribute … no longer exists" — and there
// is no correct BSON to write instead: `CustomWidgets$WidgetValue.AttributeRef` is
// typed `AttributeRef`, not the polymorphic `MemberRef`. Storing an
// `AssociationRef` there makes the project UNLOADABLE ("Object of type
// 'AssociationRef' cannot be converted to type 'AttributeRef'"), and the WidgetValue
// has no association-valued property at all — `Mendix.Modeler.WebUI.dll`, which
// defines the type, carries no `AssociationRef` member.
//
// The DataGrid column's `<associationTypes>Reference/ReferenceSet</associationTypes>`
// is what makes this look supported. It is not permission to bind the reference; it
// is permission for the attribute PATH to traverse one — `attribute: Assoc/Attr`,
// which mxcli already writes as an AttributeRef with association steps. (issue #830)
//
// where names the binding for the message (e.g. "column `colCustomer`").
func (pb *pageBuilder) rejectAssociationAsAttribute(name, entityContext, where string) error {
	assocQN, fromEntity, toEntity, ok := pb.associationNamedAsAttribute(name, entityContext)
	if !ok {
		return nil
	}
	leaf := toEntity
	if idx := strings.LastIndex(leaf, "."); idx >= 0 {
		leaf = leaf[idx+1:]
	}
	return mdlerrors.NewValidationf(
		"%s binds `%s`, which is an association (%s → %s), not an attribute — "+
			"Mendix cannot store a reference in an attribute-typed widget property and the build fails "+
			"CE1613 \"The selected attribute '%s.%s' no longer exists\". "+
			"To SHOW a value from the associated object, traverse the reference: `attribute: %s/<Attr>` "+
			"(e.g. `%s/Name`). To FILTER the grid by the reference, use the drop-down filter's association mode: "+
			"`dropdownfilter f (Association: %s, datasource: database %s, CaptionAttribute: <Attr>)`",
		where, name, fromEntity, toEntity, entityContext, name,
		name, name, assocQN, toEntity)
}

// associationNamedAsAttribute reports whether name, bound where an attribute
// belongs against entityContext, is in fact an association reachable from that
// entity — and if so which one, and the entities at its two ends. It is the
// decision behind rejectAssociationAsAttribute, shared with the combo box's own
// wording of the refusal so the two cannot disagree on what they refuse.
func (pb *pageBuilder) associationNamedAsAttribute(name, entityContext string) (assocQN, fromEntity, toEntity string, ok bool) {
	// A path (`Assoc/Attr`) is the SUPPORTED form; a `$var` reference and an
	// empty binding are not ours to judge here.
	if name == "" || strings.ContainsAny(name, "/$") || entityContext == "" {
		return "", "", "", false
	}
	// Deciding attribute-vs-association needs the model; without one (no backend
	// and nothing cached — a unit test building widgets in isolation) let the
	// binding through rather than guessing, exactly as declaringEntityFor does.
	if pb.backend == nil && (pb.execCache == nil || pb.execCache.domainModels == nil) {
		return "", "", "", false
	}
	name = storedSystemMemberName(name)
	// A three-part name is already an explicit Module.Entity.Attribute.
	if strings.Count(name, ".") >= 2 {
		return "", "", "", false
	}
	// An attribute wins: entity members share one namespace, so a name that
	// resolves as an attribute anywhere in the generalization chain is not an
	// association.
	if _, found := pb.declaringEntityFor(entityContext, name); found {
		return "", "", "", false
	}
	assocQN = pb.resolveAssociationPathIn(name, entityContext)
	fromEntity, toEntity, found := pb.associationEndpoints(assocQN)
	if !found || !pb.entityInChain(entityContext, fromEntity) {
		return "", "", "", false
	}
	return assocQN, fromEntity, toEntity, true
}

// rejectAssociationAsAttributeOn is rejectAssociationAsAttribute for widget w,
// whose `Attribute:` was written `$param.name` when param is not "".
//
// A combo box gets its own remedy (mendixlabs/mxcli#1440). `combobox x
// (Attribute: $Asset.Asset_Customer, CaptionAttribute: …)` has no `DataSource:`,
// so it is the ENUMERATION mode and the association lands in
// `attributeEnumeration`; the generic advice — traverse the reference, or use a
// drop-down filter — is advice for a grid column. What the author wants is the
// association mode, which a `DataSource:` selects, and which binds a reference
// from a parameter as `Attribute: $Param.Module.Assoc` (what describe prints for
// Studio Pro's snippets; 0 errors on mx 11.6.6).
func (pb *pageBuilder) rejectAssociationAsAttributeOn(w *ast.WidgetV3, param, name, entityContext, where string) error {
	if w == nil || !strings.EqualFold(w.Type, "combobox") {
		return pb.rejectAssociationAsAttribute(name, entityContext, where)
	}
	assocQN, fromEntity, toEntity, ok := pb.associationNamedAsAttribute(name, entityContext)
	if !ok {
		return nil
	}
	binding := assocQN
	if param != "" {
		binding = "$" + param + "." + assocQN
	}
	return mdlerrors.NewValidationf(
		"%s binds `%s`, which is an association (%s → %s), not an attribute — a combo box with "+
			"`Attribute:` and no `DataSource:` is in enumeration mode, whose attribute property cannot hold a "+
			"reference, and the build fails CE1613 \"The selected attribute '%s.%s' no longer exists\". "+
			"To pick the associated object, use the combo box's association mode, which a `DataSource:` "+
			"selects: `combobox %s (Attribute: %s, DataSource: database %s, CaptionAttribute: <attribute of %s>)`",
		where, name, fromEntity, toEntity, entityContext, name,
		w.Name, binding, toEntity, toEntity)
}

// entityInChain reports whether want is entityQN or one of its ancestors, so a
// member declared on a generalization still counts as reachable from the
// specialization the widget is bound to.
func (pb *pageBuilder) entityInChain(entityQN, want string) bool {
	if want == "" {
		return false
	}
	if entityQN == want {
		return true
	}
	_, parents, err := pb.entityAttributeOwners()
	if err != nil {
		return false
	}
	seen := map[string]bool{}
	for cur := entityQN; cur != "" && !seen[cur]; cur = parents[cur] {
		seen[cur] = true
		if cur == want {
			return true
		}
	}
	return false
}

// systemMemberBindingNames maps the name an audit member is DECLARED under to
// the name Mendix actually stores it as.
//
// `alter entity … add attribute CreatedDate: AutoCreatedDate` is the spelling
// mxcli requires (it rejects any other declared name, telling you to use this
// one), but the member is stored as `createdDate` — so binding a widget to
// `CreatedDate`, the same name you just declared, failed the build with CE1613
// "The selected attribute … no longer exists" while the undocumented lowercase
// form worked. Accept the declared spelling and write the stored one.
// (issuetracker #19)
var systemMemberBindingNames = map[string]string{
	"createddate": "createdDate",
	"changeddate": "changedDate",
	"changedby":   "changedBy",
	"owner":       "owner",
}

// storedSystemMemberName maps a bare audit-member name to its stored spelling,
// leaving every other name (and any qualified or association path) untouched.
func storedSystemMemberName(attr string) string {
	if strings.ContainsAny(attr, "./$") {
		return attr
	}
	if stored, ok := systemMemberBindingNames[strings.ToLower(attr)]; ok {
		return stored
	}
	return attr
}

// resolveAssociationPath resolves a short association name to a fully qualified name.
// Associations are module-level objects, so the path is Module.AssociationName (2-part).
// If the name already contains a dot, it's returned as-is.
func (pb *pageBuilder) resolveAssociationPath(assocName string) string {
	return pb.resolveAssociationPathIn(assocName, pb.entityContext)
}

// resolveAssociationPathIn is resolveAssociationPath against an explicit entity
// context. Callers that bind a member of a *containing* entity — while
// pb.entityContext already points at their own data source — must pass that
// containing entity, or a bare name is qualified with the wrong module.
func (pb *pageBuilder) resolveAssociationPathIn(assocName, entityContext string) string {
	if assocName == "" {
		return ""
	}
	// If already qualified (contains a dot), return as-is
	if strings.Contains(assocName, ".") {
		return assocName
	}
	if qn, ok := pb.declaredAssociationQN(assocName, entityContext); ok {
		return qn
	}
	// Not in the model (or no model loaded): guess the context entity's module
	// (e.g., "PgTest.Order" → "PgTest"), so a misspelling is reported by the
	// name the author wrote.
	if entityContext != "" {
		parts := strings.SplitN(entityContext, ".", 2)
		if len(parts) >= 1 {
			return parts[0] + "." + assocName
		}
	}
	return assocName
}

// declaredAssociationQN finds the association a bare name means from
// entityContext: the one with that name that has an end on the context entity
// or one of its generalizations, qualified with the module that DECLARES it.
//
// An association's module is where it is declared, which is neither the
// context entity's module nor the option list's. Qualifying with the first
// broke inherited associations (ako/mxcli#662): Administration.Account extends
// System.User, so `UserRoles` became `Administration.UserRoles` and mxbuild
// failed CE1613 "The selected association … no longer exists". Qualifying with
// the second was issuetracker #19. Each was right only where the two modules
// happened to coincide.
//
// The chain is walked nearest-first, so a specialization's own association
// wins over a same-named one on an ancestor. A name matching more than one
// association at the same level is ambiguous; ok is false and the caller keeps
// its fallback rather than picking one.
func (pb *pageBuilder) declaredAssociationQN(assocName, entityContext string) (string, bool) {
	if entityContext == "" {
		return "", false
	}
	// Without a model (a unit test building widgets in isolation) there is
	// nothing to look up; leave the name to the caller's fallback.
	if pb.backend == nil && (pb.execCache == nil || pb.execCache.domainModels == nil) {
		return "", false
	}
	dms, err := pb.getDomainModels()
	if err != nil {
		return "", false
	}
	h, err := pb.getHierarchy()
	if err != nil {
		return "", false
	}
	parents, err := pb.entityGeneralizations()
	if err != nil {
		return "", false
	}

	entityQN := make(map[model.ID]string)
	for _, dm := range dms {
		mod := h.GetModuleName(dm.ContainerID)
		for _, e := range dm.Entities {
			entityQN[e.ID] = mod + "." + e.Name
		}
	}
	// Every association with this name, keyed by the entities at its ends.
	type candidate struct{ qn, from, to string }
	var candidates []candidate
	for _, dm := range dms {
		mod := h.GetModuleName(dm.ContainerID)
		for _, a := range dm.Associations {
			if a.Name == assocName {
				candidates = append(candidates, candidate{mod + "." + a.Name, entityQN[a.ParentID], entityQN[a.ChildID]})
			}
		}
		for _, ca := range dm.CrossAssociations {
			if ca.Name == assocName {
				candidates = append(candidates, candidate{mod + "." + ca.Name, entityQN[ca.ParentID], ca.ChildRef})
			}
		}
	}

	seen := map[string]bool{}
	for cur := entityContext; cur != "" && !seen[cur]; cur = parents[cur] {
		seen[cur] = true
		found := ""
		for _, c := range candidates {
			if c.from != cur && c.to != cur {
				continue
			}
			if found != "" && found != c.qn {
				return "", false
			}
			found = c.qn
		}
		if found != "" {
			return found, true
		}
	}
	return "", false
}

// resolveSnippetRef resolves a snippet qualified name to its ID.
func (pb *pageBuilder) resolveSnippetRef(snippetRef string) (model.ID, error) {
	if snippetRef == "" {
		return "", mdlerrors.NewValidation("empty snippet reference")
	}

	snippetRef = unquoteQualifiedName(snippetRef)
	parts := strings.Split(snippetRef, ".")
	var moduleName, snippetName string
	if len(parts) >= 2 {
		moduleName = parts[0]
		snippetName = parts[len(parts)-1]
	} else {
		snippetName = snippetRef
	}

	// First, check if the snippet was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdSnippets != nil {
		if info, ok := pb.execCache.createdSnippets[snippetRef]; ok {
			return info.ID, nil
		}
		if moduleName != "" {
			if info, ok := pb.execCache.createdSnippets[moduleName+"."+snippetName]; ok {
				return info.ID, nil
			}
		}
	}

	snippets, err := pb.backend.ListSnippets()
	if err != nil {
		return "", err
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, s := range snippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if s.Name == snippetName && (moduleName == "" || modName == moduleName) {
			return s.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("snippet", snippetRef)
}

func (pb *pageBuilder) resolveMicroflow(qualifiedName string) (model.ID, error) {
	qualifiedName = unquoteQualifiedName(qualifiedName)
	// Parse qualified name
	parts := strings.Split(qualifiedName, ".")
	if len(parts) < 2 {
		return "", mdlerrors.NewValidationf("invalid microflow name: %s", qualifiedName)
	}
	moduleName := parts[0]
	mfName := strings.Join(parts[1:], ".")

	// First, check if the microflow was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdMicroflows != nil {
		if info, ok := pb.execCache.createdMicroflows[qualifiedName]; ok {
			return info.ID, nil
		}
	}

	// Get microflows from backend
	mfs, err := pb.getMicroflows()
	if err != nil {
		return "", mdlerrors.NewBackend("list microflows", err)
	}

	// Use hierarchy to resolve module names (handles microflows in folders)
	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find matching microflow
	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == moduleName && mf.Name == mfName {
			return mf.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("microflow", qualifiedName)
}

func (pb *pageBuilder) resolvePageRef(pageRef string) (model.ID, error) {
	if pageRef == "" {
		return "", mdlerrors.NewValidation("empty page reference")
	}

	pageRef = unquoteQualifiedName(pageRef)
	parts := strings.Split(pageRef, ".")
	var moduleName, pageName string
	if len(parts) >= 2 {
		moduleName = parts[0]
		pageName = parts[len(parts)-1]
	} else {
		pageName = pageRef
	}

	// First, check if the page was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdPages != nil {
		if info, ok := pb.execCache.createdPages[pageRef]; ok {
			return info.ID, nil
		}
		// Also check with module prefix if not found
		if moduleName != "" {
			if info, ok := pb.execCache.createdPages[moduleName+"."+pageName]; ok {
				return info.ID, nil
			}
		}
	}

	pgs, err := pb.getPages()
	if err != nil {
		return "", err
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, p := range pgs {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if p.Name == pageName && (moduleName == "" || modName == moduleName) {
			return p.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("page", pageRef)
}
