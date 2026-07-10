// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDT "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatypes"
	genPg "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/pages"
	_ "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/texts" // register Texts$Text so page titles decode concretely
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// gen→pages read adapter. SHOW PAGES needs only the page header — name, module,
// excluded, title, URL, parameter count — not the widget tree, so this stays
// shallow. Widget-tree conversion (DESCRIBE PAGE / ALTER) is a later phase.

func (b *Backend) ListPages() ([]*pages.Page, error) {
	units, err := mprread.ListUnitsWithContainer[*genPg.Page](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.Page, 0, len(units))
	for _, u := range units {
		out = append(out, pageFromGen(u.Element, u.ContainerID))
	}
	// Legacy ListPages calls listUnitsByType("Forms$Page"), which is
	// prefix-matched and therefore also sweeps in Forms$PageTemplate units.
	// Replicate that here so SHOW PAGES matches legacy. (The modelsdk reader
	// is strict-typed, so we add templates explicitly.)
	tmpls, err := mprread.ListUnitsWithContainer[*genPg.PageTemplate](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range tmpls {
		out = append(out, pageTemplateAsPage(u.Element, u.ContainerID))
	}
	return out, nil
}

// pageTemplateAsPage adapts a page template into the Page shape SHOW PAGES
// expects. Templates carry only name + excluded; title/url/params stay zero,
// matching legacy's prefix-match behaviour.
func pageTemplateAsPage(pt *genPg.PageTemplate, containerID model.ID) *pages.Page {
	out := &pages.Page{ContainerID: containerID, Name: pt.Name(), Excluded: pt.Excluded()}
	out.ID = model.ID(pt.ID())
	return out
}

func (b *Backend) GetPage(id model.ID) (*pages.Page, error) {
	units, err := mprread.ListUnitsWithContainer[*genPg.Page](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return pageFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

// ListSnippets reads snippet units (count + identity is what SHOW MODULES needs;
// the widget tree is not converted).
func (b *Backend) ListSnippets() ([]*pages.Snippet, error) {
	units, err := mprread.ListUnitsWithContainer[*genPg.Snippet](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.Snippet, 0, len(units))
	for _, u := range units {
		s := &pages.Snippet{ContainerID: u.ContainerID, Name: u.Element.Name()}
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
			if ot, ok := sp.ParameterType().(*genDT.ObjectType); ok {
				p.EntityName = ot.EntityQualifiedName()
			}
			s.Parameters = append(s.Parameters, p)
		}
		out = append(out, s)
	}
	return out, nil
}

func pageFromGen(p *genPg.Page, containerID model.ID) *pages.Page {
	out := &pages.Page{
		ContainerID: containerID,
		Name:        p.Name(),
		Excluded:    p.Excluded(),
		URL:         p.Url(),
		Title:       textElementToModel(p.Title()),
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
			IsRequired:   gp.IsRequired(),
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
	units, err := mprread.ListUnitsWithContainer[*genPg.Layout](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.Layout, 0, len(units))
	for _, u := range units {
		l := &pages.Layout{ContainerID: u.ContainerID, Name: u.Element.Name(), Documentation: u.Element.Documentation()}
		l.ID = model.ID(u.Element.ID())
		out = append(out, l)
	}
	return out, nil
}

// GetLayout returns a single layout by ID (shallow).
func (b *Backend) GetLayout(id model.ID) (*pages.Layout, error) {
	layouts, err := b.ListLayouts()
	if err != nil {
		return nil, err
	}
	for _, l := range layouts {
		if l.ID == id {
			return l, nil
		}
	}
	return nil, nil
}
