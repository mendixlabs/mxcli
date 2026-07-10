// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// MicroflowsAPI provides methods for working with microflows.
type MicroflowsAPI struct {
	api *ModelAPI
}

// CreateMicroflow starts building a new microflow.
func (m *MicroflowsAPI) CreateMicroflow(name string) *MicroflowBuilder {
	return &MicroflowBuilder{
		api:  m,
		name: name,
		microflow: &microflows.Microflow{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Microflows$Microflow",
			},
			Name: name,
		},
	}
}

// GetMicroflow retrieves a microflow by qualified name.
func (m *MicroflowsAPI) GetMicroflow(qualifiedName string) (*microflows.Microflow, error) {
	qn := ParseQualifiedName(qualifiedName)

	// List all microflows and find by module/name
	allMicroflows, err := m.api.reader.ListMicroflows()
	if err != nil {
		return nil, err
	}

	// Get the module to match container ID
	module, err := m.api.reader.GetModuleByName(qn.ModuleName)
	if err != nil {
		return nil, fmt.Errorf("module not found: %s", qn.ModuleName)
	}

	for _, mf := range allMicroflows {
		if mf.ContainerID == module.ID && mf.Name == qn.ElementName {
			return mf, nil
		}
	}

	return nil, fmt.Errorf("microflow not found: %s", qualifiedName)
}

// FindMicroflowsWithEntity finds all microflows that have parameters of a given entity type.
func (m *MicroflowsAPI) FindMicroflowsWithEntity(entityName string) ([]*microflows.Microflow, error) {
	allMicroflows, err := m.api.reader.ListMicroflows()
	if err != nil {
		return nil, err
	}

	var result []*microflows.Microflow
	for _, mf := range allMicroflows {
		// Check parameters
		for _, param := range mf.Parameters {
			if objType, ok := param.Type.(*microflows.ObjectType); ok {
				if objType.EntityQualifiedName == entityName {
					result = append(result, mf)
					break
				}
			}
		}
	}

	return result, nil
}

// MicroflowBuilder builds a new microflow with fluent API.
type MicroflowBuilder struct {
	api       *MicroflowsAPI
	name      string
	microflow *microflows.Microflow
	module    *model.Module
	err       error
}

// InModule sets the module for this microflow.
func (b *MicroflowBuilder) InModule(module *model.Module) *MicroflowBuilder {
	b.module = module
	return b
}

// WithParameter adds a parameter to the microflow.
func (b *MicroflowBuilder) WithParameter(name string, entityName string) *MicroflowBuilder {
	param := &microflows.MicroflowParameter{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Microflows$MicroflowParameter",
		},
		Name: name,
		Type: &microflows.ObjectType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DataTypes$ObjectType",
			},
			EntityQualifiedName: entityName,
		},
	}
	b.microflow.Parameters = append(b.microflow.Parameters, param)
	return b
}

// WithStringParameter adds a string parameter to the microflow.
func (b *MicroflowBuilder) WithStringParameter(name string) *MicroflowBuilder {
	param := &microflows.MicroflowParameter{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Microflows$MicroflowParameter",
		},
		Name: name,
		Type: &microflows.StringType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DataTypes$StringType",
			},
		},
	}
	b.microflow.Parameters = append(b.microflow.Parameters, param)
	return b
}

// WithBooleanParameter adds a boolean parameter to the microflow.
func (b *MicroflowBuilder) WithBooleanParameter(name string) *MicroflowBuilder {
	param := &microflows.MicroflowParameter{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "Microflows$MicroflowParameter",
		},
		Name: name,
		Type: &microflows.BooleanType{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "DataTypes$BooleanType",
			},
		},
	}
	b.microflow.Parameters = append(b.microflow.Parameters, param)
	return b
}

// ReturnsBoolean sets the return type to boolean.
func (b *MicroflowBuilder) ReturnsBoolean() *MicroflowBuilder {
	b.microflow.ReturnType = &microflows.BooleanType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DataTypes$BooleanType",
		},
	}
	return b
}

// ReturnsString sets the return type to string.
func (b *MicroflowBuilder) ReturnsString() *MicroflowBuilder {
	b.microflow.ReturnType = &microflows.StringType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DataTypes$StringType",
		},
	}
	return b
}

// ReturnsObject sets the return type to an entity object.
func (b *MicroflowBuilder) ReturnsObject(entityName string) *MicroflowBuilder {
	b.microflow.ReturnType = &microflows.ObjectType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DataTypes$ObjectType",
		},
		EntityQualifiedName: entityName,
	}
	return b
}

// ReturnsList sets the return type to a list of entities.
func (b *MicroflowBuilder) ReturnsList(entityName string) *MicroflowBuilder {
	b.microflow.ReturnType = &microflows.ListType{
		BaseElement: model.BaseElement{
			ID:       generateID(),
			TypeName: "DataTypes$ListType",
		},
		EntityQualifiedName: entityName,
	}
	return b
}

// Build creates the microflow and saves it to the project.
func (b *MicroflowBuilder) Build() (*microflows.Microflow, error) {
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
	b.microflow.ContainerID = module.ID

	// Initialize empty object collection if not set
	if b.microflow.ObjectCollection == nil {
		b.microflow.ObjectCollection = &microflows.MicroflowObjectCollection{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Microflows$MicroflowObjectCollection",
			},
		}

		// Add start event
		startEvent := &microflows.StartEvent{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Microflows$StartEvent",
				},
				Position:       model.Point{X: 200, Y: 200},
				Size:           model.Size{Width: 20, Height: 20},
				RelativeMiddle: model.Point{X: 10, Y: 10},
			},
		}

		// Add end event
		endEvent := &microflows.EndEvent{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{
					ID:       generateID(),
					TypeName: "Microflows$EndEvent",
				},
				Position:       model.Point{X: 550, Y: 200},
				Size:           model.Size{Width: 20, Height: 20},
				RelativeMiddle: model.Point{X: 10, Y: 10},
			},
		}

		b.microflow.ObjectCollection.Objects = append(b.microflow.ObjectCollection.Objects, startEvent, endEvent)

		// Add sequence flow connecting them
		flow := &microflows.SequenceFlow{
			BaseElement: model.BaseElement{
				ID:       generateID(),
				TypeName: "Microflows$SequenceFlow",
			},
			OriginID:      startEvent.ID,
			DestinationID: endEvent.ID,
		}
		b.microflow.ObjectCollection.Flows = append(b.microflow.ObjectCollection.Flows, flow)
	}

	// Create the microflow
	err := b.api.api.writer.CreateMicroflow(b.microflow)
	if err != nil {
		return nil, fmt.Errorf("failed to create microflow: %w", err)
	}

	return b.microflow, nil
}
