// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// DomainModelsAPI provides methods for working with domain models.
type DomainModelsAPI struct {
	api *ModelAPI
}

// CreateEntity starts building a new entity.
func (dm *DomainModelsAPI) CreateEntity(name string) *EntityBuilder {
	return &EntityBuilder{
		api:  dm,
		name: name,
		entity: &domainmodel.Entity{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$Entity",
			},
			Name: name,
		},
		persistent: true,
		attributes: make([]*domainmodel.Attribute, 0),
	}
}

// CreateAssociation starts building a new association.
func (dm *DomainModelsAPI) CreateAssociation(name string) *AssociationBuilder {
	return &AssociationBuilder{
		api:  dm,
		name: name,
		association: &domainmodel.Association{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$Association",
			},
			Name: name,
		},
	}
}

// GetEntity retrieves an entity by qualified name (Module.Entity).
func (dm *DomainModelsAPI) GetEntity(qualifiedName string) (*domainmodel.Entity, error) {
	qn := ParseQualifiedName(qualifiedName)

	// Get the module
	module, err := dm.api.reader.GetModuleByName(qn.ModuleName)
	if err != nil {
		return nil, fmt.Errorf("module not found: %s", qn.ModuleName)
	}

	// Get the domain model
	domainModel, err := dm.api.reader.GetDomainModel(module.ID)
	if err != nil {
		return nil, fmt.Errorf("domain model not found for module: %s", qn.ModuleName)
	}

	// Find the entity
	for _, entity := range domainModel.Entities {
		if entity.Name == qn.ElementName {
			return entity, nil
		}
	}

	return nil, fmt.Errorf("entity not found: %s", qualifiedName)
}

// GetAssociation retrieves an association by qualified name.
func (dm *DomainModelsAPI) GetAssociation(qualifiedName string) (*domainmodel.Association, error) {
	qn := ParseQualifiedName(qualifiedName)

	// Get the module
	module, err := dm.api.reader.GetModuleByName(qn.ModuleName)
	if err != nil {
		return nil, fmt.Errorf("module not found: %s", qn.ModuleName)
	}

	// Get the domain model
	domainModel, err := dm.api.reader.GetDomainModel(module.ID)
	if err != nil {
		return nil, fmt.Errorf("domain model not found for module: %s", qn.ModuleName)
	}

	// Find the association
	for _, assoc := range domainModel.Associations {
		if assoc.Name == qn.ElementName {
			return assoc, nil
		}
	}

	return nil, fmt.Errorf("association not found: %s", qualifiedName)
}

// AddAttribute starts building a new attribute to add to an existing entity.
func (dm *DomainModelsAPI) AddAttribute(entity *domainmodel.Entity) *AttributeBuilder {
	return &AttributeBuilder{
		api:    dm,
		entity: entity,
		attr: &domainmodel.Attribute{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$Attribute",
			},
		},
	}
}

// ModifyAttribute starts modifying an existing attribute.
func (dm *DomainModelsAPI) ModifyAttribute(attr *domainmodel.Attribute) *AttributeModifier {
	return &AttributeModifier{
		api:  dm,
		attr: attr,
	}
}

// RemoveAttribute removes an attribute from an entity.
func (dm *DomainModelsAPI) RemoveAttribute(entity *domainmodel.Entity, attrName string) error {
	// Find the module containing this entity
	modules, err := dm.api.reader.ListModules()
	if err != nil {
		return err
	}

	for _, module := range modules {
		domainModel, err := dm.api.reader.GetDomainModel(module.ID)
		if err != nil {
			continue
		}

		for _, e := range domainModel.Entities {
			if e.ID == entity.ID {
				// Find and remove the attribute
				for i, attr := range e.Attributes {
					if attr.Name == attrName {
						return dm.api.writer.DeleteAttribute(domainModel.ID, entity.ID, attr.ID)
					}
					_ = i // unused but needed for iteration
				}
				return fmt.Errorf("attribute not found: %s", attrName)
			}
		}
	}

	return fmt.Errorf("entity not found in any module")
}

// BatchModify starts a batch modification on an entity.
func (dm *DomainModelsAPI) BatchModify(entity *domainmodel.Entity) *EntityBatchModifier {
	return &EntityBatchModifier{
		api:    dm,
		entity: entity,
		ops:    make([]batchOp, 0),
	}
}

// EntityBuilder builds a new entity with fluent API.
type EntityBuilder struct {
	api                *DomainModelsAPI
	name               string
	entity             *domainmodel.Entity
	module             *model.Module
	persistent         bool
	generalizationName string
	attributes         []*domainmodel.Attribute
	err                error
}

// InModule sets the module for this entity.
func (b *EntityBuilder) InModule(module *model.Module) *EntityBuilder {
	b.module = module
	return b
}

