// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// The native Image (Native Mobile Resources 12.6.0) hides the image-source
// properties it is not using with a hide call whose list is computed by a
// switch, inside an immediately-invoked function:
//
//	hidePropertiesIn(t, v, function(s){switch(s){
//	    case "image":    return L.filter(p => "imageObject" !== p);
//	    case "imageUrl": return L.filter(p => "imageUrl"    !== p);
//	    case "icon":     return L.filter(p => "imageIcon"   !== p);
//	    default:         return L }}(v.datasource))
//
// The extractor read no list there and skipped the call, so `imageUrl` — a
// text template, hidden under the default source `image` — was written with
// a template where mx update-widgets writes none: CE0463 on every native
// image mxcli created (mx check 11.13.0).
//
// The source below is the widget's own getProperties, minified as shipped,
// cut after the call that matters.
const nativeImageGetProperties = `var dataSourceProperties=["imageObject","imageUrl","imageIcon"];` +
	`exports.getProperties=function(values,defaultProperties){var _values$imageIcon,_values$imageIcon2;` +
	`return distExports.hidePropertiesIn(defaultProperties,values,function(sourceProperty){switch(sourceProperty){` +
	`case"image":return dataSourceProperties.filter(function(prop){return"imageObject"!==prop});` +
	`case"imageUrl":return dataSourceProperties.filter(function(prop){return"imageUrl"!==prop});` +
	`case"icon":return dataSourceProperties.filter(function(prop){return"imageIcon"!==prop});` +
	`default:return dataSourceProperties}}(values.datasource)),` +
	`"image"!==values.datasource&&distExports.hidePropertyIn(defaultProperties,values,"defaultImageDynamic")}`

func TestExtractVisibility_SwitchComputedHideList(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(nativeImageGetProperties)
	hidden := func(key string, values map[string]string) bool {
		for _, r := range rules {
			if r.PropertyKey != key || r.ListPropertyKey != "" {
				continue
			}
			fires, _ := r.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
				v, ok := values[c.PropertyKey]
				return v, ok
			})
			if fires {
				return true
			}
		}
		return false
	}
	for _, tc := range []struct {
		source  string
		visible string
		hidden  []string
	}{
		{"image", "imageObject", []string{"imageUrl", "imageIcon"}},
		{"imageUrl", "imageUrl", []string{"imageObject", "imageIcon"}},
		{"icon", "imageIcon", []string{"imageObject", "imageUrl"}},
	} {
		values := map[string]string{"datasource": tc.source}
		if hidden(tc.visible, values) {
			t.Errorf("datasource=%s: %s is the property the source uses, but a rule hides it", tc.source, tc.visible)
		}
		for _, k := range tc.hidden {
			if !hidden(k, values) {
				t.Errorf("datasource=%s: %s is hidden by the editor, but no rule says so (rules: %+v)", tc.source, k, rules)
			}
		}
	}
	// Control: the plain guarded hide beside it is still read.
	if !hidden("defaultImageDynamic", map[string]string{"datasource": "icon"}) {
		t.Error("the guarded hidePropertyIn next to the switch is no longer read")
	}
}

// A computed list whose shape is not the one above is not guessed at: a
// default that is not the full list, or a case that keeps two members, could
// hide a property the editor shows.
func TestExtractVisibility_SwitchComputedHideList_OtherShapesNotLifted(t *testing.T) {
	for name, js := range map[string]string{
		"default is not the list": `var L=["a","b"];exports.getProperties=function(v,t){return x.hidePropertiesIn(t,v,function(s){switch(s){` +
			`case"x":return L.filter(function(p){return"a"!==p});default:return[]}}(v.mode))}`,
		"under a guard": `var L=["a","b"];exports.getProperties=function(v,t){return v.on&&x.hidePropertiesIn(t,v,function(s){switch(s){` +
			`case"x":return L.filter(function(p){return"a"!==p});default:return L}}(v.mode))}`,
	} {
		rules, _ := extractVisibilityRulesFromJS(js)
		for _, r := range rules {
			if r.PropertyKey == "a" || r.PropertyKey == "b" {
				t.Errorf("%s: lifted %+v", name, r)
			}
		}
	}
}
