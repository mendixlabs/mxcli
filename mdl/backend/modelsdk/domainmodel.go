// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDm "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	genTexts "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/texts"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// This file is the gen→domainmodel READ adapter. engalar's fork changed the
// DomainModelBackend interface to traffic in *genDm types and deleted
// sdk/domainmodel, so there is no converter to port — keeping main's executor
// and domainmodel types canonical means we own this translation. Phase 1 covers
// the breadth SHOW ENTITIES needs (names, persistability, generalization, and
// faithful member counts); full attribute-type / association-detail fidelity
// (DESCRIBE level) is a later phase.

// ListDomainModels reads every domain model through the codec engine.
func (b *Backend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	units, err := mprread.ListUnitsWithContainer[*genDm.DomainModel](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*domainmodel.DomainModel, 0, len(units)+1)
	for _, u := range units {
		out = append(out, domainModelFromGen(u.Element, u.ContainerID))
	}
	// The System module is virtual (not stored in the project); inject its domain
	// model so platform entities (System.WorkflowUserTask, User, …) resolve.
	out = append(out, buildSystemDomainModel())
	return out, nil
}

// GetDomainModel returns the domain model whose container is moduleID.
func (b *Backend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	units, err := mprread.ListUnitsWithContainer[*genDm.DomainModel](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if u.ContainerID == moduleID {
			return domainModelFromGen(u.Element, u.ContainerID), nil
		}
	}
	// The System module is virtual — its domain model is not stored in the project,
	// so serve the injected one (matching ListDomainModels) so DESCRIBE System.* and
	// any lookup of a platform entity's domain model resolves instead of erroring.
	if sys := buildSystemDomainModel(); sys.ContainerID == moduleID {
		return sys, nil
	}
	// Match the legacy contract: a missing domain model is an error, not (nil, nil).
	// Callers (e.g. finalizeProgramExecution after a module drop) rely on the error
	// to skip — returning (nil, nil) caused a nil-pointer panic downstream.
	return nil, fmt.Errorf("domain model not found for module: %s", moduleID)
}

func domainModelFromGen(dm *genDm.DomainModel, containerID model.ID) *domainmodel.DomainModel {
	out := &domainmodel.DomainModel{ContainerID: containerID}
	out.ID = model.ID(dm.ID())
	for _, el := range dm.EntitiesItems() {
		if e, ok := el.(*genDm.Entity); ok {
			out.Entities = append(out.Entities, entityFromGen(e))
		}
	}
	for _, el := range dm.AssociationsItems() {
		if a, ok := el.(*genDm.Association); ok {
			out.Associations = append(out.Associations, assocFromGen(a))
		}
	}
	// Cross-module associations live in a separate collection (the FROM entity's
	// module owns them, the TO entity is referenced BY_NAME). Without this loop
	// they were invisible to LIST/SHOW ASSOCIATIONS and DESCRIBE MODULE.
	for _, el := range dm.CrossAssociationsItems() {
		if ca, ok := el.(*genDm.CrossAssociation); ok {
			out.CrossAssociations = append(out.CrossAssociations, crossAssocFromGen(ca))
		}
	}
	return out
}

