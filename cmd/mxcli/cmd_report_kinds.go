// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/mendixlabs/mxcli/cmd/mxcli/projectreport"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

// The `mxcli report <kind>` family (ako/mxcli#2221). Bare `mxcli report` stays
// the scored best-practices report; each kind here is a subcommand that
// collects data, wraps it in an envelope stating method and limits, and renders
// markdown or a standalone HTML page from that envelope.
//
// Every kind shares four flags:
//
//	--format html|markdown|json   json is the collected data; the page is rendered from it
//	-o <file>                     where to write; the format follows the extension when --format is not given
//	--annotations <file>          a person's names, verdicts and notes, kept across re-renders
//	--from <report.json>          re-render a saved report without measuring again

// reportKindSpec is what a kind supplies to the shared command.
type reportKindSpec struct {
	name, short, long, example string
	// collect measures and returns the envelope. It is not called with --from.
	collect func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error)
	render  func(env *projectreport.Envelope, ann *projectreport.Annotations) (projectreport.Doc, error)
	// flags adds the kind's own flags.
	flags func(cmd *cobra.Command)
}

func newReportKindCmd(spec reportKindSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:     spec.name,
		Short:   spec.short,
		Long:    spec.long + "\n\n" + reportSharedHelp,
		Example: spec.example,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := runReportKind(cmd, spec); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringP("format", "f", "", "Output format: html, markdown or json (default: from the -o extension, else markdown)")
	cmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
	cmd.Flags().String("annotations", "", "JSON or YAML file with names, verdicts and notes keyed by the keys the page prints")
	cmd.Flags().String("from", "", "Render a report saved with --format json instead of measuring again")
	if spec.flags != nil {
		spec.flags(cmd)
	}
	return cmd
}

const reportSharedHelp = `Shared flags: --format html|markdown|json, -o <file>, --annotations <file>,
--from <report.json>. The json format is the collected data; html and markdown
are rendered from it, so 'report <kind> --format json -o r.json' followed by
'report <kind> --from r.json --annotations notes.yaml -o r.html' adds a
person's judgement without measuring again. Every page states how each number
was obtained and what the report does not cover.

Annotations file (YAML or JSON):
  title: Optional page title
  summary: A paragraph shown first
  items:
    "cluster:Sales.Order": {name: "Order intake", note: "..."}
    "finding:constant-secret:Crypto.Key": {status: fixed, note: "moved to env"}
  recommendations: ["...", "..."]`

func runReportKind(cmd *cobra.Command, spec reportKindSpec) error {
	outputPath, _ := cmd.Flags().GetString("output")
	format, _ := cmd.Flags().GetString("format")
	if globalJSONFlag {
		format = "json"
	}
	if format == "" {
		switch strings.ToLower(filepath.Ext(outputPath)) {
		case ".html", ".htm":
			format = "html"
		case ".json":
			format = "json"
		default:
			format = "markdown"
		}
	}
	if format == "md" {
		format = "markdown"
	}
	if format != "html" && format != "markdown" && format != "json" {
		return fmt.Errorf("unknown --format %q (want html, markdown or json)", format)
	}
	annPath, _ := cmd.Flags().GetString("annotations")
	ann, err := projectreport.LoadAnnotations(annPath)
	if err != nil {
		return fmt.Errorf("reading annotations: %w", err)
	}

	var env *projectreport.Envelope
	if from, _ := cmd.Flags().GetString("from"); from != "" {
		if env, err = projectreport.ReadEnvelope(from, spec.name); err != nil {
			return err
		}
	} else {
		projectPath, _ := cmd.Flags().GetString("project")
		if env, err = spec.collect(cmd, projectPath); err != nil {
			return err
		}
	}

	var out io.Writer = os.Stdout
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	if format == "json" {
		err = env.WriteJSON(out)
	} else {
		var doc projectreport.Doc
		if doc, err = spec.render(env, ann); err != nil {
			return err
		}
		err = projectreport.Render(out, format, doc)
	}
	if err != nil {
		return err
	}
	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Wrote %s\n", outputPath)
	}
	return nil
}

