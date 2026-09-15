// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The last widgets that sent a user to the legacy engine, now deleted
// (docs/plans/2026-09-14-retire-legacy-engine.md). Closing these was one of its
// preconditions, which is why the file is named for them.
//
// Every "not yet supported by the modelsdk engine" message was a reason the
// legacy engine had to stay shipped and tested. Measured against sdk/pages, the
// reachable set was five, not the twenty-three a name-based scan suggests:
// nineteen widget structs are never constructed by the executor at all, and two
// of the three remaining data-source / client-action gaps (EntityPathSource,
// ShowHomePageClientAction) are written by NEITHER engine and built by nothing,
// so the fallback message named a path legacy could not take either.
//
// The five that were real, each confirmed by running `mxcli exec` on a page that
// uses them:
//
//	statictext   -> pages.Text          Forms$Text
//	dropdown     -> pages.DropDown      Forms$DropDown
//	staticimage  -> pages.StaticImage   Forms$StaticImageViewer
//	dynamicimage -> pages.DynamicImage  Forms$ImageViewer
//	             -> pages.NanoflowSource Forms$NanoflowSource   (list/grid data source)
//
// None was covered by the doctype gate: no example in mdl-examples/ or in any
// skill uses those keywords, which is why the suite ran green on modelsdk while
// four plain widget keywords failed.
//
// The first of the five turned out not to be a gap at all. Closing it and
// running the result through `mx check` showed that BOTH engines wrote a project
// that could not be LOADED — Mendix has no Forms$Text — so `statictext` is now
// refused at build and check time (MDL-WIDGET29,
// mdl/executor/validate_widget_retired.go). Nothing constructs pages.Text any
// more, so there is no writer for it here either — an old project that carries
// one keeps it because ALTER PAGE mutates the stored gen document rather than
// rebuilding from the semantic model.
//
// The bar here is the Studio Pro reference, NOT parity with sdk/mpr. That was
// the starting assumption and measuring it overturned it: ako/TestApp carries
// three Studio-Pro-authored Forms$StaticImageViewer widgets (FeedbackModule),
// and legacy disagrees with all three in two ways — it omits AlternativeText
// (which generated/metamodel declares non-optional on both image types) and it
// writes BSON null for the unset Image. Across the whole corpus a by-name
// reference is written as an EMPTY STRING and never as null: 0 nulls against
// 4,400+ empty strings over 40 (type, property) pairs. Legacy's dynamic image is
// worse still — its hand-rolled AlternativeText carries a "FallbackValue" string
// that Forms$ClientTemplate does not have (metamodel: Fallback / Parameters /
// Template), which is the invent-a-key defect from CLAUDE.md, invisible to
// mxbuild. sdk/mpr was corrected to match rather than pinned as ground truth, so
// the two test files can assert one shape for both engines
// (widget_write_legacy_gaps_test.go and sdk/mpr/writer_widgets_image_test.go).
//
// Two divergences are deliberately left alone, because measuring them showed
// they are project-wide and predate this file — fixing them here would hide
// them: modelsdk writes TabIndex/Width/Height as int32 where Studio Pro and
// legacy write int64 (measured on 265 Forms$DivContainer widgets, not just
// these), and modelsdk omits an empty Widgets list where legacy writes [3].

// dropDownToGen builds a Forms$DropDown (an enumeration/association selector).
func dropDownToGen(dd *pages.DropDown) (element.Element, error) {
	g := genPg.NewDropDown()
	applyWidgetBase(g, &dd.BaseWidget)
	g.SetAriaRequired(false)
	if ref := attributeRefToGen(dd.AttributePath); ref != nil {
		g.SetAttributeRef(ref)
	}
	g.SetEditable(pages.WidgetEditability(&dd.BaseWidget))
	// An empty Texts$Text, not a null: the property is the caption of the blank
	// option and Studio Pro always writes the holder.
	g.SetEmptyOptionCaption(emptyTranslatedText())
	if dd.Label != "" {
		g.SetLabelTemplate(textAsClientTemplate(textFromString(dd.Label)))
	}
	onChange, err := clientActionToGen(dd.OnChangeAction)
	if err != nil {
		return nil, err
	}
	g.SetOnChangeAction(onChange)
	g.SetOnEnterAction(noActionGen())
	g.SetOnLeaveAction(noActionGen())
	g.SetReadOnlyStyle("Inherit")
	g.SetValidation(widgetValidationToGen())
	return g, nil
}

