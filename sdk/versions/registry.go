// SPDX-License-Identifier: Apache-2.0

// Package versions provides a version feature registry for Mendix projects.
// Feature definitions are loaded from embedded YAML files, providing a single
// source of truth for what MDL capabilities are available in each Mendix version.
//
// Version bounds use min_version / max_version semver notation, aligned with
// the Mendix Content API (Marketplace) conventions.
package versions

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed mendix-9.yaml mendix-10.yaml mendix-11.yaml
var yamlFS embed.FS

// Registry holds the loaded version feature data for all supported Mendix versions.
type Registry struct {
	specs []VersionSpec
	// index maps "area.feature" to all FeatureEntry records across specs.
	index map[string][]FeatureEntry
}

// VersionSpec represents one YAML file (one major version).
type VersionSpec struct {
	Major          int                           `yaml:"major"`
	SupportedRange string                        `yaml:"supported_range"`
	LTSVersions    []string                      `yaml:"lts_versions"`
	MTSVersions    []string                      `yaml:"mts_versions"`
	Features       map[string]map[string]Feature `yaml:"features"`
	Deprecated     []DeprecatedEntry             `yaml:"deprecated"`
	Upgrade        map[string][]UpgradeEntry     `yaml:"upgrade_opportunities"`
}

// Feature represents a single capability in the registry.
type Feature struct {
	MinVersion string      `yaml:"min_version"`
	MaxVersion string      `yaml:"max_version,omitempty"`
	MDL        string      `yaml:"mdl,omitempty"`
	Notes      string      `yaml:"notes,omitempty"`
	Workaround *Workaround `yaml:"workaround,omitempty"`
}

// Workaround describes an alternative when a feature is not available.
type Workaround struct {
	Description string `yaml:"description"`
	MaxVersion  string `yaml:"max_version"`
}

// DeprecatedEntry describes a deprecated pattern.
type DeprecatedEntry struct {
	ID         string `yaml:"id"`
	Pattern    string `yaml:"pattern"`
	ReplacedBy string `yaml:"replaced_by"`
	Since      string `yaml:"since"`
	Severity   string `yaml:"severity"`
}

// UpgradeEntry describes an upgrade opportunity.
type UpgradeEntry struct {
	Feature     string `yaml:"feature"`
	Description string `yaml:"description"`
	Effort      string `yaml:"effort"`
}

// FeatureEntry is a flattened, queryable representation of a feature.
type FeatureEntry struct {
	Area       string // e.g. "domain_model", "microflows"
	Name       string // e.g. "view_entities", "page_parameters"
	MinVersion SemVer
	MaxVersion *SemVer // nil means unbounded
	MDL        string
	Notes      string
	Workaround *Workaround
}

// SemVer is a parsed major.minor.patch version.
type SemVer struct {
	Major int
	Minor int
	Patch int
}

// ParseSemVer parses a "major.minor.patch" string. Missing components default to 0.
func ParseSemVer(s string) (SemVer, error) {
	var v SemVer
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return v, fmt.Errorf("invalid version %q: need at least major.minor", s)
	}
	var err error
	v.Major, err = strconv.Atoi(parts[0])
	if err != nil {
		return v, fmt.Errorf("invalid major in %q: %w", s, err)
	}
	v.Minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return v, fmt.Errorf("invalid minor in %q: %w", s, err)
	}
	if len(parts) >= 3 {
		v.Patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return v, fmt.Errorf("invalid patch in %q: %w", s, err)
		}
	}
	return v, nil
}

func (v SemVer) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// AtLeast returns true if v >= other.
func (v SemVer) AtLeast(other SemVer) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch >= other.Patch
}

// Load reads all embedded YAML files and returns a populated Registry.
func Load() (*Registry, error) {
	files := []string{"mendix-9.yaml", "mendix-10.yaml", "mendix-11.yaml"}
	r := &Registry{
		index: make(map[string][]FeatureEntry),
	}

	for _, name := range files {
		data, err := yamlFS.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		var spec VersionSpec
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		r.specs = append(r.specs, spec)

		// Build index from this spec.
		for area, features := range spec.Features {
			for name, f := range features {
				entry, err := toEntry(area, name, f)
				if err != nil {
					return nil, fmt.Errorf("feature %s.%s in %d: %w", area, name, spec.Major, err)
				}
				key := area + "." + name
				r.index[key] = append(r.index[key], entry)
			}
		}
	}

	// Deduplicate index entries (same feature may appear in multiple YAML files).
	for key, entries := range r.index {
		r.index[key] = dedup(entries)
	}

	return r, nil
}

