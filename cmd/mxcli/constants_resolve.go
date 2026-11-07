// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/constantstore"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/model"
)

// constantLayer names where a resolved constant value came from. It exists so a
// run can SAY which layer won: with more than one place to set a value, "the
// value is X" stops being useful on its own, and the whole family of bugs this
// resolves (mxcli-chat FINDINGS §33) is values arriving from somewhere other
// than where the author looked.
type constantLayer string

const (
	layerFlag          constantLayer = "--constant"
	layerMachine       constantLayer = "this machine"
	layerConfiguration constantLayer = "configuration"
)

// constantChain is the resolved set of values to hand a booting app, plus the
// layer each one came from.
//
// A constant absent from Values keeps its default, which the deployment already
// carries — mxbuild writes every constant's default into
// deployment/model/config.json, and mxcli layers over that rather than
// replacing it. See docs/11-proposals/PROPOSAL_constant_values.md.
type constantChain struct {
	Configuration string                   // the configuration whose values these are
	Values        map[string]string        // constant qualified name -> value
	From          map[string]constantLayer // and which layer set it
	Private       []string                 // overrides whose value is not in the model
	Stale         []string                 // machine-store entries naming no constant of this project
	Note          string                   // why no configuration contributed
	// JVMArgs are the same configuration's JVM options (runjvm.go). They ride on
	// the chain because they come from the one configuration choice and the one
	// project read; a second read would cost a second open of the project.
	JVMArgs []string
}

// parseConstantFlags turns repeated `--constant Module.Name=value` into a map.
//
// A missing "=" is an error rather than a value of "": `--constant M.C` almost
// certainly means the author expected the next argument to be the value, and
// silently setting the constant to empty is exactly the kind of quiet wrong
// value this whole feature exists to stop.
//
// The value may point at the secret instead of being it, so that it stays out of
// `ps` and shell history: `Module.Name=@env:VAR` reads an environment variable
// and `Module.Name=@file:path` reads a file. See resolveValueSource.
func parseConstantFlags(flags []string) (map[string]string, error) {
	out := make(map[string]string, len(flags))
	for _, f := range flags {
		name, value, found := strings.Cut(f, "=")
		name = strings.TrimSpace(name)
		if !found {
			return nil, fmt.Errorf("--constant %q: expected Module.Name=value", f)
		}
		if strings.Count(name, ".") != 1 || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
			return nil, fmt.Errorf("--constant %q: %q is not a qualified constant name (Module.Name)", f, name)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("--constant %s given more than once", name)
		}
		resolved, err := resolveValueSource(value)
		if err != nil {
			return nil, fmt.Errorf("--constant %s: %w", name, err)
		}
		out[name] = resolved
	}
	return out, nil
}

// resolveValueSource turns the text after the "=" of a flag value into the value
// itself:
//
//	@env:VAR    the environment variable VAR; unset or empty is an error
//	@file:PATH  the contents of the file, minus one trailing newline; an empty
//	            file is an error; on unix the file must not be readable by its
//	            group or by others (not checked on Windows)
//	@@text      the literal "@text", for a value that really starts with "@"
//	text        itself
//
// Any other value starting with "@" is refused rather than taken literally: a
// mistyped `@evn:TOKEN` would otherwise be applied as the constant's value, and
// that is the quiet wrong value this flag family is built to refuse. Errors name
// the variable or the file, never the value.
func resolveValueSource(v string) (string, error) {
	if !strings.HasPrefix(v, "@") {
		return v, nil
	}
	switch {
	case strings.HasPrefix(v, "@@"):
		return v[1:], nil
	case strings.HasPrefix(v, "@env:"):
		name := strings.TrimPrefix(v, "@env:")
		if name == "" {
			return "", fmt.Errorf("@env: needs a variable name (@env:VAR)")
		}
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", fmt.Errorf("environment variable %s is not set", name)
		}
		if val == "" {
			return "", fmt.Errorf("environment variable %s is set but empty", name)
		}
		return val, nil
	case strings.HasPrefix(v, "@file:"):
		return readValueFile(strings.TrimPrefix(v, "@file:"))
	}
	return "", fmt.Errorf("a value starting with @ is read from the environment (@env:VAR) or a file (@file:path); " +
		"write @@ for a literal leading @")
}

