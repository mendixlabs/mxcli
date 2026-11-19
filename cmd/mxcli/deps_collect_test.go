// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	mp "github.com/mendixlabs/mxcli/internal/marketplace"
)

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// ako/mxcli#2425: a jar's Maven coordinates come from its own pom.properties —
// for a shaded jar, the one named like the file — else from its file name,
// which gives no group and so cannot be looked up.
func TestJavaLibrariesReadCoordinates(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "userlib", "commons-io-2.6.jar"), map[string]string{
		"META-INF/maven/commons-io/commons-io/pom.properties": "#generated\nversion=2.6\ngroupId=commons-io\nartifactId=commons-io\n",
	})
	writeZip(t, filepath.Join(dir, "userlib", "fat-client-1.0.jar"), map[string]string{
		"META-INF/maven/org.shaded/inner/pom.properties":   "groupId=org.shaded\nartifactId=inner\nversion=9.9\n",
		"META-INF/maven/com.acme/fat-client/pom.properties": "groupId=com.acme\nartifactId=fat-client\nversion=1.0\n",
	})
	writeZip(t, filepath.Join(dir, "vendorlib", "helper-3.1.jar"), map[string]string{"x.class": ""})
	libs := javaLibraries(dir)
	got := map[string]projectreport.DepLibrary{}
	for _, l := range libs {
		got[l.File] = l
	}
	if l := got["userlib/commons-io-2.6.jar"]; l.Coordinate() != "commons-io:commons-io:2.6" || l.Coordinates != "pom.properties" {
		t.Errorf("pom.properties: %+v", l)
	}
	if l := got["userlib/fat-client-1.0.jar"]; l.Coordinate() != "com.acme:fat-client:1.0" {
		t.Errorf("a shaded jar is named by the artifact that names the file: %+v", l)
	}
	if l := got["vendorlib/helper-3.1.jar"]; l.Group != "" || l.Name != "helper" || l.Version != "3.1" || l.Coordinates != "file name" {
		t.Errorf("file name fallback: %+v", l)
	}
}

func TestWidgetPackagesReadPackageXML(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "widgets", "Badge.mpk"), map[string]string{
		"package.xml": `<?xml version="1.0"?><package xmlns="http://www.mendix.com/package/1.0/"><clientModule name="Badge" version="3.2.2" xmlns="http://www.mendix.com/clientModule/1.0/"/></package>`,
	})
	ws := widgetPackages(dir)
	if len(ws) != 1 || ws[0].Name != "Badge" || ws[0].Version != "3.2.2" || ws[0].Status != projectreport.DepNotChecked {
		t.Errorf("widgets: %+v", ws)
	}
}

