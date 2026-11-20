// SPDX-License-Identifier: Apache-2.0

package meta

// The System module's built-in enumerations, as model elements.
//
// Like the entities, associations and Java actions beside them, these are NOT
// stored in the .mpr — Mendix ships them with the platform — so a reader that
// only decodes stored units reports them as absent. The definitions in
// SystemEnumerations had been here since #889 with no consumer at all, which is
// why `describe entity` could print `ActivityType: Enumeration(System.
// WorkflowActivityType)` while `describe enumeration System.WorkflowActivityType`
// answered "enumeration not found": the entity half was wired and the
// enumeration half never was (mendixlabs/mxcli#1102).
//
// Read-only. The System module has no stored unit to contain anything, so the
// executor refuses every write that names it — see refuseSystemEnumerationWrite
// in mdl/executor/cmd_enumerations.go, which exists because making these
// visible also makes them addressable.

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// BuildSystemEnumerations returns the System module's enumerations as semantic
// model elements, ready to append to a backend's enumeration listing.
//
// IDs are deterministic, so two readers describing the same project agree about
// an enumeration's identity even though neither read it from storage, and the
// catalog (which keys enumerations_data on Id) stays stable run to run. Mirrors
// BuildSystemJavaActions.
//
// Captions are deliberately left unset: SystemEnumerations carries value NAMES
// only, and Mendix's own captions for these are not recorded here. Defaulting a
// caption to the value name would put invented text in DESCRIBE output and in
// the catalog's translation rows, where nothing could tell it from a caption a
// developer actually wrote. The cost is that `search` — which indexes captions —
// still does not match these; that needs real caption data, not a guess.
func BuildSystemEnumerations() []*model.Enumeration {
	return BuildSystemEnumerationsIn(nil)
}

// BuildSystemEnumerationsIn is BuildSystemEnumerations for one project
// version, given as its IsAtLeast; nil means the current platform.
func BuildSystemEnumerationsIn(atLeast func(major, minor int) bool) []*model.Enumeration {
	defs := SystemEnumerationsIn(atLeast)
	out := make([]*model.Enumeration, 0, len(defs))
	for _, def := range defs {
		e := &model.Enumeration{
			ContainerID: model.ID(SystemModuleID),
			// The model carries the LOCAL name; the module comes from
			// ContainerID, exactly as it does for a stored enumeration. Keeping
			// the qualified name here would make DESCRIBE emit
			// "System.System.WorkflowActivityType".
			Name: strings.TrimPrefix(def.Name, "System."),
		}
		e.ID = model.ID(types.GenerateDeterministicID(def.Name))
		e.TypeName = "Enumerations$Enumeration"
		for _, v := range def.Values {
			ev := model.EnumerationValue{Name: v}
			ev.ID = model.ID(types.GenerateDeterministicID(def.Name + "." + v))
			ev.TypeName = "Enumerations$EnumerationValue"
			e.Values = append(e.Values, ev)
		}
		out = append(out, e)
	}
	return out
}