// readValueFile reads one value from a file that only its owner can read.
func readValueFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("@file: needs a path (@file:/path/to/file)")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("@file:%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("@file:%s is not a regular file", path)
	}
	// Windows reports synthetic permission bits, so the check would only be noise.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("@file:%s is readable by group or others (mode %04o); restrict it first: chmod 600 %s",
			path, info.Mode().Perm(), path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("@file:%s: %w", path, err)
	}
	val := strings.TrimSuffix(string(body), "\n")
	val = strings.TrimSuffix(val, "\r")
	if val == "" {
		return "", fmt.Errorf("@file:%s is empty", path)
	}
	return val, nil
}

// absoluteFileRefArgs returns args with every relative `@file:path` made
// absolute against the current directory, in a --constant value and in the
// value of a secret flag (secretRunFlags), so a command stored for detach or
// restart still finds the file from another directory. The input is not
// modified.
func absoluteFileRefArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	fixRef := func(v string) string {
		p, ok := strings.CutPrefix(v, "@file:")
		if !ok || p == "" || filepath.IsAbs(p) {
			return v
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return v
		}
		return "@file:" + abs
	}
	fixConstant := func(nv string) string {
		k := strings.Index(nv, "=")
		if k < 0 {
			return nv
		}
		return nv[:k+1] + fixRef(nv[k+1:])
	}
	for i := 0; i < len(out); i++ {
		a := out[i]
		flag, val, inline := a, "", false
		if k := strings.Index(a, "="); strings.HasPrefix(a, "--") && k > 0 {
			flag, val, inline = a[:k], a[k+1:], true
		}
		var fix func(string) string
		switch {
		case flag == "--constant":
			fix = fixConstant
		case secretRunFlags[flag]:
			fix = fixRef
		default:
			continue
		}
		if inline {
			out[i] = flag + "=" + fix(val)
		} else if i+1 < len(out) {
			i++
			out[i] = fix(out[i])
		}
	}
	return out
}

