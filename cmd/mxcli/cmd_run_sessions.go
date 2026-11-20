// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

var runClearSessionsCmd = &cobra.Command{
	Use:   "clear-sessions",
	Short: "Sign every user out of the local app, freeing the unlicensed runtime's 6 session slots",
	Long: `Delete every session in the database of the 'mxcli run --local' serving this
project. The app keeps running; every signed-in browser has to sign in again.

The local runtime has no licence, and the trial licence allows 6 concurrent
sessions. Every sign-in — a Playwright probe's included — holds one until it signs
out or expires, and a 7th sign-in fails with HTTP 560, which the login page shows
as "Sign in failed" as if the password were wrong (the runtime log says "Maximum
number of sessions exceeded"). The licence counts the sessions in the database,
so deleting them frees every slot at once; 'mxcli run restart' does the same at
the cost of a boot.

The database is the one the recorded run was started with (its --db-* flags). A
--db-password given literally is not stored with the run: pass it again here. The
built-in HSQLDB database (--db-type hsqldb) is held open by the runtime; use
'mxcli run restart' there.`,
	Example: `  mxcli run clear-sessions -p app.mpr`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := lifecycleProject(cmd)
		st, err := docker.ReadRunState(projectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(runExitFailed)
		}
		noteForeignRunState(projectPath, os.Stderr)
		if st == nil || len(st.Args) == 0 {
			fmt.Fprintf(os.Stderr, "no run of this project is recorded, so its database is not known\n"+
				"  start one: mxcli run --local --detach -p %s\n", projectPath)
			os.Exit(runExitNotRunning)
		}
		password, _ := cmd.Flags().GetString("db-password")
		if password != "" {
			if password, err = resolveValueSource(password); err != nil {
				fmt.Fprintf(os.Stderr, "Error: --db-password: %v\n", err)
				os.Exit(runExitFailed)
			}
		}
		db, err := runDatabaseFromArgs(projectPath, st.Args, password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(runExitFailed)
		}
		if db.IsFileBased() {
			fmt.Fprintf(os.Stderr, "Error: this run uses the built-in HSQLDB database, which the runtime holds open.\n"+
				"  Restart it instead; the runtime deletes old sessions as it starts:\n"+
				"    mxcli run restart -p %s\n", projectPath)
			os.Exit(runExitFailed)
		}
		n, err := docker.ClearSessions(db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: clearing the sessions in %s on %s: %v\n", db.Name, db.Host, err)
			os.Exit(runExitFailed)
		}
		fmt.Fprintf(os.Stdout, "Cleared %d session(s) in %s: all 6 sign-in slots are free; signed-in browsers must sign in again.\n", n, db.Name)
	},
}

func init() {
	runClearSessionsCmd.Flags().String("db-password", "", "Database password, when the run was given one literally (it is not stored)"+secretRefHelp)
	runCmd.AddCommand(runClearSessionsCmd)
}

// runDatabaseFromArgs is the database a run started with args connects to: its
// --db-* flags with the run's own defaults. password, when set, replaces the
// stored --db-password (needed when that was redacted).
func runDatabaseFromArgs(projectPath string, args []string, password string) (docker.DBConfig, error) {
	db := docker.DBConfig{
		Type:     storedFlag(args, "db-type"),
		Host:     storedFlag(args, "db-host"),
		Name:     storedFlag(args, "db-name"),
		User:     storedFlag(args, "db-user"),
		Password: storedFlag(args, "db-password"),
	}
	switch {
	case password != "":
		db.Password = password
	case db.Password == redactedValue || strings.Contains(db.Password, redactedValue):
		return docker.DBConfig{}, fmt.Errorf("the run was given --db-password literally, and it is not stored: pass it again with --db-password")
	case db.Password != "":
		resolved, err := resolveValueSource(db.Password)
		if err != nil {
			return docker.DBConfig{}, fmt.Errorf("the run's --db-password: %w", err)
		}
		db.Password = resolved
	}
	return docker.RunDatabase(projectPath, db)
}

// storedFlag returns the last value of --name in stored run arguments, in
// either the "--name value" or the "--name=value" form, or "".
func storedFlag(args []string, name string) string {
	flag := "--" + name
	val := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == flag && i+1 < len(args):
			val = args[i+1]
			i++
		case strings.HasPrefix(args[i], flag+"="):
			val = strings.TrimPrefix(args[i], flag+"=")
		}
	}
	return val
}