// Persistent marks the entity as persistent (database-backed).
func (b *EntityBuilder) Persistent() *EntityBuilder {
	b.persistent = true
	return b
}

// NonPersistent marks the entity as non-persistent (memory only).
func (b *EntityBuilder) NonPersistent() *EntityBuilder {
	b.persistent = false
	return b
}

// WithGeneralization sets the parent entity for generalization.
func (b *EntityBuilder) WithGeneralization(entityName string) *EntityBuilder {
	b.generalizationName = entityName
	return b
}

// WithStringAttribute adds a string attribute.
func (b *EntityBuilder) WithStringAttribute(name string, length int) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.StringAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$StringAttributeType",
			},
			Length: length,
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithIntegerAttribute adds an integer attribute.
func (b *EntityBuilder) WithIntegerAttribute(name string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.IntegerAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$IntegerAttributeType",
			},
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithLongAttribute adds a long attribute.
func (b *EntityBuilder) WithLongAttribute(name string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.LongAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$LongAttributeType",
			},
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithDecimalAttribute adds a decimal attribute.
func (b *EntityBuilder) WithDecimalAttribute(name string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.DecimalAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$DecimalAttributeType",
			},
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithBooleanAttribute adds a boolean attribute.
func (b *EntityBuilder) WithBooleanAttribute(name string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.BooleanAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$BooleanAttributeType",
			},
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithDateTimeAttribute adds a datetime attribute.
func (b *EntityBuilder) WithDateTimeAttribute(name string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.DateTimeAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$DateTimeAttributeType",
			},
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithEnumerationAttribute adds an enumeration attribute.
func (b *EntityBuilder) WithEnumerationAttribute(name string, enumName string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.EnumerationAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$EnumerationAttributeType",
			},
			EnumerationRef: enumName,
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithAutoNumberAttribute adds an auto-number attribute.
func (b *EntityBuilder) WithAutoNumberAttribute(name string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.AutoNumberAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$AutoNumberAttributeType",
			},
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithHashedStringAttribute adds a hashed string attribute.
func (b *EntityBuilder) WithHashedStringAttribute(name string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.HashedStringAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$HashedStringAttributeType",
			},
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// WithBinaryAttribute adds a binary attribute.
func (b *EntityBuilder) WithBinaryAttribute(name string) *EntityBuilder {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.BinaryAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$BinaryAttributeType",
			},
		},
	}
	b.attributes = append(b.attributes, attr)
	return b
}

// Build creates the entity and saves it to the project.
func (b *EntityBuilder) Build() (*domainmodel.Entity, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Determine module
	module := b.module
	if module == nil {
		module = b.api.api.currentModule
	}
	if module == nil {
		return nil, fmt.Errorf("no module specified; use InModule() or api.SetModule()")
	}

	// Get domain model
	domainModel, err := b.api.api.reader.GetDomainModel(module.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain model: %w", err)
	}

	// Set entity properties
	b.entity.ContainerID = domainModel.ID

	// Set persistence
	b.entity.Persistable = b.persistent

	// Set generalization
	if b.generalizationName != "" {
		// Would need to resolve entity ID - for now just set the base
		b.entity.Generalization = &domainmodel.GeneralizationBase{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$Generalization",
			},
		}
	} else {
		b.entity.Generalization = &domainmodel.NoGeneralization{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$NoGeneralization",
			},
			Persistable: b.persistent,
		}
	}

	// Add attributes
	b.entity.Attributes = b.attributes

	// Create the entity
	err = b.api.api.writer.CreateEntity(domainModel.ID, b.entity)
	if err != nil {
		return nil, fmt.Errorf("failed to create entity: %w", err)
	}

	return b.entity, nil
}

// AssociationBuilder builds a new association with fluent API.
type AssociationBuilder struct {
	api         *DomainModelsAPI
	name        string
	association *domainmodel.Association
	module      *model.Module
	parentName  string
	childName   string
	err         error
}

// InModule sets the module for this association.
func (b *AssociationBuilder) InModule(module *model.Module) *AssociationBuilder {
	b.module = module
	return b
}

// From sets the parent (owner) entity.
func (b *AssociationBuilder) From(entityName string) *AssociationBuilder {
	b.parentName = entityName
	return b
}

// To sets the child entity.
func (b *AssociationBuilder) To(entityName string) *AssociationBuilder {
	b.childName = entityName
	return b
}

// OneToMany sets the association type to one-to-many (Reference).
func (b *AssociationBuilder) OneToMany() *AssociationBuilder {
	b.association.Type = domainmodel.AssociationTypeReference
	return b
}

// ManyToMany sets the association type to many-to-many (ReferenceSet).
func (b *AssociationBuilder) ManyToMany() *AssociationBuilder {
	b.association.Type = domainmodel.AssociationTypeReferenceSet
	return b
}

