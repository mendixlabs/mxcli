// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

func offlineNav(stored *types.NavPWASettings, got *types.NavigationProfileSpec) *mock.MockBackend {
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{
				{Name: "Responsive", Kind: "Responsive"},
				{Name: "PhoneOffline", Kind: "PhoneOffline", ProgressiveWebApp: stored},
				{Name: "NativePhone", IsNative: true},
			}}, nil
		},
		UpdateNavigationProfileFunc: func(_ model.ID, _ string, spec types.NavigationProfileSpec) error {
			if got != nil {
				*got = spec
			}
			return nil
		},
	}
}

const pwaWarning = "offline profile without Progressive web app settings"

// mendixlabs/mxcli#1377: the clause reaches the writer as the spec; a statement
// that does not mention it leaves the stored settings alone (nil).
func TestPWAClauseReachesTheSpec(t *testing.T) {
	cases := map[string]*types.NavPWASpec{
		"":                        nil,
		"progressive web app":     {},
		"progressive web app off": {Off: true},
		"progressive web app ( Precaching: true )":                        {Precaching: boolPtr(true)},
		"progressive web app ( Precaching: false, InstallPrompt: true, )": {Precaching: boolPtr(false), InstallPrompt: boolPtr(true)},
		// Keys match case-insensitively, as the stored names are matched everywhere else.
		"progressive web app ( installprompt: false )": {InstallPrompt: boolPtr(false)},
	}
	for clause, want := range cases {
		var got types.NavigationProfileSpec
		if _, err := runNav(t, offlineNav(nil, &got), "create or modify navigation PhoneOffline home page M.Home "+clause+";"); err != nil {
			t.Fatalf("%q: %v", clause, err)
		}
		if (want == nil) != (got.ProgressiveWebApp == nil) {
			t.Errorf("%q: spec %+v, want %+v", clause, got.ProgressiveWebApp, want)
			continue
		}
		if want == nil {
			continue
		}
		g := got.ProgressiveWebApp
		if g.Off != want.Off || !sameBoolPtr(g.Precaching, want.Precaching) || !sameBoolPtr(g.InstallPrompt, want.InstallPrompt) {
			t.Errorf("%q: spec %+v, want %+v", clause, g, want)
		}
	}
}

func sameBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestPWAClauseRejectsWhatIsNotASetting(t *testing.T) {
	for _, src := range []string{
		"create or modify navigation PhoneOffline progressive web app ( Preload: true );",
		"create or modify navigation PhoneOffline progressive web app ( Precaching: 'yes' );",
	} {
		if _, errs := visitor.Build(src); len(errs) == 0 {
			t.Errorf("%q was accepted", src)
		}
	}
}

func TestPWAClauseIsRefusedOnANativeProfile(t *testing.T) {
	wrote := false
	mb := offlineNav(nil, nil)
	mb.UpdateNavigationProfileFunc = func(model.ID, string, types.NavigationProfileSpec) error { wrote = true; return nil }
	if _, err := runNav(t, mb, "create or modify navigation NativePhone home nanoflow M.N progressive web app;"); err == nil || wrote {
		t.Errorf("err=%v wrote=%v, want a refusal and no write", err, wrote)
	}
}

// An offline profile left without settings registers no service worker, so no
// page opens offline: exec says so. The controls: with settings, on an online
// profile, and when the stored settings are kept by a silent statement.
func TestOfflineProfileWithoutPWAWarns(t *testing.T) {
	stored := &types.NavPWASettings{Precaching: true, InstallPrompt: true}
	for _, c := range []struct {
		stored *types.NavPWASettings
		script string
		warn   bool
	}{
		{nil, "create or modify navigation PhoneOffline home page M.Home;", true},
		{nil, "create or modify navigation PhoneOffline home page M.Home progressive web app ( Precaching: true );", false},
		{stored, "create or modify navigation PhoneOffline home page M.Home;", false},
		{stored, "create or modify navigation PhoneOffline home page M.Home progressive web app off;", true},
		{nil, "create or modify navigation Responsive home page M.Home;", false},
	} {
		out, err := runNav(t, offlineNav(c.stored, nil), c.script)
		if err != nil {
			t.Fatalf("%q: %v", c.script, err)
		}
		if strings.Contains(out, pwaWarning) != c.warn {
			t.Errorf("stored %+v, %q: warning=%v, want %v\n%s", c.stored, c.script, !c.warn, c.warn, out)
		}
	}
}

// Describe emits the clause when settings are stored, with only the keys that
// differ from the defaults (R12), and its output parses back to the same values.
func TestDescribePWASettings(t *testing.T) {
	for _, c := range []struct {
		pwa  *types.NavPWASettings
		want string
	}{
		{nil, ""},
		{&types.NavPWASettings{Precaching: false, InstallPrompt: true}, "  progressive web app\n"},
		{&types.NavPWASettings{Precaching: true, InstallPrompt: true}, "  progressive web app ( Precaching: true )\n"},
		{&types.NavPWASettings{Precaching: true, InstallPrompt: false}, "  progressive web app ( Precaching: true, InstallPrompt: false )\n"},
	} {
		ctx, buf := newMockCtx(t)
		outputNavigationProfile(ctx, &types.NavigationProfile{Name: "PhoneOffline", Kind: "PhoneOffline", ProgressiveWebApp: c.pwa})
		out := buf.String()
		if c.want == "" {
			if strings.Contains(out, "progressive web app") {
				t.Errorf("null settings described as\n%s", out)
			}
			continue
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%+v described as\n%s", c.pwa, out)
		}
		// Round trip: the described clause applied to null settings gives the same values.
		var got types.NavigationProfileSpec
		if _, err := runNav(t, offlineNav(nil, &got), "create or modify navigation PhoneOffline home page M.Home "+strings.TrimSpace(c.want)+";"); err != nil {
			t.Fatal(err)
		}
		pre, prompt := types.NavPWADefaultPrecaching, types.NavPWADefaultInstallPrompt
		if p := got.ProgressiveWebApp.Precaching; p != nil {
			pre = *p
		}
		if p := got.ProgressiveWebApp.InstallPrompt; p != nil {
			prompt = *p
		}
		if pre != c.pwa.Precaching || prompt != c.pwa.InstallPrompt {
			t.Errorf("%q re-read as Precaching %v, InstallPrompt %v", c.want, pre, prompt)
		}
	}
}
