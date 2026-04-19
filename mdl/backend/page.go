// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// PageBackend provides page, layout, and snippet operations.
type PageBackend interface {
	// Pages
	ListPages() ([]*pages.Page, error)
	GetPage(id model.ID) (*pages.Page, error)
	CreatePage(page *pages.Page) error
	UpdatePage(page *pages.Page) error
	DeletePage(id model.ID) error
	MovePage(page *pages.Page) error

	// Layouts
	ListLayouts() ([]*pages.Layout, error)
	GetLayout(id model.ID) (*pages.Layout, error)
	CreateLayout(layout *pages.Layout) error
	UpdateLayout(layout *pages.Layout) error
	DeleteLayout(id model.ID) error

	// Snippets — no GetSnippet: snippets are resolved by qualified name via ListSnippets.
	ListSnippets() ([]*pages.Snippet, error)
	CreateSnippet(snippet *pages.Snippet) error
	UpdateSnippet(snippet *pages.Snippet) error
	DeleteSnippet(id model.ID) error
	MoveSnippet(snippet *pages.Snippet) error

	// Building blocks and page templates (read-only)
	ListBuildingBlocks() ([]*pages.BuildingBlock, error)
	ListPageTemplates() ([]*pages.PageTemplate, error)
}