func toEntry(area, name string, f Feature) (FeatureEntry, error) {
	minV, err := ParseSemVer(f.MinVersion)
	if err != nil {
		return FeatureEntry{}, fmt.Errorf("min_version: %w", err)
	}
	entry := FeatureEntry{
		Area:       area,
		Name:       name,
		MinVersion: minV,
		MDL:        f.MDL,
		Notes:      f.Notes,
		Workaround: f.Workaround,
	}
	if f.MaxVersion != "" {
		maxV, err := ParseSemVer(f.MaxVersion)
		if err != nil {
			return FeatureEntry{}, fmt.Errorf("max_version: %w", err)
		}
		entry.MaxVersion = &maxV
	}
	return entry, nil
}

// dedup removes duplicate entries (same area+name+min_version), keeping the
// entry with the most information: a stated max_version first — the file that
// records a removal knows more than one written before it (mendix-10.yaml and
// mendix-11.yaml both list message_definition_collection from 10.0.0, and only
// the 11 file closes it at 11.14.99, ako/mxcli#2140) — then the longest MDL or
// notes.
func dedup(entries []FeatureEntry) []FeatureEntry {
	seen := make(map[string]int) // min_version string -> index in result
	var result []FeatureEntry
	for _, e := range entries {
		key := e.MinVersion.String()
		if idx, ok := seen[key]; ok {
			// Keep the entry with more detail.
			existing := result[idx]
			if (e.MaxVersion != nil) != (existing.MaxVersion != nil) {
				if e.MaxVersion != nil {
					result[idx] = e
				}
				continue
			}
			if len(e.MDL) > len(existing.MDL) || len(e.Notes) > len(existing.Notes) {
				result[idx] = e
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, e)
	}
	return result
}

// IsAvailable returns true if the feature identified by "area.name" is
// available in the given project version.
func (r *Registry) IsAvailable(area, name string, projectVersion SemVer) bool {
	key := area + "." + name
	entries, ok := r.index[key]
	if !ok {
		return false
	}
	for _, e := range entries {
		if projectVersion.AtLeast(e.MinVersion) {
			if e.MaxVersion != nil && !e.MaxVersion.AtLeast(projectVersion) {
				continue
			}
			return true
		}
	}
	return false
}

// Lookup returns the feature area/name, or false when the registry has none:
// the entry with the lowest MinVersion when the YAML files disagree, since that
// is the version below which none of it exists. `mxcli syntax` reads its
// "Requires Mendix" lines from here (ako/mxcli#1367).
func (r *Registry) Lookup(area, name string) (FeatureEntry, bool) {
	entries, ok := r.index[area+"."+name]
	if !ok || len(entries) == 0 {
		return FeatureEntry{}, false
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if best.MinVersion.AtLeast(e.MinVersion) && best.MinVersion != e.MinVersion {
			best = e
		}
	}
	return best, true
}

// Keys returns every feature as "area/name", sorted.
func (r *Registry) Keys() []string {
	out := make([]string, 0, len(r.index))
	for k := range r.index {
		out = append(out, strings.Replace(k, ".", "/", 1))
	}
	sort.Strings(out)
	return out
}

// FeaturesForVersion returns all features and their availability status for a
// given project version. Results are sorted alphabetically by display name.
func (r *Registry) FeaturesForVersion(projectVersion SemVer) []FeatureStatus {
	var result []FeatureStatus
	seen := make(map[string]bool)

	for key, entries := range r.index {
		if seen[key] {
			continue
		}
		seen[key] = true

		best, available := entryAt(entries, projectVersion)
		result = append(result, FeatureStatus{
			FeatureEntry: best,
			Available:    available,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].DisplayName() < result[j].DisplayName()
	})

	return result
}

// covers reports whether v lies in the entry's [min_version, max_version].
func (e FeatureEntry) covers(v SemVer) bool {
	return v.AtLeast(e.MinVersion) && (e.MaxVersion == nil || e.MaxVersion.AtLeast(v))
}

// entryAt picks the entry that describes a feature at version v, and reports
// whether the feature is available there.
//
// Most features have one floor, so every entry agrees and the lowest
// min_version is the one to show. A feature Mendix shipped on two release
// lines has two that do not: STRING_AGG in OQL is 10.24.22 on the 10.24 line
// (closed by a max_version) and 11.2.0 on 11 (ako/mxcli#1650). Taking the
// lowest floor there reports it unavailable on 11.6 — 10.24.22's range does
// not reach it — and tells an 11.1 project it needs "10.24.22+". So: the
// lowest entry whose range holds v; failing that, the next floor above v, the
// one an upgrade reaches first; failing that (a feature Mendix removed), the
// lowest floor.
func entryAt(entries []FeatureEntry, v SemVer) (FeatureEntry, bool) {
	var covering, next *FeatureEntry
	lowest := &entries[0]
	for i := range entries {
		e := &entries[i]
		if !e.MinVersion.AtLeast(lowest.MinVersion) {
			lowest = e
		}
		switch {
		case e.covers(v):
			if covering == nil || !e.MinVersion.AtLeast(covering.MinVersion) {
				covering = e
			}
		case !v.AtLeast(e.MinVersion):
			if next == nil || !e.MinVersion.AtLeast(next.MinVersion) {
				next = e
			}
		}
	}
	if covering != nil {
		return *covering, true
	}
	if next != nil {
		return *next, false
	}
	return *lowest, false
}

// FeaturesAddedSince returns features that become available when upgrading
// from sinceVersion. Only features whose min_version > sinceVersion are included.
func (r *Registry) FeaturesAddedSince(sinceVersion SemVer) []FeatureEntry {
	var result []FeatureEntry
	seen := make(map[string]bool)

	for key, entries := range r.index {
		if seen[key] {
			continue
		}
		seen[key] = true

		// Added since: not available at sinceVersion, and a floor above it.
		if best, available := entryAt(entries, sinceVersion); !available && !sinceVersion.AtLeast(best.MinVersion) {
			result = append(result, best)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].DisplayName() < result[j].DisplayName()
	})

	return result
}

