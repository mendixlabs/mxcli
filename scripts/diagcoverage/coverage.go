// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/scanner"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// codeRE is a Mendix consistency-diagnostic code: CE (error), CW (warning) or CI
// (deprecation / information) and four digits.
var codeRE = regexp.MustCompile(`C[EWI][0-9]{4}`)

// CatalogCodes returns every diagnostic code that appears in Mendix's text
// resources (Mendix.Modeler.Texts.dll), sorted and unique.
//
// The resources are PO files embedded as plain ASCII, one per locale, so the IDs
// are found by scanning the bytes: no monodis, no .NET. Only the IDs are kept. The
// message text next to each code is Mendix's, and the proposal that motivates this
// (docs/11-proposals/PROPOSAL_check_diagnostics_catalog.md) is explicit that it is
// not ours to copy into the repository.
func CatalogCodes(dll []byte) []string {
	seen := map[string]bool{}
	for _, m := range codeRE.FindAll(dll, -1) {
		seen[string(m)] = true
	}
	return sortedKeys(seen)
}

// sourceRoots are the directories whose code can name a diagnostic to a user.
var sourceRoots = []string{"mdl", "modelsdk", "cmd", "sdk", "api"}

var skipDirs = map[string]bool{"testdata": true, "node_modules": true, "vendor": true, ".git": true}

// maxFilesPerCode caps how many files are remembered for a code, for the report.
const maxFilesPerCode = 3

// SourceCodes returns the codes mxcli NAMES, each with up to three of the files that
// name it: a code that appears inside a string literal of non-test Go code, or
// anywhere in a Starlark lint rule.
//
// Naming is a proxy for covering, not the same thing, and it errs both ways:
// a check that does not name its code (MDL003 raises what MxBuild calls CE0067) is
// missed here, which is what covered-as.tsv is for; and a code named in a message
// may be only partly detected. Comments, tests and docs are deliberately not read —
// a code in a comment is a note about a measurement, not a diagnostic a user sees.
// go/scanner reads string literals and skips comments, so the two cannot be mixed up.
func SourceCodes(repo string) (map[string][]string, error) {
	out := map[string][]string{}
	add := func(code, path string) {
		files := out[code]
		for _, f := range files {
			if f == path {
				return
			}
		}
		if len(files) < maxFilesPerCode {
			out[code] = append(files, path)
		}
	}
	for _, root := range sourceRoots {
		dir := filepath.Join(repo, root)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(repo, path)
			switch {
			case strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go"):
				src, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				for _, c := range stringLiteralCodes(path, src) {
					add(c, rel)
				}
			case strings.HasSuffix(path, ".star"):
				src, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				for _, c := range codeRE.FindAllString(string(src), -1) {
					add(c, rel)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// stringLiteralCodes returns the diagnostic codes inside the string literals of one
// Go file. A file that does not scan cleanly still yields what it could.
func stringLiteralCodes(path string, src []byte) []string {
	fset := token.NewFileSet()
	file := fset.AddFile(path, fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, 0) // mode 0: comments are skipped
	var out []string
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.STRING {
			out = append(out, codeRE.FindAllString(lit, -1)...)
		}
	}
	return out
}

// FindingCounts counts, per code, the finding records that name it in their `ce`
// field — the diagnostics real bug reports have hit. It returns the counts and how
// many finding files it read. A finding file holds one JSON object (older shards hold
// several, one per line); both read.
func FindingCounts(dir string) (map[string]int, int, error) {
	counts := map[string]int{}
	files := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !(strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".jsonl")) {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		files++
		dec := json.NewDecoder(f)
		for {
			var rec struct {
				CE []any `json:"ce"`
			}
			if err := dec.Decode(&rec); err == io.EOF {
				break
			} else if err != nil {
				break // a record that does not decode is not a reason to stop counting
			}
			seen := map[string]bool{}
			for _, e := range rec.CE {
				for _, c := range codeRE.FindAllString(fmt.Sprint(e), -1) {
					seen[c] = true // once per record, however often it repeats
				}
			}
			for c := range seen {
				counts[c]++
			}
		}
		return nil
	})
	return counts, files, err
}

// exactCodeRE matches a whole line field that is a code and nothing else.
var exactCodeRE = regexp.MustCompile(`^C[EWI][0-9]{4}$`)

// LoadCoveredAs reads the codes mxcli covers under another name: a check that
// raises what MxBuild reports under this code but does not say so in its message.
// Lines are `CODE<TAB>covered by<TAB>note`; blank lines and #-comments are skipped.
func LoadCoveredAs(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 2 || !exactCodeRE.MatchString(cols[0]) || strings.TrimSpace(cols[1]) == "" {
			return nil, fmt.Errorf("%s:%d: want `CODE<TAB>covered by[<TAB>note]`, got %q", path, n, line)
		}
		out[cols[0]] = strings.TrimSpace(cols[1])
	}
	return out, sc.Err()
}