// openReportCatalog connects to the project and refreshes the catalog in the
// given modes ("full", "source", "communities"), in order. Progress goes to
// stderr: stdout may carry the report.
func openReportCatalog(projectPath string, modes ...string) (*executor.Executor, func(), error) {
	if projectPath == "" {
		return nil, nil, fmt.Errorf("--project (-p) is required (or --from <report.json> to re-render)")
	}
	exec, logger := newLoggedExecutorTo("subcommand", os.Stderr)
	cleanup := func() { logger.Close(); exec.Close() }
	stmts := []string{fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath))}
	for _, m := range modes {
		stmts = append(stmts, "REFRESH CATALOG "+strings.ToUpper(m))
	}
	for _, src := range stmts {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			cleanup()
			return nil, nil, errs[0]
		}
		for _, stmt := range prog.Statements {
			if err := exec.Execute(stmt); err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("%s: %w", src, err)
			}
		}
	}
	if exec.Catalog() == nil {
		cleanup()
		return nil, nil, fmt.Errorf("catalog not built")
	}
	return exec, cleanup, nil
}

func reportProjectName(projectPath string) string {
	return strings.TrimSuffix(filepath.Base(projectPath), filepath.Ext(projectPath))
}

func reportNow() string { return time.Now().Format("2006-01-02 15:04") }

// settingsMicroflows names the microflows project settings run: the
// reference graph records none of them.
func settingsMicroflows(exec *executor.Executor) []string {
	ps, err := exec.Backend().GetProjectSettings()
	if err != nil || ps == nil || ps.Model == nil {
		return nil
	}
	var out []string
	for _, mf := range []string{ps.Model.AfterStartupMicroflow, ps.Model.BeforeShutdownMicroflow, ps.Model.HealthCheckMicroflow} {
		if mf != "" {
			out = append(out, mf)
		}
	}
	return out
}

func scopeFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("include-marketplace", false, "Count Marketplace modules as part of the app")
	cmd.Flags().StringSliceP("exclude", "e", nil, "Modules to leave out of the app")
	cmd.Flags().StringSliceP("modules", "m", nil, "Only these modules are the app")
}

func scopeOptions(cmd *cobra.Command) projectreport.ScopeOptions {
	inc, _ := cmd.Flags().GetBool("include-marketplace")
	ex, _ := cmd.Flags().GetStringSlice("exclude")
	only, _ := cmd.Flags().GetStringSlice("modules")
	return projectreport.ScopeOptions{IncludeMarketplace: inc, Exclude: ex, Only: only}
}

// ---------------------------------------------------------------- architecture

var reportArchitectureCmd = newReportKindCmd(reportKindSpec{
	name:  "architecture",
	short: "Layered module map, coupling, bounded contexts and cycles of the app's own modules",
	long: `Turn the catalog's reference graph into an architecture page: each app
module on its layer, module health (cohesion, fan-in/out, size), the coupling
matrix in layer order, bounded contexts (Leiden clusters × modules), god nodes,
bridges and entity hotspots, cycles with the edges that close them, and unused
documents with a second look at whether they really are.

Domain and navigation dependencies are reported apart: menus, layouts and page
links put every screen module in one cycle, so layers and cycles are computed
from domain edges only. Marketplace modules are outside the app unless
--include-marketplace.

Annotation keys: module:<Name>, cluster:<key member>, cycle:<A+B>, unused:<Name>.`,
	example: `  mxcli report architecture -p app.mpr -o architecture.html
  mxcli report architecture -p app.mpr --format json -o arch.json
  mxcli report architecture --from arch.json --annotations arch-notes.yaml -o architecture.html`,
	flags: func(cmd *cobra.Command) {
		scopeFlags(cmd)
		cmd.Flags().Int("top", 15, "Rows in the god node, bridge and hotspot tables")
		cmd.Flags().Bool("no-communities", false, "Skip the clustering pass (no bounded-context section)")
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		modes := []string{"full"}
		if skip, _ := cmd.Flags().GetBool("no-communities"); !skip {
			modes = []string{"communities"}
		}
		exec, cleanup, err := openReportCatalog(projectPath, modes...)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		top, _ := cmd.Flags().GetInt("top")
		data, err := projectreport.CollectArchitecture(exec.Catalog(), projectreport.ArchitectureOptions{
			Scope: scopeOptions(cmd), Top: top, SettingsMicroflows: settingsMicroflows(exec),
		})
		if err != nil {
			return nil, err
		}
		return projectreport.NewEnvelope("architecture", reportProjectName(projectPath), reportNow(), version, data,
			projectreport.ArchitectureMethod, projectreport.ArchitectureLimits, nil)
	},
	render: projectreport.RenderArchitecture,
})

func init() {
	reportCmd.AddCommand(reportArchitectureCmd)
}

// ---------------------------------------------------------------- security

