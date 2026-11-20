// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// JarFile is one .jar file in the project's userlib/ or vendorlib/
// (ako/mxcli#1304). jar_dependencies() lists the Maven dependencies module
// settings declare; this lists what is on disk, so a rule can find one library
// on the classpath twice, in two versions.
type JarFile struct {
	Folder     string   // "userlib" or "vendorlib"
	FileName   string   // "commons-lang3-3.12.0.jar"
	Name       string   // "commons-lang3"
	Version    string   // "3.12.0"; "" when the file name carries none
	RequiredBy []string // modules with a <file>.<Module>.RequiredLib marker beside it
}

// jarFolders are the folders the Mendix runtime puts on the classpath. Only
// their top level is read: a jar in a subfolder is not on it.
var jarFolders = []string{"userlib", "vendorlib"}

// JarFiles reads the .jar files from disk each time it is called. They are
// not in the .mpr, so the catalog does not hold them and no refresh is needed.
// It returns nothing when the reader does not know the project's path.
func (ctx *LintContext) JarFiles() []JarFile {
	p, ok := ctx.reader.(interface{ Path() string })
	if !ok || p.Path() == "" {
		return nil
	}
	projectDir := filepath.Dir(p.Path())

	var out []JarFile
	for _, folder := range jarFolders {
		entries, err := os.ReadDir(filepath.Join(projectDir, folder))
		if err != nil {
			continue // no such folder: no jars in it
		}
		required := map[string][]string{}
		for _, e := range entries {
			// commons-lang3-3.12.0.jar.MyFirstModule.RequiredLib
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".RequiredLib") {
				continue
			}
			rest := strings.TrimSuffix(n, ".RequiredLib")
			i := strings.LastIndex(rest, ".jar.")
			if i < 0 {
				continue
			}
			jar := rest[:i+len(".jar")]
			required[jar] = append(required[jar], rest[i+len(".jar."):])
		}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || !strings.EqualFold(filepath.Ext(n), ".jar") {
				continue
			}
			name, version := splitJarName(strings.TrimSuffix(n, filepath.Ext(n)))
			by := required[n]
			sort.Strings(by)
			out = append(out, JarFile{Folder: folder, FileName: n, Name: name, Version: version, RequiredBy: by})
		}
	}
	return out
}

// splitJarName splits "commons-lang3-3.12.0" into "commons-lang3" and
// "3.12.0" at the LAST dash followed by a digit, so a name that itself holds
// a version-like part keeps it: "log4j-1.2-api-2.17.1" is log4j-1.2-api 2.17.1,
// and "guava-31.1-jre" is guava 31.1-jre.
func splitJarName(base string) (name, version string) {
	for i := len(base) - 2; i > 0; i-- {
		if base[i] == '-' && base[i+1] >= '0' && base[i+1] <= '9' {
			return base[:i], base[i+1:]
		}
	}
	return base, ""
}

func (r *StarlarkRule) builtinJarFiles(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if r.ctx == nil {
		return starlark.NewList(nil), nil
	}
	var out []starlark.Value
	for _, j := range r.ctx.JarFiles() {
		by := make([]starlark.Value, 0, len(j.RequiredBy))
		for _, m := range j.RequiredBy {
			by = append(by, starlark.String(m))
		}
		out = append(out, starlarkstruct.FromStringDict(starlark.String("jar_file"), starlark.StringDict{
			"folder":      starlark.String(j.Folder),
			"file_name":   starlark.String(j.FileName),
			"name":        starlark.String(j.Name),
			"version":     starlark.String(j.Version),
			"required_by": starlark.NewList(by),
		}))
	}
	return starlark.NewList(out), nil
}