func entityFromGen(e *genDm.Entity) *domainmodel.Entity {
	out := &domainmodel.Entity{
		Name:          e.Name(),
		Documentation: e.Documentation(),
		Persistable:   true, // default; NoGeneralization overrides below
	}
	out.ID = model.ID(e.ID())

	// Generalization element is either NoGeneralization (carries persistability
	// + system-attribute flags) or Generalization (extends a parent entity).
	switch g := e.Generalization().(type) {
	case *genDm.NoGeneralization:
		out.Persistable = g.Persistable()
		out.HasOwner = g.HasOwner()
		out.HasChangedBy = g.HasChangedBy()
		out.HasChangedDate = g.HasChangedDate()
		out.HasCreatedDate = g.HasCreatedDate()
	case *genDm.Generalization:
		out.GeneralizationRef = g.GeneralizationQualifiedName()
		// Persistability is inherited from the parent chain; default true
		// matches legacy (sdk/mpr parser_domainmodel.go).
	}

	out.Location = parseLocation(e.Location())

	// View entities carry an OqlViewEntitySource referencing their source
	// document by qualified name; surface it so read-modify-write paths (e.g.
	// MOVE ENTITY, which must reparent the source doc) can see it.
	if src, ok := e.Source().(*genDm.OqlViewEntitySource); ok {
		out.Source = "DomainModels$OqlViewEntitySource"
		out.SourceObjectID = model.ID(src.ID())
		out.SourceDocumentRef = src.SourceDocumentQualifiedName()
		out.OqlQuery = src.Oql()
	}

	for _, el := range e.AttributesItems() {
		if a, ok := el.(*genDm.Attribute); ok {
			out.Attributes = append(out.Attributes, attributeFromGen(a))
		}
	}
	for _, el := range e.AccessRulesItems() {
		if ar, ok := el.(*genDm.AccessRule); ok {
			out.AccessRules = append(out.AccessRules, accessRuleFromGen(ar))
		}
	}
	for _, el := range e.IndexesItems() {
		if idx, ok := el.(*genDm.Index); ok {
			out.Indexes = append(out.Indexes, indexFromGen(idx))
		}
	}
	for _, el := range e.ValidationRulesItems() {
		if vr, ok := el.(*genDm.ValidationRule); ok {
			out.ValidationRules = append(out.ValidationRules, validationRuleFromGen(vr))
		}
	}
	for _, el := range e.EventHandlersItems() {
		if eh, ok := el.(*genDm.EventHandler); ok {
			out.EventHandlers = append(out.EventHandlers, eventHandlerFromGen(eh))
		}
	}
	return out
}

// parseLocation converts a gen "X;Y" location string to a model.Point.
func parseLocation(s string) model.Point {
	var p model.Point
	if s == "" {
		return p
	}
	if _, err := fmt.Sscanf(s, "%d;%d", &p.X, &p.Y); err != nil {
		return model.Point{}
	}
	return p
}

// attributeFromGen converts a gen Attribute to a lossless domainmodel.Attribute
// (name, documentation, full type, and default value) so a read-modify-write
// round-trip (ALTER ENTITY) reproduces the attribute faithfully.
func attributeFromGen(a *genDm.Attribute) *domainmodel.Attribute {
	attr := &domainmodel.Attribute{
		Name:          a.Name(),
		Documentation: a.Documentation(),
		Type:          attributeTypeFromGen(a.Type()),
	}
	attr.ID = model.ID(a.ID())
	switch v := a.Value().(type) {
	case *genDm.StoredValue:
		attr.Value = &domainmodel.AttributeValue{DefaultValue: v.DefaultValue()}
	case *genDm.OqlViewValue:
		// View-entity attribute: the OQL column reference must survive a
		// read-modify-write (e.g. MOVE ENTITY) or the view goes out of sync (CE6770).
		attr.Value = &domainmodel.AttributeValue{ViewReference: v.Reference()}
	}
	return attr
}

// eventHandlerFromGen converts a gen EventHandler to a lossless
// domainmodel.EventHandler so ALTER ENTITY preserves entity events on round-trip.
// The gen reads the correct storage keys (Type, SendInputParameter) after the
// storage-name override; the microflow is a by-name reference.
func eventHandlerFromGen(eh *genDm.EventHandler) *domainmodel.EventHandler {
	out := &domainmodel.EventHandler{
		Moment:            domainmodel.EventMoment(eh.Moment()),
		Event:             domainmodel.EventType(eh.Event()),
		MicroflowName:     eh.MicroflowQualifiedName(),
		RaiseErrorOnFalse: eh.RaiseErrorOnFalse(),
		PassEventObject:   eh.PassEventObject(),
	}
	out.ID = model.ID(eh.ID())
	return out
}

