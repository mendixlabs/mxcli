// SPDX-License-Identifier: Apache-2.0

package types

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// JavaAction is a lightweight Java action descriptor.
type JavaAction struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
}

// GetName returns the Java action's name.
func (ja *JavaAction) GetName() string { return ja.Name }

// GetContainerID returns the container ID.
func (ja *JavaAction) GetContainerID() model.ID { return ja.ContainerID }

// JavaScriptAction is a JavaScript action descriptor. Stage 3.3.2.C1
// retired the mdl/* consumers (they now use *genJSA.JavaScriptAction
// directly via ctx.JavaScriptActions); the struct is preserved because
// sdk/mpr aliases it (Stage 4 territory).
type JavaScriptAction struct {
	model.BaseElement
	ContainerID             model.ID               `json:"containerId"`
	Name                    string                 `json:"name"`
	Documentation           string                 `json:"documentation,omitempty"`
	Platform                string                 `json:"platform,omitempty"`
	Excluded                bool                   `json:"excluded"`
	ExportLevel             string                 `json:"exportLevel,omitempty"`
	ActionDefaultReturnName string                 `json:"actionDefaultReturnName,omitempty"`
	ReturnType              CodeActionReturnType   `json:"returnType,omitempty"`
	Parameters              []*JavaActionParameter `json:"parameters,omitempty"`
	TypeParameters          []*TypeParameterDef    `json:"typeParameters,omitempty"`
	MicroflowActionInfo     *MicroflowActionInfo   `json:"microflowActionInfo,omitempty"`
}

// GetName returns the JavaScript action's name.
func (jsa *JavaScriptAction) GetName() string { return jsa.Name }

// GetContainerID returns the container ID.
func (jsa *JavaScriptAction) GetContainerID() model.ID { return jsa.ContainerID }

// FindTypeParameterName looks up a type parameter name by its ID.
func (jsa *JavaScriptAction) FindTypeParameterName(id model.ID) string {
	for _, tp := range jsa.TypeParameters {
		if tp.ID == id {
			return tp.Name
		}
	}
	return ""
}
