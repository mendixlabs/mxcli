// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/spf13/cobra"
)

var testCallCmd = &cobra.Command{
	Use:   "call <cases.json|->",
	Short: "Call microflows in the running app with JSON cases (no build, no restart)",
	Long: `Sends JSON cases to the test endpoint of an app started with
'mxcli run --local --test-endpoint', runs each microflow inside a transaction
that is rolled back, and prints pass/fail with expected and actual values.

A case: {"flow": "Mod.Flow", "params": {...}, "setup": [objects created before the call], "expect": [{"path": "$result/Status", "op": "=", "value": "PAID"}], "rollback": true}

params values: a string/number/bool for primitives (enum: the value key), or
an object: {"$new": "Mod.Entity", "Attr": value, "Mod.Assoc": {nested}} builds
and commits an object; {"$find": "Mod.Entity[Attr = 'x']"} uses an existing
one; {"$ref": "name"} reuses a named one ("$name" on the creating object).
Expect paths: $result, $<param>, then /Member hops; "count:Mod.Entity[xpath]"
counts rows. Ops: = != < <= > >= contains isnull notnull. "throws": "text"
expects the call to raise an error containing text.

Rollback covers the main transaction only: separate commits (ExecuteInNewTransaction),
queued or async work, REST calls, file writes and autonumber counters survive.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		repeat, _ := cmd.Flags().GetInt("repeat")
		var r io.Reader = os.Stdin
		if args[0] != "-" {
			f, err := os.Open(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()
			r = f
		}
		cases, err := testrunner.ParseCallCases(r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		ok, err := testrunner.RunCalls(projectPath, cases, repeat, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !ok {
			os.Exit(1)
		}
	},
}

func init() {
	testCallCmd.Flags().Int("repeat", 1, "Run the case list this many times and print the timing of the later rounds")
	testRunCmd.AddCommand(testCallCmd)
}
