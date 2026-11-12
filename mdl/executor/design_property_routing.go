// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
)

// Routing a design property written by `ALTER PAGE … SET` (ako/mxcli#515).
//
// `SET` reaches two places today: a handful of first-class properties, and the
// stored widget's pluggable property bag. An Atlas design property lives in
// neither — it is in `Appearance.DesignProperties` — so `set 'Row size' = 'Small'
// on lvOrders` dead-ended, and the only spelling that worked was ALTER STYLING.
//
// The two statements have identical scope: both take one page or snippet and one
// widget name, both mandatory (measured — #515 carries the grammar and the parse
// errors). One operation with two spellings is what "One Way to Do Each Thing"
// in `.claude/skills/design-mdl-syntax.md` rules out, so SET learns the third
// place rather than a second statement existing to reach it.
//
// # The resolution problem, and why there is no new resolver
//
// To tell a design property from a mistyped pluggable property, the key has to
// be resolved against the STORED widget's own type. That needs a `$Type` →
// theme-registry-key mapping — and `bsonTypeToDesignPropsKey` already is one. It
// had zero callers, so it had never been validated against anything; using it
// here is what makes it load-bearing, and
// TestBsonTypeAndKeywordDesignPropsKeysAgree now holds it to the keyword map
// beside it so the two cannot drift.
//
// ako/mxcli#509 deliberately did NOT do this — its ALTER STYLING key check asks
// the weaker question (does ANY widget type declare this key?) precisely to
// avoid standing up a third consumer of the concept before something needed it.
// This is that something.

// widgetStorageTyper is the optional capability the BSON page mutator offers:
// what the stored widget IS, as raw storage facts. Asserted rather than added to
// backend.PageMutator, for the reason pageProbe is: the MCP mutator has no
// pluggable path at all, and asserting keeps this routing off a backend whose
// SET support is different rather than reporting that difference as the author's
// mistake.
type widgetStorageTyper interface {
	WidgetStorageType(widgetRef string) (bsonType, widgetID string)
}

// storedWidgetDesignPropsKey returns the theme-registry key for a stored widget,
// or "" when it cannot be established.
//
// A pluggable widget is keyed in design-properties.json by its WIDGET ID, a
// native one by a name derived from its `$Type`. Returning "" rather than
// guessing is what keeps an unknown widget out of the routing entirely: it then
// takes the path it took before, which is the only safe default for a branch
// that decides where a value gets written.
func storedWidgetDesignPropsKey(mutator backend.PageMutator, widgetRef string) string {
	typer, ok := mutator.(widgetStorageTyper)
	if !ok {
		return ""
	}
	return designPropsKeyOfStored(typer, widgetRef)
}

// designPropsKeyOfStored is storedWidgetDesignPropsKey for a caller that already
// holds the capability — the pageProbe a check pass opened (ako/mxcli#613). One
// derivation, two entry points: ALTER STYLING's check and ALTER PAGE SET's
// routing resolve a stored widget to the same theme group by construction.
func designPropsKeyOfStored(typer widgetStorageTyper, widgetRef string) string {
	if typer == nil || widgetRef == "" {
		return ""
	}
	bsonType, widgetID := typer.WidgetStorageType(widgetRef)
	if widgetID != "" {
		return widgetID
	}
	if key, ok := bsonTypeToDesignPropsKey[bsonType]; ok {
		return key
	}
	return ""
}

// documentPlatformReporter is the optional half of a PageMutator that knows the
// platform of the document it edits.
type documentPlatformReporter interface {
	DocumentPlatform() (native bool, layout string)
}

// documentTheme is the theme registry the design properties of a mutator's
// document resolve against: the native definitions for a native page, snippet
// or layout, the web ones otherwise (ako/mxcli#1679). A page is native when its
// layout is.
func documentTheme(ctx *ExecContext, mutator backend.PageMutator) *ThemeRegistry {
	return documentThemeOf(ctx, ctx.GetThemeRegistry(), mutator)
}

// documentThemeOf is documentTheme for a registry the caller already holds and
// a document opened as a pageProbe rather than a backend.PageMutator — ALTER
// STYLING's stored-type check (ako/mxcli#613).
func documentThemeOf(ctx *ExecContext, reg *ThemeRegistry, mutator any) *ThemeRegistry {
	r, ok := mutator.(documentPlatformReporter)
	if reg == nil || !ok {
		return reg
	}
	native, layout := r.DocumentPlatform()
	if !native && layout != "" {
		native = layoutNamedIsNative(ctx, layout)
	}
	return reg.ForPlatform(native)
}

