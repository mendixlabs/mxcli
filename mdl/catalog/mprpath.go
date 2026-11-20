// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SameMprPath reports whether stored, the -p a catalog cache was built with,
// names the same .mpr file as current.
//
// The cache lives beside the project (<dir>/.mxcli/catalog.db), so whatever
// spelling found it found the right folder; what the path guards against is a
// second .mpr in that folder. Comparing the text was wrong: `App.mpr`,
// `./App.mpr`, an absolute path and, on Windows, `\` against `/` or another
// letter case all read the cache as "MPR path changed", and the query then ran
// on a narrower catalog built in memory (ako/mxcli#2200).
//
// A relative stored spelling was relative to wherever that command ran, so it
// is resolved in current's folder, where the cache that holds it sits. The
// files are compared with os.SameFile; when either cannot be stat'ed, the
// cleaned absolute paths are compared (case-insensitively on Windows).
func SameMprPath(stored, current string) bool {
	if stored == current {
		return true
	}
	cur, err := filepath.Abs(current)
	if err != nil {
		return false
	}
	st := filepath.FromSlash(stored) // on Windows, `/` and `\` are one separator
	if !filepath.IsAbs(st) {
		st = filepath.Join(filepath.Dir(cur), filepath.Base(st))
	}
	st = filepath.Clean(st)
	if a, err1 := os.Stat(st); err1 == nil {
		if b, err2 := os.Stat(cur); err2 == nil {
			return os.SameFile(a, b)
		}
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(st, cur)
	}
	return st == cur
}

// AbsMprPath is the spelling a cache records: absolute and clean, so a later
// command run from another folder can still resolve it. The input is returned
// unchanged when it cannot be made absolute.
func AbsMprPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
