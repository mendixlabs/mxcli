// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#2531: an entity named with a word the platform reserves passed
// check and exec, and mxbuild failed it with CE7247 "The name 'Owner' is a
// reserved word." Measured on 11.12.6 (and Owner/Type/Object on 11.16.1),
// one entity each: every word of types.PlatformReservedWords and
// SystemAttributeNames tried — Owner, Type, Object, ID, GUID, Context, Con,
// CurrentUser, MendixObject, SubMetaObjectName, CreatedDate, ChangedDate,
// ChangedBy, and the Java keywords Abstract, Default, Class, Interface (the
// last on a non-persistent entity) — is CE7247; String, User, System and List
// build. Quoting does not help: the platform checks the bare name.
func TestReservedEntityName_Refused_2531(t *testing.T) {
	for _, src := range []string{
		"mdl 1;\ncreate module R2531;\ncreate persistent entity R2531.Owner ( Name: String(50) );",
		"mdl 1;\ncreate module R2531;\ncreate persistent entity R2531.\"Type\" ( Name: String(50) );",
		"mdl 1;\ncreate module R2531;\ncreate non-persistent entity R2531.\"Interface\" ( Name: String(50) );",
		"mdl 1;\ncreate module R2531;\ncreate persistent entity R2531.CreatedDate ( Name: String(50) );",
		"mdl 1;\ncreate module R2531;\ncreate persistent entity R2531.Thing ( Name: String(50) );\nrename entity R2531.Thing to \"Object\";",
	} {
		t.Run(src, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, src, "CE7247")
		})
	}
}

// Controls: names that resemble system types but are not reserved.
func TestReservedEntityName_ControlsAccepted_2531(t *testing.T) {
	for _, src := range []string{
		"mdl 1;\ncreate module R2531;\ncreate persistent entity R2531.\"String\" ( Name: String(50) );",
		"mdl 1;\ncreate module R2531;\ncreate persistent entity R2531.\"User\" ( Name: String(50) );",
		"mdl 1;\ncreate module R2531;\ncreate persistent entity R2531.OwnerRecord ( Name: String(50) );",
	} {
		t.Run(src, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, src, "")
		})
	}
}
