// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	mp "github.com/mendixlabs/mxcli/internal/marketplace"
	"github.com/mendixlabs/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Collectors for `report dependencies` (ako/mxcli#2425).

// marketplaceModules lists the modules that record a Marketplace version.
func marketplaceModules(mprPath string) (mendixVersion string, mods []projectreport.DepModule, err error) {
	r, err := mpr.Open(mprPath)
	if err != nil {
		return "", nil, err
	}
	defer r.Close()
	if pv := r.ProjectVersion(); pv != nil {
		mendixVersion = pv.ProductVersion
	}
	units, err := r.ListRawUnitsByType("Projects$ModuleImpl")
	if err != nil {
		return "", nil, err
	}
	for _, u := range units {
		var d bson.D
		if err := bson.Unmarshal(u.Contents, &d); err != nil {
			continue
		}
		if fromStore, _ := field(d, "FromAppStore").(bool); !fromStore {
			continue
		}
		mods = append(mods, projectreport.DepModule{
			Name: str(field(d, "Name")), Installed: str(field(d, "AppStoreVersion")),
			VersionID: strings.ToLower(str(field(d, "AppStoreGuid"))), Status: projectreport.DepNotChecked,
		})
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Name < mods[j].Name })
	return mendixVersion, mods, nil
}

// widgetPackages lists widgets/*.mpk with the name and version their
// package.xml declares.
func widgetPackages(projectDir string) []projectreport.DepWidget {
	files, _ := filepath.Glob(filepath.Join(projectDir, "widgets", "*.mpk"))
	sort.Strings(files)
	var out []projectreport.DepWidget
	for _, f := range files {
		w := projectreport.DepWidget{Package: filepath.Base(f), Status: projectreport.DepNotChecked}
		w.Name, w.Version = mpkClientModule(f)
		if w.Name == "" {
			w.Name = strings.TrimSuffix(w.Package, ".mpk")
		}
		out = append(out, w)
	}
	return out
}

func mpkClientModule(mpkPath string) (name, version string) {
	zr, err := zip.OpenReader(mpkPath)
	if err != nil {
		return "", ""
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "package.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", ""
		}
		defer rc.Close()
		var pkg struct {
			ClientModule struct {
				Name    string `xml:"name,attr"`
				Version string `xml:"version,attr"`
			} `xml:"clientModule"`
		}
		if xml.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&pkg) == nil {
			return pkg.ClientModule.Name, pkg.ClientModule.Version
		}
	}
	return "", ""
}

var jarFileName = regexp.MustCompile(`^(.+?)-(\d[\w.\-]*)\.jar$`)

// javaLibraries lists userlib/ and vendorlib/ jars with their Maven
// coordinates, read from the jar's own pom.properties where it has one.
func javaLibraries(projectDir string) []projectreport.DepLibrary {
	var out []projectreport.DepLibrary
	for _, dir := range []string{"userlib", "vendorlib"} {
		files, _ := filepath.Glob(filepath.Join(projectDir, dir, "*.jar"))
		sort.Strings(files)
		for _, f := range files {
			l := projectreport.DepLibrary{File: dir + "/" + filepath.Base(f)}
			base := filepath.Base(f)
			if g, a, v := jarPomCoordinates(f, base); a != "" {
				l.Group, l.Name, l.Version, l.Coordinates = g, a, v, "pom.properties"
			} else if m := jarFileName.FindStringSubmatch(base); m != nil {
				l.Name, l.Version, l.Coordinates = m[1], m[2], "file name"
			} else {
				l.Name, l.Coordinates = strings.TrimSuffix(base, ".jar"), "file name"
			}
			out = append(out, l)
		}
	}
	return out
}

// jarPomCoordinates reads META-INF/maven/<group>/<artifact>/pom.properties. A
// shaded jar carries several; the one whose artifact starts the file name wins.
func jarPomCoordinates(jarPath, fileName string) (group, artifact, version string) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return "", "", ""
	}
	defer zr.Close()
	type coord struct{ g, a, v string }
	var found []coord
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "META-INF/maven/") || path.Base(f.Name) != "pom.properties" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		props := map[string]string{}
		sc := bufio.NewScanner(io.LimitReader(rc, 64<<10))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(line, "#") {
				props[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		rc.Close()
		if props["artifactId"] != "" {
			found = append(found, coord{props["groupId"], props["artifactId"], props["version"]})
		}
	}
	for _, c := range found {
		if strings.HasPrefix(fileName, c.a) {
			return c.g, c.a, c.v
		}
	}
	if len(found) == 1 {
		return found[0].g, found[0].a, found[0].v
	}
	return "", "", ""
}

