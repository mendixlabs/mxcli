// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	genJA "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/javaactions"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// ListJavaActions reads Java action units into the lightweight types.JavaAction
// (id, container, name, documentation) — enough for SHOW MODULES counts and
// listing. The rich parsed form (sdk/javaactions.JavaAction, with parameters and
// return types) is ListJavaActionsFull, deferred to a later phase.
func (b *Backend) ListJavaActions() ([]*types.JavaAction, error) {
	units, err := mprread.ListUnitsWithContainer[*genJA.JavaAction](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*types.JavaAction, 0, len(units))
	for _, u := range units {
		ja := &types.JavaAction{
			ContainerID:   u.ContainerID,
			Name:          u.Element.Name(),
			Documentation: u.Element.Documentation(),
		}
		ja.ID = model.ID(u.Element.ID())
		out = append(out, ja)
	}
	return out, nil
}
