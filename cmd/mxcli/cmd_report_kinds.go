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
	// renderHTML, when set, replaces the shared page for --format html: a
	// kind whose page is interactive rather than a document.
	renderHTML func(w io.Writer, env *projectreport.Envelope, ann *projectreport.Annotations) error
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
	} else if format == "html" && spec.renderHTML != nil {
		err = spec.renderHTML(out, env, ann)
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
	// Findings a brain decision accepts are marked, so the rule's verdict
	// comes from the brain rather than a second copy in --annotations
	// (ako/mxcli#2304).
	acceptances := loadBrainAcceptances(projectDir)
	open := acceptances.apply(vs)
	var out []projectreport.LintFinding
	add := func(v linter.Violation, by string) {
		if v.RuleFailure || linter.CategoryOf(v.RuleID) != "Security" {
			return
		}
		where := v.Location.Module
		if v.Location.DocumentName != "" {
			where = strings.TrimPrefix(v.Location.Module+"."+v.Location.DocumentName, ".")
		}
		out = append(out, projectreport.LintFinding{Rule: v.RuleID, Severity: v.Severity.String(), Message: v.Message, Where: where, AcceptedBy: by})
	}
	for _, v := range open {
		add(v, "")
	}
	for _, a := range acceptances.accepted {
		add(a.Violation, a.By.EntryID)
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
Milestones come from the plan ('mxcli brain milestone "M1" --slice 01-accounts');
without any, each slice is a milestone. A milestones file (--milestones, or the
older docs/brain/plan/traceability.json) is still read, with a warning;
'mxcli brain milestone --import <file>' moves it into the plan. Evidence flags
are the tests report's.

Annotation keys: req:<id>.`,
	example: `  mxcli report traceability -p app.mpr -o traceability.html
  mxcli report traceability -p app.mpr --junit results.xml --journeys journeys.jsonl --spans spans.jsonl -o trace.html`,
	flags: func(cmd *cobra.Command) {
		scopeFlags(cmd)
		evidenceFlags(cmd, true)
		cmd.Flags().String("milestones", "", "Milestones file, instead of the milestones in the plan (deprecated: mxcli brain milestone --import moves it into the plan)")
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
			return nil, noRequirementsError(filepath.Join(loc.Dir, brain.StoreDir, "plan"), projectPath)
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
		ms, msWarnings, err := traceabilityMilestones(cmd, store, loc.Dir)
		if err != nil {
			return nil, err
		}
		in.Milestones = ms
		warnings = append(warnings, msWarnings...)
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

// noRequirementsError is what the traceability report says on a brain with no
// plan: how to capture requirements and commit them.
func noRequirementsError(planDir, projectPath string) error {
	return fmt.Errorf("no requirements: %s has no plan slices.\n"+
		"Capture the specification's requirements first, one per line of the PRD, anchored where they will be built:\n"+
		"  mxcli brain capture \"<requirement> (p.3)\" --slice 01-<name> -a @Module.Element -p %s\n"+
		"  mxcli brain staged -p %s\n"+
		"  mxcli brain promote <id>... -p %s", planDir, projectPath, projectPath, projectPath)
}

// traceabilityMilestones reads the milestones from the plan, where the slices
// carry them. A --milestones file still works, and so does the old
// docs/brain/plan/traceability.json when the plan names no milestone, each with
// a warning that says how to move it into the plan.
func traceabilityMilestones(cmd *cobra.Command, store *brain.Store, dir string) (*projectreport.Milestones, []string, error) {
	if path, _ := cmd.Flags().GetString("milestones"); path != "" {
		ms, err := projectreport.ReadMilestones(path)
		return ms, []string{"Milestones were read from " + path + ". The plan can carry them instead: mxcli brain milestone --import " + path}, err
	}
	planned, err := store.Milestones()
	if err != nil {
		return nil, nil, err
	}
	old := filepath.Join(dir, brain.StoreDir, "plan", "traceability.json")
	if len(planned) == 0 {
		if !reportFileExists(old) {
			return nil, nil, nil
		}
		ms, err := projectreport.ReadMilestones(old)
		return ms, []string{"Milestones were read from " + old + ", which is deprecated. Move them into the plan: mxcli brain milestone --import " + old}, err
	}
	var warnings []string
	if reportFileExists(old) {
		warnings = append(warnings, "Ignored "+old+": the plan names milestones, which take precedence. Delete the file.")
	}
	out := &projectreport.Milestones{}
	for _, m := range planned {
		out.Milestones = append(out.Milestones, projectreport.MilestoneSpec{Name: m.Name, Slices: m.Slices, Requirements: m.Requirements})
	}
	return out, warnings, nil
}

func reportFileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func init() {
	reportCmd.AddCommand(reportTraceabilityCmd)
}

// ---------------------------------------------------------------- map

var reportMapCmd = newReportKindCmd(reportKindSpec{
	name:  "map",
	short: "Software map: an interactive guide to what the app contains (modules, data, logic, page flows, APIs, index)",
	long: `One page that helps someone understand an app they did not build, generated
from the catalog. Where 'report architecture' judges structure, the map
describes what is there and how it fits together:

  Landscape            user roles and where each starts; the systems around the
                       app; what runs without a user
  Features             with a project brain plan: one feature per slice, with
                       its requirements, milestone and what its anchors reach
                       (flows, pages, entities, entry points, roles), and what
                       no feature reaches
  Module map           app modules as a dependency graph, domain use apart from
                       screen links, the hub and weak links behind toggles
  Inside a module      data model (other modules' entities as ghosts, the flows
                       that write each entity), entry points by what starts
                       them, the call closure of one entry point, pages, Java
                       actions
  Page flows           per role: home pages and menu, then show-page actions and
                       the flows behind buttons, limited to what the role may
                       open; pages it may open that no path reaches
  Data across modules  every association crossing a module boundary
  Conventions          naming prefixes, flows shared by many modules, entry
                       points × module
  APIs & background    published operations with their call closure, consumed
                       services and HTTP calls, queues, schedules, Java
                       actions, constants
  Index                every document, searchable, with uses and used by

--format html is the interactive page (self-contained: no requests, diagrams
drawn in the browser, one module or entry point at a time). --format json is
the same data for another viewer (an IDE panel); markdown is a summary.

Module purposes come from each module's domain-model documentation; give the
modules that have none one sentence each in --annotations:
  items:
    "module:Rooster": {note: "Builds and publishes the monthly roster."}`,
	example: `  mxcli report map -p app.mpr -o map.html
  mxcli report map -p app.mpr --format json -o map.json
  mxcli report map --from map.json --annotations purposes.yaml -o map.html`,
	flags: scopeFlags,
	collect: func(cmd *cobra.Command, projectPath string) (*projectreport.Envelope, error) {
		exec, cleanup, err := openReportCatalog(projectPath, "full")
		if err != nil {
			return nil, err
		}
		defer cleanup()
		data, err := projectreport.CollectMap(exec.Catalog(), scopeOptions(cmd), nil)
		if err != nil {
			return nil, err
		}
		trace, sliceMilestones, warnings, err := mapFeatureTrace(cmd, projectPath, exec)
		if err != nil {
			return nil, err
		}
		projectreport.AddFeatures(data, trace, sliceMilestones)
		return projectreport.NewEnvelope("map", reportProjectName(projectPath), reportNow(), version, data,
			projectreport.MapMethod, projectreport.MapLimits, warnings)
	},
	render:     projectreport.RenderMap,
	renderHTML: projectreport.RenderMapHTML,
})

// mapFeatureTrace reads the project brain's plan, when there is one, as the
// traceability report does — the same statuses, footprints and milestones — so
// the map can show the app by feature. No brain, or no slices, is no feature
// view, not an error.
func mapFeatureTrace(cmd *cobra.Command, projectPath string, exec *executor.Executor) (*projectreport.TraceabilityData, map[string]string, []string, error) {
	loc, err := resolveBrainLocation(projectPath, "")
	if err != nil {
		return nil, nil, nil, nil
	}
	store := brain.NewStore(loc.Dir)
	if !store.Exists() {
		return nil, nil, nil, nil
	}
	sliceShards, err := store.ListSlices()
	if err != nil || len(sliceShards) == 0 {
		return nil, nil, nil, err
	}
	rep, err := brain.Check(store, &catalogResolver{cat: exec.Catalog(), be: exec.Backend()}, sliceShards)
	if err != nil {
		return nil, nil, nil, err
	}
	list, err := store.List(brain.ListFilter{})
	if err != nil {
		return nil, nil, nil, err
	}
	ms, warnings, err := traceabilityMilestones(cmd, store, loc.Dir)
	if err != nil {
		return nil, nil, nil, err
	}
	sliceMilestones := map[string]string{}
	if ms != nil {
		for _, m := range ms.Milestones {
			for _, sl := range m.Slices {
				sliceMilestones[sl] = m.Name
			}
		}
	}
	trace, err := projectreport.CollectTraceability(exec.Catalog(), projectreport.TraceabilityInput{
		Requirements: rep.Requirements, Entries: list.Entries, Milestones: ms,
	})
	return trace, sliceMilestones, warnings, err
}

func init() {
	reportCmd.AddCommand(reportMapCmd)
}
