// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <file|->",
	Short: "Execute an MDL script file",
	Long: `Execute an MDL script file containing MDL commands.

Before anything is written, the script is put through the same semantic checks
as "mxcli check". If any of them reports an error, nothing is executed.
Warnings are printed and do not
stop the run; info notes are counted on one line (--verbose prints them, as
"mxcli check" does). Use --no-check to apply a script anyway.

A deprecated MDL spelling (MDL-DEPRnnn, e.g. "create or replace" for "create or
modify") is a warning; --deprecations=error makes it an error.

By default execution stops at the first error. With --continue-on-error, every
statement is attempted; each failure is reported (prefixed with its statement
number) and execution continues, exiting non-zero if any statement failed. This
makes a partially-applied domain script re-runnable — the already-applied
statements (e.g. "attribute already exists") error individually while the not-
yet-applied ones still run — without a failure masking later work.

A write is refused while Studio Pro has the project open (its <project>.mpr.lock
is beside the .mpr): Studio Pro does not reload the model from disk, and its next
save would silently discard the change. Close the project in Studio Pro, or route
writes through it with --mcp. --force writes anyway (for a lock left behind by a
crash); MXCLI_ALLOW_STUDIO_PRO_OPEN=1 does the same for every command. Reads, and
re-running a script whose statements change nothing, are never refused.

Some statements also write files beside the model: a Java or JavaScript
action's body goes to javasource/ or javascriptsource/, and DROP MODULE removes
the module's source folders. Each file written, moved or removed is named on its
own line after the statement's report, as a path relative to the project
("Wrote javasource/probe/actions/JA_Hello.java ..."), and a run that changed any
ends with "Files changed outside the model: N (...)". Commit them with the .mpr
and mprcontents/: they are not part of the model. A rewrite that leaves a
file's content as it was does not touch it and is not reported.

A script is applied all or nothing: if a statement still fails while it is
written (after the checks passed), the writes of the statements before it are
rolled back and the project is left as it was. Only the model (.mpr and
mprcontents/) is restored; generated .java/.js source is not. With
--continue-on-error each statement stands on its own and nothing is rolled back.

Pass "-" as the file to read the script from standard input, so MDL can be
piped or written inline as a heredoc without a temporary file.

Example:
  mxcli exec setup.mdl
  mxcli exec -p app.mpr script.mdl
  mxcli exec -p app.mpr script.mdl --continue-on-error
  mxcli exec -p app.mpr script.mdl --no-check
  mxcli exec -p app.mpr - <<'EOF'
  SHOW STRUCTURE DEPTH 1;
  EOF
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		projectPath, _ := cmd.Flags().GetString("project")
		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		skipCheck, _ := cmd.Flags().GetBool("no-check")
		verbose, _ := cmd.Flags().GetBool("verbose")
		if force, _ := cmd.Flags().GetBool("force"); force {
			mmpr.AllowWritesWhileStudioProOpen = true
			if lock, _ := mmpr.StudioProLockFile(projectPath); lock != "" {
				fmt.Fprintf(os.Stderr, "Warning: Studio Pro appears to have this project open (%s); writing anyway (--force). "+
					"Studio Pro's next save will discard these changes unless the project is closed or reloaded first.\n", lock)
			}
		}
		depPolicy := deprecationPolicy(cmd)

		// Read the script (a path, or "-" for stdin)
		content, err := readMDLSource(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		// A watching dev loop is told when this script starts writing and when
		// the last write has landed, so it builds at once instead of waiting
		// for the model to go quiet (ako/mxcli#2151). Deferred first, so it
		// runs after exec.Close below.
		defer docker.BeginModelWrite(projectPath)()

		exec, logger := newLoggedExecutor("exec")
		defer logger.Close()
		defer exec.Close()
		if noReport, _ := cmd.Flags().GetBool("no-source-file-report"); noReport {
			exec.SetSourceFileReport(false)
		}

		// A relative path inside the script (a toolbox icon, an image) names a
		// file next to the script, not next to the caller. Empty for stdin.
		if filePath != "-" {
			if abs, absErr := filepath.Abs(filePath); absErr == nil {
				exec.SetScriptDir(filepath.Dir(abs))
			}
		}

		// Auto-connect if project specified
		if projectPath != "" {
			connectCmd := fmt.Sprintf("CONNECT LOCAL '%s';", visitor.QuoteString(projectPath))
			prog, _ := visitor.Build(connectCmd)
			for _, stmt := range prog.Statements {
				if err := exec.Execute(stmt); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
			// A stale MPR v2 ContentsHash index (units restored with git)
			// is invisible to mx check; say so before writing (#972).
			fmt.Fprint(os.Stderr, contentsHashDriftWarning(projectPath))
		}

		// Parse and execute the file
		prog, errs := visitor.Build(string(content))
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
			}
			os.Exit(1)
		}
		// "Apply this file" that applies nothing is never what was meant, and a
		// silent no-op is the worst outcome for a replayable mdlsource/
		// (ako/mxcli#618).
		if line, bad := unparsableInput(string(content), len(prog.Statements)); bad {
			fmt.Fprintln(os.Stderr, unparsableInputError(filePath, line))
			os.Exit(1)
		}

		// A call to a flow the script creates further down is written before
		// its callee, so only a run that rolls back on failure may make one
		// (mendixlabs/mxcli#1405); --continue-on-error keeps each statement.
		exec.SetForwardFlowCalls(!continueOnError)

		if refusal := execPreflight(exec, prog, projectPath, filePath, skipCheck, verbose, continueOnError, depPolicy, os.Stderr, true); refusal != "" {
			fmt.Fprint(os.Stderr, refusal)
			os.Exit(1)
		}

		if continueOnError {
			res, err := exec.ExecuteProgramContinueOnError(prog, os.Stderr)
			if err != nil && !errors.Is(err, executor.ErrExit) {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "%d statements: %d succeeded, %d failed\n", res.Total, res.Succeeded, res.Failed)
			if res.Failed > 0 {
				os.Exit(1)
			}
			return
		}

		if err := runScriptAtomically(exec, prog); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

// runScriptAtomically executes prog so that it lands whole or not at all: when
// a statement fails, every write the earlier ones made is undone and the
// project is left as it was (mendixlabs/mxcli#1408). The pre-check refuses a
// script with a known error, but a statement can still fail at write time — a
// property the project's Mendix version cannot store, a widget that does not
// build — and before this the statements ahead of it stayed written.
//
// The executor is closed here, success or not: a deferred write lands at
// Close, so the run is not over until then, and the .mpr can only be put back
// once nothing holds it open. An `exit` statement is a successful stop.
func runScriptAtomically(exec *executor.Executor, prog *ast.Program) error {
	journal, jerr := mmpr.BeginJournal()
	if jerr != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v; a failing statement will leave the earlier ones written\n", jerr)
		exec.SetForwardFlowCalls(false)
		if err := exec.ExecuteProgram(prog); err != nil && !errors.Is(err, executor.ErrExit) {
			return err
		}
		return nil
	}
	stoppedAt, err := exec.ExecuteProgramReportingStop(prog)
	if errors.Is(err, executor.ErrExit) {
		err = nil
	}
	if closeErr := exec.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err == nil || !journal.Touched() {
		journal.Commit()
		return err
	}
	if rerr := journal.Rollback(); rerr != nil {
		return errors.Join(err, fmt.Errorf("could not roll back the statements that ran before it, "+
			"so the project may be partly updated: %w", rerr))
	}
	return fmt.Errorf("%w\nNothing was written: %s, and the project is as it was before exec", err, rolledBackSummary(stoppedAt))
}

// rolledBackSummary says what a rollback undid, given the 0-based index of the
// failing statement (-1 when the run failed after its last statement).
func rolledBackSummary(stoppedAt int) string {
	switch {
	case stoppedAt == 0:
		return "the failing statement's partial writes were rolled back"
	case stoppedAt == 1:
		return "statement 1 was rolled back"
	case stoppedAt > 1:
		return fmt.Sprintf("statements 1-%d were rolled back", stoppedAt)
	}
	return "the script's statements were rolled back"
}

func init() {
	execCmd.Flags().Bool("no-check", false,
		"Skip the pre-flight semantic checks and apply the script even if mxcli check would report errors")
	execCmd.Flags().Bool("verbose", false,
		"Print the pre-flight checks' info notes in full instead of counting them (mxcli check always prints them)")
	execCmd.Flags().Bool("force", false,
		"Write even though Studio Pro appears to have the project open (its .mpr.lock is present) — e.g. a lock left behind by a crash")
	execCmd.Flags().Bool("continue-on-error", false,
		"Run every statement, reporting each failure instead of halting at the first (exits non-zero if any failed) — makes a partially-applied script re-runnable")
	// Hidden: for a caller whose writes are temporary. mxcli test injects a Java
	// action and deletes its source at cleanup; telling its user to commit that
	// file would be wrong (ako/mxcli#1618).
	execCmd.Flags().Bool("no-source-file-report", false,
		"Do not name the files written beside the model (for a caller that removes them again, such as mxcli test)")
	_ = execCmd.Flags().MarkHidden("no-source-file-report")
}
