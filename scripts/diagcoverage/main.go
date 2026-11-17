// SPDX-License-Identifier: Apache-2.0

// diag-coverage reports which of the diagnostics Mendix's own consistency check can
// raise (`mx check`, Studio Pro's Errors pane) mxcli's `check` names, ranked by how
// often real bug reports have hit the ones it does not.
//
//	go run ./scripts/diagcoverage                  # newest MxBuild installed
//	go run ./scripts/diagcoverage --version 11.16.1 --top 50
//	go run ./scripts/diagcoverage --all > codes.tsv  # every catalog code, one line each
//	make diag-coverage ARGS="--top 50"
//
// It needs a MxBuild installed (`mxcli setup mxbuild -p app.mpr`) for its text
// resource, and reads only the code IDs from it. Nothing of Mendix's message text is
// printed or stored. See docs/11-proposals/PROPOSAL_check_diagnostics_catalog.md.
//
// "Named" is a proxy, not a proof of coverage; see SourceCodes. The number is
// computed, never quoted: a figure copied into prose is stale the next time anyone
// adds a check.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "diag-coverage:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("diag-coverage", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dll := fs.String("dll", "", "path to Mendix.Modeler.Texts.dll (default: from --version or the newest MxBuild installed)")
	version := fs.String("version", "", "MxBuild version installed under ~/.mxcli/mxbuild (default: newest)")
	repo := fs.String("repo", ".", "mxcli repository root")
	findings := fs.String("findings", "", "finding records directory (default: <repo>/.claude/skills/fix-issue/findings)")
	coveredAs := fs.String("covered-as", "", "codes covered under another name (default: <repo>/scripts/diagcoverage/covered-as.tsv)")
	top := fs.Int("top", 30, "how many field-observed gaps to list (0 = all)")
	all := fs.Bool("all", false, "print every catalog code as TSV: code, status, findings")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(*repo, "go.mod")); err != nil {
		return fmt.Errorf("%s does not look like the mxcli repository (no go.mod); pass --repo", *repo)
	}
	if *findings == "" {
		*findings = filepath.Join(*repo, ".claude", "skills", "fix-issue", "findings")
	}
	if *coveredAs == "" {
		*coveredAs = filepath.Join(*repo, "scripts", "diagcoverage", "covered-as.tsv")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dllPath, ver, err := ResolveDLL(*dll, *version, home)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(dllPath)
	if err != nil {
		return err
	}
	named, err := SourceCodes(*repo)
	if err != nil {
		return err
	}
	counts, files, err := FindingCounts(*findings)
	if err != nil {
		return err
	}
	extras, err := LoadCoveredAs(*coveredAs)
	if err != nil {
		return err
	}
	r := Build(CatalogCodes(raw), named, extras, counts, files, ver)

	switch {
	case *asJSON:
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case *all:
		printAll(stdout, r)
	default:
		printReport(stdout, r, *top)
	}
	return nil
}

func printAll(w io.Writer, r *Report) {
	codes := make([]string, 0, len(r.Status))
	for c := range r.Status {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	fmt.Fprintln(w, "code\tstatus\tfindings")
	for _, c := range codes {
		fmt.Fprintf(w, "%s\t%s\t%d\n", c, r.Status[c], r.FindingCounts[c])
	}
}

func printReport(w io.Writer, r *Report, top int) {
	src := "Mendix.Modeler.Texts.dll"
	if r.Version != "" {
		src += " of MxBuild " + r.Version
	}
	fmt.Fprintf(w, "diagnostics coverage — %s\n\n", src)
	fmt.Fprintf(w, "  catalog        %5d codes  (CE %d, CW %d, CI %d)\n", r.Catalog, r.ByPrefix["CE"], r.ByPrefix["CW"], r.ByPrefix["CI"])
	fmt.Fprintf(w, "  named          %5d        %.1f%%  named by a message string of non-test Go code or a lint rule\n", r.Named, pct(r.Named, r.Catalog))
	fmt.Fprintf(w, "  covered-as     %5d        covered under another name (scripts/diagcoverage/covered-as.tsv)\n", r.CoveredAs)
	fmt.Fprintf(w, "  not named      %5d\n", r.NotNamed)
	if len(r.NamedNotIn) > 0 {
		fmt.Fprintf(w, "\n  named but not in this catalog (another version's code, or a typo): %s\n", strings.Join(r.NamedNotIn, " "))
	}
	fmt.Fprintf(w, "\nfield-observed codes mxcli does not name — %d finding records name %d distinct codes;\n", r.FindingFiles, r.FindingCodes)
	fmt.Fprintf(w, "%d of them are in this catalog and not named, by how many records name each:\n\n", len(r.Gaps))
	n := len(r.Gaps)
	if top > 0 && top < n {
		n = top
	}
	for _, g := range r.Gaps[:n] {
		fmt.Fprintf(w, "  %s  %4d\n", g.Code, g.Findings)
	}
	if n < len(r.Gaps) {
		fmt.Fprintf(w, "  … %d more (--top 0 for all)\n", len(r.Gaps)-n)
	}
	if len(r.FindingsOff) > 0 {
		fmt.Fprintf(w, "\n  %d codes in the finding records are not in this catalog (another MxBuild version?): %s\n", len(r.FindingsOff), strings.Join(r.FindingsOff, " "))
	}
	fmt.Fprintln(w, "\nnamed is a proxy for covered: a check that does not name its code shows as a gap (add it to covered-as.tsv),")
	fmt.Fprintln(w, "and a code named in a message may be only partly detected. --all lists every code.")
	fmt.Fprintln(w, "The finding records are fixes, so they over-represent codes mxcli already learned about: a code")
	fmt.Fprintln(w, "nobody reported is invisible in the ranking. To find those, run broken scripts through mx check")
	fmt.Fprintln(w, "(mdl-examples/check-gap/run-parity.sh) and add what it catches that check does not.")
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}
