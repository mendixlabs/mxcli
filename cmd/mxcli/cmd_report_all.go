// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// reportAllKinds is the order of the overview and of the links between pages.
// Performance and release need inputs (journeys, a deployed revision) and run
// only when they are given; an earlier run's JSON in the folder is linked
// instead.
var reportAllKinds = []string{"quality", "architecture", "security", "configuration", "dependencies", "tests", "traceability", "performance", "release", "map"}

var reportAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Write every report into one folder, with an overview page linking them",
	Long: `Run every report kind into one folder and write an overview (index.html)
with a card per report: its verdict and headline numbers, linked. Every page
links the others.

Each kind writes <kind>.html and <kind>.json (re-render with
'mxcli report <kind> --from <dir>/<kind>.json'). A <kind>-annotations.yaml in
the folder is applied to that kind's page.

Inputs go to the kinds they are for:
  --journeys, --spans        the functional journeys: tests and traceability
  --junit, --tests, --run    microflow test results: tests and traceability
                             (--run writes <folder>/junit.xml)
  --perf-journeys,           a performance run, one record per step:
  --perf-spans               performance only
  --against                  the deployed revision or .mpr: release

Performance and release run only when given their inputs.
A kind that cannot run is listed on the overview with the reason; a
<kind>.json already in the folder from an earlier run is linked instead.

--offline asks no online source (dependencies: no Marketplace, no
vulnerability lookup). --skip leaves kinds out.`,
	Example: `  mxcli report all -p app.mpr -o docs/reports
  mxcli report all -p app.mpr -o docs/reports --against v1.4.0 --journeys journeys.jsonl --spans spans.jsonl --junit results.xml
  mxcli report all -p app.mpr -o docs/reports --run --perf-journeys perf.jsonl --perf-spans perf-spans.jsonl
  mxcli report all -p app.mpr -o docs/reports --offline --skip map`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectPath, _ := cmd.Flags().GetString("project")
		dir, _ := cmd.Flags().GetString("output")
		if projectPath == "" {
			return fmt.Errorf("--project (-p) is required")
		}
		if dir == "" {
			return fmt.Errorf("-o <folder> is required: the reports are several files")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		skip := map[string]bool{}
		skips, _ := cmd.Flags().GetStringSlice("skip")
		for _, k := range skips {
			skip[strings.TrimSpace(k)] = true
		}
		return runReportAll(cmd, projectPath, dir, skip, os.Stderr)
	},
}

// reportPage is one generated (or found) report for the overview.
type reportPage struct {
	kind    string
	doc     *projectreport.Doc      // nil for the software map, which renders itself
	env     *projectreport.Envelope // nil for the scored report
	quality *linter.Report
	missing string
}

