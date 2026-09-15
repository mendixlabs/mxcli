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
	DeleteLayout(id model.ID) error

	// PageLayoutName returns the qualified name of the layout a page renders
	// inside ("Atlas_Core.Atlas_Default"), or "" when the page has none.
	//
	// ListPages is deliberately shallow — it does not decode the layout call —
	// so this is the semantic read for "which layout is this page on", the
	// question both the bulk repoint and `mxcli new` have to answer.
	PageLayoutName(id model.ID) (string, error)

	// LayoutPlaceholders returns the placeholder names a layout declares, in
	// document order. A page binds to one as Module.Layout.<Name>, so these are
	// the only valid targets for a repoint — see PageMutator.BoundPlaceholders.
	LayoutPlaceholders(id model.ID) ([]string, error)

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
