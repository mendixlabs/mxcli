// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// resolveRunSecrets returns the values of the secret `run` flags
// (secretRunFlags), keyed by flag name without the dashes, with every @env:VAR
// and @file:path resolved by resolveValueSource — the rules `--constant`
// follows: an unset or empty variable, an empty file and a file its group or
// others can read are refused, and an error names the variable or the file,
// never the value.
//
// A reference keeps the secret out of `ps`, shell history and run-state.json
// (redactArgs keeps the reference and drops a literal), so a run started with
// one can be detached and restarted: each start resolves it again, in the
// environment of the process that starts it.
func resolveRunSecrets(cmd *cobra.Command) (map[string]string, error) {
	names := make([]string, 0, len(secretRunFlags))
	for flag := range secretRunFlags {
		names = append(names, strings.TrimPrefix(flag, "--"))
	}
	sort.Strings(names) // a stable first error
	out := make(map[string]string, len(names))
	for _, name := range names {
		v, err := cmd.Flags().GetString(name)
		if err != nil {
			return nil, err
		}
		resolved, err := resolveValueSource(v)
		if err != nil {
			return nil, fmt.Errorf("--%s: %w", name, err)
		}
		out[name] = resolved
	}
	return out, nil
}

// secretRefHelp is appended to each secret flag's help text.
const secretRefHelp = "; @env:VAR or @file:path reads it from an environment variable or an owner-only file instead, keeping it off the command line and letting 'run restart' work (@@ for a literal leading @)"
