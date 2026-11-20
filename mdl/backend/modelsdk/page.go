// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDT "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/texts" // register Texts$Text so page titles decode concretely
	"github.com/mendixlabs/mxcli/modelsdk/mprread"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// gen→pages read adapter. SHOW PAGES needs only the page header — name, module,
// excluded, title, URL, parameter count — not the widget tree, so this stays
// shallow. Widget-tree conversion (DESCRIBE PAGE / ALTER) is a later phase.

// ListPages returns pages only. Page templates are a separate document type with
// separate content (their widgets hang off LayoutCall, not FormCall) and are read
// by ListPageTemplates — this used to include them, to match legacy's
// prefix-matched `Forms$Page` query, which made `show modules` report 46 pages
// for a module with none.
// ListPageHeaders lists every page's ID, container, name and Excluded flag
// without decoding any page.
func (b *Backend) ListPageHeaders() ([]types.DocumentHeader, error) {
	return unitHeaders[*genPg.Page](b)
}

func (b *Backend) ListPages() ([]*pages.Page, error) {
	units, err := listUnits[*genPg.Page](b)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.Page, 0, len(units))
	for _, u := range units {
		out = append(out, pageFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

// ListPageTemplates reads page template units. Like ListPages this is a header
// read — name, module, documentation — matching what the legacy parser captures.
// The template's content is deliberately not converted: it hangs off LayoutCall
// rather than FormCall, nothing consumes it yet, and a half-converted tree would
// compare equal to a different one.
func (b *Backend) ListPageTemplates() ([]*pages.PageTemplate, error) {
	units, err := listUnits[*genPg.PageTemplate](b)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.PageTemplate, 0, len(units))
	for _, u := range units {
		out = append(out, pageTemplateFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

func pageTemplateFromGen(pt *genPg.PageTemplate, containerID model.ID) *pages.PageTemplate {
	out := &pages.PageTemplate{
		ContainerID:   containerID,
		Name:          pt.Name(),
		Documentation: pt.Documentation(),
	}
	out.ID = model.ID(pt.ID())
	out.TypeName = "Forms$PageTemplate"
	return out
}

// GetPage decodes the page whose ID is id, and no other (#1850).
func (b *Backend) GetPage(id model.ID) (*pages.Page, error) {
	u, ok, err := mprread.UnitByID[*genPg.Page](b.reader, id)
	if err != nil || !ok {
		return nil, err
	}
	return pageFromGen(u.Element, u.ContainerID), nil
}

// ListSnippets reads snippet units (count + identity is what SHOW MODULES needs;
// the widget tree is not converted).
func (b *Backend) ListSnippets() ([]*pages.Snippet, error) {
	units, err := listUnits[*genPg.Snippet](b)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.Snippet, 0, len(units))
	for _, u := range units {
		out = append(out, snippetFromGen(u))
	}
	return out, nil
}

// GetSnippet decodes the snippet whose ID is id, and no other (#1850). nil when
// there is none.
func (b *Backend) GetSnippet(id model.ID) (*pages.Snippet, error) {
	u, ok, err := mprread.UnitByID[*genPg.Snippet](b.reader, id)
	if err != nil || !ok {
		return nil, err
	}
	return snippetFromGen(u), nil
}

// ListSnippetHeaders lists every snippet's ID, container, name and Excluded
// flag without decoding any snippet.
func (b *Backend) ListSnippetHeaders() ([]types.DocumentHeader, error) {
	return unitHeaders[*genPg.Snippet](b)
}

// ListLayoutHeaders lists every layout's ID, container, name and Excluded flag
// without decoding any layout.
func (b *Backend) ListLayoutHeaders() ([]types.DocumentHeader, error) {
	return unitHeaders[*genPg.Layout](b)
}

func snippetFromGen(u mprread.Unit[*genPg.Snippet]) *pages.Snippet {
	s := &pages.Snippet{
		ContainerID:   u.ContainerID,
		Name:          u.Element.Name(),
		Documentation: u.Element.Documentation(),
		Excluded:      u.Element.Excluded(),
	}
	s.ID = model.ID(u.Element.ID())
	// Populate declared parameters — the page builder reads these to validate
	// and wire SNIPPETCALL argument mappings (without them every parameterised
	// snippet call writes an empty argument → CE1571 in Studio Pro).
	for _, el := range u.Element.ParametersItems() {
		sp, ok := el.(*genPg.SnippetParameter)
		if !ok {
			continue
		}
		p := &pages.SnippetParameter{Name: sp.Name()}
		p.ID = model.ID(sp.ID())
		// Entity or primitive — a snippet parameter's ParameterType is the
		// polymorphic DataTypes$DataType, the same as a page parameter's.
		// Reading only the ObjectType arm made every primitive parameter
		// read back as an untyped one (mendixlabs/mxcli#1028).
		if pt := sp.ParameterType(); pt != nil {
			if ot, ok := pt.(*genDT.ObjectType); ok {
				p.EntityName = ot.EntityQualifiedName()
			} else {
				p.Type = pt.TypeName()
			}
		}
		s.Parameters = append(s.Parameters, p)
	}
	return s
}

// ListBuildingBlocks reads Pages$BuildingBlock units. Read-only: the codec engine
// lists them (name + scalar metadata) so SHOW / DESCRIBE BUILDING BLOCK work on
// this engine too; the widget tree is read separately via GetRawUnit (shared).
func (b *Backend) ListBuildingBlocks() ([]*pages.BuildingBlock, error) {
	units, err := listUnits[*genPg.BuildingBlock](b)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.BuildingBlock, 0, len(units))
	for _, u := range units {
		bb := &pages.BuildingBlock{
			ContainerID:      u.ContainerID,
			Name:             u.Element.Name(),
			Documentation:    u.Element.Documentation(),
			DisplayName:      u.Element.DisplayName(),
			Platform:         u.Element.Platform(),
			TemplateCategory: u.Element.TemplateCategory(),
		}
		bb.ID = model.ID(u.Element.ID())
		out = append(out, bb)
	}
	return out, nil
}

func pageFromGen(p *genPg.Page, containerID model.ID) *pages.Page {
	out := &pages.Page{
		ContainerID: containerID,
		Name:        p.Name(),
		// A page's documentation is written correctly and stored correctly; this
		// engine simply did not read it back, so `mxcli lint` QUAL002 reported
		// "Page 'X' has no documentation" against a page carrying a javadoc
		// comment, and the catalog's Description column was blank for every page
		// (ako/CapTrackV4 R12). The legacy parser has always carried it, as do the
		// layout, building-block and page-template readers in this file — page and
		// snippet were the two that did not.
		Documentation: p.Documentation(),
		Excluded:      p.Excluded(),
		URL:           p.Url(),
		Title:         textElementToModel(p.Title()),
	}
	out.ID = model.ID(p.ID())
	// AllowedRoles (BY_NAME module-role references, stored under the
	// "AllowedModuleRoles" BSON key). Without these, SHOW ACCESS ON PAGE and the
	// Page section of SHOW SECURITY MATRIX report "no roles" for a restricted page
	// — a security-audit hazard (issue #722). Mirrors microflowFromGen.
	for _, qn := range p.AllowedRolesQualifiedNames() {
		out.AllowedRoles = append(out.AllowedRoles, model.ID(qn))
	}
	for _, el := range p.ParametersItems() {
		gp, ok := el.(*genPg.PageParameter)
		if !ok {
			continue
		}
		pp := &pages.PageParameter{
			ContainerID:  out.ID,
			Name:         gp.Name(),
			DefaultValue: gp.DefaultValue(),
			IsRequired:   pageParameterIsRequired(gp),
		}
		pp.ID = model.ID(gp.ID())
		// Object parameters carry an entity (rendered as the qualified entity
		// name); primitive parameters carry their DataTypes$ storage name (mapped
		// to String/Integer/… by the renderer). pageParamTypeMDL checks TypeName
		// first, so leave it empty for object params and set EntityName instead.
		if pt := gp.ParameterType(); pt != nil {
			if ot, ok := pt.(*genDT.ObjectType); ok {
				pp.EntityName = ot.EntityQualifiedName()
			} else {
				pp.TypeName = pt.TypeName()
			}
		}
		out.Parameters = append(out.Parameters, pp)
	}
	return out
}

// textElementToModel converts a gen Texts$Text element to *model.Text via its
// translation accessors. Returns nil when el is nil or not a text element, so
// callers can keep the renderer's `if Title != nil` guards. Ported from
// engalar's convert_reader.go.
func textElementToModel(el element.Element) *model.Text {
	type translationsAccessor interface {
		TranslationsItems() []element.Element
	}
	type translationAccessor interface {
		LanguageCode() string
		Text() string
	}
	ta, ok := el.(translationsAccessor)
	if !ok {
		return nil
	}
	out := &model.Text{Translations: map[string]string{}}
	for _, item := range ta.TranslationsItems() {
		tr, ok := item.(translationAccessor)
		if !ok {
			continue
		}
		if code := tr.LanguageCode(); code != "" {
			out.Translations[code] = tr.Text()
		}
	}
	return out
}

// ListLayouts reads Forms$Layout units (shallow: identity + name). The page
// builder's resolveLayout matches a layout by name + module to bind a page's
// FormCall, so the header is all it needs.
func (b *Backend) ListLayouts() ([]*pages.Layout, error) {
	units, err := listUnits[*genPg.Layout](b)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.Layout, 0, len(units))
	for _, u := range units {
		out = append(out, layoutFromGen(u))
	}
	return out, nil
}

func layoutFromGen(u mprread.Unit[*genPg.Layout]) *pages.Layout {
	l := &pages.Layout{
		ContainerID:   u.ContainerID,
		Name:          u.Element.Name(),
		Documentation: u.Element.Documentation(),
		Excluded:      u.Element.Excluded(),
		LayoutType:    layoutTypeOf(u.Element),
		Native:        layoutIsNative(u.Element),
		Class:         appearanceClassOf(u.Element),
	}
	l.ID = model.ID(u.Element.ID())
	return l
}

// appearanceClassOf reads the layout's own CSS class.
//
// Not decoration: Atlas scopes its layout chrome to `.layout-atlas` and
// variants, so a describe that dropped the class produced a script whose output
// rendered unstyled — valid, buildable, and visibly wrong.
func appearanceClassOf(l *genPg.Layout) string {
	ap, ok := l.Appearance().(*genPg.Appearance)
	if !ok || ap == nil {
		return ""
	}
	return ap.Class()
}

// layoutTypeOf reads a layout's type off its content wrapper.
//
// It is NOT on Forms$Layout, despite gen exposing Layout.LayoutType() — that
// property binds a key the document does not have, so it reads "" for every
// layout ever written. Measured: a Forms$Layout's top-level keys are $ID,
// $Type, Appearance, CanvasHeight, CanvasWidth, Content, Documentation,
// Excluded, ExportLevel, Name. The type lives one level down, on
// Forms$WebLayoutContent or Forms$NativeLayoutContent, which is exactly where
// generated/metamodel says it is.
//
// The empty read was invisible because DESCRIBE LAYOUT defaulted "" to
// "Responsive", so all 22 Atlas layouts reported Responsive — including the
// five Phone, eight Tablet, one ModalPopup and five native ones.
func layoutTypeOf(l *genPg.Layout) pages.LayoutType {
	switch c := l.Content().(type) {
	case *genPg.WebLayoutContent:
		return pages.LayoutType(c.LayoutType())
	case *genPg.NativeLayoutContent:
		return pages.LayoutType(c.LayoutType())
	}
	return ""
}

// layoutIsNative reports whether a layout is a native-mobile one, which is the
// TYPE of its content wrapper (Forms$NativeLayoutContent), not a property.
func layoutIsNative(l *genPg.Layout) bool {
	_, ok := l.Content().(*genPg.NativeLayoutContent)
	return ok
}

// GetLayout returns a single layout by ID (shallow).
// GetLayout decodes the layout whose ID is id, and no other (#1850).
func (b *Backend) GetLayout(id model.ID) (*pages.Layout, error) {
	u, ok, err := mprread.UnitByID[*genPg.Layout](b.reader, id)
	if err != nil || !ok {
		return nil, err
	}
	return layoutFromGen(u), nil
}

// LayoutPlaceholders returns the placeholder names a layout declares.
//
// Read off the raw document rather than through gen: the names are the layout's
// public API (a page binds as Module.Layout.<Name>) and both engines need the
// same answer, so the walk lives in backend.LayoutPlaceholderNames.
func (b *Backend) LayoutPlaceholders(id model.ID) ([]string, error) {
	raw, err := b.GetRawUnit(id)
	if err != nil {
		return nil, err
	}
	return backend.LayoutPlaceholderNames(raw), nil
}

// PageLayoutName returns the qualified name of the layout a page renders inside.
//
// Read off the stored document: a page's layout reference is FormCall.Form, a
// qualified name Mendix resolves on load.
func (b *Backend) PageLayoutName(id model.ID) (string, error) {
	raw, err := b.GetRawUnit(id)
	if err != nil {
		return "", err
	}
	return backend.PageLayoutNameFromRaw(raw), nil
}

// pageParameterIsRequired is a stored page parameter's IsRequired. The property
// is Mendix 11.5+ (mendixlabs/mxcli#1640): a parameter stored without it — every
// one below 11.5 — is required, and only a stored false is optional. The gen
// accessor alone reads an absent key as false, which would make every older
// parameter look optional.
func pageParameterIsRequired(gp *genPg.PageParameter) bool {
	raw := gp.Raw()
	if raw == nil {
		return gp.IsRequired()
	}
	v, err := raw.LookupErr("IsRequired")
	if err != nil {
		return true
	}
	b, ok := v.BooleanOK()
	return !ok || b
}
