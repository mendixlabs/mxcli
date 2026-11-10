// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
)

// Site is one translatable text together with where it sits: the nearest
// enclosing storage object and the property the text hangs off.
//
// It exists so a consumer can name a text's location without knowing the
// document type. The catalog's string index used to reach five sites because
// each one was hand-written against a typed reader (page titles, enum captions,
// three microflow activities); the walk below reaches every site in the project
// with no per-type code — 17 distinct ones in a stock 11.13 app, and whatever a
// future Mendix version adds, for free.
type Site struct {
	// OwnerType is the $Type of the nearest enclosing storage object, e.g.
	// "Forms$ActionButton". It is the unit's own $Type for a text on the root.
	OwnerType string
	// Property is the key the Texts$Text hangs off, e.g. "Caption".
	Property string
	// ElementID is the owner's $ID as a UUID string, empty when it has none.
	//
	// Load-bearing for grouping: several sibling elements of one type carry the
	// same OwnerType and Property, so a consumer that groups without this folds
	// an enumeration's twelve values into one and calls the set complete as soon
	// as any one value is translated.
	ElementID string
	// OwnerName is the owner's Name ("tabPage2"), empty when it has none.
	OwnerName string
	// WidgetID is the $ID of the nearest named Forms$ or CustomWidgets$
	// element around the text below the document root: the widget a caption,
	// tooltip or confirmation text belongs to. ElementID is the text's direct
	// owner, which for a button's caption is its Forms$ClientTemplate and for
	// its confirmation the Forms$ConfirmationInfo, neither of them the button
	// (ako/mxcli#1303). Empty for text that is not inside a widget, such as a
	// page's own title.
	WidgetID string
	// Targets is language code → text, exactly as stored. A language present
	// with an empty string is a text that exists but is not translated yet.
	Targets map[string]string
}

// SitesIn returns every translatable text in a decoded document, in document
// order. Order is stable so a caller writing rows gets a deterministic result.
func SitesIn(doc bson.D) []Site {
	var out []Site
	var walk func(v any, ownerType, ownerID, ownerName, prop, widgetID string, root bool)
	walk = func(v any, ownerType, ownerID, ownerName, prop, widgetID string, root bool) {
		switch n := v.(type) {
		case bson.D:
			if ty, _ := lookup(n, "$Type").(string); ty == "Texts$Text" {
				out = append(out, Site{
					OwnerType: ownerType,
					Property:  prop,
					ElementID: ownerID,
					OwnerName: ownerName,
					WidgetID:  widgetID,
					Targets:   translationsOf(n),
				})
				return
			}
			// A node with its own $Type becomes the owner for everything below
			// it. A node without one (an anonymous sub-document) leaves the
			// owner as it was, so the text is still attributed to a real
			// element rather than to nothing.
			nt, nid, nname, nwidget := ownerType, ownerID, ownerName, widgetID
			if ty, _ := lookup(n, "$Type").(string); ty != "" {
				nt = ty
				nid = elementIDOf(n)
				nname, _ = lookup(n, "Name").(string)
				if !root && nname != "" && (strings.HasPrefix(ty, "Forms$") || strings.HasPrefix(ty, "CustomWidgets$")) {
					nwidget = nid
				}
			}
			for _, e := range n {
				walk(e.Value, nt, nid, nname, e.Key, nwidget, false)
			}
		case bson.A:
			// An array element inherits the property its array hangs off, so a
			// text inside `Widgets` is not reported as living at "Widgets".
			for _, e := range n {
				walk(e, ownerType, ownerID, ownerName, prop, widgetID, false)
			}
		}
	}
	walk(doc, "", "", "", "", "", true)
	return out
}

// SitesInUnit is SitesIn over a unit's raw stored bytes.
func SitesInUnit(raw []byte) ([]Site, error) {
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return SitesIn(doc), nil
}

// elementIDOf reads a storage object's $ID as a UUID string. Real documents
// store it as a 16-byte binary in .NET field order; tests and a few synthetic
// documents use a plain string, which is passed through unchanged.
func elementIDOf(d bson.D) string {
	switch v := lookup(d, "$ID").(type) {
	case bson.Binary:
		return codec.BinaryToUUID(v.Data)
	case string:
		return v
	default:
		return ""
	}
}
