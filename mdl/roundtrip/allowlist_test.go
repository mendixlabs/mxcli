// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

// knownFailures is the allowlist of documents in testdata/pedapp that break a
// round-trip law today. It may only SHRINK.
//
// The test fails in both directions:
//   - a document that breaks a law it is not listed for is a regression;
//   - a document that no longer breaks a law it IS listed for must have that
//     law struck from its entry, and the entry deleted when none is left. A fix
//     that lands without shrinking this list fails the build, so whichever of a
//     fix and this harness merges second has to update it.
//
// Keys are the describe target: "<describe keyword> <qualified name>". Every
// entry names the issue that tracks it. Documents missing from this list
// round-trip today (enumerations, constants, JSON structures, mappings, image
// collections) or are refused as a whole with nothing written (layouts in a
// marketplace module, see refusals).
//
// The initial list was measured on 2026-09-26: 42 of 223 documents keep both
// laws. #704 and #705 were being fixed in parallel; their entries are here
// so that whoever merges second strikes them. #721 L2 (client-action settings)
// struck the two password pages. #721 L4 (a pluggable widget rebuilt from its
// template instead of kept as stored) struck six pages and a snippet whose data
// grids, images and language selector were its only loss.
var knownFailures = map[string]knownFailure{
	// Plain `create` where `create or modify` is needed (#705 item 5).

	// #721 G: describe output that does not parse.
	"building block Atlas_Web_Content.Alert":                        {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.AlertIcon":                    {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.AlertIcon_WithAction":         {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Alert_WithAction":             {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Breadcrumb":                   {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Breadcrumb_Underline":         {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Card":                         {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Card_Action":                  {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Card_ActionWithImage":         {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Card_Background":              {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Card_WithImage":               {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Form_Horizontal":              {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Form_Horizontal_WithAction":   {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Form_Horizontal_WithTitle":    {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Form_Vertical":                {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Form_Vertical_WithAction":     {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Form_Vertical_WithTitle":      {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Heroheader":                   {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Heroheader_Background":        {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Heroheader_WithAction":        {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.ListItem_DoubleLine":          {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.ListItem_SingleLine":          {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.ListItem_WithImage":           {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.List_Cards":                   {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.List_WithImage":               {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Master_Detail":                {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Pageheader":                   {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.PageheaderImage":              {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.PageheaderImage_WithBack":     {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.PageheaderImage_WithControls": {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Pageheader_WithBack":          {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Pageheader_WithControls":      {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Pageheader_WithSearch":        {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Timeline":                     {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Timeline_WithImage":           {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Wizard_Arrow":                 {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Wizard_Arrow_Step":            {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Wizard_Circle":                {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block Atlas_Web_Content.Wizard_Circle_Step":           {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},
	"building block FeedbackModule.FeedbackWidget":                  {laws: []law{lawParse}, issue: "#721", why: "read-only, but describe prints a widget body that does not parse (#721 G)"},

	// #721 E: menus — struck by ako/mxcli#980 (the menu item writer's missing
	// AlternativeText and Items).

	// Pages: #705 item 1 plus #721 C.
}
