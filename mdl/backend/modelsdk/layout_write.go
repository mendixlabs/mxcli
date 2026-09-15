// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// layoutToGen builds a Forms$Layout.
//
// The shape is measured against Atlas_Core.Atlas_Default on 11.13.0, whose
// top-level keys are exactly $ID, $Type, Appearance, CanvasHeight, CanvasWidth,
// Content, Documentation, Excluded, ExportLevel, Name. Two things follow that a
// reading of gen alone would get wrong:
//
//   - The widget tree hangs off Content, a Forms$WebLayoutContent (or
//     Forms$NativeLayoutContent), never off the layout directly.
//   - LayoutType is a property of that wrapper. gen exposes Layout.LayoutType()
//     bound to a key the document does not carry, so setting it there writes a
//     property Mendix ignores and reads back "" — which is how DESCRIBE LAYOUT
//     came to report every layout as Responsive.
//
// Those ten keys are also the whole type: generated/metamodel's PagesLayout
// declares exactly them. gen additionally exposes a family of placeholder
// properties on Layout — MainPlaceholderName, AcceptPlaceholderName,
// UseMainPlaceholderForPopups and four more — that no Atlas layout carries and
// the metamodel does not declare. Writing one produces a document mxbuild
// accepts (measured: 0 errors) and Studio Pro refuses to open, because it
// resolves every stored property against the type's list. Which placeholder is
// "main" is a naming convention instead: 22 of 22 Atlas layouts name one Main,
// and a page binds to a placeholder by qualified name anyway
// (Atlas_Core.Atlas_Default.Main), so there is nothing for the document to say.
func layoutToGen(l *pages.Layout) (*genPg.Layout, error) {
	if l.Name == "" {
		return nil, fmt.Errorf("layout needs a name")
	}
	if l.LayoutType == "" {
		return nil, fmt.Errorf("layout %q needs a layout type (%s)", l.Name, layoutTypeChoices(l.Native))
	}
	// Validated against the platform rather than one flat list: Responsive on a
	// native layout is as meaningless as Default on a web one, and Mendix does
	// not report either.
	if !pages.ValidLayoutType(l.LayoutType, l.Native) {
		return nil, fmt.Errorf("layout %q: %q is not a %s layout type (%s)",
			l.Name, l.LayoutType, platformWord(l.Native), layoutTypeChoices(l.Native))
	}

	out := genPg.NewLayout()
	if l.ID != "" {
		out.SetID(element.ID(l.ID))
	}
	assignID(out)
	out.SetName(l.Name)
	out.SetDocumentation(l.Documentation)
	out.SetExportLevel("Hidden")
	out.SetCanvasWidth(1280)
	out.SetCanvasHeight(800)
	out.SetAppearance(newAppearance(l.Class, l.Style, "", nil))
	// Written explicitly rather than left to the load-time default, so the
	// document carries the same ten keys Studio Pro writes.
	out.SetExcluded(false)

	// mxbuild requires EXACTLY ONE placeholder named `Main`, and unique names
	// besides. This guard used to ask only whether there was A placeholder under
	// any name — which is what mxcli's docs claimed the rule was — so it wrote
	// the very document that fails CE0848 (mendixlabs/mxcli#1063). The rule is
	// shared with `mxcli check` rather than restated, so the two cannot drift.
	if err := checkLayoutPlaceholders(l); err != nil {
		return nil, err
	}

	content, err := layoutContentToGen(l)
	if err != nil {
		return nil, err
	}
	out.SetContent(content)
	return out, nil
}

// layoutContentToGen builds the wrapper the widget tree hangs off.
func layoutContentToGen(l *pages.Layout) (element.Element, error) {
	widgets := make([]element.Element, 0, len(l.Widgets))
	for _, w := range l.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		widgets = append(widgets, wg)
	}

	if l.Native {
		c := genPg.NewNativeLayoutContent()
		assignID(c)
		c.SetLayoutType(string(l.LayoutType))
		for _, w := range widgets {
			c.AddWidgets(w)
		}
		return c, nil
	}
	c := genPg.NewWebLayoutContent()
	assignID(c)
	c.SetLayoutType(string(l.LayoutType))
	for _, w := range widgets {
		c.AddWidgets(w)
	}
	return c, nil
}