var reportSecurityCmd = newReportKindCmd(reportKindSpec{
	name:  "security",
	short: "Security review: access grid, caller-trusting flows, unsafe defaults, lint verdicts",
	long: `A security review of the model that says what protects the data, what does
not, and what to fix — instead of a score.

  - Findings by severity, each with a fix and a status (open, fixed, accepted)
    from --annotations, and the controls that hold.
  - Who can read what: user role × persistent entity, each access rule's XPath
    classified as own rows, via user, filtered or all.
  - Client-callable flows: microflows granted to a role that take a persistent
    object, triaged by whether they check the caller in their own body or rely
    on access rules. A flow that takes the acting user as a parameter is
    flagged: an object parameter is a plain id and can be substituted.
  - Unsafe defaults: constants whose default looks like key material or is
    named like a secret, test switches that default to true, demo users at
    Production, guest access, strict mode, password policy.
  - Lint security rules grouped, each with a verdict slot (intended, accepted,
    real).

It is a model review, not a penetration test; the page lists what it does not
cover. Annotation keys: finding:<id>, flow:<Microflow>, rule:<ID>.`,
	example: `  mxcli report security -p app.mpr -o security.html
  mxcli report security -p app.mpr --format json -o sec.json
  mxcli report security --from sec.json --annotations sec-status.yaml -o security.html`,
	flags: func(cmd *cobra.Command) {
		scopeFlags(cmd)
		cmd.Flags().Bool("no-lint", false, "Skip the lint security rules")
		cmd.Flags().Bool("no-source", false, "Do not index microflow bodies (faster; no flow can then be seen to check its caller)")
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		modes := []string{"full"}
		if noSrc, _ := cmd.Flags().GetBool("no-source"); !noSrc {
			modes = append(modes, "source")
		}
		exec, cleanup, err := openReportCatalog(projectPath, modes...)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		var warnings []string
		settings := projectreport.SecuritySettings{}
		if ps, err := exec.Backend().GetProjectSecurity(); err == nil && ps != nil {
			settings = projectreport.SecuritySettings{Known: true, Level: ps.SecurityLevel, StrictMode: ps.StrictMode,
				DemoUsersEnabled: ps.EnableDemoUsers, GuestAccess: ps.EnableGuestAccess, GuestRole: ps.GuestUserRole,
				AdminUser: ps.AdminUserName, AdminRole: ps.AdminUserRole}
			if ps.PasswordPolicy != nil {
				settings.PasswordMinLength = ps.PasswordPolicy.MinimumLength
			}
			for _, u := range ps.DemoUsers {
				settings.DemoUsers = append(settings.DemoUsers, projectreport.DemoUser{Name: u.UserName, Roles: u.UserRoles})
			}
			for _, r := range ps.UserRoles {
				if r.ManageAllRoles {
					settings.AdminRoles = append(settings.AdminRoles, r.Name)
				}
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("Project security settings could not be read (%v); the settings checks are skipped.", err))
		}
		var lint []projectreport.LintFinding
		if noLint, _ := cmd.Flags().GetBool("no-lint"); !noLint {
			lint, err = securityLintFindings(exec, projectPath, scopeOptions(cmd))
			if err != nil {
				warnings = append(warnings, "Lint did not run: "+err.Error())
			}
		}
		data, err := projectreport.CollectSecurity(exec.Catalog(), projectreport.SecurityInput{
			Scope: scopeOptions(cmd), Settings: settings, Lint: lint,
		})
		if err != nil {
			return nil, err
		}
		return projectreport.NewEnvelope("security", reportProjectName(projectPath), reportNow(), version, data,
			projectreport.SecurityMethod, projectreport.SecurityLimits, warnings)
	},
	render: projectreport.RenderSecurity,
})

// securityLintFindings runs the project's lint rules (with its lint config) and
// keeps the ones the scored report counts as security.
func securityLintFindings(exec *executor.Executor, projectPath string, scope projectreport.ScopeOptions) ([]projectreport.LintFinding, error) {
	ctx := linter.NewLintContext(exec.Catalog(), exec.Backend())
	ctx.SetExcludedModules(scope.Exclude)
	if len(scope.Only) > 0 {
		ctx.SetIncludedModules(scope.Only)
	}
	projectDir := filepath.Dir(projectPath)
	lint := linter.New(ctx)
	for _, rule := range projectLintRules(projectDir, io.Discard) {
		lint.AddRule(rule)
	}
	applyLintConfig(lint, projectDir, io.Discard)
	vs, err := lint.Run(context.Background())
	if err != nil {
		return nil, err
	}
	var out []projectreport.LintFinding
	for _, v := range vs {
		if v.RuleFailure || linter.CategoryOf(v.RuleID) != "Security" {
			continue
		}
		where := v.Location.Module
		if v.Location.DocumentName != "" {
			where = strings.TrimPrefix(v.Location.Module+"."+v.Location.DocumentName, ".")
		}
		out = append(out, projectreport.LintFinding{Rule: v.RuleID, Severity: v.Severity.String(), Message: v.Message, Where: where})
	}
	return out, nil
}

