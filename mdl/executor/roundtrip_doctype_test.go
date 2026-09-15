// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// gateEngine pairs an engine name with its backend factory. The gate runs every
// doctype script through exec + mx check on every engine in the matrix.
//
// The matrix has ONE entry since the legacy sdk/mpr engine was deleted
// (docs/plans/2026-09-14-retire-legacy-engine.md). It is kept as a matrix rather
// than collapsed into a single call for two reasons, neither of them symmetry:
// it is the seam a future backend plugs into, and — the load-bearing one — the
// selection below still turns a stale `MXCLI_TEST_ENGINES=legacy` into a loud
// failure instead of a gate that runs nothing and reports success.
type gateEngine struct {
	name    string
	factory func() backend.FullBackend
}

var allGateEngines = []gateEngine{
	{"modelsdk", func() backend.FullBackend { return modelsdkbackend.New() }},
}

// gateEnginesEnv narrows the matrix above to a subset, as a comma- or
// space-separated list of engine names; empty or "all" means every engine.
//
// With a one-engine matrix it narrows nothing, and what remains is the guard:
// a name that is not in the matrix is REPORTED, so a workflow or a shell still
// exporting MXCLI_TEST_ENGINES=legacy fails in TestMain rather than selecting
// zero engines and reporting the gate green. That is why the deletion of legacy
// left this in place instead of taking it along.
//
// The DEFAULT is every engine, deliberately: a workflow file that forgets to set
// this loses minutes, never coverage. A lost gate is the failure this repo has
// already shipped once (#808, an integration test that had only ever skipped),
// so the failure mode is biased the other way on purpose.
//
// One limit on TestMain's announcement of a narrowed matrix, measured rather
// than assumed: `go test` without -v DISCARDS a passing package's output, so the
// notice does not reach a green CI log — only a -v run or a FAILING package
// shows it. The fatal path is unaffected (an unknown name exits non-zero, and a
// failing package's output is shown).
const gateEnginesEnv = "MXCLI_TEST_ENGINES"

// gateEngines is the matrix every gate test loops over.
var gateEngines, unknownGateEngines = selectGateEngines(os.Getenv(gateEnginesEnv), allGateEngines)

// selectGateEngines filters the engine matrix by name, preserving matrix order
// so the subset runs in the same sequence as the whole. Unrecognised names are
// returned rather than ignored: a typo that silently selects NOTHING would turn
// the gate into a no-op that still reports success, which is the one outcome a
// gate must never have.
func selectGateEngines(spec string, all []gateEngine) (selected []gateEngine, unknown []string) {
	fields := strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return all, nil
	}
	wanted := make(map[string]bool, len(fields))
	for _, f := range fields {
		name := strings.ToLower(strings.TrimSpace(f))
		if name == "all" {
			return all, nil
		}
		wanted[name] = true
	}
	for _, eng := range all {
		if wanted[eng.name] {
			selected = append(selected, eng)
			delete(wanted, eng.name)
		}
	}
	for name := range wanted {
		unknown = append(unknown, name)
	}
	sort.Strings(unknown)
	return selected, unknown
}

// gateEngineNames renders an engine list for a log line.
func gateEngineNames(engines []gateEngine) string {
	names := make([]string, 0, len(engines))
	for _, e := range engines {
		names = append(names, e.name)
	}
	return strings.Join(names, ", ")
}