// hasAnyPlaceholder walks the tree for a Forms$Placeholder.
// checkLayoutPlaceholders applies the rule mxbuild enforces, citing the CE
// number the author would otherwise meet a whole build later — and, until
// DROP LAYOUT existed, with no way to undo the document mxcli had just written.
func checkLayoutPlaceholders(l *pages.Layout) error {
	names := layoutPlaceholderNames(l.Widgets, nil)
	issue, mains, dups := types.CheckLayoutPlaceholderNames(names)
	switch issue {
	case types.LayoutPlaceholderNoMain:
		have := "declares no placeholder at all"
		if len(names) > 0 {
			have = fmt.Sprintf("declares only %s", quoteNames(names))
		}
		return fmt.Errorf("layout %q %s: mxbuild fails this with CE0848 "+
			"(\"No placeholder with the name 'Main' found. There should be exactly one.\"). "+
			"Add `placeholder Main` to the region that should hold page content", l.Name, have)
	case types.LayoutPlaceholderManyMains:
		return fmt.Errorf("layout %q declares %d placeholders named %q: mxbuild fails this with "+
			"CE0849 (\"Multiple placeholders with name 'Main' found. There can be only one.\")",
			l.Name, mains, types.MainPlaceholderName)
	case types.LayoutPlaceholderDuplicateName:
		return fmt.Errorf("layout %q declares more than one placeholder named %s: mxbuild fails "+
			"this with CE0495 (\"Duplicate name '%s'.\") — a page binds to a placeholder as "+
			"Module.Layout.<Name>, which cannot pick between two that share it",
			l.Name, quoteNames(dups), dups[0])
	}
	return nil
}

func quoteNames(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(q, ", ")
}

// layoutPlaceholderNames collects placeholder names anywhere in the tree, in
// document order — a real layout nests them inside a scroll container's regions.
func layoutPlaceholderNames(widgets []pages.Widget, acc []string) []string {
	for _, w := range widgets {
		switch x := w.(type) {
		case *pages.LayoutPlaceholder:
			acc = append(acc, x.Name)
		case *pages.ScrollContainer:
			for _, r := range x.Regions {
				acc = layoutPlaceholderNames(r.Widgets, acc)
			}
			acc = layoutPlaceholderNames(x.Widgets, acc)
		case *pages.Container:
			acc = layoutPlaceholderNames(x.Widgets, acc)
		case *pages.GroupBox:
			acc = layoutPlaceholderNames(x.Widgets, acc)
		}
	}
	return acc
}

func platformWord(native bool) string {
	if native {
		return "native"
	}
	return "web"
}

func layoutTypeChoices(native bool) string {
	set := pages.WebLayoutTypes
	if native {
		set = pages.NativeLayoutTypes
	}
	out := ""
	for i, v := range set {
		if i > 0 {
			out += ", "
		}
		out += string(v)
	}
	return out
}

// CreateLayout inserts a new Forms$Layout unit.
func (b *Backend) CreateLayout(layout *pages.Layout) error {
	if layout == nil {
		return fmt.Errorf("CreateLayout: nil layout")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateLayout: not connected for writing")
	}
	if layout.ID == "" {
		layout.ID = model.ID(mmpr.GenerateID())
	}
	g, err := layoutToGen(layout)
	if err != nil {
		return err
	}
	g.SetID(element.ID(layout.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateLayout: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(layout.ID), string(layout.ContainerID), "Documents", "Forms$Layout", contents); err != nil {
		return fmt.Errorf("CreateLayout: insert: %w", err)
	}
	return nil
}

// DeleteLayout removes a Forms$Layout unit. CREATE OR REPLACE LAYOUT is a
// delete followed by a create, so this is on the write path, not a convenience.
func (b *Backend) DeleteLayout(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteLayout: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}
