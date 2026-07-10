// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// ---------------------------------------------------------------------------
// execShowFeatures
// ---------------------------------------------------------------------------

func TestShowFeatures_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

func TestShowFeatures_ForVersion(t *testing.T) {
	// ForVersion doesn't require connection — uses embedded registry directly.
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{ForVersion: "10.0"})
	assertNoError(t, err)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected output, got empty")
	}
	assertContainsStr(t, out, "Features for Mendix")
}

func TestShowFeatures_ForVersion_InvalidVersion(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{ForVersion: "not-a-version"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "invalid version")
}

func TestShowFeatures_AddedSince(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{AddedSince: "10.0"})
	assertNoError(t, err)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected output, got empty")
	}
	assertContainsStr(t, out, "Features added since Mendix")
}

func TestShowFeatures_AddedSince_InvalidVersion(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{AddedSince: "xyz"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "invalid version")
}

func TestShowFeatures_Connected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{
				ProductVersion: "10.6.0",
				MajorVersion:   10,
				MinorVersion:   6,
				PatchVersion:   0,
			}
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{})
	assertNoError(t, err)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected output, got empty")
	}
	assertContainsStr(t, out, "Features for Mendix")
}

func TestShowFeatures_InArea_ForVersion(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{ForVersion: "10.6", InArea: "domain_model"})
	assertNoError(t, err)
	// Area filter narrows output; assert header contains area name.
	assertContainsStr(t, buf.String(), "domain_model")
}
