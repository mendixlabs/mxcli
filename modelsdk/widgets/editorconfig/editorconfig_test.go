// SPDX-License-Identifier: Apache-2.0

package editorconfig

import (
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// A widget's own hide helper, not the toolkit's — the shape the static reader
// cannot see (ChessClock, ako/mxcli#2507).
const ownHelper = `"use strict";
function t(groups, keys) {
  groups.forEach(function (g) {
    g.properties && (g.properties = g.properties.filter(function (p) { return keys.indexOf(p.key) < 0 }));
    g.propertyGroups && t(g.propertyGroups, keys);
  });
}
exports.getProperties = function (values, props) {
  values.isCorrespondence || t(props, ["textDays", "textHours"]);
  "none" === values.ownColor && t(props, ["tickSound"]);
  return props;
};`

var clockProps = []mpk.PropertyDef{
	{Key: "isCorrespondence", Type: "boolean", DefaultValue: "false", Category: "General::Mode"},
	{Key: "ownColor", Type: "enumeration", DefaultValue: "white", Category: "General::Mode"},
	{Key: "textDays", Type: "textTemplate", Category: "Texts::Labels"},
	{Key: "textHours", Type: "textTemplate", Category: "Texts::Labels"},
	{Key: "tickSound", Type: "boolean", DefaultValue: "true", Category: "General::Sound"},
}

func TestHiddenRunsTheWidgetsOwnLogic(t *testing.T) {
	ev, err := New(ownHelper, clockProps)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ev.Hidden(nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "textDays,textHours"; strings.Join(SortedKeys(got), ",") != want {
		t.Errorf("defaults hide %v, want %s", SortedKeys(got), want)
	}
	// Keys are matched case-insensitively and booleans arrive as booleans: the
	// string "true" must reach the script as true, or `values.isCorrespondence ||`
	// would still hide the texts.
	got, err = ev.Hidden(map[string]string{"iscorrespondence": "true", "ownColor": "none"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "tickSound"; strings.Join(SortedKeys(got), ",") != want {
		t.Errorf("correspondence + none hides %v, want %s", SortedKeys(got), want)
	}
}

// A hide that depends on the target is not one mxbuild enforces: TreeNode hides
// advancedMode unless the target is "web", and animate (with advancedMode off)
// when it is — and on 11.16.1 both `advancedMode: true` and `animate: false`
// build with 0 errors (ako/mxcli#2549). Only what both targets hide counts.
func TestHiddenRequiresBothTargets(t *testing.T) {
	js := `exports.getProperties = function (values, props, target) {
  var drop = target === "web" ? ["animate", "always"] : ["advancedMode", "always"];
  props.forEach(function (g) { g.propertyGroups.forEach(function (pg) {
    pg.properties = pg.properties.filter(function (p) { return drop.indexOf(p.key) < 0 }) }) });
  return props;
};`
	ev, err := New(js, []mpk.PropertyDef{{Key: "animate", Type: "boolean"}, {Key: "advancedMode", Type: "boolean"}, {Key: "always", Type: "boolean"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ev.Hidden(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got["advancedMode"] || got["animate"] || !got["always"] {
		t.Errorf("hidden %v, want only `always`, the property both targets hide", SortedKeys(got))
	}
}

func TestNoGetPropertiesHidesNothing(t *testing.T) {
	ev, err := New(`exports.getPreview = function () { return {} };`, clockProps)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ev.Hidden(nil)
	if err != nil || len(got) != 0 {
		t.Errorf("hidden %v, err %v; want none", got, err)
	}
}

// Third-party code: a loop that never returns is stopped, a throw is an error,
// and require is refused — none of them may hang or crash the caller.
func TestHostileBundlesFailSafely(t *testing.T) {
	for name, js := range map[string]string{
		"loop":    `exports.getProperties = function (v, p) { for (;;) {} };`,
		"throw":   `exports.getProperties = function (v, p) { throw new Error("boom") };`,
		"require": `var fs = require("fs"); exports.getProperties = function (v, p) { return p };`,
	} {
		t.Run(name, func(t *testing.T) {
			ev, err := New(js, clockProps)
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			if _, err := ev.Hidden(nil); err == nil {
				t.Error("want an error")
			}
			if d := time.Since(start); d > 3*Timeout {
				t.Errorf("took %v", d)
			}
		})
	}
}

func TestHiddenIsCachedPerConfiguration(t *testing.T) {
	ev, err := New(ownHelper, clockProps)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := ev.Hidden(map[string]string{"isCorrespondence": "false"})
	b, _ := ev.Hidden(nil) // the same configuration: false is the default
	if len(ev.cache) != 1 || len(a) != len(b) {
		t.Errorf("cache holds %d entries, want 1 for one configuration", len(ev.cache))
	}
}
