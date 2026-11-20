// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

// FillMissingUserRoleGUIDs gives every Security$UserRole in contents that has
// NO GUID key the GUID mxcli's create path mints for one: its own $ID, as
// binary subtype 0 (codec EmitGUID). A role that has a GUID keeps it, whatever
// it holds. It returns contents unchanged — the same slice — when nothing was
// filled.
//
// # Why
//
// Studio Pro gives every user role a GUID; mxcli wrote roles without one
// before the create path minted it (mendixlabs/mxcli#1373). Such a role is a
// property-name gap Mendix's merge engine throws on, and — the part a constant
// default cannot fix, so it is not a row in the CompletePropertySets table —
// with no GUID in the model mxbuild invents a RANDOM one on every build.
// Measured on Mendix 11.15.0 with PostgreSQL: the runtime matches
// system$userrole rows by modelguid first and by name second, and never
// refreshes modelguid on a name match. Filling GUID = $ID kept the row id and
// every user-to-role link.
//
// # Absent only, never changed
//
// An existing GUID is the runtime's primary match key for the role's row; when
// it matches, it is the row's identity, and changing it is #1119 for users
// instead of columns. So this fills an ABSENT key and nothing else, and the
// storage-GUID guard (which compares a GUID present on both sides) still
// refuses any write that changes one.
//
// The fill makes a later rename safe only for a database first synchronised
// after it: one synchronised before keeps the stale random modelguid and is
// still matched by name, so a rename there still deletes the row.
func FillMissingUserRoleGUIDs(contents []byte) []byte {
	if len(contents) == 0 {
		return contents
	}
	var doc bson.D
	if err := parseForStorageForm(contents, &doc); err != nil {
		return contents
	}
	if !fillUserRoleGUIDs(&doc) {
		return contents
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		return contents
	}
	return out
}

func fillUserRoleGUIDs(d *bson.D) bool {
	changed := false
	for i := range *d {
		if fillUserRoleGUIDsIn(&(*d)[i].Value) {
			changed = true
		}
	}
	if t, _ := lookupString(*d, "$Type"); t != "Security$UserRole" || hasKey(*d, "GUID") {
		return changed
	}
	id := elementIDBytes(*d)
	if id == nil {
		return changed
	}
	*d = append(*d, bson.E{Key: "GUID", Value: bson.Binary{Subtype: 0, Data: append([]byte(nil), id...)}})
	return true
}

func fillUserRoleGUIDsIn(v *any) bool {
	switch x := (*v).(type) {
	case bson.D:
		if fillUserRoleGUIDs(&x) {
			*v = x
			return true
		}
	case bson.A:
		changed := false
		for i := range x {
			if fillUserRoleGUIDsIn(&x[i]) {
				changed = true
			}
		}
		return changed
	}
	return false
}