// FeaturesInArea returns all features in a given area for the specified version.
func (r *Registry) FeaturesInArea(area string, projectVersion SemVer) []FeatureStatus {
	var result []FeatureStatus
	seen := make(map[string]bool)

	for key, entries := range r.index {
		if seen[key] {
			continue
		}
		best, available := entryAt(entries, projectVersion)
		if best.Area != area {
			continue
		}
		seen[key] = true

		result = append(result, FeatureStatus{
			FeatureEntry: best,
			Available:    available,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].DisplayName() < result[j].DisplayName()
	})

	return result
}

// Areas returns the sorted list of distinct feature areas.
func (r *Registry) Areas() []string {
	set := make(map[string]bool)
	for _, entries := range r.index {
		for _, e := range entries {
			set[e.Area] = true
		}
	}
	var areas []string
	for a := range set {
		areas = append(areas, a)
	}
	sort.Strings(areas)
	return areas
}

// DeprecatedPatterns returns all deprecated entries applicable to the given version.
func (r *Registry) DeprecatedPatterns(projectVersion SemVer) []DeprecatedEntry {
	var result []DeprecatedEntry
	for _, spec := range r.specs {
		for _, d := range spec.Deprecated {
			since, err := ParseSemVer(d.Since)
			if err != nil {
				continue
			}
			if projectVersion.AtLeast(since) {
				result = append(result, d)
			}
		}
	}
	return result
}

// UpgradeOpportunities returns upgrade suggestions from the current major to a target.
func (r *Registry) UpgradeOpportunities(fromMajor, toMajor int) []UpgradeEntry {
	key := fmt.Sprintf("from_%d_to_%d", fromMajor, toMajor)
	for _, spec := range r.specs {
		if entries, ok := spec.Upgrade[key]; ok {
			return entries
		}
	}
	return nil
}

// FeatureStatus pairs a feature entry with its availability for a specific version.
type FeatureStatus struct {
	FeatureEntry
	Available bool
}

// DisplayName returns a human-readable name for the feature (e.g., "View entities").
func (e *FeatureEntry) DisplayName() string {
	return strings.ReplaceAll(strings.ReplaceAll(e.Name, "_", " "), "  ", " ")
}
