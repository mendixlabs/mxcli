// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/generated/metamodel"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genRest "github.com/mendixlabs/mxcli/modelsdk/gen/rest"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func init() {
	// applyDefaults for domain-model elements: the fields Studio Pro adds
	// internally on create that genDm.NewEntity() does not yet set (confirmed
	// against real Studio-Pro BSON in mx-test-projects/test7-app). The encoder
	// emits these for fresh elements of the registered $Type.
	codec.RegisterTypeDefaults("DomainModels$EntityImpl", codec.TypeDefaults{
		EmitGUID:       true,
		MandatoryLists: []string{"Attributes", "AccessRules", "ValidationRules", "Indexes", "Events"},
	})
	// Attributes carry a GUID too (= their own $ID), but no member collections.
	codec.RegisterTypeDefaults("DomainModels$Attribute", codec.TypeDefaults{EmitGUID: true})
	// Associations carry a GUID and an always-null Source (for non-remote ones).
	codec.RegisterTypeDefaults("DomainModels$Association", codec.TypeDefaults{
		EmitGUID:   true,
		NullFields: []string{"Source"},
	})
	// The association's delete behavior always serializes both error-message
	// reference slots as null.
	codec.RegisterTypeDefaults("DomainModels$DeleteBehavior", codec.TypeDefaults{
		NullFields: []string{"ChildErrorMessage", "ParentErrorMessage"},
	})
	// An entity index carries a GUID (= its own $ID). Confirmed against real
	// Studio-Pro 11.x BSON (mx-test-projects/test7-app: IdxProbe).
	codec.RegisterTypeDefaults("DomainModels$EntityIndex", codec.TypeDefaults{EmitGUID: true})
	// Each index segment carries an AssociationPointer — an all-zero GUID for an
	// attribute-based segment (the gen IndexedAttribute exposes no such property).
	codec.RegisterTypeDefaults("DomainModels$IndexedAttribute", codec.TypeDefaults{
		ZeroGUIDFields: []string{"AssociationPointer"},
	})
	// The index's IndexedAttribute list uses typed-array marker 2, not the
	// domain-model default of 3 (verified in the same reference project).
	codec.RegisterListMarker("DomainModels$IndexedAttribute", 2)
}