// accessRuleFromGen converts a gen AccessRule to a lossless domainmodel.AccessRule
// (module roles by qualified name, allow flags, default member rights, XPath, and
// per-member accesses) so ALTER ENTITY preserves entity security on round-trip.
// AllowRead/AllowWrite are intentionally absent — Mendix stores read/write per
// member (MemberAccess.AccessRights), not at the rule level.
func accessRuleFromGen(ar *genDm.AccessRule) *domainmodel.AccessRule {
	out := &domainmodel.AccessRule{
		ModuleRoleNames:           ar.ModuleRolesQualifiedNames(),
		AllowCreate:               ar.AllowCreate(),
		AllowDelete:               ar.AllowDelete(),
		DefaultMemberAccessRights: domainmodel.MemberAccessRights(ar.DefaultMemberAccessRights()),
		XPathConstraint:           ar.XPathConstraint(),
	}
	out.ID = model.ID(ar.ID())
	for _, el := range ar.MemberAccessesItems() {
		if ma, ok := el.(*genDm.MemberAccess); ok {
			out.MemberAccesses = append(out.MemberAccesses, memberAccessFromGen(ma))
		}
	}
	return out
}

// memberAccessFromGen converts a gen MemberAccess (attribute/association ref by
// qualified name + access rights) to a domainmodel.MemberAccess.
func memberAccessFromGen(ma *genDm.MemberAccess) *domainmodel.MemberAccess {
	out := &domainmodel.MemberAccess{
		AttributeName:   ma.AttributeQualifiedName(),
		AssociationName: ma.AssociationQualifiedName(),
		AccessRights:    domainmodel.MemberAccessRights(ma.AccessRights()),
	}
	out.ID = model.ID(ma.ID())
	return out
}

// validationRuleFromGen converts a gen ValidationRule to a lossless
// domainmodel.ValidationRule (attribute ref by qualified name, rule type, and
// error message text) so ALTER ENTITY preserves validations on round-trip.
func validationRuleFromGen(vr *genDm.ValidationRule) *domainmodel.ValidationRule {
	out := &domainmodel.ValidationRule{
		AttributeID: model.ID(vr.AttributeQualifiedName()), // qualified name; ruleInfoToGen handles it
		Type:        ruleTypeFromGen(vr.RuleInfo()),
	}
	out.ID = model.ID(vr.ID())
	if txt, ok := vr.ErrorMessage().(*genTexts.Text); ok {
		out.ErrorMessage = textFromGen(txt)
	}
	return out
}

// ruleTypeFromGen maps a gen RuleInfo element back to the domainmodel rule-type
// string (reverse of ruleInfoToGen, which today emits Unique/Required).
func ruleTypeFromGen(ri element.Element) string {
	if ri == nil {
		return "Required"
	}
	switch ri.TypeName() {
	case "DomainModels$UniqueRuleInfo":
		return "Unique"
	default:
		return "Required"
	}
}

// textFromGen converts a gen Text (translations) back to a model.Text.
func textFromGen(t *genTexts.Text) *model.Text {
	out := &model.Text{Translations: map[string]string{}}
	for _, el := range t.TranslationsItems() {
		if tr, ok := el.(*genTexts.Translation); ok {
			out.Translations[tr.LanguageCode()] = tr.Text()
		}
	}
	return out
}

// indexFromGen converts a gen EntityIndex to a lossless domainmodel.Index so an
// ALTER ENTITY round-trip preserves the index (segment attribute + sort order).
func indexFromGen(idx *genDm.Index) *domainmodel.Index {
	out := &domainmodel.Index{}
	out.ID = model.ID(idx.ID())
	for _, el := range idx.AttributesItems() {
		if ia, ok := el.(*genDm.IndexedAttribute); ok {
			seg := &domainmodel.IndexAttribute{
				AttributeID: model.ID(ia.AttributeRefID()),
				Ascending:   ia.Ascending(),
			}
			seg.ID = model.ID(ia.ID())
			out.Attributes = append(out.Attributes, seg)
		}
	}
	return out
}

