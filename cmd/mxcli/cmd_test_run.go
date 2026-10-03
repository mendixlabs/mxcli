// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/spf13/cobra"
)

var testRunCmd = &cobra.Command{
	Use:   "test <file|dir> [file|dir...]",
	Short: "Run MDL tests against a Mendix project",
	Long: `Run microflow tests defined in .test.mdl or .test.md files.

Tests use MDL syntax with javadoc-style annotations for expectations:

  /**
   * @test String concatenation
   * @expect $result = 'John Doe'
   */
  $result = CALL MICROFLOW MyModule.ConcatNames(
    FirstName = 'John', LastName = 'Doe'
  );
  /

With --local the app runs on mxcli's own runtime instead of a container — the
same boot as 'mxcli run --local', so no Docker daemon is needed. It uses its own
ports (8081/8091) and its own '<project>_test' database, so a warm 'run --local'
loop can keep serving the same project while tests run.

--local also uses a different, better mechanism to run the tests:

  1. Parses test files and extracts test blocks with @test/@expect annotations
  2. Generates one microflow per test, plus a Java action that registers a
     token-guarded HTTP endpoint
  3. Boots the app once — startup registers the endpoint and then runs your own
     after-startup microflow, so tests see the app as it really boots. No test
     runs during startup
  4. Invokes each test by name over HTTP; the verdict comes back in the response
  5. Restores original project settings

Your after-startup microflow running is what makes a suite behave the same under
--local and --attach. Pass --skip-app-startup for an empty, deterministic
baseline instead — the run always prints which of the two it did.

Because each test is its own microflow invoked on its own, a test that throws
fails only itself instead of ending the run, and results are returned rather
than recovered from the runtime log.

It also makes @cleanup real. By default (@cleanup rollback) each test runs in a
transaction the endpoint rolls back afterwards, so its database writes do not
survive — use @cleanup none when the writes are the point. The Docker path
always commits: it runs tests inside the after-startup action and has no
context of its own to roll back.

The endpoint is only reachable from loopback, only with a per-run token passed
to the runtime through its environment (never written into your project), and
will only ever invoke the generated MxTest.Test_* microflows. With no token in
the environment it is not registered at all, so a project that kept the MxTest
module through a failed cleanup exposes nothing when deployed elsewhere.

--local --watch keeps the runtime and the build server up between runs and
re-runs the suite on every change — to a test file, or to the project's model.
The first run pays the cold boot (~30s); each one after it is a warm rebuild
(~1-4s) plus the tests themselves (milliseconds). Editing a microflow and seeing
whether it still passes is the loop this exists for. Ctrl-C stops watching and
restores the project.

--attach skips the boot entirely and runs against an app already started with
'mxcli run --local --test-endpoint'. That app's runtime is already warm, so a run
costs only the test-microflow injection, a warm rebuild, and the tests: about two
seconds, with no cold boot at all. It combines with --watch.

The trade is deliberate and worth knowing: the tests run against the running
app's database, not a scratch one, so they can leave data behind in the app you
are looking at. An attach only ever adds and removes its own test microflows —
the endpoint and the after-startup setting belong to the app hosting them. A
change that needs a runtime restart (a new entity or association) is refused,
since that runtime belongs to the other process.

Without --local the Docker path is used instead: the suite is compiled into a
single after-startup microflow, the container is restarted, and results are
parsed out of its log. Pass --legacy-runner to use that mechanism on a local run
too, if the endpoint ever misbehaves.

Supports two file formats:
  .test.mdl  — Pure MDL test blocks separated by /
  .test.md   — Markdown specification with embedded mdl-test code blocks

Examples:
  # Run tests from a test file
  mxcli test tests/microflows.test.mdl -p app.mpr

  # Run all tests in a directory
  mxcli test tests/ -p app.mpr

  # Output JUnit XML for CI
  mxcli test tests/ -p app.mpr --junit results.xml

  # Fail the run on any test that asserts nothing
  mxcli test tests/ -p app.mpr --local --require-assertions

  # List tests without executing
  mxcli test tests/ -p app.mpr --list

  # Run without Docker, on mxcli's own local runtime
  mxcli test tests/ -p app.mpr --local

  # Keep the runtime warm and re-run on every change
  mxcli test tests/ -p app.mpr --local --watch

  # Run against an app already up (mxcli run --local --test-endpoint) — no boot
  mxcli test tests/ -p app.mpr --attach

  # ...and re-run on every change, still without owning the runtime
  mxcli test tests/ -p app.mpr --attach --watch

  # Skip build (reuse existing deployment)
  mxcli test tests/ -p app.mpr --skip-build

  # Verbose output (show all runtime logs)
  mxcli test tests/ -p app.mpr --verbose
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		list, _ := cmd.Flags().GetBool("list")
		junitOutput, _ := cmd.Flags().GetString("junit")
		requireAssertions, _ := cmd.Flags().GetBool("require-assertions")
		skipBuild, _ := cmd.Flags().GetBool("skip-build")
		local, _ := cmd.Flags().GetBool("local")
		legacyRunner, _ := cmd.Flags().GetBool("legacy-runner")
		watch, _ := cmd.Flags().GetBool("watch")
		attach, _ := cmd.Flags().GetBool("attach")
		skipAppStartup, _ := cmd.Flags().GetBool("skip-app-startup")
		configuration, _ := cmd.Flags().GetString("configuration")
		constantArgs, _ := cmd.Flags().GetStringArray("constant")
		mxbuildPath, _ := cmd.Flags().GetString("mxbuild-path")
		verbose, _ := cmd.Flags().GetBool("verbose")
		color, _ := cmd.Flags().GetBool("color")
		timeoutStr, _ := cmd.Flags().GetString("timeout")

		// Resolved before anything boots: an unknown --db-type is a typo, and
		// reporting it here names the flag instead of surfacing later as a
		// connection failure to a database the user never asked for.
		dbTypeRaw, _ := cmd.Flags().GetString("db-type")
		dbType, err := testrunner.ResolveTestDBType(dbTypeRaw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid timeout: %v\n", err)
			os.Exit(1)
		}

		if list {
			// resolveTestPaths here too: listing that cannot find a path execution
			// finds is a confusing split, and `mxcli test tests/ -p app/App.mpr
			// --list` hit exactly that (mxcli-formula1 findings #15).
			if err := testrunner.ListTests(resolveTestPaths(args, projectPath), os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}

		// Execution requires a project
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for test execution")
			os.Exit(1)
		}

		opts := testrunner.RunOptions{
			ProjectPath:       projectPath,
			TestFiles:         resolveTestPaths(args, projectPath),
			SkipBuild:         skipBuild,
			Local:             local,
			LegacyRunner:      legacyRunner,
			Watch:             watch,
			Attach:            attach,
			SkipAppStartup:    skipAppStartup,
			MxBuildPath:       mxbuildPath,
			DBType:            dbType,
			Timeout:           timeout,
			JUnitOutput:       junitOutput,
			RequireAssertions: requireAssertions,
			Verbose:           verbose,
			Color:             color,
			Stdout:            os.Stdout,
			Stderr:            os.Stderr,
		}

		// Only a --local run boots an app of its own, so only it decides which
		// constants that app runs with. --attach inherits the constants of the app
		// it attaches to, and the Docker path configures the container — reporting
		// a resolution neither of them uses would be a lie in the output.
		if local && !attach {
			constantFlags, err := parseConstantFlags(constantArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			overrides, err := constantChainFor(projectPath, configuration, constantFlags)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			reportConstantChain(os.Stdout, overrides)
			opts.ConstantOverrides = overrides.Values
		} else if len(constantArgs) > 0 {
			// --attach runs against an app someone else booted, and the Docker path
			// configures the container. Accepting --constant there and doing nothing
			// with it is the failure this feature exists to stop.
			fmt.Fprintln(os.Stderr, "Error: --constant applies only to a --local test run; "+
				"--attach uses the constants of the app it attaches to")
			os.Exit(1)
		}

		// A --local run that BUILDS recompiles the project's Java into the
		// classpath a concurrent `mxcli run --local` is holding open, which can
		// leave that app answering HTTP 200 with an empty body (mxcli-formula1
		// §81). Neither --attach nor --skip-build builds, so neither can cause it.
		if local && !attach && !skipBuild {
			warnIfDevLoopServing(projectPath, os.Stdout)
		}

		result, err := testrunner.Run(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// A --watch session interrupted before any run completed has no result to
		// report. Exiting 0 is right: nothing failed, the user just stopped watching.
		if result == nil {
			return
		}
		if !result.AllPassed() {
			os.Exit(1)
		}
	},
}

// resolveTestPaths lets a relative test path be relative to the PROJECT as well
// as to the working directory.
//
// `mxcli test tests/ -p app/App.mpr` used to fail with "no such file or
// directory" for a tests/ that sits right next to the .mpr — the path resolved
// against the process CWD only. That is defensible on its own, but mxcli
// otherwise encourages naming the project rather than standing in its
// directory, so the two conventions collide (mxcli-formula1 findings #13).
//
// The working directory still wins: a tests/ in both places resolves to the one
// the user is standing in, which is what every other tool does.
func resolveTestPaths(paths []string, projectPath string) []string {
	if projectPath == "" || len(paths) == 0 {
		return paths
	}
	projectDir := filepath.Dir(projectPath)
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if filepath.IsAbs(p) {
			out = append(out, p)
			continue
		}
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
			continue
		}
		if alt := filepath.Join(projectDir, p); alt != p {
			if _, err := os.Stat(alt); err == nil {
				out = append(out, alt)
				continue
			}
		}
		// Neither exists: keep the original so the error names what was typed.
		out = append(out, p)
	}
	return out
}
