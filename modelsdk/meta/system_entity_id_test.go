// SPDX-License-Identifier: Apache-2.0

package meta

import "testing"

// ako/mxcli#2292: the reference is a Studio Pro-authored value, not the table
// checked against itself. A Studio Pro 11.16.1 save of an AgentCommons agent
// whose context entity is System.Language stores
// "entity":{"documentId":"83d81a7b-4a84-416e-a64f-0ffa981c8408",…}.
func TestSystemEntityID_StudioProReference(t *testing.T) {
	if got, ok := SystemEntityID("Language"); !ok || got != "83d81a7b-4a84-416e-a64f-0ffa981c8408" {
		t.Errorf("SystemEntityID(Language) = %q, %v; want Studio Pro's 83d81a7b-4a84-416e-a64f-0ffa981c8408", got, ok)
	}
	if got, ok := SystemEntityID("User"); !ok || got != "0e439bf8-87c0-406f-ad3f-630ec563f770" {
		t.Errorf("SystemEntityID(User) = %q, %v", got, ok)
	}
	// An id mxbuild mints at load differs between two builds of one project:
	// there is nothing to write, so it is not in the table.
	if got, ok := SystemEntityID("Thumbnail"); ok {
		t.Errorf("SystemEntityID(Thumbnail) = %q; a per-load id must not be listed", got)
	}
	// The table is keyed by the unqualified name.
	if _, ok := SystemEntityID("System.User"); ok {
		t.Error("SystemEntityID takes the unqualified name")
	}
}
