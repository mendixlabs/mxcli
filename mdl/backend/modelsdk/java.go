// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	genJA "github.com/mendixlabs/mxcli/modelsdk/gen/javaactions"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
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
			Excluded:      u.Element.Excluded(),
		}
		ja.ID = model.ID(u.Element.ID())
		out = append(out, ja)
	}
	// The System module's built-in Java actions are not stored in the .mpr, so a
	// reader that only decodes units reports them as absent. The legacy reader
	// appended them; not doing so lost System.VerifyPassword from `project-tree`
	// the moment it moved onto this backend.
	out = append(out, meta.BuildSystemJavaActions()...)
	return out, nil
}