// engineScriptSkip marks (engine/script) pairs to skip, with a reason. Use only
// for a script that fails on ONE engine for a tracked, not-yet-actionable reason
// (e.g. a known modelsdk gap on a specific document type). A script broken on
// BOTH engines belongs in scriptSkipList instead. Key format: "<engine>/<file>".
var engineScriptSkip = map[string]string{
	// (modelsdk/06b-soap-examples.mdl was skipped here until the codec engine
	// learned to write Microflows$CallWebServiceAction. It ran on BOTH engines
	// from then on, which is the point of the removal: SOAP was the documented
	// reason the legacy engine still had to exist.)
	// The legacy widget builder has no `barchart` pluggable-widget template, so
	// page build fails ("template not found: barchart"). Passes on modelsdk.
	"legacy/34-chart-widget-examples.mdl": "legacy widget builder lacks the barchart template (works on modelsdk); tracked",
	// Menu-document authoring is modelsdk-only *by design*, not a gap: Studio Pro
	// stores a menu document's item lists with typed-array marker 3, which the
	// codec emits by default, while the legacy navigation writer hand-builds menu
	// items with marker 1. Rather than ship a second writer of unverified shape,
	// the legacy backend refuses create/modify/drop — so the script cannot pass
	// there and the refusal is the intended behaviour.
	"legacy/26-menu-examples.mdl": "menu authoring is modelsdk-only by design; the legacy backend refuses it",
	// Same shape as the menu skip: rule authoring is modelsdk-only. sdk/mpr has
	// no serializeRule, and a rule document is close enough to a microflow that
	// a half-written one would look valid, so the legacy backend refuses
	// create/modify/drop rather than emitting one. Reads work on both engines.
	"legacy/rules.mdl": "rule authoring is modelsdk-only by design; the legacy backend refuses it",
	// Same shape once more. The script creates a PhoneOffline profile, and
	// creating a profile means writing a fourteen-key
	// Navigation$NavigationProfile pinned against a Studio Pro reference. The
	// legacy writer has no such path and refuses rather than approximating —
	// "a profile assembled from a guess builds clean and will not open in
	// Studio Pro" (mdl/backend/mpr/backend.go). The SYNC block itself is
	// implemented on BOTH engines and covered by unit tests on each; it is
	// reaching an offline profile that legacy cannot do.
	"legacy/navigation-offline-sync.mdl": "creating a navigation profile is modelsdk-only by design; the legacy backend refuses it",
	// Same shape again: layout authoring is modelsdk-only. A layout's widget tree
	// hangs off a Forms$WebLayoutContent wrapper the legacy writer cannot build —
	// its serializeLayout emitted four header keys, a string $ID where Studio Pro
	// stores binary, and a LayoutType on the layout element rather than on the
	// wrapper. Nothing had ever called it, because nothing created a layout until
	// now. The legacy backend refuses rather than writing a document with nowhere
	// for the tree to go. Reads (SHOW/DESCRIBE LAYOUT) work on both engines.
	"legacy/layouts.mdl":             "layout authoring is modelsdk-only by design; the legacy backend refuses it",
	"legacy/navigation-profiles.mdl": "creating a navigation profile is modelsdk-only by design; the legacy backend refuses it",
	// Same shape again: message definition collection authoring is
	// modelsdk-only. The legacy writer has no serializer for the document, and
	// building one would duplicate a shape the codec already gets right —
	// including a typed-array marker of 2 and an empty-but-present Children
	// list, neither of which is the codec's default. The legacy backend refuses
	// create/modify/drop rather than emitting one. Reads work on both engines.
	"legacy/40-message-definition-examples.mdl": "message definition authoring is modelsdk-only by design; the legacy backend refuses it",
	// Enabling a language writes Settings$LanguageSettings.Languages, which the
	// legacy serializer carries through from the stored document rather than
	// writing — so the list cannot change on that engine. The backend refuses it
	// rather than reporting a write that never lands.
	"legacy/languages.mdl": "enabling a language is modelsdk-only by design; the legacy backend refuses it",
	// The legacy widget builder has no `linechart` template either, so the OL08
	// LineChart object-list example (added in 6b837ad7) fails page build
	// ("template not found: linechart"). Passes on modelsdk. Same class as the
	// 34-chart barchart skip above.
	"legacy/32-pluggable-widget-object-lists-v010.mdl": "legacy widget builder lacks the linechart template (works on modelsdk); tracked",
	// DG16 (association-navigating DataGrid2 column + a custom-content dynamictext
	// over an association) is a modelsdk-only capability (Bug 6/7): legacy writes a
	// flat, unresolvable binding, so mxbuild rejects the dynamictext with CE1365
	// "Attribute … is not an attribute of entity …". Legacy known issue L1; do not
	// fix legacy. See docs/03-development/LEGACY_ENGINE_KNOWN_ISSUES.md.
	"legacy/29-datagrid-examples.mdl": "legacy can't serialize association-navigating bindings (DG16) → CE1365 (works on modelsdk); known issue L1, tracked",
	// The "data from context" association DataView (P_OrderWithCustomer, added in
	// 7b9c251b) serializes correctly on modelsdk (Forms$DataViewSource) but the
	// legacy widget builder writes an association source → CE6705 "Data view
	// cannot have a data source of type association". Legacy known issue L1;
	// do not fix legacy. See docs/03-development/LEGACY_ENGINE_KNOWN_ISSUES.md.
	"legacy/03-page-examples.mdl": "legacy can't serialize the association DataView source → CE6705 (works on modelsdk); known issue L1, tracked",
}

