// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/constantstore"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

var constantCmd = &cobra.Command{
	Use:   "constant",
	Short: "Set constant values that stay on this machine, and see which value a run will use",
	Long: `Manage the constant values that belong to THIS MACHINE.

They live in <project>/.mxcli/constants.json, mode 0600. 'constant set' adds
.mxcli/ to the project's .gitignore if it is missing and then asks git whether
the path is really ignored — refusing to write the value if it is not, because
a store that leaks is worse than none. It is the slot for an API key or a
connection string that must not reach version control.

This is mxcli's own store, not Mendix's. Mendix has the same concept — a
configuration value marked private — but from 10.9 it is encrypted per user
account by Studio Pro, so nothing headless can read or write it. mxcli mirrors
the idea with a file it can actually reach, at the security of file permissions
rather than encryption.

Where a value comes from, highest first:

  1. --constant Module.Name=value    this run only, written nowhere
                                     (=@env:VAR or =@file:path keeps the value
                                     out of 'ps' and shell history)
  2. this store                      this machine, gitignored
  3. the project configuration       shared, in git ('alter settings constant')
  4. the constant's default          shared, in git

'mxcli constant list' shows the winner for every constant and which layer set
it. Values from this store are masked unless you pass --show-values.`,
}

var constantSetCmd = &cobra.Command{
	Use:   "set <Module.Name> <value>",
	Short: "Set a constant's value on this machine (gitignored, never committed)",
	Args:  cobra.ExactArgs(2),
	Example: `  mxcli constant set MyModule.ApiKey 'sk-live-...' -p app.mpr
  mxcli constant list -p app.mpr`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := requireProjectPath(cmd)
		name, value := args[0], args[1]

		read, err := projectConstantDefaults(projectPath)
		if err != nil {
			exitf("could not read the project's constants: %v", err)
		}
		if _, ok := read.defaults[name]; !ok {
			exitf("no constant named %s in this project\n"+
				"  hint: 'mxcli constant list -p %s' shows the ones there are; a value for a "+
				"constant that does not exist is ignored by the runtime, so this would apply to nothing",
				name, projectPath)
		}

		// Establish the promise BEFORE writing the value. A secret written into a
		// path git would commit is worse than no store at all.
		switch status, err := ensureStoreIgnored(projectPath); {
		case err != nil:
			exitf("could not make %s git-ignored, so the value was not written: %v",
				constantstore.Path(projectPath), err)
		case status == ignoreNotIgnored:
			exitf("git says %s would still be committed even after adding .mxcli/ to .gitignore\n"+
				"  (something later in the ignore rules re-includes it, or the path is already tracked)\n"+
				"  the value was NOT written — this store exists to keep values out of version control",
				constantstore.Path(projectPath))
		case status == ignoreUnverified:
			fmt.Printf("note: could not ask git whether %s is ignored (no git, or not a repository)\n",
				filepath.Dir(constantstore.Path(projectPath)))
		}

		store, err := constantstore.Load(projectPath)
		if err != nil {
			exitf("%v", err)
		}
		store.Constants[name] = value
		if err := constantstore.Save(projectPath, store); err != nil {
			exitf("%v", err)
		}
		fmt.Printf("Set %s in %s (this machine only; not in version control)\n",
			name, constantstore.Path(projectPath))

		// Naming the shadowed layer matters: the author may have just set a value
		// here that the team's configuration also sets, and silently winning is
		// how "but the configuration says X" starts.
		if chain, err := resolveConstantChain(read.settings, "", nil, nil, nil); err == nil {
			if _, alsoShared := chain.Values[name]; alsoShared {
				fmt.Printf("  note: configuration %q also sets %s; this machine's value wins\n",
					chain.Configuration, name)
			}
		}

		applyIfAsked(cmd, projectPath, read, store.Constants)
	},
}

// applyIfAsked pushes the resolved values into a running `mxcli run --local`
// when --apply is set.
//
// Without it, the store is a boot-time layer only: an app already up keeps
// serving the old value until someone restarts it. With it, the change lands in
// about a second.
//
// Failure to apply is NOT failure to set. The value is already on disk and the
// next boot will use it, so this reports and returns rather than exiting
// non-zero — the alternative is a command that half-succeeded and looks like it
// did nothing.
func applyIfAsked(cmd *cobra.Command, projectPath string, read projectRead, machine map[string]string) {
	apply, _ := cmd.Flags().GetBool("apply")
	if !apply {
		return
	}
	configuration, _ := cmd.Flags().GetString("configuration")

	hs, err := readDevLoopHandshake(projectPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nThe value is saved, but not applied to a running app:\n  %v\n", err)
		return
	}
	known := make(map[string]bool, len(read.defaults))
	for n := range read.defaults {
		known[n] = true
	}
	chain, err := resolveConstantChain(read.settings, configuration, nil, machine, known)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nThe value is saved, but not applied: %v\n", err)
		return
	}

	fmt.Printf("\nApplying to the app on port %d (pid %d)...\n", hs.AppPort, hs.PID)
	if err := docker.ApplyConstants(
		docker.M2EEOptions{Host: "127.0.0.1", Port: hs.AdminPort, Token: hs.AdminPass, Direct: true},
		hs.BootConfig, chain.Values,
	); err != nil {
		fmt.Fprintf(os.Stderr, "  the value is saved, but the running app still has the old one: %v\n", err)
		return
	}
	fmt.Println("  configuration updated and model reloaded; the app is now serving the new value.")
	// Said plainly because it cannot be checked from here. The M2EE admin API has
	// no read-back — get_configuration, get_current_configuration, runtime_config
	// and get_current_runtime_status are all "Action not found" on 11.12.1 — and
	// update_configuration answers result:0 even when it changes nothing. So the
	// two calls succeeding is the strongest evidence available, and the honest
	// framing is to name what would actually confirm it.
	fmt.Println("  (the admin API offers no way to read a constant back, so confirm from the app " +
		"itself — a microflow that returns it, or the behaviour that depends on it.)")
}

