// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A pluggable widget's own property key can share its name with a core widget
// property: PlatformDesignSystem's Button declares `caption`, `buttonStyle` and
// `icon`, its Form label `label` (ako/mxcli#2180, mendixlabs/mxcli#1833). The
// CustomWidget stores no core property of those names, so on such a widget the
// name can only mean the widget's own key — which check, create page and ALTER
// all have to agree on. The names are pages.PluggableKeyShadowsCoreName's; the
// ALTER side asks the same question of the stored widget
// (pagemutator.pluggableDeclaresOwnKey).

// defOwnsCoreName reports whether def declares name — a core property name a
// pluggable key takes over — as a property of its own that the engine writes.
// Only a mapped key counts: a key of a kind the engine does not write (an
// Icon-typed `icon`, as on the Star rating) is in KnownProperties, and the core
// reading stays, as before.
func defOwnsCoreName(def *WidgetDefinition, name string) bool {
	if def == nil || !pages.PluggableKeyShadowsCoreName(name) {
		return false
	}
	sets := [][]PropertyMapping{def.PropertyMappings}
	for _, m := range def.Modes {
		sets = append(sets, m.PropertyMappings)
	}
	for _, set := range sets {
		for _, m := range set {
			if strings.EqualFold(m.PropertyKey, name) {
				return true
			}
		}
	}
	return false
}

// templateOwnsCoreName is defOwnsCoreName asked of the template being built:
// whether its PropertyTypes declare name as a key of the widget's own, not as a
// system property (a combo box's `Label`, ValueType System, is the
// widget-level label).
func templateOwnsCoreName(propertyTypeIDs map[string]pages.PropertyTypeIDEntry, name string) bool {
	if !pages.PluggableKeyShadowsCoreName(name) {
		return false
	}
	for key, entry := range propertyTypeIDs {
		if strings.EqualFold(key, name) && entry.ValueType != "System" {
			return true
		}
	}
	return false
}