// moduleDep is one marketplace-module MPK a script requires, optionally gated to
// a Mendix version range. A marketplace .mpk is built with a specific Studio Pro
// version and refuses to import into an older one ("created with a newer version
// of Mendix Studio Pro"), while an older .mpk can be rejected as incompatible by
// a newer runtime — so a single file often can't span the whole tested range.
// When constraint is nil the MPK is used for every version; otherwise it is only
// imported when the project's Mendix version satisfies the constraint.
type moduleDep struct {
	mpk        string
	constraint *versionConstraint // nil = any version
}

// mustConstraint parses a "-- @version:" style range ("..11.11", "11.12+",
// "10.6..11.11") for a module gate. Panics on an invalid literal (test-time
// constant, so a typo should fail loudly rather than silently gate nothing).
func mustConstraint(s string) *versionConstraint {
	vc := parseVersionDirective("-- @version:" + s)
	if vc == nil {
		panic(fmt.Sprintf("mustConstraint: invalid version constraint %q", s))
	}
	return vc
}

// scriptModuleDeps maps script filenames to the marketplace module MPKs they
// require. These modules are imported via `mx module-import` before executing the
// script; version-gated entries pick the MPK compatible with the test's Mendix
// version (see moduleDep).
var scriptModuleDeps = map[string][]moduleDep{
	"05-database-connection-examples.mdl": {
		// EDC 6.2.3 imports on <= 11.11 but the 11.12 runtime reports it
		// incompatible; EDC 6.3.0 is built with Studio Pro 11.12 and refuses to
		// import into anything older. Split the range between them. (The bundled
		// 6.3.0 has its ~100 MB snowflake JDBC driver stripped to fit under GitHub's
		// file-size limit — mx check needs the model, not the driver; see
		// mx-modules/README.md.)
		{mpk: "ExternalDatabaseConnector-v6.2.3.mpk", constraint: mustConstraint("..11.11")},
		{mpk: "ExternalDatabaseConnector-v6.3.0.mpk", constraint: mustConstraint("11.12+")},
	},
	"13-business-events-examples.mdl": {
		{mpk: "BusinessEvents_3.12.0.mpk"},
	},
}

// scriptSkipList marks fixtures that should be skipped, with the reason.
// Use sparingly — only for fixtures whose failure is tracked elsewhere and
// not actionable on this branch.
var scriptSkipList = map[string]string{}

