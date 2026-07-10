// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

func TestShowNavigation_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{
				Profiles: []*types.NavigationProfile{{
					Name: "Responsive",
					Kind: "Responsive",
					MenuItems: []*types.NavMenuItem{
						{Caption: "Home"},
						{Caption: "Admin"},
						{Caption: "Settings"},
					},
				}},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listNavigation(ctx))

	out := buf.String()
	assertContainsStr(t, out, "Profile")
	assertContainsStr(t, out, "Responsive")
}

func TestShowNavigationMenu_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{
				Profiles: []*types.NavigationProfile{{
					Name: "Responsive",
					Kind: "Responsive",
					MenuItems: []*types.NavMenuItem{
						{Caption: "Dashboard", Page: "MyModule.Dashboard"},
					},
				}},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listNavigationMenu(ctx, nil))
	assertContainsStr(t, buf.String(), "Dashboard")
}

func TestShowNavigationHomes_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{
				Profiles: []*types.NavigationProfile{{
					Name:     "Responsive",
					Kind:     "Responsive",
					HomePage: &types.NavHomePage{Page: "MyModule.Home"},
				}},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listNavigationHomes(ctx))
	assertContainsStr(t, buf.String(), "Default Home:")
}

func TestDescribeNavigation_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{
				Profiles: []*types.NavigationProfile{{
					Name:     "Responsive",
					Kind:     "Responsive",
					HomePage: &types.NavHomePage{Page: "MyModule.Home"},
				}},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeNavigation(ctx, ast.QualifiedName{Name: "Responsive"}))
	assertContainsStr(t, buf.String(), "create or replace navigation")
}

func TestDescribeNavigation_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{
				Profiles: []*types.NavigationProfile{{
					Name: "Responsive",
					Kind: "Responsive",
				}},
			}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeNavigation(ctx, ast.QualifiedName{Name: "NonExistent"}))
}

func TestDescribeNavigation_AllProfiles(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{
				Profiles: []*types.NavigationProfile{
					{Name: "Responsive", Kind: "Responsive"},
					{Name: "NativePhone", Kind: "NativePhone"},
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeNavigation(ctx, ast.QualifiedName{Name: ""}))

	out := buf.String()
	assertContainsStr(t, out, "Responsive")
	assertContainsStr(t, out, "NativePhone")
}