// staticImageToGen builds a Forms$StaticImageViewer.
//
// Deprecated in the Mendix 11 React client (CE0582) — `image` routes to the
// pluggable widget instead — but `staticimage` is still a keyword the executor
// dispatches, so the writer has to answer for it. Unlike `statictext` the TYPE
// exists: the project loads, and CE0582 is Mendix's own advice rather than a
// defect, so refusing it would be over-reach.
func staticImageToGen(img *pages.StaticImage) (element.Element, error) {
	g := genPg.NewStaticImageViewer()
	applyWidgetBase(g, &img.BaseWidget)
	g.SetAlternativeText(emptyClientTemplate())
	click, err := clientActionToGen(img.OnClickAction)
	if err != nil {
		return nil, err
	}
	g.SetClickAction(click)
	// MDL cannot name an image (the builder never fills ImageID), so this is
	// always the unset value — and unset is "", not null; see the header.
	g.SetImageQualifiedName("")
	g.SetHeight(int32(img.Height))
	g.SetHeightUnit("Auto")
	g.SetResponsive(img.Responsive)
	g.SetWidth(int32(img.Width))
	g.SetWidthUnit("Auto")
	return g, nil
}

// dynamicImageToGen builds a Forms$ImageViewer — gen calls the type
// DynamicImageViewer, and its storage name is the one that matters.
func dynamicImageToGen(img *pages.DynamicImage) (element.Element, error) {
	g := genPg.NewDynamicImageViewer()
	applyWidgetBase(g, &img.BaseWidget)
	g.SetAlternativeText(emptyClientTemplate())
	click, err := clientActionToGen(img.OnClickAction)
	if err != nil {
		return nil, err
	}
	g.SetClickAction(click)
	g.SetDataSource(imageViewerSourceToGen())
	g.SetDefaultImageQualifiedName("")
	g.SetHeight(int32(img.Height))
	g.SetHeightUnit("Auto")
	g.SetOnClickEnlarge(false)
	g.SetResponsive(img.Responsive)
	g.SetShowAsThumbnail(false)
	g.SetWidth(int32(img.Width))
	g.SetWidthUnit("Auto")
	return g, nil
}

// imageViewerSourceToGen builds the empty Forms$ImageViewerSource a dynamic
// image carries when no entity path has been set.
func imageViewerSourceToGen() element.Element {
	src := genPg.NewImageViewerSource()
	assignID(src)
	return src
}

// nanoflowSourceToGen builds a Forms$NanoflowSource — a list widget's "nanoflow"
// data source.
//
// Built raw, and this one is a judgement rather than a limitation: gen's
// NanoflowSource offers ForceFullObjects and NanoflowQualifiedName, binding the
// nanoflow name DIRECTLY on the source, while Studio Pro nests it inside a
// Forms$NanoflowSettings child alongside ParameterMappings — which is what
// sdk/mpr writes. Writing gen's shape would put the name in a key Studio Pro
// does not read there, the same class of defect as the storage-name overrides
// (CLAUDE.md). Legacy's shape is the one with a working project behind it.
func nanoflowSourceToGen(d *pages.NanoflowSource) element.Element {
	g := newElem("Forms$NanoflowSource", string(d.ID))
	settings := newElem("Forms$NanoflowSettings", "")
	addStr(settings, "Nanoflow", d.Nanoflow)
	addEmptyTypedList(settings, "ParameterMappings", 3)
	addPart(g, "NanoflowSettings", settings)
	return g
}

// emptyClientTemplate is the Forms$ClientTemplate an image's AlternativeText
// carries when no alt text has been set: an empty Template, an empty Fallback
// and no parameters. Matches the three Studio-Pro-authored StaticImageViewer
// widgets in ako/TestApp element for element.
func emptyClientTemplate() element.Element {
	return textAsClientTemplate(nil)
}

// emptyTranslatedText is a Texts$Text with no translations — the holder Studio
// Pro writes for an unset caption.
func emptyTranslatedText() element.Element {
	holder := newElem("Texts$Text", "")
	addEmptyTypedList(holder, "Items", 3)
	return holder
}

func init() {
	// Every widget here needs its null slots and its list marker registered, or
	// it serializes with keys missing and under the wrong array version. Both
	// were caught by diffing the two engines' output for one page carrying all
	// four widgets — neither shows up as a build error.
	//
	// Marker 2, not the codec's default of 3: a container's Widgets list takes
	// its marker from the CHILD type, so an unregistered widget silently changed
	// the marker of the list it sits in.
	codec.RegisterTypeDefaults("Forms$DropDown", codec.TypeDefaults{
		NullFields: []string{
			"AttributeRef", "ScreenReaderLabel", "SourceVariable", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
			"NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$DropDown", 2)

	// Image / DefaultImage are by-name references and are set to "" above, not
	// listed here: an unset one is an empty string in every Studio Pro document
	// measured. Only the two child slots are genuinely null.
	codec.RegisterTypeDefaults("Forms$StaticImageViewer", codec.TypeDefaults{
		NullFields: []string{
			"ConditionalVisibilitySettings", "NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$StaticImageViewer", 2)

	codec.RegisterTypeDefaults("Forms$ImageViewer", codec.TypeDefaults{
		NullFields: []string{
			"ConditionalVisibilitySettings", "NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$ImageViewer", 2)

	// EntityRef is the attribute path the image comes from — null when unbound.
	codec.RegisterTypeDefaults("Forms$ImageViewerSource", codec.TypeDefaults{
		NullFields: []string{"EntityRef"},
	})
}