func runReportAll(cmd *cobra.Command, projectPath, dir string, skip map[string]bool, log io.Writer) error {
	passThrough(cmd, dir, skip)
	var pages []reportPage
	for _, kind := range reportAllKinds {
		if skip[kind] {
			continue
		}
		fmt.Fprintf(log, "report %s ...\n", kind)
		p := reportPage{kind: kind}
		if kind == "quality" {
			r, _, err := buildScoredReport(reportCmd, projectPath, io.Discard)
			if err != nil {
				p.missing = err.Error()
			} else {
				p.quality = r
			}
			pages = append(pages, p)
			continue
		}
		rk, ok := reportKinds[kind]
		if !ok {
			continue
		}
		if why := needsInput(cmd, kind); why != "" {
			if env, err := projectreport.ReadEnvelope(filepath.Join(dir, kind+".json"), kind); err == nil {
				p.env = env
				p.missing = ""
			} else {
				p.missing = why
			}
			pages = append(pages, p)
			continue
		}
		env, err := rk.spec.collect(rk.cmd, projectPath)
		if err != nil {
			p.missing = err.Error()
		} else {
			p.env = env
		}
		pages = append(pages, p)
	}

	// Render with the navigation between the pages that exist.
	nav := []projectreport.NavLink{{Label: "Overview", Href: "index.html"}}
	if !skip["readiness"] {
		nav = append(nav, projectreport.NavLink{Label: projectreport.KindLabel("readiness"), Href: "readiness.html"})
	}
	for _, p := range pages {
		if p.missing == "" {
			nav = append(nav, projectreport.NavLink{Label: projectreport.KindLabel(p.kind), Href: p.kind + ".html"})
		}
	}
	navFor := func(kind string) []projectreport.NavLink {
		out := append([]projectreport.NavLink(nil), nav...)
		for i := range out {
			out[i].Current = out[i].Href == kind+".html"
		}
		return out
	}
	project := reportProjectName(projectPath)
	var cards []projectreport.LinkCard
	for i := range pages {
		p := &pages[i]
		card := projectreport.LinkCard{Title: projectreport.KindLabel(p.kind), Href: p.kind + ".html"}
		if p.missing != "" {
			card.Missing = p.missing
			cards = append(cards, card)
			continue
		}
		switch {
		case p.quality != nil:
			ann, err := projectreport.LoadAnnotations(existing(filepath.Join(dir, "quality-annotations.yaml")))
			if err != nil {
				return fmt.Errorf("quality annotations: %w", err)
			}
			doc := projectreport.QualityDoc(p.quality, project, p.quality.Date, version, ann)
			doc.Nav = navFor(p.kind)
			p.doc = &doc
			if err := writeFileWith(filepath.Join(dir, "quality.json"), func(w io.Writer) error {
				return linter.GetReportFormatter("json").FormatReport(p.quality, w)
			}); err != nil {
				return err
			}
		default:
			ann, err := projectreport.LoadAnnotations(existing(filepath.Join(dir, p.kind+"-annotations.yaml")))
			if err != nil {
				return fmt.Errorf("%s annotations: %w", p.kind, err)
			}
			if err := writeFileWith(filepath.Join(dir, p.kind+".json"), p.env.WriteJSON); err != nil {
				return err
			}
			rk := reportKinds[p.kind]
			if rk.spec.renderHTML != nil {
				if err := writeFileWith(filepath.Join(dir, p.kind+".html"), func(w io.Writer) error { return rk.spec.renderHTML(w, p.env, ann) }); err != nil {
					return err
				}
			}
			doc, err := rk.spec.render(p.env, ann)
			if err != nil {
				return err
			}
			doc.Nav = navFor(p.kind)
			p.doc = &doc
		}
		if p.kind != "map" {
			if err := writeFileWith(filepath.Join(dir, p.kind+".html"), func(w io.Writer) error { return projectreport.RenderHTML(w, *p.doc) }); err != nil {
				return err
			}
		}
		card.Lede = p.doc.Lede
		if card.Lede == "" {
			card.Lede = p.doc.Description
		}
		card.Stats = firstStats(p.doc.Stats, 4)
		cards = append(cards, card)
	}

	// Readiness rolls the pages up, so it is written last and shown first.
	if !skip["readiness"] {
		fmt.Fprintln(log, "report readiness ...")
		card, err := writeReadiness(cmd, dir, project, pages, navFor("readiness"))
		if err != nil {
			return err
		}
		cards = append([]projectreport.LinkCard{card}, cards...)
	}

	index := projectreport.Doc{
		Title: project + " reports", Project: project, Kind: "Overview", Generated: reportNow(), Mxcli: version,
		Lede: "Every report on how the app is built and how it holds up. Each card is one report's verdict and headline numbers; " +
			"each page also has its data as JSON, to re-render with annotations (`mxcli report <kind> --from <kind>.json`).",
		Nav:      navFor("index"),
		Sections: []projectreport.Section{{ID: "reports", Title: "Reports", Blocks: []projectreport.Block{projectreport.Cards{Items: cards}}}},
	}
	index.Nav[0].Current = true
	if err := writeFileWith(filepath.Join(dir, "index.html"), func(w io.Writer) error { return projectreport.RenderHTML(w, index) }); err != nil {
		return err
	}
	fmt.Fprintf(log, "Wrote %s\n", filepath.Join(dir, "index.html"))
	return nil
}

