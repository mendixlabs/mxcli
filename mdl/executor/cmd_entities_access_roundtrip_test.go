// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// TestFormatAccessRuleRights_BareMemberNames is a regression test for issue #633:
// DESCRIBE emitted member-level grants with fully-qualified member names
// ("Module.Entity.Attr" / "Module.Assoc"), which the grant grammar rejects — it
// accepts a bare member IDENTIFIER only. The emitted MDL must use bare names so it
// re-parses.
func TestFormatAccessRuleRights_BareMemberNames(t *testing.T) {
	rule := &domainmodel.AccessRule{
		AllowCreate:               false,
		AllowDelete:               false,
		DefaultMemberAccessRights: domainmodel.MemberAccessRightsNone,
		MemberAccesses: []*domainmodel.MemberAccess{
			// BSON stores BY_NAME references fully qualified.
			{AttributeName: "RT.GraphTraversalHelper.DataJSON", AccessRights: domainmodel.MemberAccessRightsReadWrite},
			{AssociationName: "RT.ChatContext_Control_ChatContext", AccessRights: domainmodel.MemberAccessRightsReadOnly},
		},
	}

	got := formatAccessRuleRights(nil, rule, nil)

	for _, qualified := range []string{
		"RT.GraphTraversalHelper.DataJSON",
		"RT.ChatContext_Control_ChatContext",
		"RT.GraphTraversalHelper",
	} {
		if strings.Contains(got, qualified) {
			t.Errorf("rights string must not contain qualified member %q; got: %s", qualified, got)
		}
	}
	for _, bare := range []string{"DataJSON", "ChatContext_Control_ChatContext"} {
		if !strings.Contains(got, bare) {
			t.Errorf("rights string must contain bare member %q; got: %s", bare, got)
		}
	}
}