// Status of a catalog code.
const (
	StatusNamed     = "named"      // a message string or lint rule of mxcli names it
	StatusCoveredAs = "covered-as" // covered under another name (covered-as.tsv)
	StatusNotNamed  = "not-named"
)

// Gap is a code real bug reports have hit that mxcli does not name.
type Gap struct {
	Code     string `json:"code"`
	Findings int    `json:"findings"`
}

// Report is the coverage of the catalog.
type Report struct {
	Version       string            `json:"mxbuildVersion,omitempty"`
	Catalog       int               `json:"catalog"`
	ByPrefix      map[string]int    `json:"byPrefix"`
	Named         int               `json:"named"`
	CoveredAs     int               `json:"coveredAs"`
	NotNamed      int               `json:"notNamed"`
	NamedNotIn    []string          `json:"namedButNotInCatalog,omitempty"`
	FindingFiles  int               `json:"findingFiles"`
	FindingCodes  int               `json:"findingCodes"`
	FindingsOff   []string          `json:"findingCodesNotInCatalog,omitempty"`
	Gaps          []Gap             `json:"fieldGaps"`
	Status        map[string]string `json:"-"`
	FindingCounts map[string]int    `json:"-"`
}

// Build classifies every catalog code and ranks the field-observed gaps by how many
// finding records name them (ties broken by code, so the order is stable).
func Build(catalog []string, named map[string][]string, coveredAs map[string]string, findings map[string]int, findingFiles int, version string) *Report {
	r := &Report{
		Version:       version,
		Catalog:       len(catalog),
		ByPrefix:      map[string]int{},
		FindingFiles:  findingFiles,
		FindingCodes:  len(findings),
		Status:        map[string]string{},
		FindingCounts: findings,
	}
	inCatalog := map[string]bool{}
	for _, c := range catalog {
		inCatalog[c] = true
		r.ByPrefix[c[:2]]++
		switch {
		case len(named[c]) > 0:
			r.Status[c] = StatusNamed
			r.Named++
		case coveredAs[c] != "":
			r.Status[c] = StatusCoveredAs
			r.CoveredAs++
		default:
			r.Status[c] = StatusNotNamed
			r.NotNamed++
		}
	}
	for c := range named {
		if !inCatalog[c] {
			r.NamedNotIn = append(r.NamedNotIn, c)
		}
	}
	sort.Strings(r.NamedNotIn)
	for c, n := range findings {
		switch {
		case !inCatalog[c]:
			r.FindingsOff = append(r.FindingsOff, c)
		case r.Status[c] == StatusNotNamed:
			r.Gaps = append(r.Gaps, Gap{Code: c, Findings: n})
		}
	}
	sort.Strings(r.FindingsOff)
	sort.Slice(r.Gaps, func(i, j int) bool {
		if r.Gaps[i].Findings != r.Gaps[j].Findings {
			return r.Gaps[i].Findings > r.Gaps[j].Findings
		}
		return r.Gaps[i].Code < r.Gaps[j].Code
	})
	return r
}

// versionPattern pulls the MxBuild version out of .../mxbuild/<version>/modeler/....
var versionPattern = regexp.MustCompile(`mxbuild[/\\]([0-9][0-9.]*)[/\\]modeler`)

// textsDLL is the resource assembly, relative to a MxBuild's modeler directory.
const textsDLL = "Mendix.Modeler.Texts.dll"

// ResolveDLL finds the text assembly to read: an explicit --dll, the --version
// MxBuild that `mxcli setup mxbuild` installed under ~/.mxcli/mxbuild, or the newest
// one installed. The version is "" when the path does not say.
func ResolveDLL(dll, version, home string) (path, ver string, err error) {
	if dll != "" {
		if _, err := os.Stat(dll); err != nil {
			return "", "", err
		}
		return dll, versionOf(dll), nil
	}
	base := filepath.Join(home, ".mxcli", "mxbuild")
	if version != "" {
		p := filepath.Join(base, version, "modeler", textsDLL)
		if _, err := os.Stat(p); err != nil {
			return "", "", fmt.Errorf("no %s for MxBuild %s under %s — run `mxcli setup mxbuild --version %s`", textsDLL, version, base, version)
		}
		return p, version, nil
	}
	entries, _ := os.ReadDir(base)
	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(base, e.Name(), "modeler", textsDLL)); err == nil {
				versions = append(versions, e.Name())
			}
		}
	}
	if len(versions) == 0 {
		return "", "", fmt.Errorf("no MxBuild with %s under %s — run `mxcli setup mxbuild -p <app.mpr>` (or pass --dll)", textsDLL, base)
	}
	sort.Slice(versions, func(i, j int) bool { return versionLess(versions[j], versions[i]) }) // newest first
	return filepath.Join(base, versions[0], "modeler", textsDLL), versions[0], nil
}

func versionOf(path string) string {
	if m := versionPattern.FindStringSubmatch(path); m != nil {
		return m[1]
	}
	return ""
}

// versionLess orders dotted version numbers numerically (11.9.0 < 11.16.1).
func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return len(pa) < len(pb)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
