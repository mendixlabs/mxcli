// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/xpathrefs"
)

// missingEntityStepHint recognises `<association>.<attribute>` read as one
// qualified name — `[Car_Assignment.ReturnedOn = empty]` — where the first
// part is an association of the entity and the second an attribute of the
// entity at its other end (ako/mxcli#2491). What is missing is the entity
// step between them: `[Repro.Car_Assignment/Repro.Assignment/ReturnedOn …]`.
// The messages that called the name an unknown association, and MDL047's
// `not(…)` fix, kept the dotted name, and an agent followed them for 40
// minutes. It returns the clause that says so, "" when the name is not that
// shape.
func missingEntityStepHint(m xpathrefs.Model, resolve func(entityQN, member string) memberResolution, entityQN, name string) string {
	dot := strings.LastIndex(name, ".")
	if dot <= 0 || dot == len(name)-1 {
		return ""
	}
	assoc, attr := name[:dot], name[dot+1:]
	if !strings.Contains(assoc, ".") {
		mod := entityQN
		if i := strings.Index(entityQN, "."); i > 0 {
			mod = entityQN[:i]
		}
		assoc = mod + "." + assoc
	}
	target, ok := m.AssociationTarget(assoc, entityQN)
	if !ok || target == "" || resolve(target, attr) != memberFound {
		return ""
	}
	return fmt.Sprintf("association %s followed by attribute %s of %s, with the entity step between them missing — "+
		"a path goes association/entity/attribute: `[%s/%s/%s …]`, or with a predicate `[%s/%s[%s …]]`",
		assoc, attr, target, assoc, target, attr, assoc, target, attr)
}

// missingStepSuffix appends the clause to a message that has named the step.
func missingStepSuffix(hint string) string {
	if hint == "" {
		return ""
	}
	return ": it is " + hint
}
