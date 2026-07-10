// SPDX-License-Identifier: Apache-2.0

package mprread

import (
	genMf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/microflows"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

// ListMicroflows decodes every Microflows$Microflow unit in the project
// into the gen-typed *microflows.Microflow form.
//
// Microflows$Rule is a sibling type, not a Microflow alias — it gets
// its own lister (TODO: ListRules) once a caller needs it.
func ListMicroflows(r *mmpr.Reader) ([]*genMf.Microflow, error) {
	return ListUnitsByType[*genMf.Microflow](r)
}

// ListNanoflows decodes every Microflows$Nanoflow unit in the project
// into the gen-typed *microflows.Nanoflow form.
func ListNanoflows(r *mmpr.Reader) ([]*genMf.Nanoflow, error) {
	return ListUnitsByType[*genMf.Nanoflow](r)
}
