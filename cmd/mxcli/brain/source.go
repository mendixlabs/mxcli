// SPDX-License-Identifier: Apache-2.0

// source.go - which document, and which part of it, an entry came from.
//
// A requirements tool is believable when clicking a requirement shows the line
// it came from, and useful when it can say which parts of a document produced
// no requirement at all (ako/mxcli#1951). Both need the source recorded on the
// entry, in a form that survives a checkout on another machine — so a path
// relative to the project directory, never an absolute one — and pointing at a
// file the repository actually has.
package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NormalizeSource turns what a user typed — a path, absolute or relative to
// base, optionally followed by #locator — into the stored form: relative to
// base, / separated. It refuses a file that does not exist or lies outside
// base: provenance pointing at a file nobody else has is not provenance.
//
// The locator is free text ("L18-21", "Goals", "section 3"); only a backtick or
// a line break is refused, since either would break the shard line.
func NormalizeSource(base, raw string) (string, error) {
	path, locator, hasLocator := strings.Cut(strings.TrimSpace(raw), "#")
	if path == "" {
		return "", fmt.Errorf("--source needs a document path, optionally followed by #<locator>")
	}
	if strings.ContainsAny(locator, "`\n\r") {
		return "", fmt.Errorf("source locator %q: no backticks or line breaks", locator)
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(base, abs)
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	abs, err = filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absBase, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source %s is outside the project (%s); keep the document in the repository so the provenance travels with it", path, absBase)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("source %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("source %s is a directory, not a document", path)
	}
	out := filepath.ToSlash(rel)
	if hasLocator && locator != "" {
		out += "#" + locator
	}
	return out, nil
}

// SourcePath is the document part of a stored source, without the locator.
func SourcePath(source string) string {
	path, _, _ := strings.Cut(source, "#")
	return path
}

// SourceFinding is an entry whose source document is missing.
type SourceFinding struct {
	Shard   string `json:"shard"`
	EntryID string `json:"entry_id"`
	Title   string `json:"title"`
	Source  string `json:"source"`
}

// ProjectDir is the directory the store belongs to — what sources are
// relative to.
func (s *Store) ProjectDir() string {
	return filepath.Clean(strings.TrimSuffix(s.Root, filepath.FromSlash(StoreDir)))
}

func (s *Store) missingSources(shard string, entries []Entry) []SourceFinding {
	var out []SourceFinding
	for _, e := range entries {
		if e.Source == "" {
			continue
		}
		p := filepath.Join(s.ProjectDir(), filepath.FromSlash(SourcePath(e.Source)))
		if _, err := os.Stat(p); err != nil {
			out = append(out, SourceFinding{Shard: shard, EntryID: e.ID, Title: e.Title, Source: e.Source})
		}
	}
	return out
}