// ---------------------------------------------------------------- OSV

// osvBaseURL is the advisory database; a seam for tests.
var osvBaseURL = "https://api.osv.dev"

var osvHTTP = &http.Client{Timeout: 30 * time.Second}

// checkVulnerabilities looks up each library with Maven coordinates in OSV and
// fills in its advisories. A library without them is marked not checked.
func checkVulnerabilities(ctx context.Context, libs []projectreport.DepLibrary) error {
	type query struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		Version string `json:"version"`
	}
	var qs []query
	var idx []int
	for i := range libs {
		l := &libs[i]
		if l.Group == "" || l.Version == "" {
			l.Note = "no Maven coordinates in the jar (it has no pom.properties), so it cannot be looked up"
			continue
		}
		var q query
		q.Package.Ecosystem, q.Package.Name, q.Version = "Maven", l.Group+":"+l.Name, l.Version
		qs = append(qs, q)
		idx = append(idx, i)
	}
	if len(qs) == 0 {
		return nil
	}
	var batch struct {
		Results []struct {
			Vulns []struct {
				ID string `json:"id"`
			} `json:"vulns"`
		} `json:"results"`
	}
	if err := osvPost(ctx, "/v1/querybatch", map[string]any{"queries": qs}, &batch); err != nil {
		return err
	}
	if len(batch.Results) != len(qs) {
		return fmt.Errorf("osv answered %d results for %d queries", len(batch.Results), len(qs))
	}
	// Each advisory's details are one request; fetch the distinct ones in
	// parallel (a library like jackson-databind 2.9.x has dozens).
	var ids []string
	seen := map[string]string{} // id -> package it was found for
	for n, res := range batch.Results {
		l := &libs[idx[n]]
		for _, v := range res.Vulns {
			if _, ok := seen[v.ID]; !ok {
				seen[v.ID] = l.Group + ":" + l.Name
				ids = append(ids, v.ID)
			}
		}
	}
	details := make(map[string]projectreport.DepVuln, len(ids))
	var mu sync.Mutex
	sem := make(chan struct{}, osvConcurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			d, err := osvVuln(ctx, id, seen[id])
			if err != nil {
				d = projectreport.DepVuln{ID: id}
			}
			mu.Lock()
			details[id] = d
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	for n, res := range batch.Results {
		l := &libs[idx[n]]
		l.Checked = true
		for _, v := range res.Vulns {
			l.Vulns = append(l.Vulns, details[v.ID])
		}
		sort.SliceStable(l.Vulns, func(i, j int) bool { return vulnRank(l.Vulns[i]) > vulnRank(l.Vulns[j]) })
	}
	return nil
}

// osvConcurrency bounds the parallel advisory lookups.
const osvConcurrency = 8

func vulnRank(v projectreport.DepVuln) int {
	return map[string]int{"LOW": 1, "MODERATE": 2, "MEDIUM": 2, "HIGH": 3, "CRITICAL": 4}[v.Severity]
}

func osvVuln(ctx context.Context, id, pkg string) (projectreport.DepVuln, error) {
	var raw struct {
		ID               string   `json:"id"`
		Summary          string   `json:"summary"`
		Aliases          []string `json:"aliases"`
		DatabaseSpecific struct {
			Severity string `json:"severity"`
		} `json:"database_specific"`
		Affected []struct {
			Package struct {
				Name string `json:"name"`
			} `json:"package"`
			Ranges []struct {
				Events []struct {
					Fixed string `json:"fixed"`
				} `json:"events"`
			} `json:"ranges"`
		} `json:"affected"`
	}
	if err := osvGet(ctx, "/v1/vulns/"+id, &raw); err != nil {
		return projectreport.DepVuln{ID: id}, err
	}
	v := projectreport.DepVuln{ID: raw.ID, Summary: raw.Summary, Aliases: raw.Aliases, Severity: strings.ToUpper(raw.DatabaseSpecific.Severity)}
	seen := map[string]bool{}
	for _, a := range raw.Affected {
		if !strings.EqualFold(a.Package.Name, pkg) {
			continue
		}
		for _, r := range a.Ranges {
			for _, e := range r.Events {
				if e.Fixed != "" && !seen[e.Fixed] {
					seen[e.Fixed] = true
					v.Fixed = append(v.Fixed, e.Fixed)
				}
			}
		}
	}
	return v, nil
}

