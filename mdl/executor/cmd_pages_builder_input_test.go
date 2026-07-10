// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// TestResolveSnippetRef_FromCache verifies that resolveSnippetRef finds snippets
// that were created earlier in the same session (before the backend sees them).
// Regression test for issue #509.
func TestResolveSnippetRef_FromCache(t *testing.T) {
	mod := mkModule("MyModule")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		// Backend returns nothing — snippet only exists in session cache
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return nil, nil },
	}

	snpID := model.ID("snp-session-1")
	cache := &executorCache{
		createdSnippets: map[string]*createdSnippetInfo{
			"MyModule.NavMenu": {
				ID:         snpID,
				Name:       "NavMenu",
				ModuleName: "MyModule",
			},
		},
	}

	h := mkHierarchy(mod)
	withContainer(h, mod.ID, mod.ID)

	pb := &pageBuilder{
		backend:   mb,
		execCache: cache,
	}
	pb.execCache.hierarchy = h

	// Should resolve from cache, not from backend
	id, err := pb.resolveSnippetRef("MyModule.NavMenu")
	if err != nil {
		t.Fatalf("resolveSnippetRef returned error: %v", err)
	}
	if id != snpID {
		t.Fatalf("expected ID %q, got %q", snpID, id)
	}
}

// TestResolveSnippetRef_NotFoundInCache verifies that when a snippet is absent
// from both cache and backend, a "not found" error is returned.
func TestResolveSnippetRef_NotFoundInCache(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return nil, nil },
		ListModulesFunc:  func() ([]*model.Module, error) { return nil, nil },
	}

	cache := &executorCache{
		createdSnippets: map[string]*createdSnippetInfo{},
	}

	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	withContainer(h, mod.ID, mod.ID)

	pb := &pageBuilder{
		backend:   mb,
		execCache: cache,
	}
	pb.execCache.hierarchy = h

	_, err := pb.resolveSnippetRef("MyModule.Missing")
	if err == nil {
		t.Fatal("expected error for missing snippet, got nil")
	}
}
