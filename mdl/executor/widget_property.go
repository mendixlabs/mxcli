// SPDX-License-Identifier: Apache-2.0

// Package executor - Widget tree walking for styling and introspection commands.
package executor

import (
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// walkPageWidgets walks all widgets in a page and calls the visitor function.
func walkPageWidgets(page *pages.Page, visitor func(widget any) error) error {
	if page == nil || page.LayoutCall == nil {
		return nil
	}

	// Walk through layout call arguments (each argument has a single widget)
	for _, arg := range page.LayoutCall.Arguments {
		if arg.Widget != nil {
			if err := walkWidget(arg.Widget, visitor); err != nil {
				return err
			}
		}
	}

	return nil
}

// walkSnippetWidgets walks all widgets in a snippet and calls the visitor function.
func walkSnippetWidgets(snippet *pages.Snippet, visitor func(widget any) error) error {
	if snippet == nil {
		return nil
	}

	for _, widget := range snippet.Widgets {
		if err := walkWidget(widget, visitor); err != nil {
			return err
		}
	}

	return nil
}

// walkWidget recursively walks a widget and its children.
func walkWidget(widget pages.Widget, visitor func(widget any) error) error {
	if widget == nil {
		return nil
	}

	// Visit this widget
	if err := visitor(widget); err != nil {
		return err
	}

	// Recursively walk children based on widget type
	switch w := widget.(type) {
	case *pages.LayoutGrid:
		for _, row := range w.Rows {
			for _, col := range row.Columns {
				for _, child := range col.Widgets {
					if err := walkWidget(child, visitor); err != nil {
						return err
					}
				}
			}
		}
	case *pages.DataView:
		for _, child := range w.Widgets {
			if err := walkWidget(child, visitor); err != nil {
				return err
			}
		}
		for _, child := range w.FooterWidgets {
			if err := walkWidget(child, visitor); err != nil {
				return err
			}
		}
	case *pages.ListView:
		for _, child := range w.Widgets {
			if err := walkWidget(child, visitor); err != nil {
				return err
			}
		}
	case *pages.Container:
		for _, child := range w.Widgets {
			if err := walkWidget(child, visitor); err != nil {
				return err
			}
		}
	case *pages.GroupBox:
		for _, child := range w.Widgets {
			if err := walkWidget(child, visitor); err != nil {
				return err
			}
		}
	case *pages.TabContainer:
		for _, pg := range w.TabPages {
			for _, child := range pg.Widgets {
				if err := walkWidget(child, visitor); err != nil {
					return err
				}
			}
		}
	case *pages.ScrollContainer:
		for _, child := range w.Widgets {
			if err := walkWidget(child, visitor); err != nil {
				return err
			}
		}
	case *pages.CustomWidget:
		// Custom widgets may have nested widgets in their value properties
		if w.WidgetObject != nil {
			for _, prop := range w.WidgetObject.Properties {
				if prop.Value != nil {
					for _, child := range prop.Value.Widgets {
						if err := walkWidget(child, visitor); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}