// CreateAssociation adds an association to a domain model. Associations are
// children of the DomainModel unit (its Associations list), so this mirrors
// CreateEntity: load → add → roundtrip-encode → persist.
func (b *Backend) CreateAssociation(domainModelID model.ID, assoc *domainmodel.Association) error {
	if assoc == nil {
		return fmt.Errorf("CreateAssociation: nil association")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateAssociation: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	ga := assocToGen(assoc)
	assignAssociationIDs(ga)
	// Same contract as CreateEntity: report the minted identity rather than
	// leaving the caller holding an empty one.
	if assoc.ID == "" {
		assoc.ID = model.ID(ga.ID())
	}
	dm.AddAssociations(ga)

	contents, err := (&codec.Encoder{}).Encode(dm)
	if err != nil {
		return fmt.Errorf("CreateAssociation: encode domain model: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(domainModelID), contents); err != nil {
		return fmt.Errorf("CreateAssociation: persist domain model %s: %w", domainModelID, err)
	}
	return nil
}

// assocToGen converts a domainmodel.Association. Note the (counter-intuitive but
// internally consistent) pointer convention: ParentPointer = the FROM entity
// (FK owner), ChildPointer = the TO entity — domainmodel.ParentID/ChildID already
// follow it, so the mapping is direct. Connections are the fixed defaults Studio
// Pro/legacy emit. GUID is added by the encoder via registered defaults.
func assocToGen(a *domainmodel.Association) *genDm.Association {
	out := genDm.NewAssociation()
	out.SetName(a.Name)
	out.SetDocumentation(a.Documentation)
	out.SetExportLevel("Hidden")
	out.SetParentID(element.ID(string(a.ParentID)))
	out.SetChildID(element.ID(string(a.ChildID)))
	out.SetType(string(a.Type))
	out.SetOwner(string(a.Owner))
	sf := string(a.StorageFormat)
	if sf == "" {
		sf = "Column"
	}
	out.SetStorageFormat(sf)
	// Carry the stored line anchors rather than resetting them — assocToGen runs
	// on every association write, so hardcoding here discarded whatever the
	// developer had dragged the connector to in Studio Pro (issue #872).
	out.SetParentConnection(domainmodel.FormatConnectionPoint(a.ParentConnection, domainmodel.DefaultParentConnection))
	out.SetChildConnection(domainmodel.FormatConnectionPoint(a.ChildConnection, domainmodel.DefaultChildConnection))

	db := genDm.NewAssociationDeleteBehavior()
	parentDB, childDB := "DeleteMeButKeepReferences", "DeleteMeButKeepReferences"
	if a.ParentDeleteBehavior != nil && a.ParentDeleteBehavior.Type != "" {
		parentDB = string(a.ParentDeleteBehavior.Type)
	}
	if a.ChildDeleteBehavior != nil && a.ChildDeleteBehavior.Type != "" {
		childDB = string(a.ChildDeleteBehavior.Type)
	}
	db.SetParentDeleteBehavior(parentDB)
	db.SetChildDeleteBehavior(childDB)
	// A "delete me if no references" child side carries the message the user sees
	// when the delete is refused. Studio Pro writes a Texts$Text there and mxcli
	// wrote null, which does not fail the BUILD — it stops the runtime STARTING,
	// with a stack trace naming SchemeFactory and nothing about the model
	// (CapTrackV2 §1).
	//
	// Only that behaviour, and only the child side. A census of 47,789 units
	// across 122 projects found 423 delete behaviours, every one of them keep or
	// cascade with both messages null — so writing a text element unconditionally
	// would differ from Studio Pro on every association that exists in the wild.
	// The Studio Pro reference (ako/TestApp, Mappings.Order_Customer) shows the
	// same: the message appears on the restrict side, and ParentErrorMessage
	// stays null.
	if childDB == string(domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences) {
		db.SetChildErrorMessage(textToGen(deleteErrorText(a.ChildDeleteBehavior)))
	}
	out.SetDeleteBehavior(db)

	// An association between external entities carries a Rest$OData* source; a
	// plain association leaves Source unset (the registered TypeDefaults null it).
	if src := externalAssociationSourceToGen(a); src != nil {
		out.SetSource(src)
	}
	return out
}

func assignAssociationIDs(a *genDm.Association) {
	assignID(a)
	assignID(a.DeleteBehavior())
}

// CreateEntity is the Phase-2 write slice: add an entity to a domain model
// through the codec engine. Entities are children of the DomainModel unit, so
// this loads the DM element, adds the new entity child (marking it dirty), and
// re-encodes the whole unit — the roundtrip encoder passes existing sibling
// entities through verbatim and freshly encodes only the new one.
func (b *Backend) CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	if entity == nil {
		return fmt.Errorf("CreateEntity: nil entity")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateEntity: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	ge := entityToGen(entity, b.moduleNameFor(domainModelID), b.majorVersion())
	assignEntityIDs(ge)
	// Report the minted identities back to the caller. assignEntityIDs fills
	// them on the gen element, so a caller that did not pre-assign an ID is
	// otherwise left holding an entity with an empty one — and its very next
	// call (AddAttribute, CreateAssociation) fails with "entity not found: ".
	// The legacy writer populated these, so not doing it is a silent behaviour
	// difference rather than a design choice; examples/modify_project caught it.
	copyAssignedIDs(entity, ge)
	dm.AddEntities(ge)

	enc := &codec.Encoder{}
	contents, err := enc.Encode(dm)
	if err != nil {
		return fmt.Errorf("CreateEntity: encode domain model: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(domainModelID), contents); err != nil {
		return fmt.Errorf("CreateEntity: persist domain model %s: %w", domainModelID, err)
	}
	return nil
}

// loadDomainModelGen decodes a DomainModel unit into its gen element so it can
// be mutated and re-encoded.
func (b *Backend) loadDomainModelGen(id model.ID) (*genDm.DomainModel, error) {
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return nil, fmt.Errorf("load domain model %s: %w", id, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("domain model %s not found", id)
	}
	dec := codec.NewDecoder(codec.DefaultRegistry)
	el, err := dec.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode domain model %s: %w", id, err)
	}
	dm, ok := el.(*genDm.DomainModel)
	if !ok {
		return nil, fmt.Errorf("unit %s is not a domain model (got %T)", id, el)
	}
	return dm, nil
}

// entityToGen is the write-direction adapter (domainmodel.Entity → genDm.Entity).
// Phase-2 slice scope: the entity header — name, documentation, location, and
// generalization (NoGeneralization with persistability + system-attribute flags,
// or a Generalization parent ref). Attributes/indexes/access rules come with the
// domain-model breadth step.
func entityToGen(e *domainmodel.Entity, moduleName string, major int) *genDm.Entity {
	out := genDm.NewEntity()
	// Honor a caller-provided ID so the persisted entity keeps the same $ID the
	// caller recorded. Otherwise assignEntityIDs generates a fresh one and any
	// association the caller wires up with ChildID/ParentID = entity.ID dangles
	// — e.g. the Trip_TripTag NPE association in the external-entities import,
	// which made the project unopenable in Studio Pro (KeyNotFoundException).
	if e.ID != "" {
		out.SetID(element.ID(e.ID))
	}
	out.SetName(e.Name)
	out.SetDocumentation(e.Documentation)
	out.SetLocation(fmt.Sprintf("%d;%d", e.Location.X, e.Location.Y))
	// View entities carry an OqlViewEntitySource referencing their source document.
	// Mendix <11 also stores the OQL inline on the source object; 11+ keeps it only
	// on the ViewEntitySourceDocument (verified against the legacy serializer).
	if e.Source == "DomainModels$OqlViewEntitySource" && e.SourceDocumentRef != "" {
		src := genDm.NewOqlViewEntitySource()
		if e.SourceObjectID != "" {
			src.SetID(element.ID(e.SourceObjectID))
		}
		src.SetSourceDocumentQualifiedName(e.SourceDocumentRef)
		if major < 11 {
			src.SetOql(e.OqlQuery)
		}
		out.SetSource(src)
	}
	// External (OData remote) entities carry a Rest$OData* source referencing the
	// consumed service (issue #718). Without it the entity serializes as a plain
	// persistent/non-persistent entity, losing its "external / from service"
	// nature. The attributes also switch to OData mapped values (isExternal).
	isExternal := e.Source == "Rest$ODataRemoteEntitySource" ||
		e.Source == "Rest$ODataEntityTypeSource" ||
		e.Source == "Rest$ODataPrimitiveCollectionEntitySource"
	if isExternal && e.RemoteServiceName != "" {
		out.SetSource(externalEntitySourceToGen(e))
	}
	// ExportLevel is a mandatory scalar NewEntity's pending applyDefaults does not
	// yet set (engalar tech-debt Fix 4). The entity GUID and the empty member
	// arrays (Attributes/AccessRules/…) legacy also emits are NOT settable on the
	// gen Entity / not emitted for unset PartLists — that residual is the
	// documented applyDefaults gap (see the write-parity known-gap test).
	out.SetExportLevel("Hidden")

	if e.GeneralizationRef != "" {
		g := genDm.NewGeneralization()
		g.SetGeneralizationQualifiedName(e.GeneralizationRef)
		out.SetGeneralization(g)
	} else {
		ng := genDm.NewNoGeneralization()
		ng.SetPersistable(e.Persistable)
		// Legacy omits the system-attribute flags when false, so only set the
		// true ones to keep the BSON in parity.
		if e.HasOwner {
			ng.SetHasOwner(true)
		}
		if e.HasChangedBy {
			ng.SetHasChangedBy(true)
		}
		if e.HasCreatedDate {
			ng.SetHasCreatedDate(true)
		}
		if e.HasChangedDate {
			ng.SetHasChangedDate(true)
		}
		out.SetGeneralization(ng)
	}

	for _, a := range e.Attributes {
		out.AddAttributes(attributeToGen(a, isExternal))
	}
	// Validation rules reference their attribute by qualified name
	// (Module.Entity.Attr); resolve attribute IDs → names within this entity.
	if len(e.ValidationRules) > 0 {
		attrName := make(map[model.ID]string, len(e.Attributes))
		for _, a := range e.Attributes {
			attrName[a.ID] = a.Name
		}
		for _, vr := range e.ValidationRules {
			out.AddValidationRules(validationRuleToGen(vr, moduleName, e.Name, attrName))
		}
	}
	for _, idx := range e.Indexes {
		out.AddIndexes(indexToGen(idx))
	}
	for _, eh := range e.EventHandlers {
		out.AddEventHandlers(eventHandlerToGen(eh))
	}
	if len(e.AccessRules) > 0 {
		// Member accesses are kept in sync with the entity's attributes: every
		// attribute has a MemberAccess in each rule, new ones joining with the
		// rule's default rights. This matches Studio Pro / the legacy writer,
		// which auto-extend access rules when an attribute is added.
		attrQNames := make([]string, 0, len(e.Attributes))
		for _, a := range e.Attributes {
			attrQNames = append(attrQNames, fmt.Sprintf("%s.%s.%s", moduleName, e.Name, a.Name))
		}
		for _, ar := range e.AccessRules {
			gar := accessRuleToGen(ar)
			syncMemberAccesses(gar, attrQNames)
			out.AddAccessRules(gar)
		}
	}
	return out
}

// syncMemberAccesses ensures the access rule has a MemberAccess for every
// attribute qualified name, adding any missing one with the rule's default
// rights (mirrors Studio Pro extending access rules when an attribute is added).
func syncMemberAccesses(gar *genDm.AccessRule, attrQNames []string) {
	have := make(map[string]bool)
	for _, el := range gar.MemberAccessesItems() {
		if ma, ok := el.(*genDm.MemberAccess); ok {
			have[ma.AttributeQualifiedName()] = true
		}
	}
	rights := gar.DefaultMemberAccessRights()
	for _, qn := range attrQNames {
		if !have[qn] {
			ma := genDm.NewMemberAccess()
			ma.SetAccessRights(rights)
			ma.SetAttributeQualifiedName(qn)
			gar.AddMemberAccesses(ma)
		}
	}
}

// eventHandlerToGen converts a domainmodel.EventHandler to a gen EventHandler.
// Mirrors the legacy serializer's defaults (Moment→Before, Event→Commit) and the
// by-name microflow reference; the gen emits the correct storage keys (Type,
// SendInputParameter) after the override.
func eventHandlerToGen(eh *domainmodel.EventHandler) *genDm.EventHandler {
	out := genDm.NewEventHandler()
	moment := string(eh.Moment)
	if moment == "" {
		moment = "Before"
	}
	out.SetMoment(moment)
	event := string(eh.Event)
	if event == "" {
		event = "Commit"
	}
	out.SetEvent(event)
	if eh.MicroflowName != "" {
		out.SetMicroflowQualifiedName(eh.MicroflowName)
	}
	out.SetRaiseErrorOnFalse(eh.RaiseErrorOnFalse)
	out.SetPassEventObject(eh.PassEventObject)
	return out
}

// accessRuleToGen converts a domainmodel.AccessRule to a gen AccessRule. Mirrors
// the legacy serializer: module roles as a by-name list (the codec emits the
// marker-1 ByNameRefList), AllowCreate/AllowDelete (read/write are per-member),
// DefaultMemberAccessRights defaulting to "None", XPath + empty caption/doc, and
// the per-member accesses.
func accessRuleToGen(ar *domainmodel.AccessRule) *genDm.AccessRule {
	out := genDm.NewAccessRule()
	out.SetModuleRolesQualifiedNames(ar.ModuleRoleNames)
	out.SetAllowCreate(ar.AllowCreate)
	out.SetAllowDelete(ar.AllowDelete)
	dmar := string(ar.DefaultMemberAccessRights)
	if dmar == "" {
		dmar = "None"
	}
	out.SetDefaultMemberAccessRights(dmar)
	out.SetXPathConstraint(ar.XPathConstraint)
	out.SetXPathConstraintCaption("")
	out.SetDocumentation("")
	for _, ma := range ar.MemberAccesses {
		out.AddMemberAccesses(memberAccessToGen(ma))
	}
	return out
}

// memberAccessToGen converts a domainmodel.MemberAccess to a gen MemberAccess.
// A member targets either an attribute or an association (by qualified name).
func memberAccessToGen(ma *domainmodel.MemberAccess) *genDm.MemberAccess {
	out := genDm.NewMemberAccess()
	out.SetAccessRights(string(ma.AccessRights))
	if ma.AttributeName != "" {
		out.SetAttributeQualifiedName(ma.AttributeName)
	}
	if ma.AssociationName != "" {
		out.SetAssociationQualifiedName(ma.AssociationName)
	}
	return out
}

// indexToGen converts a domainmodel.Index to a gen EntityIndex. Each segment
// references its attribute by AttributePointer (= the attribute's ID, carried
// onto the gen attribute in attributeToGen); Ascending/Type replace legacy's
// stale SortOrder string, and AssociationPointer (zero GUID) comes from the
// registered ZeroGUIDFields default. Verified against real Studio-Pro 11.x BSON.
func indexToGen(idx *domainmodel.Index) *genDm.Index {
	out := genDm.NewIndex()
	out.SetIncludeInOffline(false)
	segs := idx.Attributes
	for _, ia := range segs {
		seg := genDm.NewIndexedAttribute()
		seg.SetType("Normal")
		seg.SetAscending(ia.Ascending)
		seg.SetAttributeID(element.ID(ia.AttributeID))
		out.AddAttributes(seg)
	}
	// Fall back to AttributeIDs (ascending) when the structured segments are absent.
	if len(segs) == 0 {
		for _, aid := range idx.AttributeIDs {
			seg := genDm.NewIndexedAttribute()
			seg.SetType("Normal")
			seg.SetAscending(true)
			seg.SetAttributeID(element.ID(aid))
			out.AddAttributes(seg)
		}
	}
	return out
}

// validationRuleToGen converts a domainmodel.ValidationRule: the attribute it
// targets (by qualified name), an optional Message text, and the RuleInfo.
func validationRuleToGen(vr *domainmodel.ValidationRule, moduleName, entityName string, attrName map[model.ID]string) *genDm.ValidationRule {
	out := genDm.NewValidationRule()
	qn := string(vr.AttributeID)
	if !strings.Contains(qn, ".") {
		if n, ok := attrName[vr.AttributeID]; ok {
			qn = fmt.Sprintf("%s.%s.%s", moduleName, entityName, n)
		}
	}
	out.SetAttributeQualifiedName(qn)
	if vr.ErrorMessage != nil && len(vr.ErrorMessage.Translations) > 0 {
		out.SetErrorMessage(textToGen(vr.ErrorMessage))
	}
	out.SetRuleInfo(ruleInfoToGen(vr))
	return out
}

// validationRulesAreReproducible reports the first stored validation rule this
// writer cannot serialize, so the caller can refuse the write instead of
// downgrading it. See ruleInfoToGen.
func validationRulesAreReproducible(e *domainmodel.Entity) (string, bool) {
	for _, vr := range e.ValidationRules {
		if !reproducibleRule(vr) {
			return vr.Type, false
		}
	}
	return "", true
}

// ruleInfoToGen builds the RuleInfo element for a validation rule, or nil for
// one this writer cannot reproduce.
//
// A nil return is a REFUSAL, not a default. The previous code fell back to
// RequiredRuleInfo, which turned every entity rewrite into a silent downgrade:
// a RegEx rule came back as Required, the pattern reference gone and the field
// merely mandatory, with mxbuild none the wiser because both are valid rules.
//
// It takes the whole rule rather than its type, because the type alone does not
// determine the document: a RegEx rule is its reference and a Range rule is its
// bounds. Writing the bare RuleInfo for either would produce a rule Mendix
// accepts and that constrains nothing — the same silent downgrade wearing the
// right type name. So a rule whose payload did not survive the read is refused
// as firmly as a type this writer does not know.
func ruleInfoToGen(vr *domainmodel.ValidationRule) element.Element {
	if vr == nil {
		return nil
	}
	switch vr.Type {
	case "Unique":
		return genDm.NewUniqueRuleInfo()
	case "Required", "":
		return genDm.NewRequiredRuleInfo()

	case "RegEx":
		// Writable only since the RegExIdentifier storage-name override in
		// modelsdk/gen — before it, this emitted a key Mendix does not read.
		info, ok := vr.Rule.(*domainmodel.RegexValidationRuleInfo)
		if !ok || info.RegularExpressionQualifiedName == "" {
			return nil
		}
		out := genDm.NewRegExRuleInfo()
		out.SetRegularExpressionQualifiedName(info.RegularExpressionQualifiedName)
		return out

	case "Range":
		info, ok := vr.Rule.(*domainmodel.RangeValidationRuleInfo)
		if !ok {
			return nil
		}
		typeOfRange, ok := rangeKind(info)
		if !ok {
			return nil
		}
		out := genDm.NewRangeRuleInfo()
		out.SetTypeOfRange(typeOfRange)
		out.SetUseMinValue(info.UseMinValue)
		out.SetUseMaxValue(info.UseMaxValue)
		if info.MinValue != nil {
			out.SetMinValue(*info.MinValue)
		}
		if info.MaxValue != nil {
			out.SetMaxValue(*info.MaxValue)
		}
		// A bound may point at another attribute instead of a literal. MDL
		// cannot author that, but a stored rule must survive the rewrite.
		if info.MinAttributeQualifiedName != "" {
			out.SetMinAttributeQualifiedName(info.MinAttributeQualifiedName)
		}
		if info.MaxAttributeQualifiedName != "" {
			out.SetMaxAttributeQualifiedName(info.MaxAttributeQualifiedName)
		}
		return out

	default:
		// MaxLength and EqualsTo. Neither is authorable in MDL, so one only
		// reaches here through a read-modify-write, and neither survives the
		// read — the model carries no payload type for them. (EqualsTo would
		// also need a gen storage-name override: it binds "EqualsToValue"
		// where Studio Pro stores "value".)
		return nil
	}
}

// rangeKind derives the TypeOfRange enum from which bounds are in use. Mendix
// has exactly three values and no strict inequality, so a range using neither
// bound has no representation and is refused.
func rangeKind(info *domainmodel.RangeValidationRuleInfo) (string, bool) {
	switch {
	case info.UseMinValue && info.UseMaxValue:
		return string(metamodel.DomainModelsTypeOfRangeBetween), true
	case info.UseMinValue:
		return string(metamodel.DomainModelsTypeOfRangeGreaterThanOrEqualTo), true
	case info.UseMaxValue:
		return string(metamodel.DomainModelsTypeOfRangeSmallerThanOrEqualTo), true
	default:
		return "", false
	}
}

// reproducibleRule reports whether this writer can serialize a validation rule.
// Everything else must be preserved rather than rewritten.
func reproducibleRule(vr *domainmodel.ValidationRule) bool {
	return ruleInfoToGen(vr) != nil
}

// textToGen converts a model.Text to a gen Texts$Text with sorted translations.
func textToGen(t *model.Text) *genTexts.Text {
	out := genTexts.NewText()
	langs := make([]string, 0, len(t.Translations))
	for lang := range t.Translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		tr := genTexts.NewTranslation()
		tr.SetLanguageCode(lang)
		tr.SetText(t.Translations[lang])
		out.AddTranslations(tr)
	}
	return out
}

// moduleNameFor returns the name of the module that contains the given unit.
func (b *Backend) moduleNameFor(unitID model.ID) string {
	units, err := b.reader.ListUnits()
	if err != nil {
		return ""
	}
	parentOf := make(map[string]string, len(units))
	for _, u := range units {
		parentOf[u.ID] = u.ContainerID
	}

	// A document is not necessarily a direct child of its module — Studio Pro
	// nests documents in folders, and folders in folders, which is the norm in
	// marketplace modules. So walk the containment chain up to the first
	// enclosing module rather than reading only the immediate container.
	// The project root is its own container, so stop on a self-reference as
	// well as on a missing entry.
	seen := make(map[string]bool, 8)
	for id := parentOf[string(unitID)]; id != "" && !seen[id]; id = parentOf[id] {
		seen[id] = true
		if mi, _ := b.reader.GetModule(id); mi != nil {
			return mi.Name
		}
	}
	return ""
}

// attributeToGen converts a domainmodel.Attribute to its gen form: name,
// documentation, ExportLevel, the typed NewType element, and a StoredValue
// holding the default. The attribute's GUID is added by the encoder via the
// registered DomainModels$Attribute defaults.
func attributeToGen(a *domainmodel.Attribute, isExternal bool) *genDm.Attribute {
	out := genDm.NewAttribute()
	out.SetName(a.Name)
	out.SetDocumentation(a.Documentation)
	out.SetExportLevel("Hidden")
	out.SetType(attributeTypeToGen(a.Type))

	def := ""
	if a.Value != nil {
		def = a.Value.DefaultValue
	}
	switch {
	case a.Value != nil && a.Value.ViewReference != "":
		// View-entity attributes carry an OqlViewValue referencing the OQL column
		// (not a StoredValue) — emitting StoredValue triggers CE6770 "View Entity
		// out of sync".
		vv := genDm.NewOqlViewValue()
		vv.SetReference(a.Value.ViewReference)
		out.SetValue(vv)
	case a.Value != nil && a.Value.Type == "CalculatedValue":
		// Calculated attribute: the value is computed by a microflow, not stored.
		// Without this arm the attribute fell through to the default StoredValue
		// below, so `calculated by` parsed, validated and reported success while
		// the binding never reached the model — an attribute that stays empty at
		// runtime, in a project that builds at 0 errors (#917). The legacy
		// serializer had the arm; the default engine did not.
		//
		// Microflow is a ByNameReference (gen binds the "Microflow" key to the
		// qualified name), and PassEntity says whether the microflow takes the
		// owning entity as its parameter.
		cv := genDm.NewCalculatedValue()
		cv.SetMicroflowQualifiedName(a.Value.MicroflowName)
		cv.SetPassEntity(a.Value.PassEntity)
		out.SetValue(cv)
	case isExternal && a.IsPrimitiveCollection:
		// The single attribute of a primitive-collection NPE (e.g. TripTag.Tag) is
		// backed by a Rest$ODataMappedPrimitiveCollectionValue (issue #718).
		mv := genRest.NewODataMappedPrimitiveCollectionValue()
		mv.SetDefaultValueDesignTime(def)
		mv.SetRemoteName(a.RemoteName)
		mv.SetRemoteType(a.RemoteType)
		out.SetValue(mv)
	case isExternal && a.RemoteName != "":
		// External-entity attribute backed by an OData property → Rest$ODataMappedValue
		// instead of DomainModels$StoredValue (issue #718).
		mv := genRest.NewODataMappedValue()
		mv.SetCreatable(a.Creatable)
		mv.SetDefaultValueDesignTime(def)
		mv.SetFilterable(a.Filterable)
		mv.SetRemoteName(a.RemoteName)
		mv.SetRemoteType(a.RemoteType)
		mv.SetRepresentsStream(false)
		mv.SetSortable(a.Sortable)
		mv.SetUpdatable(a.Updatable)
		out.SetValue(mv)
	default:
		// Regular entity attribute — Studio Pro always serializes a StoredValue
		// (empty DefaultValue when none), so set it unconditionally.
		sv := genDm.NewStoredValue()
		sv.SetDefaultValue(def)
		out.SetValue(sv)
	}
	// Carry the domainmodel attribute ID onto the gen element so an index's
	// AttributePointer (which references this same ID) resolves to it. assignID
	// leaves non-empty IDs untouched; the canonical comparison masks IDs anyway.
	if a.ID != "" {
		out.SetID(element.ID(a.ID))
	}
	return out
}

// attributeTypeToGen maps a domainmodel attribute type to the gen NewType element.
func attributeTypeToGen(t domainmodel.AttributeType) element.Element {
	switch at := t.(type) {
	case *domainmodel.StringAttributeType:
		g := genDm.NewStringAttributeType()
		// Always emit Length — including 0 (= unlimited) — to match the legacy
		// serializer. If Length is omitted, Studio Pro falls back to its own UI
		// default of 200, which for external (OData) attributes without a MaxLength
		// contradicts the service's "unlimited" type and raises CE6621.
		g.SetLength(int32(at.Length))
		return g
	case *domainmodel.IntegerAttributeType:
		return genDm.NewIntegerAttributeType()
	case *domainmodel.LongAttributeType:
		return genDm.NewLongAttributeType()
	case *domainmodel.DecimalAttributeType:
		return genDm.NewDecimalAttributeType()
	case *domainmodel.BooleanAttributeType:
		return genDm.NewBooleanAttributeType()
	case *domainmodel.DateTimeAttributeType, *domainmodel.DateAttributeType:
		return genDm.NewDateTimeAttributeType()
	case *domainmodel.AutoNumberAttributeType:
		return genDm.NewAutoNumberAttributeType()
	case *domainmodel.BinaryAttributeType:
		return genDm.NewBinaryAttributeType()
	case *domainmodel.HashedStringAttributeType:
		return genDm.NewHashedStringAttributeType()
	case *domainmodel.EnumerationAttributeType:
		g := genDm.NewEnumerationAttributeType()
		g.SetEnumerationQualifiedName(at.EnumerationRef)
		return g
	default:
		return genDm.NewStringAttributeType()
	}
}

// externalEntitySourceToGen builds the Rest$OData* entity source element for an
// external (OData remote) entity, mirroring the legacy serializer
// (sdk/mpr/writer_domainmodel.go). Nested key elements get fresh IDs here because
// assignEntityIDs only stamps the top-level source object (issue #718).
func externalEntitySourceToGen(e *domainmodel.Entity) element.Element {
	switch e.Source {
	case "Rest$ODataRemoteEntitySource":
		// Top-level external entity (has an entity set): CRUD + paging capabilities.
		src := genRest.NewODataRemoteEntitySource()
		src.SetCountable(e.Countable)
		src.SetCreatable(e.Creatable)
		src.SetCreateChangeLocally(e.CreateChangeLocally)
		src.SetDeletable(e.Deletable)
		src.SetEntitySet(e.RemoteEntitySet)
		if key := odataKeyToGen(e.RemoteKeyParts); key != nil {
			src.SetKey(key)
		}
		src.SetRemoteName(e.RemoteEntityName)
		src.SetSkipSupported(e.SkipSupported)
		src.SetSourceDocumentQualifiedName(e.RemoteServiceName)
		src.SetTopSupported(e.TopSupported)
		return src
	case "Rest$ODataEntityTypeSource":
		// Derived/abstract/contained type (no entity set): type name + key only.
		src := genRest.NewODataEntityTypeSource()
		src.SetEntityTypeName(e.RemoteEntityName)
		src.SetIsOpen(e.IsOpen)
		if key := odataKeyToGen(e.RemoteKeyParts); key != nil {
			src.SetKey(key)
		}
		src.SetSourceDocumentQualifiedName(e.RemoteServiceName)
		return src
	case "Rest$ODataPrimitiveCollectionEntitySource":
		// Primitive-collection NPE (e.g. TripTag for Trip.Tags = Collection(Edm.String)).
		src := genRest.NewODataPrimitiveCollectionEntitySource()
		src.SetSourceDocumentQualifiedName(e.RemoteServiceName)
		return src
	}
	return nil
}

// odataKeyToGen builds a Rest$ODataKey from the entity's remote key parts, or nil
// when there are none.
func odataKeyToGen(parts []*domainmodel.RemoteKeyPart) element.Element {
	if len(parts) == 0 {
		return nil
	}
	key := genRest.NewODataKey()
	for _, kp := range parts {
		key.AddParts(odataKeyPartToGen(kp))
	}
	assignID(key)
	return key
}

func odataKeyPartToGen(kp *domainmodel.RemoteKeyPart) element.Element {
	part := genRest.NewODataKeyPart()
	part.SetEntityKeyPartName(kp.Name)
	part.SetName(kp.RemoteName)
	part.SetFilterable(true)
	part.SetRemoteType(kp.RemoteType)
	t := attributeTypeToGen(kp.Type)
	assignID(t)
	part.SetType(t)
	assignID(part)
	return part
}

// externalAssociationSourceToGen builds the Rest$OData* association source for an
// association between external entities, or nil for a plain association (issue
// #718). The source object gets a fresh ID here (assignAssociationIDs stamps only
// the association and its delete behavior).
func externalAssociationSourceToGen(a *domainmodel.Association) element.Element {
	switch a.Source {
	case "Rest$ODataRemoteAssociationSource":
		nav := a.Navigability2
		if nav == "" {
			nav = "ParentToChild"
		}
		src := genRest.NewODataRemoteAssociationSource()
		src.SetCreatableFromChild(a.CreatableFromChild)
		src.SetCreatableFromParent(a.CreatableFromParent)
		src.SetNavigability2(nav)
		src.SetRemoteChildNavigationProperty(a.RemoteChildNavigationProperty)
		src.SetRemoteParentNavigationProperty(a.RemoteParentNavigationProperty)
		src.SetUpdatableFromChild(a.UpdatableFromChild)
		src.SetUpdatableFromParent(a.UpdatableFromParent)
		assignID(src)
		return src
	case "Rest$ODataPrimitiveCollectionAssociationSource":
		// Marker source pairing with Rest$ODataPrimitiveCollectionEntitySource on
		// the child — no extra fields.
		src := genRest.NewODataPrimitiveCollectionAssociationSource()
		assignID(src)
		return src
	case domainmodel.OqlViewAssociationSource:
		return oqlViewAssociationSourceToGen(a.ViewSourceReference)
	}
	return nil
}

// oqlViewAssociationSourceToGen builds it. `Reference` is the OQL select alias;
// gen binds that exact storage name (checked against initOqlViewAssociationSource
// and against a real document), so no STORAGE-NAME OVERRIDE is needed here.
func oqlViewAssociationSourceToGen(reference string) element.Element {
	src := genDm.NewOqlViewAssociationSource()
	src.SetReference(reference)
	assignID(src)
	return src
}

// assignEntityIDs gives the entity, its generalization, and each attribute
// (plus the attribute's type and stored value) fresh IDs (mirrors engalar's
// assignEntityIDsGen).
func assignEntityIDs(e *genDm.Entity) {
	assignID(e)
	assignID(e.Generalization())
	if src := e.Source(); src != nil { // view/external entity source object (nil for plain entities)
		assignID(src)
	}
	for _, el := range e.AttributesItems() {
		assignID(el)
		if a, ok := el.(*genDm.Attribute); ok {
			assignID(a.Type())
			assignID(a.Value())
		}
	}
	for _, el := range e.IndexesItems() {
		assignID(el)
		if idx, ok := el.(*genDm.Index); ok {
			for _, seg := range idx.AttributesItems() {
				assignID(seg)
			}
		}
	}
	for _, el := range e.AccessRulesItems() {
		assignID(el)
		if ar, ok := el.(*genDm.AccessRule); ok {
			for _, ma := range ar.MemberAccessesItems() {
				assignID(ma)
			}
		}
	}
	for _, el := range e.EventHandlersItems() {
		assignID(el)
	}
	for _, el := range e.ValidationRulesItems() {
		assignID(el)
		if vr, ok := el.(*genDm.ValidationRule); ok {
			assignID(vr.RuleInfo())
			if msg := vr.ErrorMessage(); msg != nil {
				assignID(msg)
				if txt, ok := msg.(*genTexts.Text); ok {
					for _, tr := range txt.TranslationsItems() {
						assignID(tr)
					}
				}
			}
		}
	}
}

func assignID(elem element.Element) {
	ider, ok := elem.(interface {
		ID() element.ID
		SetID(element.ID)
	})
	if !ok || ider == nil || ider.ID() != "" {
		return
	}
	ider.SetID(element.ID(mmpr.GenerateID()))
}

// deleteErrorText is the message element a restrict behaviour must carry.
//
// An omitted message becomes an empty en_US translation rather than a null: null
// is the shape that stops the runtime starting, and Studio Pro's dialog lets the
// box be left empty, so an empty text is the closest thing to "no message" that
// still boots.
//
// NOTE: the empty case is INFERRED, not measured. The reference document
// (ako/TestApp) captures a message that was filled in; nobody has yet saved a
// restrict association with the box cleared, so what Studio Pro writes then is
// unverified. If this ever misbehaves, that is the line to check first.
func deleteErrorText(db *domainmodel.DeleteBehavior) *model.Text {
	msg := ""
	if db != nil {
		msg = db.ErrorMessage
	}
	return &model.Text{Translations: map[string]string{"en_US": msg}}
}

// copyAssignedIDs writes the identities minted on a gen entity back onto the
// semantic entity the caller passed in.
//
// Attributes are matched BY NAME rather than by position: entityToGen may add,
// skip or reorder elements (an audit pseudo-type becomes a System-module
// generalization, not an attribute), so an index-based copy would hand the
// caller another attribute's identity — a worse failure than the empty ID it
// replaces, because it looks like it worked.
func copyAssignedIDs(dst *domainmodel.Entity, src *genDm.Entity) {
	if dst == nil || src == nil {
		return
	}
	if dst.ID == "" {
		dst.ID = model.ID(src.ID())
	}
	byName := map[string]element.ID{}
	for _, el := range src.AttributesItems() {
		a, ok := el.(*genDm.Attribute)
		if !ok {
			continue
		}
		byName[a.Name()] = a.ID()
	}
	for _, attr := range dst.Attributes {
		if attr == nil || attr.ID != "" {
			continue
		}
		if id, ok := byName[attr.Name]; ok {
			attr.ID = model.ID(id)
		}
	}
}