func osvPost(ctx context.Context, p string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, osvBaseURL+p, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return osvDo(req, out)
}

func osvGet(ctx context.Context, p string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, osvBaseURL+p, nil)
	if err != nil {
		return err
	}
	return osvDo(req, out)
}

func osvDo(req *http.Request, out any) error {
	resp, err := osvHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("osv %s: %s", req.URL.Path, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

// ---------------------------------------------------------------- Marketplace

// marketplaceSource is what the report asks the Marketplace; a seam for tests.
type marketplaceSource interface {
	Catalog(ctx context.Context) ([]mp.Content, error)
	Versions(ctx context.Context, contentID int) ([]mp.Version, error)
}

// maxContentCandidates bounds the version lookups per module: a module name
// that matches many catalog items is reported not identified rather than
// costing dozens of requests.
const maxContentCandidates = 6

// checkMarketplace finds each module's content by the version UUID it records
// and fills in the newest and newest compatible versions; widgets are matched
// by name and get the newest published version.
func checkMarketplace(ctx context.Context, src marketplaceSource, mendixVersion string, mods []projectreport.DepModule, widgets []projectreport.DepWidget) error {
	catalog, err := src.Catalog(ctx)
	if err != nil {
		return err
	}
	for i := range mods {
		m := &mods[i]
		if m.VersionID == "" {
			m.Status, m.Note = projectreport.DepNotIdentified, "the module records no Marketplace version ID"
			continue
		}
		// The version it records is a content's latest: identified, and up
		// to date, without a versions lookup.
		if c := latestIs(catalog, m.VersionID); c != nil {
			m.ContentID, m.Latest, m.LatestCompatible = c.ContentID, c.LatestVersion.VersionNumber, c.LatestVersion.VersionNumber
			m.Status = projectreport.DepUpToDate
			continue
		}
		var candidates []mp.Content
		for _, c := range catalog {
			if c.Type == "Module" && c.LatestVersion != nil && nameContains(c.LatestVersion.Name, m.Name) {
				candidates = append(candidates, c)
			}
		}
		if len(candidates) > maxContentCandidates {
			m.Status, m.Note = projectreport.DepNotIdentified, fmt.Sprintf("%d catalog items match the name", len(candidates))
			continue
		}
		identified := false
		var sameName *mp.Content // a candidate whose name is the module's
		var sameNameVersions []mp.Version
		for j, c := range candidates {
			vs, err := src.Versions(ctx, c.ContentID)
			if err != nil {
				continue
			}
			if foldName(c.LatestVersion.Name) == foldName(m.Name) {
				sameName, sameNameVersions = &candidates[j], vs
			}
			mine := false
			for _, v := range vs {
				if strings.EqualFold(v.VersionID, m.VersionID) {
					mine = true
					break
				}
			}
			if !mine {
				continue
			}
			identified = true
			m.ContentID = c.ContentID
			newest, compatible := newestVersions(vs, mendixVersion)
			m.Latest, m.LatestCompatible = newest, compatible
			m.Status = projectreport.DepUpToDate
			if compatible != "" && compareSemverLike(compatible, m.Installed) > 0 {
				m.Status = projectreport.DepNewer
			}
			if newest != "" && newest != compatible && compareSemverLike(newest, m.Installed) > 0 {
				m.Note = "newest needs a newer Mendix version"
			}
			break
		}
		switch {
		case identified:
		case sameName != nil:
			// The name is the module's, but the version it records is not
			// published there (any more): withdrawn, or another publisher's.
			_, compatible := newestVersions(sameNameVersions, mendixVersion)
			m.Status = projectreport.DepNotIdentified
			m.Note = fmt.Sprintf("content %d (%q) has this name but does not publish the installed version: withdrawn, or another publisher's module", sameName.ContentID, sameName.LatestVersion.Name)
			if compatible != "" {
				m.Note += "; its newest compatible version is " + compatible
			}
		default:
			m.Status, m.Note = projectreport.DepNotIdentified, "no catalog item publishes the version the module records (private or removed content)"
		}
	}
	for i := range widgets {
		w := &widgets[i]
		match, note := widgetContent(catalog, w.Name)
		if match == nil {
			w.Status, w.Note = projectreport.DepNotIdentified, note
			continue
		}
		w.Latest = match.LatestVersion.VersionNumber
		w.Status = projectreport.DepUpToDate
		if w.Version != "" && compareSemverLike(w.Latest, w.Version) > 0 {
			w.Status = projectreport.DepNewer
		}
	}
	return nil
}

// widgetContent finds a widget's catalog item by name: an exact (folded) name
// first, so "Badge" is not confused with "Badge Button", else a unique prefix
// match. A widget package records no Marketplace version ID, so the name is all
// there is; several items with one name are not guessed between.
func widgetContent(catalog []mp.Content, name string) (*mp.Content, string) {
	var exact, prefix []int
	for j, c := range catalog {
		if c.Type != "Widget" || c.LatestVersion == nil {
			continue
		}
		switch {
		case foldName(c.LatestVersion.Name) == foldName(name):
			exact = append(exact, j)
		case namesMatch(name, c.LatestVersion.Name):
			prefix = append(prefix, j)
		}
	}
	switch {
	case len(exact) == 1:
		return &catalog[exact[0]], ""
	case len(exact) > 1:
		return nil, fmt.Sprintf("%d catalog widgets have this name", len(exact))
	case len(prefix) == 1:
		return &catalog[prefix[0]], ""
	case len(prefix) > 1:
		return nil, fmt.Sprintf("%d catalog widgets start with this name", len(prefix))
	}
	return nil, "no catalog widget has this name (it may come with a module, as the Datagrid filters come with Data Widgets)"
}

func latestIs(catalog []mp.Content, versionID string) *mp.Content {
	for i, c := range catalog {
		if c.LatestVersion != nil && strings.EqualFold(c.LatestVersion.VersionID, versionID) {
			return &catalog[i]
		}
	}
	return nil
}

// newestVersions returns the newest version overall and the newest whose
// minimum Mendix version is at most the project's.
func newestVersions(vs []mp.Version, mendixVersion string) (newest, compatible string) {
	for _, v := range vs {
		if v.VersionNumber == "" {
			continue
		}
		if newest == "" || compareSemverLike(v.VersionNumber, newest) > 0 {
			newest = v.VersionNumber
		}
		if mendixVersion != "" && v.MinSupportedMendixVersion != "" && compareSemverLike(v.MinSupportedMendixVersion, mendixVersion) > 0 {
			continue
		}
		if compatible == "" || compareSemverLike(v.VersionNumber, compatible) > 0 {
			compatible = v.VersionNumber
		}
	}
	return newest, compatible
}

// namesMatch compares a module or widget name with a catalog name, folding
// case and separators: "DataWidgets" matches "Data Widgets", "Administration"
// matches "Administration module".
func namesMatch(local, catalog string) bool {
	a, b := foldName(local), foldName(catalog)
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasPrefix(b, a) || strings.HasPrefix(a, b)
}

// nameContains reports whether a catalog name contains a module's name,
// folded: "Mendix Feedback Module" contains "FeedbackModule". It finds
// candidates only; the version ID decides.
func nameContains(catalog, local string) bool {
	a, b := foldName(local), foldName(catalog)
	return a != "" && strings.Contains(b, a)
}

func foldName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// clientSource asks the Marketplace through the authenticated client, with
// the catalog cache `marketplace search` keeps.
type clientSource struct {
	client *mp.Client
	cache  *mp.CatalogCache
}

func (s clientSource) Catalog(ctx context.Context) ([]mp.Content, error) {
	if items, fresh := s.cache.Load(); fresh {
		return items, nil
	}
	items, err := s.client.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Save(items)
	return items, nil
}

func (s clientSource) Versions(ctx context.Context, contentID int) ([]mp.Version, error) {
	vl, err := s.client.Versions(ctx, contentID)
	if err != nil {
		return nil, err
	}
	return vl.Items, nil
}
