// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

// redactedValue stands in for a secret in the arguments stored in run-state.json.
const redactedValue = "<redacted>"

// secretRunFlags are the `run` flags whose value is a credential. The state file
// is 0600, but it sits in .mxcli/ next to the project and travels with a copy of
// the folder, a sync client or an archive. Each accepts @env:VAR and @file:path
// (resolveRunSecrets), which keep the value off the command line and out of the
// state file, so a run started that way can still be restarted.
var secretRunFlags = map[string]bool{
	"--db-password":         true,
	"--db-admin-password":   true,
	"--screenshot-password": true,
	"--hub-secret":          true,
	"--debug-pass":          true,
}

// redactArgs returns args with the value of every secret flag replaced by
// redactedValue, and whether it replaced any. For --constant NAME=VALUE the name
// is kept and the value replaced. A value written as an indirection
// (@env:VAR, @file:path) is not a secret, only a pointer to one, and is kept so
// `run restart` can resolve it again.
func redactArgs(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	changed := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		flag, val, inline := a, "", false
		if k := strings.Index(a, "="); strings.HasPrefix(a, "--") && k > 0 {
			flag, val, inline = a[:k], a[k+1:], true
		}
		switch {
		case secretRunFlags[flag]:
			if inline {
				// An empty value holds no secret, and a reference (@env:VAR,
				// @file:path) only points at one: keep both, so a restart is not
				// refused for a value it can resolve again.
				if val == "" || isValueReference(val) {
					out = append(out, a)
					continue
				}
				out = append(out, flag+"="+redactedValue)
				changed = true
				continue
			}
			out = append(out, a)
			if i+1 < len(args) {
				i++
				if args[i] == "" || isValueReference(args[i]) {
					out = append(out, args[i])
				} else {
					out = append(out, redactedValue)
					changed = true
				}
			}
		case flag == "--runtime-setting":
			if !inline {
				out = append(out, a)
				if i+1 < len(args) {
					i++
					v, c := redactRuntimeSetting(args[i])
					out = append(out, v)
					changed = changed || c
				}
				continue
			}
			v, c := redactRuntimeSetting(val)
			out = append(out, flag+"="+v)
			changed = changed || c
		case flag == "--constant":
			if !inline {
				out = append(out, a)
				if i+1 < len(args) {
					i++
					v, c := redactConstant(args[i])
					out = append(out, v)
					changed = changed || c
				}
				continue
			}
			v, c := redactConstant(val)
			out = append(out, flag+"="+v)
			changed = changed || c
		default:
			out = append(out, a)
		}
	}
	return out, changed
}

// secretRuntimeSettings are the runtime settings whose value is a credential or
// holds constant values.
var secretRuntimeSettings = map[string]bool{
	"DatabasePassword":   true,
	"MicroflowConstants": true,
}

// redactRuntimeSetting redacts the value of one Key=Value when Key is a secret setting.
func redactRuntimeSetting(kv string) (string, bool) {
	k := strings.Index(kv, "=")
	if k < 0 || !secretRuntimeSettings[kv[:k]] || kv[k+1:] == "" {
		return kv, false
	}
	return kv[:k+1] + redactedValue, true
}

// redactConstant redacts the value of one NAME=VALUE.
func redactConstant(nv string) (string, bool) {
	k := strings.Index(nv, "=")
	if k < 0 {
		return nv, false
	}
	if isValueReference(nv[k+1:]) {
		return nv, false
	}
	return nv[:k+1] + redactedValue, true
}

// isValueReference reports whether a flag value names where the secret is
// (@env:VAR, @file:path; see resolveValueSource) rather than being it.
func isValueReference(v string) bool {
	return strings.HasPrefix(v, "@env:") || strings.HasPrefix(v, "@file:")
}

// hasRedactedArgs reports whether args carry a value redactArgs replaced, so a
// restart that would send the placeholder to the app can say so instead.
func hasRedactedArgs(args []string) bool {
	for _, a := range args {
		if a == redactedValue || strings.HasSuffix(a, "="+redactedValue) {
			return true
		}
	}
	return false
}