// attributeTypeFromGen is the reverse of attributeTypeToGen: a gen attribute-type
// element back to a domainmodel.AttributeType (with Length / enumeration ref).
func attributeTypeFromGen(t element.Element) domainmodel.AttributeType {
	switch at := t.(type) {
	case *genDm.StringAttributeType:
		return &domainmodel.StringAttributeType{Length: int(at.Length())}
	case *genDm.IntegerAttributeType:
		return &domainmodel.IntegerAttributeType{}
	case *genDm.LongAttributeType:
		return &domainmodel.LongAttributeType{}
	case *genDm.DecimalAttributeType:
		return &domainmodel.DecimalAttributeType{}
	case *genDm.BooleanAttributeType:
		return &domainmodel.BooleanAttributeType{}
	case *genDm.DateTimeAttributeType:
		return &domainmodel.DateTimeAttributeType{}
	case *genDm.AutoNumberAttributeType:
		return &domainmodel.AutoNumberAttributeType{}
	case *genDm.BinaryAttributeType:
		return &domainmodel.BinaryAttributeType{}
	case *genDm.HashedStringAttributeType:
		return &domainmodel.HashedStringAttributeType{}
	case *genDm.EnumerationAttributeType:
		return &domainmodel.EnumerationAttributeType{EnumerationRef: at.EnumerationQualifiedName()}
	default:
		return &domainmodel.StringAttributeType{}
	}
}

func assocFromGen(a *genDm.Association) *domainmodel.Association {
	out := &domainmodel.Association{
		Name:          a.Name(),
		Documentation: a.Documentation(),
		ParentID:      model.ID(a.ParentRefID()), // FROM entity (owns the FK)
		ChildID:       model.ID(a.ChildRefID()),  // TO entity
		// Type/Owner/StorageFormat/DeleteBehavior MUST be read here: the executor's
		// CREATE OR MODIFY ASSOCIATION path re-serializes the whole domain model via
		// UpdateDomainModel→assocToGen, which reads these off the semantic model.
		// Dropping them here wiped Type/Owner on every *other* association (Studio
		// Pro then fails to load the domain model: "cannot destructure property
		// 'child' from null").
		Type:          domainmodel.AssociationType(a.Type()),
		Owner:         domainmodel.AssociationOwner(a.Owner()),
		StorageFormat: domainmodel.AssociationStorageFormat(a.StorageFormat()),
	}
	out.ID = model.ID(a.ID())
	if db, ok := a.DeleteBehavior().(*genDm.AssociationDeleteBehavior); ok && db != nil {
		out.ParentDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorType(db.ParentDeleteBehavior())}
		out.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorType(db.ChildDeleteBehavior())}
	}
	return out
}

// crossAssocFromGen converts a gen CrossAssociation (cross-module: local FROM
// entity by ID, remote TO entity by qualified name) to the semantic model.
func crossAssocFromGen(ca *genDm.CrossAssociation) *domainmodel.CrossModuleAssociation {
	out := &domainmodel.CrossModuleAssociation{
		Name:          ca.Name(),
		Documentation: ca.Documentation(),
		ParentID:      model.ID(ca.ParentRefID()), // local FROM entity (owns the FK)
		ChildRef:      ca.ChildQualifiedName(),    // remote TO entity, BY_NAME
		Type:          domainmodel.AssociationType(ca.Type()),
		Owner:         domainmodel.AssociationOwner(ca.Owner()),
		StorageFormat: domainmodel.AssociationStorageFormat(ca.StorageFormat()),
	}
	out.ID = model.ID(ca.ID())
	if db, ok := ca.DeleteBehavior().(*genDm.AssociationDeleteBehavior); ok && db != nil {
		out.ParentDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorType(db.ParentDeleteBehavior())}
		out.ChildDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorType(db.ChildDeleteBehavior())}
	}
	return out
}
