// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"slices"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// EnumerationsAPI provides methods for working with enumerations.
type EnumerationsAPI struct {
	api *ModelAPI
}

// CreateEnumeration starts building a new enumeration.
func (e *EnumerationsAPI) CreateEnumeration(name string) *EnumerationBuilder {
	return &EnumerationBuilder{
		api:  e,
		name: name,
		enum: &model.Enumeration{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Enumerations$Enumeration",
			},
			Name:   name,
			Values: make([]model.EnumerationValue, 0),
		},
	}
}

// GetEnumeration retrieves an enumeration by qualified name.
func (e *EnumerationsAPI) GetEnumeration(qualifiedName string) (*model.Enumeration, error) {
	qn := ParseQualifiedName(qualifiedName)

	// List all enumerations and find by module/name
	enums, err := e.api.reader.ListEnumerations()
	if err != nil {
		return nil, err
	}

	// Get the module to match container ID
	module, err := e.api.reader.GetModuleByName(qn.ModuleName)
	if err != nil {
		return nil, fmt.Errorf("module not found: %s", qn.ModuleName)
	}

	for _, enum := range enums {
		if enum.ContainerID == module.ID && enum.Name == qn.ElementName {
			return enum, nil
		}
	}

	return nil, fmt.Errorf("enumeration not found: %s", qualifiedName)
}

// AddValue starts building a new value to add to an existing enumeration.
func (e *EnumerationsAPI) AddValue(enum *model.Enumeration) *EnumValueBuilder {
	return &EnumValueBuilder{
		api:  e,
		enum: enum,
		value: model.EnumerationValue{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Enumerations$EnumerationValue",
			},
		},
	}
}

// RemoveValue removes a value from an enumeration.
func (e *EnumerationsAPI) RemoveValue(enum *model.Enumeration, valueName string) error {
	// Find the value by name
	for i, v := range enum.Values {
		if v.Name == valueName {
			// Remove from slice
			enum.Values = append(enum.Values[:i], enum.Values[i+1:]...)
			// Update the enumeration
			return e.api.writer.UpdateEnumeration(enum)
		}
	}
	return fmt.Errorf("value not found: %s", valueName)
}

// ReorderValues reorders the values in an enumeration.
func (e *EnumerationsAPI) ReorderValues(enum *model.Enumeration, order []string) error {
	// Create a map for quick lookup
	valueMap := make(map[string]model.EnumerationValue)
	for _, v := range enum.Values {
		valueMap[v.Name] = v
	}

	// Reorder based on provided order
	newValues := make([]model.EnumerationValue, 0, len(order))
	for _, name := range order {
		if v, ok := valueMap[name]; ok {
			newValues = append(newValues, v)
		}
	}

	// Add any values not in the order list at the end
	for _, v := range enum.Values {
		found := slices.Contains(order, v.Name)
		if !found {
			newValues = append(newValues, v)
		}
	}

	enum.Values = newValues
	return e.api.writer.UpdateEnumeration(enum)
}

// EnumerationBuilder builds a new enumeration with fluent API.
type EnumerationBuilder struct {
	api    *EnumerationsAPI
	name   string
	enum   *model.Enumeration
	module *model.Module
	err    error
}

// InModule sets the module for this enumeration.
func (b *EnumerationBuilder) InModule(module *model.Module) *EnumerationBuilder {
	b.module = module
	return b
}

// WithValue adds a value to the enumeration.
func (b *EnumerationBuilder) WithValue(name string, caption string) *EnumerationBuilder {
	value := model.EnumerationValue{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Enumerations$EnumerationValue",
		},
		Name: name,
		Caption: &model.Text{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{
				"en_US": caption,
			},
		},
	}
	b.enum.Values = append(b.enum.Values, value)
	return b
}

// WithValues adds multiple values at once.
func (b *EnumerationBuilder) WithValues(values map[string]string) *EnumerationBuilder {
	for name, caption := range values {
		b.WithValue(name, caption)
	}
	return b
}

// Build creates the enumeration and saves it to the project.
func (b *EnumerationBuilder) Build() (*model.Enumeration, error) {
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

	// Set container
	b.enum.ContainerID = module.ID

	// Create the enumeration
	err := b.api.api.writer.CreateEnumeration(b.enum)
	if err != nil {
		return nil, fmt.Errorf("failed to create enumeration: %w", err)
	}

	return b.enum, nil
}

// EnumValueBuilder builds a new value for an existing enumeration.
type EnumValueBuilder struct {
	api   *EnumerationsAPI
	enum  *model.Enumeration
	value model.EnumerationValue
	err   error
}

// Name sets the value name.
func (b *EnumValueBuilder) Name(name string) *EnumValueBuilder {
	b.value.Name = name
	return b
}

// Caption sets the value caption.
func (b *EnumValueBuilder) Caption(caption string) *EnumValueBuilder {
	b.value.Caption = &model.Text{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Texts$Text",
		},
		Translations: map[string]string{
			"en_US": caption,
		},
	}
	return b
}

// Build adds the value to the enumeration and saves it.
func (b *EnumValueBuilder) Build() (*model.EnumerationValue, error) {
	if b.err != nil {
		return nil, b.err
	}

	if b.value.Name == "" {
		return nil, fmt.Errorf("value name is required")
	}

	// Add the value to the enumeration
	b.enum.Values = append(b.enum.Values, b.value)

	// Update the enumeration
	err := b.api.api.writer.UpdateEnumeration(b.enum)
	if err != nil {
		return nil, fmt.Errorf("failed to update enumeration: %w", err)
	}

	return &b.value, nil
}

// EnumValueModifier modifies an existing enumeration value.
type EnumValueModifier struct {
	api   *EnumerationsAPI
	enum  *model.Enumeration
	value *model.EnumerationValue
}

// WithCaption changes the caption.
func (m *EnumValueModifier) WithCaption(caption string) *EnumValueModifier {
	m.value.Caption = &model.Text{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Texts$Text",
		},
		Translations: map[string]string{
			"en_US": caption,
		},
	}
	return m
}

// Apply saves the modifications.
func (m *EnumValueModifier) Apply() error {
	return m.api.api.writer.UpdateEnumeration(m.enum)
}
