// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// cmd_git_doctor.go: `mxcli git doctor` audits a repository for everything a
// commit made outside Studio Pro gets wrong, and with --fix repairs it. The
// checks and their fixes live in gitdoctor.go.

var gitDoctorCmd = &cobra.Command{
	Use:   "doctor [-p <project.mpr>]",
	Short: "Audit (and with --fix, repair) a repository for Studio Pro",
	Long: `Report what in this repository breaks Studio Pro, and with --fix repair it.

Checked:
  1. Local git config: core.autocrlf=False (a checkout must not rewrite line
     endings inside .mxunit files; --fix rewrites any other value), and the
     mendix.* keys (Studio Pro's own bookkeeping) backfilled when missing —
     a value Studio Pro wrote stays.
  2. The remote URL, as configured. Studio Pro can only use HTTPS; an SSH
     remote (git@host:org/app.git or ssh://…) is rewritten to HTTPS by --fix.
     Other non-HTTPS URLs are reported without being changed. A
     url.<base>.insteadOf rule in your git config that makes git use another
     URL is reported too; --fix leaves it alone.
  3. The remote's refs/notes/mx_metadata. Absent or unreachable is a hint:
     push the notes ref together with the branch. When the remote has notes
     the local ref does not hold (git clone never fetches refs/notes/*), the
     commits are not judged: that is the finding, and --fix fetches the ref,
     as a fast-forward only.
  4. Every commit's mx_metadata note. Missing notes make Studio Pro show
     "(unknown)"; a malformed note it cannot read at all. --fix computes real
     notes from the model (with -p), and falls back to a template note with a
     --version ModelerVersion where the model at a commit cannot be read.
     Notes Studio Pro wrote are never touched.

Exits 1 while an issue remains, so the audit can gate CI; fetch the notes
first there. --fix does not push: push the branch and the notes ref together
yourself.

  git fetch <remote> refs/notes/mx_metadata:refs/notes/mx_metadata
  git push <remote> <branch> refs/notes/mx_metadata`,
	Example: `  mxcli git doctor
  mxcli git doctor -p app.mpr --fix
  mxcli git doctor --fix --version 10.21.0`,
	Args:          cobra.NoArgs,
	RunE:          runGitDoctor,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	gitDoctorCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr); lets --fix compute real notes")
	gitDoctorCmd.Flags().Bool("fix", false, "apply the repairs (default: report only)")
	gitDoctorCmd.Flags().String("remote", "", "remote to check (default: tracking remote, then origin)")
	gitDoctorCmd.Flags().String("version", "", "ModelerVersion for template notes (default: the newest valid note in history)")
	gitCmd.AddCommand(gitDoctorCmd)
}

func runGitDoctor(cmd *cobra.Command, _ []string) error {
	project, _ := cmd.Flags().GetString("project")
	fix, _ := cmd.Flags().GetBool("fix")
	remote, _ := cmd.Flags().GetString("remote")
	version, _ := cmd.Flags().GetString("version")
	out := cmd.OutOrStdout()

	base := project
	dir := ""
	if base == "" {
		var err error
		if base, err = os.Getwd(); err != nil {
			return err
		}
		dir = base // a working directory is already a directory
	} else if _, err := os.Stat(project); err != nil {
		return fmt.Errorf("project not found: %s", project)
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	if dir == "" {
		dir = filepath.Dir(abs) // a .mpr path: git must run beside the file
	}
	root, err := gitOut(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("%s is not inside a git repository: %w", base, err)
	}
	if err := doctorRun(strings.TrimSpace(root), project, remote, version, fix, out); err != nil {
		if errors.Is(err, errDoctorIssuesRemain) {
			cmd.SilenceErrors = true
			os.Exit(1)
		}
		return err
	}
	return nil
}
