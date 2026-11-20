// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/backend/widgetobj"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// SetWidgetTemplateParameters replaces the parameters of a built-in widget's
// text template — a button's caption (CaptionParams), a dynamic text's content
// (ContentParams) — with params, resolved by the caller as CREATE PAGE resolves
// them (ako/mxcli#1966). The template's text is left as it is: a statement that
// changes both sets Caption / Content too, and properties apply in name order,
// so the text is written first.
func (m *Mutator) SetWidgetTemplateParameters(widgetRef, prop string, params []*pages.ClientTemplateParameter) error {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil {
		return m.widgetNotFoundError(widgetRef)
	}
	if len(result.colPropKeys) > 0 {
		return fmt.Errorf("%s on a data grid column is not supported by alter page set: use `replace` on the column", prop)
	}
	var tmpl bson.D
	switch strings.ToLower(prop) {
	case "captionparams":
		tmpl = bsonnav.DGetDoc(result.widget, "CaptionTemplate")
	case "contentparams":
		tmpl = bsonnav.DGetDoc(result.widget, "Content")
		if tmpl != nil && bsonnav.DGetString(tmpl, "$Type") != "Forms$ClientTemplate" {
			tmpl = nil
		}
	}
	if tmpl == nil {
		return fmt.Errorf("a %s has no %s: CaptionParams are a button's caption parameters, "+
			"ContentParams a dynamic text's", widgetTypeLabel(bsonnav.DGetString(result.widget, "$Type")), prop)
	}
	list := bson.A{int32(2)}
	for _, p := range params {
		list = append(list, widgetobj.SerializeColumnClientTemplateParameter(p))
	}
	bsonnav.DSet(tmpl, "Parameters", list)
	return nil
}