// resolveConstantChain layers `--constant` values over a configuration's shared
// values. known is the set of constants the project declares; a flag naming
// anything else is refused.
//
// Refusing an unknown name is the point of passing `known` at all. The runtime
// ignores a MicroflowConstants entry that matches no constant, so a typo would
// otherwise be accepted, reported as applied, and do nothing — the §33 failure
// shape, reintroduced by the very flag meant to fix it.
func resolveConstantChain(ps *model.ProjectSettings, want string, flags, machine map[string]string, known map[string]bool) (constantChain, error) {
	base := resolveConstantOverrides(ps, want)
	chain := constantChain{
		Configuration: base.Configuration,
		Values:        map[string]string{},
		From:          map[string]constantLayer{},
		Private:       base.Private,
		Note:          base.Note,
	}
	for name, value := range base.Values {
		chain.Values[name] = value
		chain.From[name] = layerConfiguration
	}
	if chosen, _ := chooseConfiguration(ps, want); chosen != nil {
		args, err := configurationJVMArgs(chosen)
		if err != nil {
			return constantChain{}, err
		}
		chain.JVMArgs = args
	}

	var unknown []string
	for name := range flags {
		if known != nil && !known[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return constantChain{}, fmt.Errorf("no constant named %s in this project\n"+
			"  hint: 'mxcli -p <app.mpr> -c \"show constant values\"' lists them; the runtime ignores "+
			"a value for a constant that does not exist, so this would have been applied to nothing",
			strings.Join(unknown, ", "))
	}

	// A constant the project no longer declares must not be applied from the
	// machine store either — but a stale entry there is the user's own file, not
	// a typo they can fix by rerunning, so it is skipped and named rather than
	// refused. Refusing would make every run fail until the file was edited by
	// hand, over a value the project stopped caring about.
	var stale []string
	for name, value := range machine {
		if known != nil && !known[name] {
			stale = append(stale, name)
			continue
		}
		chain.Values[name] = value
		chain.From[name] = layerMachine
	}
	sort.Strings(stale)
	chain.Stale = stale

	for name, value := range flags {
		chain.Values[name] = value
		chain.From[name] = layerFlag
	}
	// A value set above the configuration means the default is NOT what runs, so
	// the constant must stop being reported as private-and-defaulted, or the
	// report contradicts what the app is about to do.
	if len(chain.Private) > 0 {
		kept := chain.Private[:0]
		for _, name := range chain.Private {
			if _, overridden := chain.Values[name]; !overridden {
				kept = append(kept, name)
			}
		}
		chain.Private = kept
	}
	return chain, nil
}

// constantChainFor reads a project and resolves the values a run should use.
//
// A project that cannot be read yields no values and a note — a run must not be
// blocked by this, since every run before configurations were applied used the
// defaults anyway. The exception is when `--constant` was passed: the author
// named something specific, and applying it unvalidated (or dropping it
// silently) are both worse than saying the project could not be read.
func constantChainFor(projectPath, configuration string, flags map[string]string) (constantChain, error) {
	// The machine store is read FIRST and its failure is always fatal. Unlike a
	// project that cannot be read — where falling back to the defaults is what
	// every earlier run did anyway — a store that exists and cannot be parsed
	// means values the author deliberately set are about to be silently absent.
	store, err := constantstore.Load(projectPath)
	if err != nil {
		return constantChain{}, err
	}

	defaults, err := projectConstantDefaults(projectPath)
	if err != nil {
		return unreadableProject(err, flags)
	}
	known := make(map[string]bool, len(defaults.defaults))
	for name := range defaults.defaults {
		known[name] = true
	}
	return resolveConstantChain(defaults.settings, configuration, flags, store.Constants, known)
}

// projectRead is what one open of the project yields: its settings and every
// constant it declares, with each default value.
type projectRead struct {
	settings *model.ProjectSettings
	defaults map[string]string // constant qualified name -> default value
}

func projectConstantDefaults(projectPath string) (projectRead, error) {
	b := newBackendFactory()()
	if err := b.Connect(projectPath); err != nil {
		return projectRead{}, err
	}
	defer func() { _ = b.Disconnect() }()

	ps, err := b.GetProjectSettings()
	if err != nil {
		return projectRead{}, err
	}
	defaults, err := constantDefaults(b)
	if err != nil {
		return projectRead{}, err
	}
	return projectRead{settings: ps, defaults: defaults}, nil
}

func unreadableProject(err error, flags map[string]string) (constantChain, error) {
	if len(flags) > 0 {
		return constantChain{}, fmt.Errorf("could not read the project's constants to apply --constant: %w", err)
	}
	return constantChain{
		Values: map[string]string{},
		From:   map[string]constantLayer{},
		Note:   "could not read the project's settings: " + err.Error(),
	}, nil
}

// constantDefaults maps every constant the project declares to its default
// value. Folders do not appear in a qualified name, so the hierarchy is used
// only to find each constant's module.
func constantDefaults(b backend.FullBackend) (map[string]string, error) {
	constants, err := b.ListConstants()
	if err != nil {
		return nil, err
	}
	h, err := executor.NewContainerHierarchyFromBackend(b)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(constants))
	for _, c := range constants {
		if module := h.GetModuleName(h.FindModuleID(c.ContainerID)); module != "" {
			out[module+"."+c.Name] = c.DefaultValue
		}
	}
	return out, nil
}

// reportConstantChain says what will be applied before the app boots, and from
// where.
//
// It prints even when nothing is applied. The failure this fixes was invisible
// precisely because the run said nothing about constants either way, so silence
// has to stop meaning "your override is in effect".
func reportConstantChain(w io.Writer, c constantChain) {
	switch {
	case len(c.Values) > 0:
		names := make([]string, 0, len(c.Values))
		width := 0
		for k := range c.Values {
			names = append(names, k)
			if len(k) > width {
				width = len(k)
			}
		}
		sort.Strings(names)
		fmt.Fprintf(w, "Applying %d constant value(s):\n", len(names))
		for _, n := range names {
			from := string(c.From[n])
			if c.From[n] == layerConfiguration {
				from = fmt.Sprintf("configuration %q", c.Configuration)
			}
			fmt.Fprintf(w, "  %-*s  %s\n", width, n, from)
		}
	case c.Configuration != "":
		fmt.Fprintf(w, "Configuration %q sets no constant values; using each constant's default.\n", c.Configuration)
	case c.Note != "":
		fmt.Fprintf(w, "Using each constant's default value (%s).\n", c.Note)
	}
	if len(c.Private) > 0 {
		fmt.Fprintf(w, "  %d override(s) are private, so their value is not in the model and the default is used:\n    %s\n",
			len(c.Private), strings.Join(c.Private, "\n    "))
	}
	if len(c.Stale) > 0 {
		fmt.Fprintf(w, "  %d value(s) in %s name no constant of this project and were skipped:\n    %s\n",
			len(c.Stale), constantstore.FileName, strings.Join(c.Stale, "\n    "))
	}
}
