// SPDX-License-Identifier: Apache-2.0

// Package domainmodel provides types for Mendix domain models.
package domainmodel

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// DomainModel represents a module's domain model containing entities and associations.
type DomainModel struct {
	model.BaseElement
	ContainerID       model.ID                  `json:"containerId"`
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
	ViewReference string   `json:"viewReference,omitempty"` // OQL column reference for view entity attributes
}

// Association represents an association between entities.
type Association struct {
	model.BaseElement
	ContainerID      model.ID                 `json:"containerId"`
	Name             string                   `json:"name"`
	Documentation    string                   `json:"documentation,omitempty"`
	ParentID         model.ID                 `json:"parentId"`
	ChildID          model.ID                 `json:"childId"`
	Type             AssociationType          `json:"type"`
	Owner            AssociationOwner         `json:"owner"`
	StorageFormat    AssociationStorageFormat `json:"storageFormat,omitempty"`
	ParentConnection model.Point              `json:"parentConnection,omitempty"`
	ChildConnection  model.Point              `json:"childConnection,omitempty"`

	// Delete behavior
	ParentDeleteBehavior *DeleteBehavior `json:"parentDeleteBehavior,omitempty"`
	ChildDeleteBehavior  *DeleteBehavior `json:"childDeleteBehavior,omitempty"`

	// External association source (for OData remote associations between
	// external entities). When Source = "Rest$ODataRemoteAssociationSource",
	// the writer emits a Source block carrying the OData navigation property
	// names instead of leaving the association as a plain persistent one.
	Source                         string `json:"source,omitempty"`
	RemoteParentNavigationProperty string `json:"remoteParentNavigationProperty,omitempty"`
	RemoteChildNavigationProperty  string `json:"remoteChildNavigationProperty,omitempty"`
	CreatableFromParent            bool   `json:"creatableFromParent,omitempty"`
	CreatableFromChild             bool   `json:"creatableFromChild,omitempty"`
	UpdatableFromParent            bool   `json:"updatableFromParent,omitempty"`
	UpdatableFromChild             bool   `json:"updatableFromChild,omitempty"`
	Navigability2                  string `json:"navigability2,omitempty"` // "ParentToChild" or "BothDirections"
}

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
type RangeValidationRuleInfo struct {
	model.BaseElement
	MinValue    *string `json:"minValue,omitempty"`
	MaxValue    *string `json:"maxValue,omitempty"`
	UseMinValue bool    `json:"useMinValue"`
	UseMaxValue bool    `json:"useMaxValue"`
}

func (RangeValidationRuleInfo) isValidationRuleInfo() {}

// RegexValidationRuleInfo represents a regex validation.
type RegexValidationRuleInfo struct {
	model.BaseElement
	RegularExpressionID model.ID `json:"regularExpressionId"`
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
}

// GetName returns the cross-module association's name.
func (cma *CrossModuleAssociation) GetName() string {
	return cma.Name
}

// GetContainerID returns the ID of the containing domain model.
func (cma *CrossModuleAssociation) GetContainerID() model.ID {
	return cma.ContainerID
}
