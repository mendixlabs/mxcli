// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func (m *MockBackend) ListPages() ([]*pages.Page, error) {
	if m.ListPagesFunc != nil {
		return m.ListPagesFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetPage(id model.ID) (*pages.Page, error) {
	if m.GetPageFunc != nil {
		return m.GetPageFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) CreatePage(page *pages.Page) error {
	if m.CreatePageFunc != nil {
		return m.CreatePageFunc(page)
	}
	return nil
}

func (m *MockBackend) UpdatePage(page *pages.Page) error {
	if m.UpdatePageFunc != nil {
		return m.UpdatePageFunc(page)
	}
	return nil
}

func (m *MockBackend) DeletePage(id model.ID) error {
	if m.DeletePageFunc != nil {
		return m.DeletePageFunc(id)
	}
	return nil
}

func (m *MockBackend) MovePage(page *pages.Page) error {
	if m.MovePageFunc != nil {
		return m.MovePageFunc(page)
	}
	return nil
}

func (m *MockBackend) ListLayouts() ([]*pages.Layout, error) {
	if m.ListLayoutsFunc != nil {
		return m.ListLayoutsFunc()
	}
	return nil, nil
}

func (m *MockBackend) GetLayout(id model.ID) (*pages.Layout, error) {
	if m.GetLayoutFunc != nil {
		return m.GetLayoutFunc(id)
	}
	return nil, nil
}

func (m *MockBackend) CreateLayout(layout *pages.Layout) error {
	if m.CreateLayoutFunc != nil {
		return m.CreateLayoutFunc(layout)
	}
	return nil
}

func (m *MockBackend) UpdateLayout(layout *pages.Layout) error {
	if m.UpdateLayoutFunc != nil {
		return m.UpdateLayoutFunc(layout)
	}
	return nil
}

func (m *MockBackend) DeleteLayout(id model.ID) error {
	if m.DeleteLayoutFunc != nil {
		return m.DeleteLayoutFunc(id)
	}
	return nil
}

func (m *MockBackend) ListSnippets() ([]*pages.Snippet, error) {
	if m.ListSnippetsFunc != nil {
		return m.ListSnippetsFunc()
	}
	return nil, nil
}

func (m *MockBackend) CreateSnippet(snippet *pages.Snippet) error {
	if m.CreateSnippetFunc != nil {
		return m.CreateSnippetFunc(snippet)
	}
	return nil
}

func (m *MockBackend) UpdateSnippet(snippet *pages.Snippet) error {
	if m.UpdateSnippetFunc != nil {
		return m.UpdateSnippetFunc(snippet)
	}
	return nil
}

func (m *MockBackend) DeleteSnippet(id model.ID) error {
	if m.DeleteSnippetFunc != nil {
		return m.DeleteSnippetFunc(id)
	}
	return nil
}

func (m *MockBackend) MoveSnippet(snippet *pages.Snippet) error {
	if m.MoveSnippetFunc != nil {
		return m.MoveSnippetFunc(snippet)
	}
	return nil
}

func (m *MockBackend) ListBuildingBlocks() ([]*pages.BuildingBlock, error) {
	if m.ListBuildingBlocksFunc != nil {
		return m.ListBuildingBlocksFunc()
	}
	return nil, nil
}

func (m *MockBackend) ListPageTemplates() ([]*pages.PageTemplate, error) {
	if m.ListPageTemplatesFunc != nil {
		return m.ListPageTemplatesFunc()
	}
	return nil, nil
}