// passThrough gives the kinds the inputs `report all` was given, each to the
// kinds it is for (ako/mxcli#2451):
//   - the functional evidence — the journey log and its spans, the microflow
//     test sources and results — to tests AND traceability: traceability
//     proves requirements from exactly that evidence;
//   - the performance run's own log and spans to performance only. A
//     functional log has one record per journey, which performance would read
//     as one "step" per journey, each over any step budget;
//   - --run runs the tests once, in the tests report, which writes the results
//     to <dir>/junit.xml for traceability to read after it.
func passThrough(cmd *cobra.Command, dir string, skip map[string]bool) {
	values := func(from string) []string {
		f := cmd.Flags().Lookup(from)
		if f == nil || !f.Changed {
			return nil
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			return sv.GetSlice()
		}
		return []string{f.Value.String()}
	}
	fwd := func(kind, from, to string, vals []string) {
		rk, ok := reportKinds[kind]
		if !ok || len(vals) == 0 {
			return
		}
		f := rk.cmd.Flags().Lookup(to)
		if f == nil {
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(vals)
		} else {
			_ = f.Value.Set(vals[0])
		}
		f.Changed = true
	}
	set := func(kind, flag string) { fwd(kind, flag, flag, values(flag)) }

	set("release", "against")
	for _, k := range []string{"tests", "traceability"} {
		set(k, "journeys")
		set(k, "spans")
		set(k, "tests")
		set(k, "junit")
	}
	fwd("performance", "perf-journeys", "journeys", values("perf-journeys"))
	fwd("performance", "perf-spans", "spans", values("perf-spans"))
	if run, _ := cmd.Flags().GetBool("run"); run && !skip["tests"] {
		set("tests", "run")
		set("tests", "test-arg")
		if len(values("junit")) == 0 && dir != "" {
			junit := filepath.Join(dir, "junit.xml")
			fwd("tests", "junit", "junit", []string{junit})
			fwd("traceability", "junit", "junit", []string{junit})
		}
	}
	if ex, _ := cmd.Flags().GetStringSlice("exclude-docs"); len(ex) > 0 {
		if rk, ok := reportKinds["architecture"]; ok {
			_ = rk.cmd.Flags().Set("exclude-docs", strings.Join(ex, ","))
		}
	}
	if off, _ := cmd.Flags().GetBool("offline"); off {
		if rk, ok := reportKinds["dependencies"]; ok {
			_ = rk.cmd.Flags().Set("offline", "true")
		}
	}
}

// needsInput says why a kind cannot run without inputs that were not given.
func needsInput(cmd *cobra.Command, kind string) string {
	get := func(f string) string { v, _ := cmd.Flags().GetString(f); return v }
	switch kind {
	case "performance":
		if get("perf-journeys") == "" {
			return "needs a performance run: --perf-journeys <log> --perf-spans <files>, or 'mxcli report performance' into this folder"
		}
	case "release":
		if get("against") == "" {
			return "needs the deployed revision: run with --against <tag, branch or .mpr>"
		}
	}
	return ""
}