// storedSnippetIsNative reports whether the stored snippet's Type is Native.
func storedSnippetIsNative(ctx *ExecContext, id model.ID) bool {
	raw, err := ctx.Backend.GetRawUnitBytes(id)
	if err != nil {
		return false
	}
	t, ok := bson.Raw(raw).Lookup("Type").StringValueOK()
	return ok && t == "Native"
}

// layoutNamedIsNative reports whether the layout with this qualified name is a
// native one; false when there is none.
func layoutNamedIsNative(ctx *ExecContext, qualifiedName string) bool {
	layouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return false
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return false
	}
	for _, l := range layouts {
		if h.GetModuleName(h.FindModuleID(l.ContainerID))+"."+l.Name == qualifiedName {
			return l.Native
		}
	}
	return false
}

// designPropertyForStoredWidget returns the theme's declaration of key for the
// stored widget, or nil when this is not a design property of it.
//
// nil is the signal to leave `SET` on the path it was already on, so a mistyped
// pluggable key still reaches the pluggable setter and still gets the error that
// names the widget's own declared keys. Routing on "the theme says nothing, so
// it must be a design property" is the inverse, and is how a typo becomes a
// silently-written design property — the "no silent side effects on typos" line
// in CLAUDE.md's checklist.
func designPropertyForStoredWidget(theme *ThemeRegistry, mutator backend.PageMutator, widgetRef, key string) *ThemeProperty {
	if theme == nil || key == "" {
		return nil
	}
	dpKey := storedWidgetDesignPropsKey(mutator, widgetRef)
	if widgetRef == "" && mutator != nil && mutator.ContainerType() == backend.ContainerPage {
		// A page-level SET: the page's own design properties, which the theme
		// declares under "Page" (ako/mxcli#1678).
		dpKey = pageThemeKey
	}
	if dpKey == "" {
		return nil
	}
	for _, p := range theme.GetPropertiesForWidget(dpKey) {
		if strings.EqualFold(p.Name, key) {
			found := p
			return &found
		}
	}
	return nil
}

// designPropertyAssignment converts an MDL value into the (valueType, option)
// pair SetDesignProperty takes, or refuses it.
//
// Refusals here are the two shapes ALTER PAGE SET cannot carry, both already
// refused on the paths that can carry them, so the vocabulary does not fork:
//
//   - a flat value on a multi-select property, which Mendix stores as a compound
//     of one entry per selection and mxbuild rejects as CE6084 (ako/mxcli#511)
//   - a compound value, which a SET assignment has no shape for — it holds one
//     scalar, exactly as a StylingAssignment does
func designPropertyAssignment(p *ThemeProperty, value any) (valueType, option string, err error) {
	str := fmt.Sprintf("%v", value)
	switch v := value.(type) {
	case bool:
		if v {
			str = "on"
		} else {
			str = "off"
		}
	case string:
		str = v
	}

	if p.MultiSelect {
		return "", "", fmt.Errorf(
			"design property %q takes a SET of options, not one value — `set` carries a single "+
				"value, so write it inline instead: `DesignProperties: ('%s': ('%s': on))` on the "+
				"widget, in CREATE PAGE or an ALTER PAGE REPLACE. A single value is stored as an "+
				"Option and mxbuild refuses it with CE6084",
			p.Name, p.Name, firstOptionName(p, str))
	}

	switch strings.ToLower(str) {
	case "on", "true":
		return "toggle", "", nil
	case "off", "false":
		// Handled by the caller as a removal: Mendix stores a toggle's OFF state
		// as the absence of the entry, not as a stored false.
		return "", "", nil
	}
	return resolveDesignPropertyValueType(p.Name, str, []ThemeProperty{*p}), str, nil
}

// applyDesignPropertySet writes one design property onto a stored widget.
//
// OFF is a removal, not a stored false: Mendix represents a toggle's off state
// as the absence of the entry, which is what `alter styling … = off` already
// does (applyStylingMutator). Writing a "false" toggle instead would leave a
// DesignProperties entry Studio Pro shows as on.
func applyDesignPropertySet(mutator backend.PageMutator, target ast.WidgetRef, p *ThemeProperty, value any) error {
	valueType, option, err := designPropertyAssignment(p, value)
	if err != nil {
		return err
	}
	if valueType == "" {
		return mutator.RemoveDesignProperty(target.Widget, p.Name)
	}
	return mutator.SetDesignProperty(target.Widget, p.Name, valueType, option)
}