// OneToOne sets the association type to one-to-one (Reference with navigability).
func (b *AssociationBuilder) OneToOne() *AssociationBuilder {
	b.association.Type = domainmodel.AssociationTypeReference
	return b
}

// StorageColumn sets the storage format to Column (foreign key in parent table).
func (b *AssociationBuilder) StorageColumn() *AssociationBuilder {
	b.association.StorageFormat = domainmodel.StorageFormatColumn
	return b
}

// StorageTable sets the storage format to Table (junction/link table).
func (b *AssociationBuilder) StorageTable() *AssociationBuilder {
	b.association.StorageFormat = domainmodel.StorageFormatTable
	return b
}

// WithDeleteBehavior sets the delete behavior for parent and child.
func (b *AssociationBuilder) WithDeleteBehavior(parent, child string) *AssociationBuilder {
	// Create delete behavior structs
	parentBehavior := &domainmodel.DeleteBehavior{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$DeleteBehavior",
		},
	}
	childBehavior := &domainmodel.DeleteBehavior{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$DeleteBehavior",
		},
	}

	switch parent {
	case "DeleteMeAndReferences":
		parentBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeAndReferences
	case "DeleteMeIfNoReferences":
		parentBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences
	default:
		parentBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	}

	switch child {
	case "DeleteMeAndReferences":
		childBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeAndReferences
	case "DeleteMeIfNoReferences":
		childBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences
	default:
		childBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	}

	b.association.ParentDeleteBehavior = parentBehavior
	b.association.ChildDeleteBehavior = childBehavior
	return b
}

// Build creates the association and saves it to the project.
func (b *AssociationBuilder) Build() (*domainmodel.Association, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Determine module
	module := b.module
	if module == nil {
		module = b.api.api.currentModule
	}
	if module == nil {
		return nil, fmt.Errorf("no module specified; use InModule() or api.SetModule()")
	}

	// Get domain model
	domainModel, err := b.api.api.reader.GetDomainModel(module.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain model: %w", err)
	}

	// Resolve parent entity ID
	if b.parentName != "" {
		parentEntity, err := b.api.GetEntity(b.parentName)
		if err != nil {
			return nil, fmt.Errorf("parent entity not found: %s", b.parentName)
		}
		b.association.ParentID = parentEntity.ID
	}

	// Resolve child entity ID
	if b.childName != "" {
		childEntity, err := b.api.GetEntity(b.childName)
		if err != nil {
			return nil, fmt.Errorf("child entity not found: %s", b.childName)
		}
		b.association.ChildID = childEntity.ID
	}

	// Set defaults if not specified
	if b.association.Type == "" {
		b.association.Type = domainmodel.AssociationTypeReference
	}

	// Create the association
	err = b.api.api.writer.CreateAssociation(domainModel.ID, b.association)
	if err != nil {
		return nil, fmt.Errorf("failed to create association: %w", err)
	}

	return b.association, nil
}

// AttributeBuilder builds a new attribute for an existing entity.
type AttributeBuilder struct {
	api    *DomainModelsAPI
	entity *domainmodel.Entity
	attr   *domainmodel.Attribute
	err    error
}

// Name sets the attribute name.
func (b *AttributeBuilder) Name(name string) *AttributeBuilder {
	b.attr.Name = name
	return b
}

// String sets the attribute type to string with the given length.
func (b *AttributeBuilder) String(length int) *AttributeBuilder {
	b.attr.Type = &domainmodel.StringAttributeType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$StringAttributeType",
		},
		Length: length,
	}
	return b
}

// Integer sets the attribute type to integer.
func (b *AttributeBuilder) Integer() *AttributeBuilder {
	b.attr.Type = &domainmodel.IntegerAttributeType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$IntegerAttributeType",
		},
	}
	return b
}

// Long sets the attribute type to long.
func (b *AttributeBuilder) Long() *AttributeBuilder {
	b.attr.Type = &domainmodel.LongAttributeType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$LongAttributeType",
		},
	}
	return b
}

// Decimal sets the attribute type to decimal.
func (b *AttributeBuilder) Decimal() *AttributeBuilder {
	b.attr.Type = &domainmodel.DecimalAttributeType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$DecimalAttributeType",
		},
	}
	return b
}

// Boolean sets the attribute type to boolean.
func (b *AttributeBuilder) Boolean() *AttributeBuilder {
	b.attr.Type = &domainmodel.BooleanAttributeType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$BooleanAttributeType",
		},
	}
	return b
}

// DateTime sets the attribute type to datetime.
func (b *AttributeBuilder) DateTime() *AttributeBuilder {
	b.attr.Type = &domainmodel.DateTimeAttributeType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$DateTimeAttributeType",
		},
	}
	return b
}

