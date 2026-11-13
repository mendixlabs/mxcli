// SPDX-License-Identifier: Apache-2.0

package canon

import "go.mongodb.org/mongo-driver/v2/bson"

// unstatedFlowDefaults are the microflow and nanoflow properties no MDL
// statement can state, by element $Type, each with the value the writer puts
// in its place. A rebuild — `create or modify` that cannot splice, or `drop` +
// `create` in one script, whose re-insert the writer reconciles with the
// deleted unit (ako/mxcli#2178, mendixlabs/mxcli#1831) — writes that default,
// so CarryUnexpressed puts the stored value back on the element of the same
// $ID:
//
//   - a flow's Mark as used;
//   - the documentation of a parameter, an activity, a loop or a decision
//     (MDL has a doc comment only for the flow itself);
//   - a loop's error handling — Continue reset to Rollback made one failing
//     iteration abort the whole loop;
//   - a REST call's proxy (Override with its expressions, or NoProxy), which
//     the in-place edit already keeps (ako/mxcli#2169).
//
// A written value other than the default came from somewhere that CAN state
// it, and is left alone. The day MDL gains a spelling for one of these, its
// entry goes: a statement that sets it back to the default would otherwise be
// undone.
var unstatedFlowDefaults = map[string]map[string]any{
	"Microflows$Microflow":          {"MarkAsUsed": false},
	"Microflows$Nanoflow":           {"MarkAsUsed": false},
	"Microflows$MicroflowParameter": {"Documentation": ""},
	"Microflows$ActionActivity":     {"Documentation": ""},
	"Microflows$ExclusiveSplit":     {"Documentation": ""},
	"Microflows$InheritanceSplit":   {"Documentation": ""},
	"Microflows$LoopedActivity":     {"Documentation": "", "ErrorHandlingType": "Rollback"},
	"Microflows$RestCallAction":     {"RequestProxyType": "DefaultProxy", "ProxyConfiguration": nil},
}

// carryUnstatedFlowProperties puts on d, an element of the written document,
// the stored value of each of its unstatedFlowDefaults the writer left at the
// default. used holds the written document's element ids: a carried
// sub-element whose id is already there would make two elements share one.
func carryUnstatedFlowProperties(d, old bson.D, used map[string]bool) bool {
	t, _ := docLookup(d, "$Type").(string)
	defaults := unstatedFlowDefaults[t]
	if defaults == nil || docLookup(old, "$Type") != t {
		return false
	}
	changed := false
	for i, e := range d {
		def, ok := defaults[e.Key]
		if !ok || e.Value != def {
			continue
		}
		sv, has := lookupKey(old, e.Key)
		if !has || sv == def {
			continue
		}
		if sd, isDoc := sv.(bson.D); isDoc && anyIDUsed(sd, used) {
			continue
		}
		d[i].Value = sv
		changed = true
	}
	return changed
}

// lookupKey is docLookup that also says whether the key is there at all.
func lookupKey(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}
