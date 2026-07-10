// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

func TestEntityAccessFromMemberRights(t *testing.T) {
	tests := []struct {
		name      string
		rule      *domainmodel.AccessRule
		wantRead  bool
		wantWrite bool
	}{
		{
			name: "no member accesses, default None",
			rule: &domainmodel.AccessRule{
				DefaultMemberAccessRights: domainmodel.MemberAccessRightsNone,
			},
			wantRead:  false,
			wantWrite: false,
		},
		{
			name: "no member accesses, default ReadOnly",
			rule: &domainmodel.AccessRule{
				DefaultMemberAccessRights: domainmodel.MemberAccessRightsReadOnly,
			},
			wantRead:  true,
			wantWrite: false,
		},
		{
			name: "no member accesses, default ReadWrite",
			rule: &domainmodel.AccessRule{
				DefaultMemberAccessRights: domainmodel.MemberAccessRightsReadWrite,
			},
			wantRead:  true,
			wantWrite: true,
		},
		{
			name: "explicit member ReadOnly",
			rule: &domainmodel.AccessRule{
				MemberAccesses: []*domainmodel.MemberAccess{
					{AttributeName: "Name", AccessRights: domainmodel.MemberAccessRightsReadOnly},
				},
			},
			wantRead:  true,
			wantWrite: false,
		},
		{
			name: "explicit member ReadWrite",
			rule: &domainmodel.AccessRule{
				MemberAccesses: []*domainmodel.MemberAccess{
					{AttributeName: "Name", AccessRights: domainmodel.MemberAccessRightsReadWrite},
				},
			},
			wantRead:  true,
			wantWrite: true,
		},
		{
			name: "mixed members — one ReadOnly one None",
			rule: &domainmodel.AccessRule{
				MemberAccesses: []*domainmodel.MemberAccess{
					{AttributeName: "Name", AccessRights: domainmodel.MemberAccessRightsReadOnly},
					{AttributeName: "Age", AccessRights: domainmodel.MemberAccessRightsNone},
				},
			},
			wantRead:  true,
			wantWrite: false,
		},
		{
			name: "mixed members — one ReadOnly one ReadWrite",
			rule: &domainmodel.AccessRule{
				MemberAccesses: []*domainmodel.MemberAccess{
					{AttributeName: "Name", AccessRights: domainmodel.MemberAccessRightsReadOnly},
					{AttributeName: "Age", AccessRights: domainmodel.MemberAccessRightsReadWrite},
				},
			},
			wantRead:  true,
			wantWrite: true,
		},
		{
			name: "all members None",
			rule: &domainmodel.AccessRule{
				MemberAccesses: []*domainmodel.MemberAccess{
					{AttributeName: "Name", AccessRights: domainmodel.MemberAccessRightsNone},
				},
			},
			wantRead:  false,
			wantWrite: false,
		},
		{
			name: "empty member accesses falls through to default",
			rule: &domainmodel.AccessRule{
				MemberAccesses:            []*domainmodel.MemberAccess{},
				DefaultMemberAccessRights: domainmodel.MemberAccessRightsReadOnly,
			},
			wantRead:  true,
			wantWrite: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRead, gotWrite := entityAccessFromMemberRights(tt.rule)
			if gotRead != tt.wantRead {
				t.Errorf("hasRead = %v, want %v", gotRead, tt.wantRead)
			}
			if gotWrite != tt.wantWrite {
				t.Errorf("hasWrite = %v, want %v", gotWrite, tt.wantWrite)
			}
		})
	}
}