var constantUnsetCmd = &cobra.Command{
	Use:   "unset <Module.Name>",
	Short: "Remove a constant's machine-local value",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := requireProjectPath(cmd)
		name := args[0]

		store, err := constantstore.Load(projectPath)
		if err != nil {
			exitf("%v", err)
		}
		if _, ok := store.Constants[name]; !ok {
			fmt.Printf("%s has no machine-local value; nothing to remove.\n", name)
			return
		}
		delete(store.Constants, name)
		if err := constantstore.Save(projectPath, store); err != nil {
			exitf("%v", err)
		}
		fmt.Printf("Removed the machine-local value for %s; runs now use the configuration or the default.\n", name)

		read, err := projectConstantDefaults(projectPath)
		if err == nil {
			applyIfAsked(cmd, projectPath, read, store.Constants)
		}
	},
}

var constantListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the value each constant resolves to, and which layer set it",
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := requireProjectPath(cmd)
		configuration, _ := cmd.Flags().GetString("configuration")
		showValues, _ := cmd.Flags().GetBool("show-values")

		read, err := projectConstantDefaults(projectPath)
		if err != nil {
			exitf("could not read the project's constants: %v", err)
		}
		store, err := constantstore.Load(projectPath)
		if err != nil {
			exitf("%v", err)
		}
		known := make(map[string]bool, len(read.defaults))
		for n := range read.defaults {
			known[n] = true
		}
		chain, err := resolveConstantChain(read.settings, configuration, nil, store.Constants, known)
		if err != nil {
			exitf("%v", err)
		}

		names := make([]string, 0, len(read.defaults))
		for n := range read.defaults {
			names = append(names, n)
		}
		sort.Strings(names)
		private := map[string]bool{}
		for _, n := range chain.Private {
			private[n] = true
		}

		nameW, valueW := len("CONSTANT"), len("VALUE")
		type row struct{ name, value, from string }
		rows := make([]row, 0, len(names))
		for _, n := range names {
			value, from := read.defaults[n], "default"
			if v, ok := chain.Values[n]; ok {
				value = v
				from = string(chain.From[n])
				if chain.From[n] == layerConfiguration {
					from = fmt.Sprintf("configuration %q", chain.Configuration)
				}
				// A value from this store is the one that exists BECAUSE it should not
				// be seen; printing it into a terminal transcript by default would
				// undo the point of the layer.
				if chain.From[n] == layerMachine && !showValues {
					value = "****"
				}
			}
			if private[n] {
				from = "default (a private override exists; its value is not in the model)"
			}
			rows = append(rows, row{n, value, from})
			if len(n) > nameW {
				nameW = len(n)
			}
			if len(value) > valueW {
				valueW = len(value)
			}
		}

		fmt.Printf("%-*s  %-*s  %s\n", nameW, "CONSTANT", valueW, "VALUE", "FROM")
		for _, r := range rows {
			fmt.Printf("%-*s  %-*s  %s\n", nameW, r.name, valueW, r.value, r.from)
		}
		if len(rows) == 0 {
			fmt.Println("(this project declares no constants)")
		}
		if chain.Note != "" {
			fmt.Printf("\nNote: %s\n", chain.Note)
		}
		if len(chain.Stale) > 0 {
			fmt.Printf("\n%d value(s) in %s name no constant of this project and are skipped:\n  %s\n",
				len(chain.Stale), constantstore.FileName, strings.Join(chain.Stale, "\n  "))
		}
		if !showValues && len(store.Constants) > 0 {
			fmt.Println("\nMachine-local values are masked; pass --show-values to print them.")
		}
	},
}

// requireProjectPath resolves -p, exiting with the same message every other
// project-scoped command uses.
func requireProjectPath(cmd *cobra.Command) string {
	projectPath, _ := cmd.Flags().GetString("project")
	if projectPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
		os.Exit(1)
	}
	return projectPath
}

func exitf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", a...)
	os.Exit(1)
}

func init() {
	for _, c := range []*cobra.Command{constantSetCmd, constantUnsetCmd} {
		c.Flags().Bool("apply", false,
			"Also apply the change to a running 'mxcli run --local' (update_configuration + reload_model). Without this the value takes effect at the next boot")
		c.Flags().String("configuration", "",
			"With --apply, which configuration's values to resolve the rest of the chain against")
	}
	constantListCmd.Flags().String("configuration", "",
		"Which configuration's values to resolve against (default: the only one, or \"Default\")")
	constantListCmd.Flags().Bool("show-values", false,
		"Print machine-local values instead of masking them")
	constantCmd.AddCommand(constantSetCmd, constantUnsetCmd, constantListCmd)
}