// scriptKnownCEErrors lists CE error codes (or, where an error carries no CE
// code, a distinctive substring of its message) that are expected for specific
// scripts. These are syntax showcase scripts that intentionally omit entities,
// constants, headers etc. that full validation requires. Matched by substring
// against each `[error]` line (see allErrorsKnown).
var scriptKnownCEErrors = map[string][]string{
	"03-page-examples.mdl": {
		"CE3637", // Data view listen to gallery in sibling layout-grid column — Mendix scoping limitation
	},
	"06b-soap-examples.mdl": {
		"CE1613", // Dangling service/mapping refs — no web service defined in the test project
	},
	"17-custom-widget-examples.mdl": {
		// The IMAGE showcases author bare/default (static-image-mode) Image
		// widgets to demonstrate the IMAGE keyword's basic/dimensions/onclick
		// syntax. MDL's image widget exposes no property to bind a static image
		// resource (only imageUrl mode), so a default Image is legitimately
		// "No image selected." — exactly what a freshly-dropped Image shows in
		// Studio Pro until you pick one. Not an mxcli defect: the widget BSON is
		// valid. It previously passed only because a stale
		// `Atlas_Core.Content.Mendix` template default masked it — that default
		// caused CE0463 and was removed in the Image CE0463 fix (549c44f).
		"No image selected.",
	},
}