// Enumeration sets the attribute type to enumeration.
func (b *AttributeBuilder) Enumeration(enumName string) *AttributeBuilder {
	b.attr.Type = &domainmodel.EnumerationAttributeType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$EnumerationAttributeType",
		},
		EnumerationRef: enumName,
	}
	return b
}

// Build creates the attribute and adds it to the entity.
func (b *AttributeBuilder) Build() (*domainmodel.Attribute, error) {
	if b.err != nil {
		return nil, b.err
	}

	if b.attr.Name == "" {
		return nil, fmt.Errorf("attribute name is required")
	}

	if b.attr.Type == nil {
		return nil, fmt.Errorf("attribute type is required")
	}

	// Find the domain model containing this entity
	modules, err := b.api.api.reader.ListModules()
	if err != nil {
		return nil, err
	}

	for _, module := range modules {
		domainModel, err := b.api.api.reader.GetDomainModel(module.ID)
		if err != nil {
			continue
		}

		for _, e := range domainModel.Entities {
			if e.ID == b.entity.ID {
				// Add the attribute
				err = b.api.api.writer.AddAttribute(domainModel.ID, b.entity.ID, b.attr)
				if err != nil {
					return nil, fmt.Errorf("failed to add attribute: %w", err)
				}
				return b.attr, nil
			}
		}
	}

	return nil, fmt.Errorf("entity not found in any module")
}

// AttributeModifier modifies an existing attribute.
type AttributeModifier struct {
	api  *DomainModelsAPI
	attr *domainmodel.Attribute
}

// WithLength changes the length (for string attributes).
func (m *AttributeModifier) WithLength(length int) *AttributeModifier {
	if strType, ok := m.attr.Type.(*domainmodel.StringAttributeType); ok {
		strType.Length = length
	}
	return m
}

// Required marks the attribute as required (not null).
func (m *AttributeModifier) Required() *AttributeModifier {
	// This would set validation rules - simplified for now
	return m
}

// Apply saves the modifications.
func (m *AttributeModifier) Apply() error {
	// Find the domain model and entity containing this attribute
	modules, err := m.api.api.reader.ListModules()
	if err != nil {
		return err
	}

	for _, module := range modules {
		domainModel, err := m.api.api.reader.GetDomainModel(module.ID)
		if err != nil {
			continue
		}

		for _, entity := range domainModel.Entities {
			for _, attr := range entity.Attributes {
				if attr.ID == m.attr.ID {
					return m.api.api.writer.UpdateAttribute(domainModel.ID, entity.ID, m.attr)
				}
			}
		}
	}

	return fmt.Errorf("attribute not found")
}

// EntityBatchModifier performs batch modifications on an entity.
type EntityBatchModifier struct {
	api    *DomainModelsAPI
	entity *domainmodel.Entity
	ops    []batchOp
}

type batchOp struct {
	opType string
	name   string
	attr   *domainmodel.Attribute
}

// AddStringAttribute adds a string attribute in the batch.
func (m *EntityBatchModifier) AddStringAttribute(name string, length int) *EntityBatchModifier {
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DomainModels$Attribute",
		},
		Name: name,
		Type: &domainmodel.StringAttributeType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DomainModels$StringAttributeType",
			},
			Length: length,
		},
	}
	m.ops = append(m.ops, batchOp{opType: "add", name: name, attr: attr})
	return m
}

// RemoveAttribute marks an attribute for removal in the batch.
func (m *EntityBatchModifier) RemoveAttribute(name string) *EntityBatchModifier {
	m.ops = append(m.ops, batchOp{opType: "remove", name: name})
	return m
}

// Apply executes all batch operations.
func (m *EntityBatchModifier) Apply() error {
	// Find the domain model containing this entity
	modules, err := m.api.api.reader.ListModules()
	if err != nil {
		return err
	}

	for _, module := range modules {
		domainModel, err := m.api.api.reader.GetDomainModel(module.ID)
		if err != nil {
			continue
		}

		for _, entity := range domainModel.Entities {
			if entity.ID == m.entity.ID {
				// Execute operations
				for _, op := range m.ops {
					switch op.opType {
					case "add":
						if err := m.api.api.writer.AddAttribute(domainModel.ID, entity.ID, op.attr); err != nil {
							return fmt.Errorf("failed to add attribute %s: %w", op.name, err)
						}
					case "remove":
						// Find attribute by name
						for _, attr := range entity.Attributes {
							if attr.Name == op.name {
								if err := m.api.api.writer.DeleteAttribute(domainModel.ID, entity.ID, attr.ID); err != nil {
									return fmt.Errorf("failed to remove attribute %s: %w", op.name, err)
								}
								break
							}
						}
					}
				}
				return nil
			}
		}
	}

	return fmt.Errorf("entity not found")
}
