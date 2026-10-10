// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	genNav "github.com/mendixlabs/mxcli/modelsdk/gen/navigation"
	bsonv1 "go.mongodb.org/mongo-driver/bson"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"
)

func pwaOf(doc bsonv1.D) any {
	for _, e := range doc {
		if e.Key == "ProgressiveWebAppSettings" {
			return e.Value
		}
	}
	return "absent"
}

func pwaStored(id string, installPrompt, precaching bool) bsonv1.D {
	return bsonv1.D{
		{Key: "$ID", Value: id},
		{Key: "$Type", Value: "Navigation$ProgressiveWebAppSettings"},
		{Key: "InstallPrompt", Value: installPrompt},
		{Key: "Precaching", Value: precaching},
	}
}

// An offline profile with null settings gets no service worker (#1377), so the
// clause must write them -- and a rewrite that never mentions it must leave the
// stored settings exactly as they are, including both booleans in each value:
// a fixture that stored only the defaults could not tell a preserving writer
// from one that rewrites the defaults every time.
func TestPWASettingsAreLeftAloneWhenTheStatementIsSilent(t *testing.T) {
	for _, stored := range []any{nil, pwaStored("keep", true, false), pwaStored("keep", false, true)} {
		doc := bsonv1.D{{Key: "Name", Value: "PhoneOffline"}, {Key: "ProgressiveWebAppSettings", Value: stored}}
		out := mustNavPatchWebProfile(t, doc, types.NavigationProfileSpec{})
		got := pwaOf(out)
		if stored == nil {
			if got != nil {
				t.Errorf("a silent rewrite turned null settings into %v", got)
			}
			continue
		}
		if g, ok := got.(bsonv1.D); !ok || navGetBool(g, "InstallPrompt") != navGetBool(stored.(bsonv1.D), "InstallPrompt") ||
			navGetBool(g, "Precaching") != navGetBool(stored.(bsonv1.D), "Precaching") {
			t.Errorf("a silent rewrite changed %v to %v", stored, got)
		}
	}
}

func TestPWAClauseWritesTheSettings(t *testing.T) {
	null := bsonv1.D{{Key: "Name", Value: "PhoneOffline"}, {Key: "ProgressiveWebAppSettings", Value: nil}}

	// A bare clause writes the platform defaults, as Studio Pro's checkbox does.
	out := mustNavPatchWebProfile(t, null, types.NavigationProfileSpec{ProgressiveWebApp: &types.NavPWASpec{}})
	d, ok := pwaOf(out).(bsonv1.D)
	if !ok {
		t.Fatalf("`progressive web app` wrote %v, want a settings element", pwaOf(out))
	}
	if navGetString(d, "$Type") != "Navigation$ProgressiveWebAppSettings" {
		t.Errorf("$Type = %q", navGetString(d, "$Type"))
	}
	if navGetBool(d, "Precaching") != false || navGetBool(d, "InstallPrompt") != true {
		t.Errorf("defaults written as %v, want Precaching false, InstallPrompt true", d)
	}

	// Named keys are written, both ways.
	for _, v := range []bool{true, false} {
		out = mustNavPatchWebProfile(t, null, types.NavigationProfileSpec{ProgressiveWebApp: &types.NavPWASpec{Precaching: boolPtr(v), InstallPrompt: boolPtr(!v)}})
		d = pwaOf(out).(bsonv1.D)
		if navGetBool(d, "Precaching") != v || navGetBool(d, "InstallPrompt") != !v {
			t.Errorf("Precaching: %v, InstallPrompt: %v written as %v", v, !v, d)
		}
	}

	// Over stored settings: their $ID is kept, and a key the clause leaves out
	// takes its default, not the stored value -- describe omits a default, so
	// keeping the stored value would make describe -> exec change nothing back.
	stored := bsonv1.D{{Key: "Name", Value: "PhoneOffline"}, {Key: "ProgressiveWebAppSettings", Value: pwaStored("id-1", false, false)}}
	out = mustNavPatchWebProfile(t, stored, types.NavigationProfileSpec{ProgressiveWebApp: &types.NavPWASpec{Precaching: boolPtr(true)}})
	d = pwaOf(out).(bsonv1.D)
	if navGetString(d, "$ID") != "id-1" || navGetBool(d, "InstallPrompt") != true || navGetBool(d, "Precaching") != true {
		t.Errorf("`( Precaching: true )` over stored {InstallPrompt false, Precaching false} gave %v, want the same $ID, InstallPrompt true", d)
	}

	// OFF stores null again.
	out = mustNavPatchWebProfile(t, stored, types.NavigationProfileSpec{ProgressiveWebApp: &types.NavPWASpec{Off: true}})
	if got := pwaOf(out); got != nil {
		t.Errorf("`progressive web app off` wrote %v, want null", got)
	}
}

func navProfileFromDoc(t *testing.T, doc bsonv2.D) *types.NavigationProfile {
	t.Helper()
	raw, err := bsonv2.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	g := genNav.NewNavigationProfile()
	g.SetRaw(raw)
	g.InitFromRaw(raw)
	return webNavProfileFromGen(g)
}

func TestPWASettingsAreRead(t *testing.T) {
	p := navProfileFromDoc(t, bsonv2.D{{Key: "$Type", Value: "Navigation$NavigationProfile"}, {Key: "Name", Value: "PhoneOffline"},
		{Key: "Kind", Value: "PhoneOffline"}, {Key: "ProgressiveWebAppSettings", Value: nil}})
	if p.ProgressiveWebApp != nil {
		t.Errorf("null settings read as %+v", p.ProgressiveWebApp)
	}
	for _, v := range []bool{true, false} {
		p = navProfileFromDoc(t, bsonv2.D{{Key: "$Type", Value: "Navigation$NavigationProfile"}, {Key: "Name", Value: "PhoneOffline"},
			{Key: "Kind", Value: "PhoneOffline"}, {Key: "ProgressiveWebAppSettings", Value: bsonv2.D{
				{Key: "$ID", Value: "x"}, {Key: "$Type", Value: "Navigation$ProgressiveWebAppSettings"},
				{Key: "InstallPrompt", Value: !v}, {Key: "Precaching", Value: v}}}})
		if p.ProgressiveWebApp == nil || p.ProgressiveWebApp.Precaching != v || p.ProgressiveWebApp.InstallPrompt != !v {
			t.Errorf("stored Precaching %v, InstallPrompt %v read as %+v", v, !v, p.ProgressiveWebApp)
		}
	}
}