func firstStats(s []projectreport.Stat, n int) []projectreport.Stat {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func existing(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func writeFileWith(path string, write func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func init() {
	reportAllCmd.Flags().StringP("project", "p", "", "Path to the .mpr file")
	reportAllCmd.Flags().StringP("output", "o", "", "Folder to write the reports into (created if needed)")
	reportAllCmd.Flags().String("milestone", "", "Readiness: the milestone being released, whose unbuilt requirements block")
	reportAllCmd.Flags().StringSlice("exclude-docs", nil, "Architecture: name globs of documents to leave out (generated code, probes, fixtures)")
	reportAllCmd.Flags().Bool("offline", false, "Ask no online source (dependencies: no Marketplace, no vulnerability lookup)")
	reportAllCmd.Flags().String("against", "", "The deployed revision or .mpr, for the release report")
	reportAllCmd.Flags().String("journeys", "", "Functional journey log, for the tests and traceability reports")
	reportAllCmd.Flags().StringSlice("spans", nil, "Span files of the functional journeys, for the tests and traceability reports")
	reportAllCmd.Flags().StringSlice("tests", nil, "Microflow test files or directories, for the tests and traceability reports (default: tests/ beside the project)")
	reportAllCmd.Flags().String("junit", "", "Microflow test results (JUnit, as `mxcli test --junit` writes it), for the tests and traceability reports")
	reportAllCmd.Flags().Bool("run", false, "Run the microflow tests first; the results go to <folder>/junit.xml for tests and traceability")
	reportAllCmd.Flags().StringArray("test-arg", nil, "With --run: extra argument for `mxcli test` (repeatable)")
	reportAllCmd.Flags().String("perf-journeys", "", "Journey log of a performance run (one record per step), for the performance report")
	reportAllCmd.Flags().StringSlice("perf-spans", nil, "Span files of the performance run, for the performance report")
	reportAllCmd.Flags().StringSlice("skip", nil, "Kinds to leave out (e.g. --skip map,dependencies)")
	reportCmd.AddCommand(reportAllCmd)
}

// writeReadiness rolls the generated pages up into readiness.json and
// readiness.html, signed off with readiness-annotations.yaml from the folder.
func writeReadiness(cmd *cobra.Command, dir, project string, pages []reportPage, nav []projectreport.NavLink) (projectreport.LinkCard, error) {
	card := projectreport.LinkCard{Title: projectreport.KindLabel("readiness"), Href: "readiness.html"}
	envs := map[string]*projectreport.Envelope{}
	var quality *linter.JSONReport
	for _, p := range pages {
		switch {
		case p.quality != nil:
			q, err := qualityJSON(p.quality)
			if err != nil {
				return card, err
			}
			quality = q
		case p.env != nil:
			envs[p.kind] = p.env
		}
	}
	ann := map[string]*projectreport.Annotations{}
	for _, k := range projectreport.ReadinessKinds {
		a, err := projectreport.LoadAnnotations(existing(filepath.Join(dir, k+"-annotations.yaml")))
		if err != nil {
			return card, fmt.Errorf("%s annotations: %w", k, err)
		}
		ann[k] = a
	}
	in, err := projectreport.ReadinessInputFrom(envs, quality, ann)
	if err != nil {
		return card, err
	}
	signOff, err := projectreport.LoadAnnotations(existing(filepath.Join(dir, "readiness-annotations.yaml")))
	if err != nil {
		return card, fmt.Errorf("readiness annotations: %w", err)
	}
	in.SignOff = signOff
	in.Milestone, _ = cmd.Flags().GetString("milestone")
	in.BudgetMs = 1000
	if rk, ok := reportKinds["readiness"]; ok {
		in.BudgetMs, _ = rk.cmd.Flags().GetFloat64("budget-ms")
	}
	env, err := projectreport.NewEnvelope("readiness", project, reportNow(), version, projectreport.ComputeReadiness(in),
		projectreport.ReadinessMethod, projectreport.ReadinessLimits, nil)
	if err != nil {
		return card, err
	}
	if err := writeFileWith(filepath.Join(dir, "readiness.json"), env.WriteJSON); err != nil {
		return card, err
	}
	doc, err := projectreport.RenderReadiness(env, signOff)
	if err != nil {
		return card, err
	}
	doc.Nav = nav
	if err := writeFileWith(filepath.Join(dir, "readiness.html"), func(w io.Writer) error { return projectreport.RenderHTML(w, doc) }); err != nil {
		return card, err
	}
	card.Lede, card.Stats = doc.Lede, firstStats(doc.Stats, 4)
	return card, nil
}