func init() {
	reportCmd.AddCommand(reportSecurityCmd)
}

// ---------------------------------------------------------------- performance

var reportPerformanceCmd = newReportKindCmd(reportKindSpec{
	name:  "performance",
	short: "Journeys at production volume: per-step server and database time, flame charts, plans, before/after",
	long: `Answer "will this app be fast with real data volumes?" from a measured run:
each journey step's browser, server and database time and query count (median
of the warm runs), a flame chart of its spans, the most frequent statements,
Postgres plans with their JIT cost, and — with --baseline — before and after.

The protocol (seed an app-shaped volume dated around today, back up first,
instrument Postgres with pg_stat_statements and auto_explain, warm up, run each
step several times, restore everything) is in the performance-report skill.
This command measures and renders; it does not change the database.

Inputs: --journeys (a JSONL log with one record per step and run, each with
its time window) and --spans (from 'mxcli report collect-spans'), or
--journeys-cmd to run the journey script with a collector listening. -p adds
the model's N+1 candidates (retrieve in a loop, list data sources).

Annotation keys: step:<journey / step>, candidate:<Microflow>.`,
	example: `  mxcli report performance --journeys-cmd "node perf/journeys.mjs" -p app.mpr --plans pg.log -f json -o after.json
  mxcli report performance --journeys steps.jsonl --spans spans.jsonl --baseline before.json -o perf.html
  mxcli report performance --from after.json --annotations perf-notes.yaml -o perf.html`,
	flags: func(cmd *cobra.Command) {
		scopeFlags(cmd)
		evidenceFlags(cmd, false)
		cmd.Flags().String("plans", "", "Postgres log with auto_explain output in JSON format")
		cmd.Flags().String("baseline", "", "An earlier `report performance --format json` of the same journeys: the before")
		cmd.Flags().Int("top", 8, "Microflows and statements listed per step")
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		ev, err := collectEvidence(cmd, projectPath, false)
		if err != nil {
			return nil, err
		}
		in := projectreport.PerformanceInput{Journeys: ev.journeys, Spans: ev.spans}
		in.Top, _ = cmd.Flags().GetInt("top")
		var warnings []string
		if len(ev.spans) == 0 {
			warnings = append(warnings, "No spans were read: server and database times are empty. Run the app with `mxcli run --local --trace-otlp http://127.0.0.1:4318` while the collector listens.")
		}
		if p, _ := cmd.Flags().GetString("plans"); p != "" {
			if in.Plans, err = projectreport.ReadAutoExplain(p); err != nil {
				return nil, err
			}
		}
		if b, _ := cmd.Flags().GetString("baseline"); b != "" {
			if in.Baseline, err = projectreport.ReadEnvelope(b, "performance"); err != nil {
				return nil, err
			}
		}
		name := "journeys"
		if projectPath != "" {
			name = reportProjectName(projectPath)
			exec, cleanup, err := openReportCatalog(projectPath, "full")
			if err != nil {
				return nil, err
			}
			defer cleanup()
			scope, err := projectreport.LoadScope(exec.Catalog(), scopeOptions(cmd))
			if err != nil {
				return nil, err
			}
			if in.Candidates, err = projectreport.CollectCandidates(exec.Catalog(), scope); err != nil {
				return nil, err
			}
		}
		data, err := projectreport.CollectPerformance(in)
		if err != nil {
			return nil, err
		}
		return projectreport.NewEnvelope("performance", name, reportNow(), version, data,
			projectreport.PerformanceMethod, projectreport.PerformanceLimits, append(warnings, ev.warnings...))
	},
	render: projectreport.RenderPerformance,
})

func init() {
	reportCmd.AddCommand(reportPerformanceCmd)
}

// ---------------------------------------------------------------- traceability