// TestMxCheck_DoctypeScripts executes each doctype-tests/*.mdl example script
// in its own fresh Mendix project and validates the result with mx check.
//
// Each script runs in isolation so errors are cleanly attributed.
// Files matching *.test.mdl or *.tests.mdl are skipped (they require Docker).
func TestMxCheck_DoctypeScripts(t *testing.T) {
	if !mxCheckAvailable() {
		t.Skip("mx command not available")
	}

	// Locate doctype-tests directory
	doctypeDir, err := filepath.Abs("../../mdl-examples/doctype-tests")
	if err != nil {
		t.Fatalf("Failed to resolve doctype-tests path: %v", err)
	}
	if _, err := os.Stat(doctypeDir); err != nil {
		t.Skipf("doctype-tests directory not found at %s", doctypeDir)
	}

	// Locate mx-modules directory for marketplace dependencies
	modulesDir, err := filepath.Abs("../../mx-modules")
	if err != nil {
		t.Logf("Warning: could not resolve mx-modules path: %v", err)
	}

	// Collect eligible scripts (skip .test.mdl and .tests.mdl)
	entries, err := os.ReadDir(doctypeDir)
	if err != nil {
		t.Fatalf("Failed to read doctype-tests directory: %v", err)
	}

	var scripts []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".mdl") {
			continue
		}
		if strings.HasSuffix(name, ".test.mdl") || strings.HasSuffix(name, ".tests.mdl") {
			continue
		}
		if _, skip := scriptSkipList[name]; skip {
			continue
		}
		scripts = append(scripts, name)
	}
	sort.Strings(scripts)

	if len(scripts) == 0 {
		t.Skip("no eligible MDL scripts found")
	}

	mxPath := findMxBinary()

	for _, name := range scripts {
		scriptPath := filepath.Join(doctypeDir, name)
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", name, err)
		}

		for _, eng := range gateEngines {
			name, content, eng := name, content, eng // capture per iteration
			subName := name + "/" + eng.name
			if reason, skip := engineScriptSkip[eng.name+"/"+name]; skip {
				t.Run(subName, func(t *testing.T) { t.Skipf("engine-specific skip: %s", reason) })
				continue
			}
			t.Run(subName, func(t *testing.T) {
				t.Parallel()

				// Fresh project for each script/engine
				env := setupTestEnvWithBackend(t, eng.factory)
				defer env.teardown()

				// Import required marketplace modules before executing script,
				// selecting the MPK compatible with this project's Mendix version.
				if deps, ok := scriptModuleDeps[name]; ok && modulesDir != "" && mxPath != "" {
					// Read the version before disconnecting (the backend can't be
					// queried once disconnected).
					depVersion := env.executor.Backend().ProjectVersion()

					// Disconnect so mx can access the MPR file
					env.executor.Execute(&ast.DisconnectStmt{})

					for _, dep := range deps {
						if dep.constraint != nil && !dep.constraint.matches(depVersion) {
							continue
						}
						mpkPath := filepath.Join(modulesDir, dep.mpk)
						if _, err := os.Stat(mpkPath); err != nil {
							t.Logf("Skipping module import (not found): %s", mpkPath)
							continue
						}
						cmd := exec.Command(mxPath, "module-import", mpkPath, env.projectPath)
						if out, err := cmd.CombinedOutput(); err != nil {
							t.Logf("Warning: module import failed for %s: %v\n%s", dep.mpk, err, string(out))
						}
					}

					// Reconnect after module import
					if err := env.executor.Execute(&ast.ConnectStmt{Path: env.projectPath}); err != nil {
						t.Fatalf("Failed to reconnect after module import: %v", err)
					}
				}

				// Filter out version-gated sections that don't match this project's Mendix version
				pv := env.executor.Backend().ProjectVersion()
				filtered, skippedLines := filterByVersion(string(content), pv)
				if skippedLines > 0 {
					t.Logf("Mendix %s: skipped %d version-gated lines", pv.ProductVersion, skippedLines)
				}

				// A relative path inside a script (a toolbox icon PNG) names a file
				// next to that script. The harness's working directory is this
				// package, so without this every such fixture would fail here and
				// nowhere else.
				env.executor.SetScriptDir(doctypeDir)

				// Execute the script
				prog, errs := visitor.Build(filtered)
				if len(errs) > 0 {
					t.Fatalf("Parse error: %v", errs[0])
				}

				// A trailing `exit;` is a legitimate clean halt — several fixtures use
				// it to leave artifacts in the project for Studio Pro inspection. Treat
				// ErrExit as success, not an execution failure.
				if err := env.executor.ExecuteProgram(prog); err != nil && !errors.Is(err, ErrExit) {
					t.Errorf("Execution error: %v", err)
				}

				// Flush to disk
				env.executor.Execute(&ast.DisconnectStmt{})

				// NB: we deliberately do NOT run `mx update-widgets` here. mxcli is
				// expected to emit valid pluggable-widget BSON on its own (verified by
				// mx check below), and `mx update-widgets` has been observed to CORRUPT
				// otherwise-valid projects on 11.12 — e.g. it rewrites a widget's
				// ConditionalEditabilitySettings.Attribute with the int32(3) list marker,
				// yielding a StorageLoadException. Running mx check directly on mxcli's
				// output is both the stricter and the correct gate.

				// Run mx check
				output, mxErr := runMxCheck(t, env.projectPath)
				if mxErr != nil {
					// Check for actual errors: [error] lines, crash messages, or unhandled exceptions
					hasErrors := strings.Contains(output, "[error]") || strings.Contains(output, "error:") ||
						strings.Contains(output, "Exception:")
					if hasErrors {
						// Check if all errors are from known CE codes (limitations of syntax showcases)
						knownCodes := []string{
							"CE0161", // XPath serializer limitation (global)
							"CE0463", // Widget template version mismatch (templates are from 11.6, may differ on 10.x)
						}
						if codes, ok := scriptKnownCEErrors[name]; ok {
							knownCodes = append(knownCodes, codes...)
						}
						if allErrorsKnown(output, knownCodes) {
							t.Logf("mx check has known limitations only (%d errors):\n%s",
								strings.Count(output, "[error]"), output)
						} else {
							t.Errorf("mx check found errors:\n%s", output)
						}
					} else {
						t.Logf("mx check output:\n%s", output)
					}
				} else {
					t.Logf("mx check passed: 0 errors")
				}
			})
		}
	}
}

// allErrorsKnown returns true if every [error] line in the mx check output
// contains at least one of the known CE codes.
func allErrorsKnown(output string, knownCodes []string) bool {
	if strings.Contains(output, "error:") || strings.Contains(output, "Exception:") {
		return false // Crash-level errors and unhandled exceptions are never known
	}
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "[error]") {
			continue
		}
		known := false
		for _, code := range knownCodes {
			if strings.Contains(line, code) {
				known = true
				break
			}
		}
		if !known {
			return false
		}
	}
	return true
}

