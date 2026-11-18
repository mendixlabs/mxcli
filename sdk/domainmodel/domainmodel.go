// SPDX-License-Identifier: Apache-2.0

// Package domainmodel provides types for Mendix domain models.
package domainmodel

import (
	"github.com/mendixlabs/mxcli/model"
)

// DomainModel represents a module's domain model containing entities and associations.
type DomainModel struct {
	model.BaseElement
	ContainerID model.ID `json:"containerId"`
	// Documentation is the domain model's own documentation — Studio Pro's
	// "Documentation" pane with nothing selected on the canvas. It belongs to the
	// DomainModels$DomainModel unit, not to the module: a Mendix module has no
	// documentation property of its own (see linter.documentableSources).
	//
	// Read-only here. The write paths load the stored unit and mutate it in place,
	// so the stored value is carried by construction; nothing writes this field
	// back, and a semantic model that leaves it empty does not blank it.
	Documentation     string                    `json:"documentation,omitempty"`
	Entities          []*Entity                 `json:"entities,omitempty"`
	Associations      []*Association            `json:"associations,omitempty"`
	CrossAssociations []*CrossModuleAssociation `json:"crossAssociations,omitempty"`
	Annotations       []*Annotation             `json:"annotations,omitempty"`
}

// GetContainerID returns the ID of the containing module.
func (dm *DomainModel) GetContainerID() model.ID {
	return dm.ContainerID
}