var reportTraceabilityCmd = newReportKindCmd(reportKindSpec{
	name:  "traceability",
	short: "Requirements per milestone: built, partial, changed or not built, with what implements and proves each",
	long: `How a specification turned into the app: every requirement in the project
brain, grouped by milestone, with

  - status: built / partial / changed / not built / not traceable — derived
    from its anchors and from decisions that say "Changes requirement ` + "`<id>`" + `: …"
    or "Partly meets requirement ` + "`<id>`" + `: …", never ticked off by hand;
  - implementation: the microflows that implement it (its footprint);
  - proof: the microflow tests and browser journeys that reach the footprint;
  - context: the decisions and open questions that bear on it;

and three gap lists: not fully built as asked, built but untested, and open
questions that affect a requirement.

It needs requirements in docs/brain/plan/ (see 'mxcli brain capture --slice').
Milestones come from docs/brain/plan/traceability.json, or --milestones:
  {"milestones": [{"name": "M1", "slices": ["01-accounts"], "requirements": ["a1b2c3"]}]}
Without it, each slice is a milestone. Evidence flags are the tests report's.

Annotation keys: req:<id>.`,
	example: `  mxcli report traceability -p app.mpr -o traceability.html
  mxcli report traceability -p app.mpr --junit results.xml --journeys journeys.jsonl --spans spans.jsonl -o trace.html`,
	flags: func(cmd *cobra.Command) {
		scopeFlags(cmd)
		evidenceFlags(cmd, true)
		cmd.Flags().String("milestones", "", "Milestones file (default: docs/brain/plan/traceability.json)")
	},
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		loc, err := resolveBrainLocation(projectPath, "")
		if err != nil {
			return nil, err
		}
		store := brain.NewStore(loc.Dir)
		sliceShards, err := store.ListSlices()
		if err != nil {
			return nil, err
		}
		if len(sliceShards) == 0 {
			return nil, fmt.Errorf("no requirements: %s has no plan slices.\n"+
				"Capture the specification's requirements first, one per line of the PRD, anchored where they will be built:\n"+
				"  mxcli brain capture \"<requirement> (p.3)\" --slice 01-<name> -a @Module.Element -p %s\n"+
				"  mxcli brain promote --all -p %s", filepath.Join(loc.Dir, brain.StoreDir, "plan"), projectPath, projectPath)
		}
		var warnings []string
		in := projectreport.TraceabilityInput{}
		var resolver brain.Resolver = brain.NoModel{}
		var q projectreport.Querier
		modelPresent := loc.MPR != "" && reportFileExists(loc.MPR)
		if modelPresent {
			exec, cleanup, err := openReportCatalog(loc.MPR, "full")
			if err != nil {
				return nil, err
			}
			defer cleanup()
			resolver = &catalogResolver{cat: exec.Catalog(), be: exec.Backend()}
			q = exec.Catalog()
			ev, err := collectEvidence(cmd, loc.MPR, true)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, ev.warnings...)
			if len(ev.sources) > 0 || len(ev.results) > 0 || ev.journeys != nil {
				in.Evidence, err = projectreport.CollectTests(q, projectreport.TestsInput{
					Scope: scopeOptions(cmd), Sources: ev.sources, Results: ev.results,
					Journeys: ev.journeys, Spans: ev.spans, StartupFlows: settingsMicroflows(exec),
				})
				if err != nil {
					return nil, err
				}
			}
		}
		in.ModelAbsent = !modelPresent
		rep, err := brain.Check(store, resolver, sliceShards)
		if err != nil {
			return nil, err
		}
		// The plan's own count is the cross-check: a report that disagrees with
		// `brain plan` about what is built is wrong, whichever of them is.
		built := 0
		for _, sl := range rep.Slices {
			built += sl.Built
		}
		reqBuilt := 0
		for _, r := range rep.Requirements {
			if r.Built {
				reqBuilt++
			}
		}
		if built != reqBuilt {
			return nil, fmt.Errorf("internal disagreement: brain plan counts %d built, per-requirement results %d — refusing to report", built, reqBuilt)
		}
		in.Requirements = rep.Requirements
		list, err := store.List(brain.ListFilter{})
		if err != nil {
			return nil, err
		}
		for _, m := range list.Malformed {
			warnings = append(warnings, "Malformed brain entry left out: "+m)
		}
		in.Entries = list.Entries
		msPath, _ := cmd.Flags().GetString("milestones")
		if msPath == "" {
			if def := filepath.Join(loc.Dir, brain.StoreDir, "plan", "traceability.json"); reportFileExists(def) {
				msPath = def
			}
		}
		if msPath != "" {
			if in.Milestones, err = projectreport.ReadMilestones(msPath); err != nil {
				return nil, err
			}
		}
		data, err := projectreport.CollectTraceability(q, in)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(loc.Dir)
		if loc.MPR != "" {
			name = reportProjectName(loc.MPR)
		}
		return projectreport.NewEnvelope("traceability", name, reportNow(), version, data,
			projectreport.TraceabilityMethod, projectreport.TraceabilityLimits, warnings)
	},
	render: projectreport.RenderTraceability,
})

func reportFileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func init() {
	reportCmd.AddCommand(reportTraceabilityCmd)
}