// versionConstraint represents a min/max Mendix version range for -- @version: directives.
type versionConstraint struct {
	minMajor, minMinor int // -1 means no minimum
	maxMajor, maxMinor int // -1 means no maximum
}

// matches returns true if the project version satisfies this constraint.
func (vc *versionConstraint) matches(pv *types.ProjectVersion) bool {
	if vc.minMajor >= 0 {
		if !pv.IsAtLeast(vc.minMajor, vc.minMinor) {
			return false
		}
	}
	if vc.maxMajor >= 0 {
		// Check that version is at most maxMajor.maxMinor
		if pv.MajorVersion > vc.maxMajor || (pv.MajorVersion == vc.maxMajor && pv.MinorVersion > vc.maxMinor) {
			return false
		}
	}
	return true
}

func (vc *versionConstraint) String() string {
	if vc.minMajor >= 0 && vc.maxMajor >= 0 {
		return fmt.Sprintf("%d.%d..%d.%d", vc.minMajor, vc.minMinor, vc.maxMajor, vc.maxMinor)
	}
	if vc.minMajor >= 0 {
		return fmt.Sprintf("%d.%d+", vc.minMajor, vc.minMinor)
	}
	if vc.maxMajor >= 0 {
		return fmt.Sprintf("..%d.%d", vc.maxMajor, vc.maxMinor)
	}
	return "any"
}

// parseVersionDirective parses a "-- @version: <constraint>" line.
// Returns nil for "any" or unparseable directives.
// Formats: "11.0+", "10.6..10.24", "..10.24", "any"
func parseVersionDirective(line string) *versionConstraint {
	s := strings.TrimPrefix(line, "-- @version:")
	s = strings.TrimSpace(s)

	if s == "" || s == "any" {
		return nil
	}

	// Range: "10.6..10.24"
	if parts := strings.SplitN(s, "..", 2); len(parts) == 2 {
		vc := &versionConstraint{minMajor: -1, minMinor: -1, maxMajor: -1, maxMinor: -1}
		if parts[0] != "" {
			major, minor, ok := parseMajorMinor(parts[0])
			if !ok {
				return nil
			}
			vc.minMajor, vc.minMinor = major, minor
		}
		if parts[1] != "" {
			major, minor, ok := parseMajorMinor(parts[1])
			if !ok {
				return nil
			}
			vc.maxMajor, vc.maxMinor = major, minor
		}
		return vc
	}

	// Minimum: "11.0+"
	if strings.HasSuffix(s, "+") {
		s = strings.TrimSuffix(s, "+")
		major, minor, ok := parseMajorMinor(s)
		if !ok {
			return nil
		}
		return &versionConstraint{minMajor: major, minMinor: minor, maxMajor: -1, maxMinor: -1}
	}

	return nil
}

// parseMajorMinor parses "10.24" into (10, 24, true).
func parseMajorMinor(s string) (int, int, bool) {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// filterByVersion removes MDL content sections that don't match the project's Mendix version.
// Sections are delimited by "-- @version: <constraint>" directives.
// A directive applies to all following lines until the next directive or end of file.
// "-- @version: any" resets to unconditional inclusion.
func filterByVersion(content string, pv *types.ProjectVersion) (string, int) {
	var result strings.Builder
	var currentConstraint *versionConstraint // nil = no constraint (always include)
	skippedLines := 0

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- @version:") {
			currentConstraint = parseVersionDirective(trimmed)
			// Keep the directive line as a comment (so line numbers stay close)
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}
		if currentConstraint == nil || currentConstraint.matches(pv) {
			result.WriteString(line)
			result.WriteString("\n")
		} else {
			// Replace with empty line to preserve line numbering for error messages
			result.WriteString("\n")
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(trimmed, "--") {
				skippedLines++
			}
		}
	}
	return result.String(), skippedLines
}