// OSV is asked once for all libraries and once per distinct advisory; a
// library without coordinates is marked not checked, with the reason.
func TestCheckVulnerabilitiesAgainstOSV(t *testing.T) {
	var detailCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/querybatch":
			var body struct {
				Queries []struct {
					Package struct{ Name string } `json:"package"`
				} `json:"queries"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			var results []map[string]any
			for _, q := range body.Queries {
				if q.Package.Name == "commons-io:commons-io" {
					results = append(results, map[string]any{"vulns": []map[string]string{{"id": "GHSA-high"}, {"id": "GHSA-low"}}})
				} else {
					results = append(results, map[string]any{})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
		case strings.HasPrefix(r.URL.Path, "/v1/vulns/"):
			detailCalls++
			id := strings.TrimPrefix(r.URL.Path, "/v1/vulns/")
			sev := map[string]string{"GHSA-high": "HIGH", "GHSA-low": "LOW"}[id]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": id, "summary": "s " + id, "database_specific": map[string]string{"severity": sev},
				"affected": []map[string]any{{"package": map[string]string{"name": "commons-io:commons-io"},
					"ranges": []map[string]any{{"events": []map[string]string{{"introduced": "0"}, {"fixed": "2.7"}}}}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	orig := osvBaseURL
	osvBaseURL = srv.URL
	t.Cleanup(func() { osvBaseURL = orig })

	libs := []projectreport.DepLibrary{
		{Group: "commons-io", Name: "commons-io", Version: "2.6"},
		{Group: "org.slf4j", Name: "slf4j-api", Version: "2.0.13"},
		{Name: "custom-helper"},
	}
	if err := checkVulnerabilities(context.Background(), libs); err != nil {
		t.Fatal(err)
	}
	io := libs[0]
	if !io.Checked || len(io.Vulns) != 2 || io.Vulns[0].ID != "GHSA-high" || !io.Vulns[0].Blocking() || io.Vulns[1].Blocking() ||
		len(io.Vulns[0].Fixed) != 1 || io.Vulns[0].Fixed[0] != "2.7" {
		t.Errorf("commons-io, worst first with its fix: %+v", io)
	}
	if !libs[1].Checked || len(libs[1].Vulns) != 0 {
		t.Errorf("a clean library is checked with none: %+v", libs[1])
	}
	if libs[2].Checked || !strings.Contains(libs[2].Note, "no Maven coordinates") {
		t.Errorf("no coordinates is not checked, never clean: %+v", libs[2])
	}
	if detailCalls != 2 {
		t.Errorf("one detail request per distinct advisory, got %d", detailCalls)
	}

	osvBaseURL = "http://127.0.0.1:1"
	if err := checkVulnerabilities(context.Background(), []projectreport.DepLibrary{{Group: "g", Name: "a", Version: "1"}}); err == nil {
		t.Error("an unreachable database is an error, so the report says it was not checked")
	}
}

type fakeMarketplace struct {
	catalog  []mp.Content
	versions map[int][]mp.Version
	calls    int
}

func (f *fakeMarketplace) Catalog(context.Context) ([]mp.Content, error) { return f.catalog, nil }
func (f *fakeMarketplace) Versions(_ context.Context, id int) ([]mp.Version, error) {
	f.calls++
	if v, ok := f.versions[id]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

func content(id int, typ, name, latest, latestID string) mp.Content {
	return mp.Content{ContentID: id, Type: typ, LatestVersion: &mp.Version{Name: name, VersionNumber: latest, VersionID: latestID}}
}

// A module is identified by the version UUID it records, never by its name or
// number: names collide ("Feedback"), numbers collide across content.
func TestCheckMarketplaceIdentifiesByVersionID(t *testing.T) {
	src := &fakeMarketplace{
		catalog: []mp.Content{
			content(23513, "Module", "Administration module", "4.5.0", "id-admin-450"),
			content(205506, "Module", "Mendix Feedback Module", "5.0.0", "id-fb-500"),
			content(110282, "Module", "Validation Feedback Collector", "3.4.0", "id-other"),
			content(109515, "Module", "Nanoflow Commons", "7.4.1", "id-nc-741"),
			content(1, "Module", "Up To Date", "1.0.0", "id-utd"),
			content(50325, "Widget", "Badge", "3.2.3", "w1"),
			content(52705, "Widget", "Badge Button", "3.3.0", "w2"),
			content(9, "Widget", "Barcode Scanner", "1.0.0", "w3"),
			content(10, "Widget", "Barcode Scanner", "2.0.0", "w4"),
		},
		versions: map[int][]mp.Version{
			23513:  {{VersionNumber: "4.5.0", VersionID: "id-admin-450", MinSupportedMendixVersion: "10.24.0"}, {VersionNumber: "4.3.2", VersionID: "id-admin-432"}},
			205506: {{VersionNumber: "5.0.0", VersionID: "id-fb-500", MinSupportedMendixVersion: "12.0.0"}, {VersionNumber: "4.1.1", VersionID: "id-fb-411", MinSupportedMendixVersion: "10.0.0"}, {VersionNumber: "4.0.2", VersionID: "id-fb-402"}},
			110282: {{VersionNumber: "3.4.0", VersionID: "id-other"}},
			109515: {{VersionNumber: "7.4.1", VersionID: "id-nc-741", MinSupportedMendixVersion: "11.12.5"}, {VersionNumber: "6.1.1", VersionID: "id-nc-611"}},
		},
	}
	mods := []projectreport.DepModule{
		{Name: "Administration", Installed: "4.3.2", VersionID: "id-admin-432"},
		{Name: "FeedbackModule", Installed: "4.0.2", VersionID: "id-fb-402"},
		{Name: "NanoflowCommons", Installed: "6.0.0", VersionID: "id-withdrawn"},
		{Name: "UpToDate", Installed: "1.0.0", VersionID: "id-utd"},
		{Name: "Private", Installed: "1.0.0", VersionID: "id-private"},
	}
	widgets := []projectreport.DepWidget{{Name: "Badge", Version: "3.2.2"}, {Name: "Barcode Scanner", Version: "1.0.0"}, {Name: "Gallery", Version: "1.0.0"}}
	if err := checkMarketplace(context.Background(), src, "11.13.0", mods, widgets); err != nil {
		t.Fatal(err)
	}
	m := map[string]projectreport.DepModule{}
	for _, x := range mods {
		m[x.Name] = x
	}
	if a := m["Administration"]; a.ContentID != 23513 || a.Status != projectreport.DepNewer || a.LatestCompatible != "4.5.0" {
		t.Errorf("Administration: %+v", a)
	}
	// FeedbackModule's name is inside "Mendix Feedback Module"; the UUID picks
	// it over "Validation Feedback Collector", and the newest needs Mendix 12.
	if f := m["FeedbackModule"]; f.ContentID != 205506 || f.LatestCompatible != "4.1.1" || f.Latest != "5.0.0" || !strings.Contains(f.Note, "newer Mendix") {
		t.Errorf("FeedbackModule: %+v", f)
	}
	if n := m["NanoflowCommons"]; n.Status != projectreport.DepNotIdentified || !strings.Contains(n.Note, "content 109515") || !strings.Contains(n.Note, "7.4.1") {
		t.Errorf("a withdrawn version is not identified, and says which content has the name: %+v", n)
	}
	if u := m["UpToDate"]; u.Status != projectreport.DepUpToDate {
		t.Errorf("the latest version's ID identifies it directly: %+v", u)
	}
	if p := m["Private"]; p.Status != projectreport.DepNotIdentified || p.ContentID != 0 {
		t.Errorf("unknown content: %+v", p)
	}
	if widgets[0].Latest != "3.2.3" || widgets[0].Status != projectreport.DepNewer {
		t.Errorf("Badge is Badge, not Badge Button: %+v", widgets[0])
	}
	if widgets[1].Status != projectreport.DepNotIdentified || !strings.Contains(widgets[1].Note, "2 catalog widgets") {
		t.Errorf("two widgets of one name are not guessed between: %+v", widgets[1])
	}
	if widgets[2].Status != projectreport.DepNotIdentified {
		t.Errorf("Gallery: %+v", widgets[2])
	}
}