// FindEntityByName finds an entity by name.
func (dm *DomainModel) FindEntityByName(name string) *Entity {
	for _, e := range dm.Entities {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// FindAssociationByName finds an association by name.
func (dm *DomainModel) FindAssociationByName(name string) *Association {
	for _, a := range dm.Associations {
		if a.Name == name {
			return a
		}
	}
	return nil
}

// Entity represents an entity in the domain model.
type Entity struct {
	model.BaseElement
	ContainerID      model.ID       `json:"containerId"`
	Name             string         `json:"name"`
	Documentation    string         `json:"documentation,omitempty"`
	Location         model.Point    `json:"location"`
	Generalization   Generalization `json:"generalization,omitempty"`
	GeneralizationID model.ID       `json:"generalizationId,omitempty"`

	// ExportLevel is "Hidden" or "API" (Studio Pro's "Usable"). It only means
	// something in a Protected (add-on) module, where a Hidden entity may not be
	// referenced from outside it (CE1406) nor exposed by a Usable microflow
	// (CE5400). The writer writes a set level and, for "", keeps the stored one
	// (Hidden on a new entity) — the same holds for Attribute, Association and
	// CrossModuleAssociation (mendixlabs/mxcli#1498).
	ExportLevel string `json:"exportLevel,omitempty"`

	// Persistability
	Persistable bool `json:"persistable"`

	// System attribute flags (stored in NoGeneralization)
	HasOwner       bool `json:"hasOwner,omitempty"`
	HasChangedBy   bool `json:"hasChangedBy,omitempty"`
	HasChangedDate bool `json:"hasChangedDate,omitempty"`
	HasCreatedDate bool `json:"hasCreatedDate,omitempty"`

	// Attributes and other members
	Attributes      []*Attribute      `json:"attributes,omitempty"`
	Indexes         []*Index          `json:"indexes,omitempty"`
	AccessRules     []*AccessRule     `json:"accessRules,omitempty"`
	ValidationRules []*ValidationRule `json:"validationRules,omitempty"`
	EventHandlers   []*EventHandler   `json:"eventHandlers,omitempty"`

	// Remote entity properties (for external entities)
	RemoteSource         string   `json:"remoteSource,omitempty"`
	RemoteSourceDocument model.ID `json:"remoteSourceDocument,omitempty"`

	// Source indicates the entity source type (for view/external entities)
	// Possible values: "", "OqlViewEntitySource", "ODataRemoteEntitySource", etc.
	Source string `json:"source,omitempty"`

	// SourceObjectID is the $ID of the embedded Source object in BSON.
	// Must be preserved across updates to avoid CE-6770 "View Entity is out of sync".
	SourceObjectID model.ID `json:"sourceObjectId,omitempty"`

	// SourceDocumentRef is the qualified name reference to the source document (for view entities)
	// e.g., "MyFirstModule.SubUsersVe"
	SourceDocumentRef string `json:"sourceDocumentRef,omitempty"`

	// OqlQuery contains the OQL query for view entities (loaded from ViewEntitySourceDocument)
	OqlQuery string `json:"oqlQuery,omitempty"`

	// GeneralizationRef is the qualified name of the parent entity (e.g., "System.User")
	GeneralizationRef string `json:"generalizationRef,omitempty"`

	// OData remote entity source fields (for external entities)
	RemoteServiceName   string           `json:"remoteServiceName,omitempty"` // Qualified name of consumed OData service
	RemoteEntitySet     string           `json:"remoteEntitySet,omitempty"`   // Entity set name (Rest$ODataRemoteEntitySource only)
	RemoteEntityName    string           `json:"remoteEntityName,omitempty"`  // Remote type name
	Countable           bool             `json:"countable,omitempty"`
	Creatable           bool             `json:"creatable,omitempty"`
	Deletable           bool             `json:"deletable,omitempty"`
	Updatable           bool             `json:"updatable,omitempty"`
	SkipSupported       bool             `json:"skipSupported,omitempty"`
	TopSupported        bool             `json:"topSupported,omitempty"`
	CreateChangeLocally bool             `json:"createChangeLocally,omitempty"`
	IsOpen              bool             `json:"isOpen,omitempty"`         // Rest$ODataEntityTypeSource: <EntityType OpenType="true">
	RemoteKeyParts      []*RemoteKeyPart `json:"remoteKeyParts,omitempty"` // OData key properties
}

// RemoteKeyPart describes one key property of an external entity, used to
// generate the Rest$ODataKey block in BSON output.
type RemoteKeyPart struct {
	Name       string // Mendix attribute name
	RemoteName string // OData property name (often same as Name)
	RemoteType string // OData EDM type, e.g. "Edm.String"
	Type       AttributeType
	// NotFilterable is true when the service says the key cannot be filtered on.
	// A key part carries its own Filterable, which mxbuild compares with the
	// service's (CE6630), so it is a model fact like the attribute's. The zero
	// value keeps the historic filterable key, so code that builds a key part
	// without a contract in hand is unchanged.
	NotFilterable bool
}

// GetName returns the entity's name.
func (e *Entity) GetName() string {
	return e.Name
}

// GetContainerID returns the ID of the containing domain model.
func (e *Entity) GetContainerID() model.ID {
	return e.ContainerID
}

// FindAttributeByName finds an attribute by name.
func (e *Entity) FindAttributeByName(name string) *Attribute {
	for _, a := range e.Attributes {
		if a.Name == name {
			return a
		}
	}
	return nil
}

// IsPersistable returns whether the entity is persistable.
func (e *Entity) IsPersistable() bool {
	return e.Persistable
}

// Generalization represents the generalization (inheritance) of an entity.
type Generalization interface {
	isGeneralization()
}

// NoGeneralization indicates the entity has no parent.
type NoGeneralization struct {
	model.BaseElement
	Persistable bool `json:"persistable"`
}

func (NoGeneralization) isGeneralization() {}

// GeneralizationBase indicates the entity inherits from another entity.
type GeneralizationBase struct {
	model.BaseElement
	GeneralizationID model.ID `json:"generalizationId"`
}

func (GeneralizationBase) isGeneralization() {}

// Attribute represents an attribute of an entity.
type Attribute struct {
	model.BaseElement
	ContainerID   model.ID        `json:"containerId"`
	Name          string          `json:"name"`
	Documentation string          `json:"documentation,omitempty"`
	Type          AttributeType   `json:"type"`
	Value         *AttributeValue `json:"value,omitempty"`
	// ExportLevel: see Entity.ExportLevel.
	ExportLevel string `json:"exportLevel,omitempty"`

	// External entity attribute fields (for OData remote entities).
	// When RemoteName is set on an attribute of an external entity, the writer
	// emits a Rest$ODataMappedValue instead of DomainModels$StoredValue.
	RemoteName string `json:"remoteName,omitempty"` // OData property name
	RemoteType string `json:"remoteType,omitempty"` // OData EDM type, e.g. "Edm.String"
	Filterable bool   `json:"filterable,omitempty"`
	Sortable   bool   `json:"sortable,omitempty"`
	Creatable  bool   `json:"creatable,omitempty"`
	Updatable  bool   `json:"updatable,omitempty"`

	// IsPrimitiveCollection marks the single attribute of a primitive
	// collection NPE (e.g. TripTag.Tag). When set, the writer emits
	// Rest$ODataMappedPrimitiveCollectionValue instead of Rest$ODataMappedValue.
	IsPrimitiveCollection bool `json:"isPrimitiveCollection,omitempty"`
}

// GetName returns the attribute's name.
func (a *Attribute) GetName() string {
	return a.Name
}

// GetContainerID returns the ID of the containing entity.
func (a *Attribute) GetContainerID() model.ID {
	return a.ContainerID
}

// AttributeType represents the type of an attribute.
type AttributeType interface {
	GetTypeName() string
}

// StringAttributeType represents a string attribute type.
type StringAttributeType struct {
	model.BaseElement
	Length int `json:"length,omitempty"`
}

// GetTypeName returns the type name.
func (t *StringAttributeType) GetTypeName() string {
	return "String"
}

// IntegerAttributeType represents an integer attribute type.
type IntegerAttributeType struct {
	model.BaseElement
}

// GetTypeName returns the type name.
func (t *IntegerAttributeType) GetTypeName() string {
	return "Integer"
}

// LongAttributeType represents a long integer attribute type.
type LongAttributeType struct {
	model.BaseElement
}

// GetTypeName returns the type name.
func (t *LongAttributeType) GetTypeName() string {
	return "Long"
}

// DecimalAttributeType represents a decimal attribute type.
type DecimalAttributeType struct {
	model.BaseElement
}

// GetTypeName returns the type name.
func (t *DecimalAttributeType) GetTypeName() string {
	return "Decimal"
}

// BooleanAttributeType represents a boolean attribute type.
type BooleanAttributeType struct {
	model.BaseElement
}

// GetTypeName returns the type name.
func (t *BooleanAttributeType) GetTypeName() string {
	return "Boolean"
}

// DateTimeAttributeType represents a date/time attribute type.
type DateTimeAttributeType struct {
	model.BaseElement
	LocalizeDate bool `json:"localizeDate"`
}

// GetTypeName returns the type name.
func (t *DateTimeAttributeType) GetTypeName() string {
	return "DateTime"
}

// DateAttributeType represents a date-only attribute type (no time component).
// Stored as DomainModels$DateTimeAttributeType with LocalizeDate=false in BSON.
type DateAttributeType struct {
	model.BaseElement
}

// GetTypeName returns the type name.
func (t *DateAttributeType) GetTypeName() string {
	return "Date"
}

// EnumerationAttributeType represents an enumeration attribute type.
type EnumerationAttributeType struct {
	model.BaseElement
	EnumerationID  model.ID `json:"enumerationId"`
	EnumerationRef string   `json:"enumerationRef"` // Qualified name like "Module.EnumName"
}

// GetTypeName returns the type name.
func (t *EnumerationAttributeType) GetTypeName() string {
	return "Enumeration"
}

// AutoNumberAttributeType represents an auto-number attribute type.
type AutoNumberAttributeType struct {
	model.BaseElement
}

// GetTypeName returns the type name.
func (t *AutoNumberAttributeType) GetTypeName() string {
	return "AutoNumber"
}

// BinaryAttributeType represents a binary attribute type.
type BinaryAttributeType struct {
	model.BaseElement
}

// GetTypeName returns the type name.
func (t *BinaryAttributeType) GetTypeName() string {
	return "Binary"
}

// HashedStringAttributeType represents a hashed string attribute type.
type HashedStringAttributeType struct {
	model.BaseElement
}

// GetTypeName returns the type name.
func (t *HashedStringAttributeType) GetTypeName() string {
	return "HashedString"
}

// AttributeValue represents a default value for an attribute.
type AttributeValue struct {
	model.BaseElement
	Type          string   `json:"type,omitempty"`
	DefaultValue  string   `json:"defaultValue,omitempty"`
	MicroflowID   model.ID `json:"microflowId,omitempty"`
	MicroflowName string   `json:"microflowName,omitempty"` // Qualified name (e.g. "Module.Microflow") — BSON stores ByNameReference as string
	// PassEntity is DomainModels$CalculatedValue.PassEntity: whether the
	// calculation microflow receives the owning entity as its parameter (#917).
	PassEntity    bool   `json:"passEntity,omitempty"`
	ViewReference string `json:"viewReference,omitempty"` // OQL column reference for view entity attributes
}

// Association represents an association between entities.
type Association struct {
	model.BaseElement
	ContainerID   model.ID                 `json:"containerId"`
	Name          string                   `json:"name"`
	Documentation string                   `json:"documentation,omitempty"`
	ParentID      model.ID                 `json:"parentId"`
	ChildID       model.ID                 `json:"childId"`
	Type          AssociationType          `json:"type"`
	Owner         AssociationOwner         `json:"owner"`
	StorageFormat AssociationStorageFormat `json:"storageFormat,omitempty"`
	// ExportLevel: see Entity.ExportLevel.
	ExportLevel string `json:"exportLevel,omitempty"`
	// Line anchors in the domain model editor. Pointers because the zero Point
	// is a legitimate anchor (top-left corner), so it cannot double as "unset";
	// nil means nothing was stored and the writer uses DefaultParentConnection /
	// DefaultChildConnection. See connection.go. (issue #872)
	ParentConnection *model.Point `json:"parentConnection,omitempty"`
	ChildConnection  *model.Point `json:"childConnection,omitempty"`

	// Delete behavior
	ParentDeleteBehavior *DeleteBehavior `json:"parentDeleteBehavior,omitempty"`
	ChildDeleteBehavior  *DeleteBehavior `json:"childDeleteBehavior,omitempty"`

	// External association source (for OData remote associations between
	// external entities). When Source = "Rest$ODataRemoteAssociationSource",
	// the writer emits a Source block carrying the OData navigation property
	// names instead of leaving the association as a plain persistent one.
	//
	// A VIEW ENTITY's association to a persistent entity uses the same slot,
	// with Source = "DomainModels$OqlViewAssociationSource" and
	// ViewSourceReference naming the OQL select alias it comes from. Measured on
	// 11.13.0: that one subdocument is the whole difference between a model
	// mxbuild accepts and one it refuses with CE6771 "It is not possible to
	// create associations to/from View Entities" — and it also clears the
	// CE6770 the id column otherwise causes on the view entity itself.
	Source string `json:"source,omitempty"`

	// ViewSourceReference is the OQL select alias an OqlViewAssociationSource
	// points at. It equals the association's own Name in everything Studio Pro
	// writes, but it is stored separately and read back separately: they are two
	// different things (a name and a column reference), and treating them as one
	// would silently rename the reference whenever the association is renamed.
	ViewSourceReference string `json:"viewSourceReference,omitempty"`

	// (see OqlViewAssociationSource for the $Type this Source takes)

	RemoteParentNavigationProperty string `json:"remoteParentNavigationProperty,omitempty"`
	RemoteChildNavigationProperty  string `json:"remoteChildNavigationProperty,omitempty"`
	CreatableFromParent            bool   `json:"creatableFromParent,omitempty"`
	CreatableFromChild             bool   `json:"creatableFromChild,omitempty"`
	UpdatableFromParent            bool   `json:"updatableFromParent,omitempty"`
	UpdatableFromChild             bool   `json:"updatableFromChild,omitempty"`
	Navigability2                  string `json:"navigability2,omitempty"` // "ParentToChild" or "BothDirections"
}

// OqlViewAssociationSource is the Source $Type that makes an association to a
// VIEW ENTITY legal, and the one thing that distinguishes it from an ordinary
// persistent association.
//
// Measured on Mendix 11.13.0: adding exactly this subdocument — three keys,
// $ID / $Type / Reference — takes a project from
//
//	[CE6771] "It is not possible to create associations to/from View Entities."
//	[CE6770] "View Entity is out of sync with the OQL Query."
//
// to 0 errors. One field clears both, because the association is also what makes
// the OQL's `<alias>.ID` select column legal on the view entity.
//
// It lives here, in the semantic model, rather than in either backend: both
// engines write it and both read it, and a second copy of the string is how the
// two would come to disagree.
const OqlViewAssociationSource = "DomainModels$OqlViewAssociationSource"

// GetName returns the association's name.
func (a *Association) GetName() string {
	return a.Name
}

// GetContainerID returns the ID of the containing domain model.
func (a *Association) GetContainerID() model.ID {
	return a.ContainerID
}

// AssociationType represents the type of association.
type AssociationType string

const (
	AssociationTypeReference    AssociationType = "Reference"
	AssociationTypeReferenceSet AssociationType = "ReferenceSet"
)

// AssociationOwner represents the owner of an association.
type AssociationOwner string

const (
	AssociationOwnerDefault AssociationOwner = "Default"
	AssociationOwnerBoth    AssociationOwner = "Both"
)

// AssociationStorageFormat represents how an association is stored in the database.
type AssociationStorageFormat string

const (
	StorageFormatTable  AssociationStorageFormat = "Table"
	StorageFormatColumn AssociationStorageFormat = "Column"
)

// DeleteBehavior represents delete behavior for an association.
type DeleteBehavior struct {
	model.BaseElement
	Type DeleteBehaviorType `json:"type"`
	// ErrorMessage is the text the user sees when a DeleteMeIfNoReferences
	// delete is refused — Studio Pro's "Error message if 'X' object cannot be
	// deleted", which the dialog only shows once that behaviour is selected.
	//
	// It is stored as a Texts$Text, so it is translatable like any caption. Its
	// absence is not cosmetic: an association with DeleteMeIfNoReferences and a
	// null message stops the RUNTIME STARTING, which is a worse failure than a
	// build error and names nothing about the model (CapTrackV2 §1).
	ErrorMessage string `json:"errorMessage,omitempty"`
	// ErrorMessageLanguage is the LanguageCode a write stores ErrorMessage
	// under. The executor sets the project's default language, as for every
	// other text it authors (mendixlabs/mxcli#970, #1344); empty means
	// model.AuthoringLanguage, which is en_US until a project's settings are read.
	ErrorMessageLanguage string `json:"errorMessageLanguage,omitempty"`
	// ErrorMessageTranslations holds every translation the stored message
	// carries, keyed by LanguageCode. Read side only: ErrorMessage alone cannot
	// say which language it came from, so DESCRIBE could not pick the project's
	// default language out of a text that has several.
	ErrorMessageTranslations map[string]string `json:"-"`
}

// DeleteBehaviorType represents the type of delete behavior.
type DeleteBehaviorType string

const (
	DeleteBehaviorTypeDeleteMeAndReferences     DeleteBehaviorType = "DeleteMeAndReferences"
	DeleteBehaviorTypeDeleteMeIfNoReferences    DeleteBehaviorType = "DeleteMeIfNoReferences"
	DeleteBehaviorTypeDeleteMeButKeepReferences DeleteBehaviorType = "DeleteMeButKeepReferences"
)

// Index represents an index on an entity.
type Index struct {
	model.BaseElement
	ContainerID  model.ID          `json:"containerId"`
	Name         string            `json:"name,omitempty"`
	Attributes   []*IndexAttribute `json:"attributes,omitempty"`
	AttributeIDs []model.ID        `json:"attributeIds,omitempty"`
}

// GetName returns the index's name.
func (i *Index) GetName() string {
	return i.Name
}

// GetContainerID returns the ID of the containing entity.
func (i *Index) GetContainerID() model.ID {
	return i.ContainerID
}

// IndexAttribute represents an attribute in an index.
type IndexAttribute struct {
	model.BaseElement
	AttributeID model.ID `json:"attributeId"`
	Ascending   bool     `json:"ascending"`
	// Type is the segment kind Studio Pro stores (DomainModels$IndexedAttribute
	// Type): "" or "Normal" for an attribute, "CreatedDate" / "ChangedDate" for
	// the system member of that name, which is not an attribute of the entity
	// and so has nothing behind AttributeID (mendixlabs/mxcli#1401).
	Type string `json:"type,omitempty"`
}

// IsSystemMember reports whether the segment indexes a system date member
// (createdDate / changedDate) rather than an attribute.
func (ia *IndexAttribute) IsSystemMember() bool {
	return ia.Type == "CreatedDate" || ia.Type == "ChangedDate"
}

// AccessRule represents an access rule for an entity.
type AccessRule struct {
	model.BaseElement
	ContainerID               model.ID           `json:"containerId"`
	ModuleRoles               []model.ID         `json:"moduleRoles,omitempty"`
	ModuleRoleNames           []string           `json:"moduleRoleNames,omitempty"`
	AllowCreate               bool               `json:"allowCreate"`
	AllowRead                 bool               `json:"allowRead"`
	AllowWrite                bool               `json:"allowWrite"`
	AllowDelete               bool               `json:"allowDelete"`
	DefaultMemberAccessRights MemberAccessRights `json:"defaultMemberAccessRights,omitempty"`
	XPathConstraint           string             `json:"xPathConstraint,omitempty"`
	MemberAccesses            []*MemberAccess    `json:"memberAccesses,omitempty"`
}

// GetContainerID returns the ID of the containing entity.
func (ar *AccessRule) GetContainerID() model.ID {
	return ar.ContainerID
}

// MemberAccess represents access rights to a specific member.
type MemberAccess struct {
	model.BaseElement
	AttributeID     model.ID           `json:"attributeId,omitempty"`
	AttributeName   string             `json:"attributeName,omitempty"`
	AssociationID   model.ID           `json:"associationId,omitempty"`
	AssociationName string             `json:"associationName,omitempty"`
	AccessRights    MemberAccessRights `json:"accessRights"`
}

// MemberAccessRights represents the access rights for a member.
type MemberAccessRights string

const (
	MemberAccessRightsNone      MemberAccessRights = "None"
	MemberAccessRightsReadOnly  MemberAccessRights = "ReadOnly"
	MemberAccessRightsReadWrite MemberAccessRights = "ReadWrite"
)

// ValidationRule represents a validation rule for an entity.
type ValidationRule struct {
	model.BaseElement
	ContainerID  model.ID           `json:"containerId"`
	AttributeID  model.ID           `json:"attributeId"`
	Type         string             `json:"type"`
	ErrorMessage *model.Text        `json:"errorMessage,omitempty"`
	Rule         ValidationRuleInfo `json:"rule,omitempty"`
}

// GetContainerID returns the ID of the containing entity.
func (vr *ValidationRule) GetContainerID() model.ID {
	return vr.ContainerID
}

// ValidationRuleInfo contains the validation rule details.
type ValidationRuleInfo interface {
	isValidationRuleInfo()
}

// RangeValidationRuleInfo represents a range validation.
//
// A bound is either a literal (MinValue/MaxValue) or another attribute of the
// same entity (MinAttributeQualifiedName/MaxAttributeQualifiedName) — Mendix
// stores both, and a reader that carried only the literals would silently drop
// an attribute-bounded rule on the next rewrite. MDL can only author literal
// bounds; the attribute fields exist so a stored rule survives a round trip.
type RangeValidationRuleInfo struct {
	model.BaseElement
	MinValue                  *string `json:"minValue,omitempty"`
	MaxValue                  *string `json:"maxValue,omitempty"`
	UseMinValue               bool    `json:"useMinValue"`
	UseMaxValue               bool    `json:"useMaxValue"`
	MinAttributeQualifiedName string  `json:"minAttribute,omitempty"`
	MaxAttributeQualifiedName string  `json:"maxAttribute,omitempty"`
}

func (RangeValidationRuleInfo) isValidationRuleInfo() {}

// RegexValidationRuleInfo represents a regex validation.
//
// The rule points at a RegularExpressions$RegularExpression document by
// QUALIFIED NAME, not by ID — Mendix stores it as a by-name reference under the
// key "RegExIdentifier". (This field used to be a model.ID, which no reader
// ever populated.)
type RegexValidationRuleInfo struct {
	model.BaseElement
	RegularExpressionQualifiedName string `json:"regularExpression"`
}

func (RegexValidationRuleInfo) isValidationRuleInfo() {}

// RequiredValidationRuleInfo represents a required validation.
type RequiredValidationRuleInfo struct {
	model.BaseElement
}

func (RequiredValidationRuleInfo) isValidationRuleInfo() {}

// UniqueValidationRuleInfo represents a unique validation.
type UniqueValidationRuleInfo struct {
	model.BaseElement
}

func (UniqueValidationRuleInfo) isValidationRuleInfo() {}

// EventHandler represents an event handler for an entity.
type EventHandler struct {
	model.BaseElement
	ContainerID       model.ID    `json:"containerId"`
	Moment            EventMoment `json:"moment"`
	Event             EventType   `json:"event"`
	MicroflowID       model.ID    `json:"microflowId"`
	MicroflowName     string      `json:"microflowName,omitempty"` // Qualified name for BY_NAME serialization
	RaiseErrorOnFalse bool        `json:"raiseErrorOnFalse"`
	PassEventObject   bool        `json:"passEventObject"`
}

// GetContainerID returns the ID of the containing entity.
func (eh *EventHandler) GetContainerID() model.ID {
	return eh.ContainerID
}

// EventMoment represents when an event handler runs (Before or After the event).
type EventMoment string

const (
	EventMomentBefore EventMoment = "Before"
	EventMomentAfter  EventMoment = "After"
)

// EventType represents the type of entity event.
// Note: "RollBack" matches the Mendix metamodel spelling.
type EventType string

const (
	EventTypeCreate   EventType = "Create"
	EventTypeCommit   EventType = "Commit"
	EventTypeDelete   EventType = "Delete"
	EventTypeRollback EventType = "RollBack"
)

// Annotation represents an annotation in the domain model.
type Annotation struct {
	model.BaseElement
	ContainerID model.ID    `json:"containerId"`
	Caption     string      `json:"caption"`
	Location    model.Point `json:"location"`
	Width       int         `json:"width,omitempty"`
}

// GetContainerID returns the ID of the containing domain model.
func (a *Annotation) GetContainerID() model.ID {
	return a.ContainerID
}

// CrossModuleAssociation represents an association that crosses module boundaries.
// In BSON, this is stored as "DomainModels$CrossAssociation" with:
//   - ParentPointer: BY_ID reference to the local entity (in this module)
//   - Child: BY_NAME reference to the remote entity (qualified name in another module)
type CrossModuleAssociation struct {
	model.BaseElement
	ContainerID model.ID `json:"containerId"`
	Name        string   `json:"name"`

	// ParentID is the BY_ID reference to the local entity in this domain model.
	ParentID model.ID `json:"parentId"`
	// ChildRef is the BY_NAME qualified name of the remote entity (e.g., "OtherModule.Entity").
	ChildRef string `json:"childRef"`

	Documentation        string                   `json:"documentation,omitempty"`
	Type                 AssociationType          `json:"type"`
	Owner                AssociationOwner         `json:"owner"`
	StorageFormat        AssociationStorageFormat `json:"storageFormat,omitempty"`
	ParentDeleteBehavior *DeleteBehavior          `json:"parentDeleteBehavior,omitempty"`
	ChildDeleteBehavior  *DeleteBehavior          `json:"childDeleteBehavior,omitempty"`
	// ExportLevel: see Entity.ExportLevel.
	ExportLevel string `json:"exportLevel,omitempty"`

	// Source / ViewSourceReference carry a view entity's OqlViewAssociationSource,
	// exactly as on Association. A view entity's association is cross-module
	// whenever the entity it points at lives elsewhere, which is the shape
	// ako/view-entity-examples reported it in — so the two association types have
	// to grow the field together or the fix works for one project layout and not
	// the other.
	Source              string `json:"source,omitempty"`
	ViewSourceReference string `json:"viewSourceReference,omitempty"`
}

// GetName returns the cross-module association's name.
func (cma *CrossModuleAssociation) GetName() string {
	return cma.Name
}

// GetContainerID returns the ID of the containing domain model.
func (cma *CrossModuleAssociation) GetContainerID() model.ID {
	return cma.ContainerID
}
