// SPDX-License-Identifier: Apache-2.0

// Command mprsnapshot emits a stable, line-oriented dump of every element in an
// MPR project, keyed by name and valued by $ID.
//
// It exists to answer one research question: when Studio Pro updates an
// installed marketplace module, does it merge into the existing elements
// (preserving in-project $IDs) or replace them (renumbering)? See
// docs/11-proposals/PROPOSAL_marketplace_module_upgrade.md.
//
// The output is designed so that `diff before.txt after.txt` classifies every
// element into exactly one of four cases:
//
//	line identical          element untouched
//	same id=, changed h=    merged in place        <- the ID-preserving hypothesis
//	changed id=             replaced / renumbered  <- the destructive hypothesis
//	line added / removed    element added / deleted
//
// Note this is the OPPOSITE normalisation from drift detection, which compares
// DESCRIBE output precisely because $IDs are noise there. Here the $ID is the
// entire signal, so nothing is normalised away except inside the content hash.
//
// Usage:
//
//	go run ./scripts/mprsnapshot -p app.mpr > before.txt
//	go run ./scripts/mprsnapshot -p app.mpr --module Administration > before-admin.txt
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func main() {
	projectPath := flag.String("p", "", "path to the .mpr project (required)")
	moduleFilter := flag.String("module", "", "restrict output to a single module")
	includeUnnamed := flag.Bool("all", false, "include nested elements that carry no Name (index-keyed, noisy)")
	showRefs := flag.Bool("refs", false, "also emit R lines: every cross-element pointer and its target $ID")
	canon := flag.Bool("canon", false, "emit only C lines: per-unit canonical digests, with element IDs normalised away (for idempotence measurement)")
	flag.Parse()

	if *projectPath == "" {
		fmt.Fprintln(os.Stderr, "error: -p is required")
		os.Exit(2)
	}

	if err := run(*projectPath, *moduleFilter, *includeUnnamed, *showRefs, *canon); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// unit is one row of the Unit table joined with the name read out of its contents.
type unit struct {
	id          string
	containerID string
	containment string
	typ         string
	name        string
	contents    map[string]any
	path        string // resolved lazily from the containment tree
}

func run(projectPath, moduleFilter string, includeUnnamed, showRefs, canon bool) error {
	reader := modelsdkbackend.New()
	err := reader.ConnectReadOnly(projectPath)
	if err != nil {
		return fmt.Errorf("opening project: %w", err)
	}
	defer func() { _ = reader.Disconnect() }()

	infos, err := reader.ListUnits()
	if err != nil {
		return fmt.Errorf("listing units: %w", err)
	}

	units := make(map[string]*unit, len(infos))
	for _, in := range infos {
		u := &unit{
			id:          string(in.ID),
			containerID: string(in.ContainerID),
			containment: in.ContainmentName,
			typ:         in.Type,
		}
		// A unit with unreadable contents is still reported — silently dropping it
		// would make a deleted element indistinguishable from a parse failure.
		if raw, err := reader.GetRawUnit(in.ID); err == nil {
			u.contents = raw
			if n, ok := raw["Name"].(string); ok {
				u.name = n
			}
		}
		units[u.id] = u
	}

	for _, u := range units {
		u.path = resolvePath(u, units, 0)
	}

	mendixVersion, _ := reader.GetMendixVersion()

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	fmt.Fprintf(out, "# mprsnapshot/1\n")
	fmt.Fprintf(out, "# project\t%s\n", projectPath)
	fmt.Fprintf(out, "# mendix\t%s\n", mendixVersion)
	fmt.Fprintf(out, "# units\t%d\n", len(units))
	if moduleFilter != "" {
		fmt.Fprintf(out, "# module\t%s\n", moduleFilter)
	}
	fmt.Fprintf(out, "#\n")

	// Canonical mode answers a different question from the rest of this tool, so
	// it emits its own line type and returns: "if mxcli skipped writes that
	// change nothing, how many of these units would it skip?"
	//
	// C <unit-id> <path> <type> <canon> <masked>
	//
	// Keyed by unit id, not path: several unnamed units share a path (they fall
	// back to their containment name), so a path key is ambiguous and pairs the
	// wrong units between two runs. A document keeps its own id across writes —
	// that is the whole premise being measured — so the id is the stable join.
	//
	// Compare two runs with `diff`. A unit absent from the diff would have been
	// skipped. A unit whose canon differs but whose masked matches differs only
	// in a field mxcli regenerates by policy (volatileFields) — reachable, but
	// only after that policy changes.
	if canon {
		sel := make([]*unit, 0, len(units))
		for _, u := range units {
			// inModule("x", "") is false — it tests prefix, not "unfiltered" —
			// so an empty filter must be handled explicitly or this emits only
			// the project root and an empty diff reads as "nothing changed".
			if u.contents == nil || (moduleFilter != "" && !inModule(u.path, moduleFilter)) {
				continue
			}
			sel = append(sel, u)
		}
		sort.Slice(sel, func(i, j int) bool { return sel[i].id < sel[j].id })
		for _, u := range sel {
			c, m := canonicalDigests(u.contents)
			path := u.path
			if path == "" {
				path = "(root)"
			}
			fmt.Fprintf(out, "C\t%s\t%s\t%s\t%s\t%s\n", u.id, path, u.typ, c, m)
		}
		return nil
	}
	fmt.Fprintf(out, "# U = unit, E = nested element, R = cross-element pointer.\n")
	fmt.Fprintf(out, "# Columns: kind, path, $Type, id, hash-or-guid\n")
	fmt.Fprintf(out, "# hash covers the unit's contents with every $ID removed, so a changed\n")
	fmt.Fprintf(out, "# hash on an unchanged id means the element was edited in place.\n")

	var lines []string
	for _, u := range units {
		if moduleFilter != "" && !inModule(u.path, moduleFilter) {
			continue
		}
		lines = append(lines, fmt.Sprintf("U\t%s\t%s\tid=%s\th=%s",
			u.path, u.typ, u.id, hashWithoutIDs(u.contents)))

		for _, e := range nestedElements(u.contents, u.path, includeUnnamed) {
			guid := ""
			if e.guid != "" {
				guid = "guid=" + e.guid
			}
			lines = append(lines, fmt.Sprintf("E\t%s\t%s\tid=%s\t%s", e.path, e.typ, e.id, guid))
		}

		if showRefs {
			for _, r := range references(u.contents) {
				lines = append(lines, fmt.Sprintf("R\t%s%s\t\t\t", u.path, r))
			}
		}
	}

	sort.Strings(lines)
	for _, l := range lines {
		fmt.Fprintln(out, strings.TrimRight(l, "\t"))
	}
	return nil
}

// resolvePath walks up the containment chain to build a name-based path.
// The depth guard stops a corrupt project from hanging the tool.
func resolvePath(u *unit, units map[string]*unit, depth int) string {
	if depth > 32 {
		return "<cycle>/" + u.name
	}
	segment := u.name
	if segment == "" {
		segment = u.containment
	}
	if segment == "" {
		segment = u.id
	}

	parent, ok := units[u.containerID]
	if !ok || u.containerID == "" || u.containerID == u.id {
		// The project root is unnamed, so its segment would be a bare UUID
		// prefixing every path — noise in the diff and it breaks --module.
		// Paths start at the module instead.
		if u.typ == projectRootType {
			return ""
		}
		return segment
	}

	prefix := resolvePath(parent, units, depth+1)
	if prefix == "" {
		return segment
	}
	return prefix + "/" + segment
}

// projectRootType is the $Type of the single unit that contains every module.
const projectRootType = "Projects$Project"

func inModule(path, module string) bool {
	return path == module || strings.HasPrefix(path, module+"/")
}

type element struct {
	path string
	typ  string
	id   string
	guid string // domain-model elements carry a GUID distinct from their $ID
}

// The mongo driver decodes nested documents as primitive.M and arrays as
// primitive.A. Those are *named* types, so a plain `.(map[string]any)`
// assertion fails on them and a walk written against the builtin types silently
// finds nothing. Every traversal below goes through these two helpers.

func asMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case bson.M:
		return t, true
	case bson.D:
		out := make(map[string]any, len(t))
		for _, e := range t {
			out[e.Key] = e.Value
		}
		return out, true
	}
	return nil, false
}

func asSlice(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case bson.A:
		return t, true
	}
	return nil, false
}

func asString(m map[string]any, key string) (string, bool) {
	s, ok := m[key].(string)
	return s, ok
}

// elementID reads a $ID, which Mendix stores as a 16-byte BSON binary UUID
// (matching the Unit table's BLOB primary key) rather than as a string. Reading
// it as a string yields nothing, which is how an entire snapshot can come back
// with no nested elements at all.
func elementID(m map[string]any) (string, bool) {
	switch t := m["$ID"].(type) {
	case primitive.Binary:
		if len(t.Data) != 16 {
			return "", false
		}
		return types.BlobToUUID(t.Data), true
	case string:
		return t, true
	}
	return "", false
}

// binaryUUID renders a 16-byte binary field, or "" when the field is absent.
//
// GUID is not a reference and not the $ID: domain-model elements carry it as a
// second, separate identity. It is tracked on its own column because $ID and
// GUID can move independently, and an upgrade that preserves one while
// renumbering the other has very different consequences depending on which.
func binaryUUID(v any) string {
	if b, ok := v.(primitive.Binary); ok && len(b.Data) == 16 {
		return types.BlobToUUID(b.Data)
	}
	return ""
}

// nestedElements walks a unit's BSON and reports every nested object carrying a
// $ID. Named objects get a name-keyed path so the key stays stable across an
// upgrade; unnamed ones are index-keyed and off by default, because a reordered
// list would otherwise read as a wall of changes.
func nestedElements(v any, basePath string, includeUnnamed bool) []element {
	var found []element
	walk(v, basePath, includeUnnamed, 0, &found)
	sort.Slice(found, func(i, j int) bool { return found[i].path < found[j].path })
	return found
}

func walk(v any, path string, includeUnnamed bool, depth int, found *[]element) {
	if depth > 64 {
		return
	}

	if m, ok := asMap(v); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			if k == "$ID" || k == "$Type" {
				continue
			}
			child := m[k]
			childPath := path + "/" + k

			if cm, ok := asMap(child); ok {
				emit(cm, childPath, includeUnnamed, depth, found)
				continue
			}
			if arr, ok := asSlice(child); ok {
				for i, item := range arr {
					if im, ok := asMap(item); ok {
						emit(im, indexedPath(childPath, im, i), includeUnnamed, depth, found)
					}
				}
			}
		}
		return
	}

	if arr, ok := asSlice(v); ok {
		for i, item := range arr {
			walk(item, fmt.Sprintf("%s[%d]", path, i), includeUnnamed, depth+1, found)
		}
	}
}

// emit records a nested object if it has a $ID, then recurses into it.
func emit(m map[string]any, path string, includeUnnamed bool, depth int, found *[]element) {
	id, hasID := elementID(m)
	name, hasName := asString(m, "Name")

	if hasID && (hasName || includeUnnamed) {
		p := path
		if hasName {
			p = path + "/" + name
		}
		typ, _ := asString(m, "$Type")
		*found = append(*found, element{path: p, typ: typ, id: id, guid: binaryUUID(m["GUID"])})
		path = p
	}

	walk(m, path, includeUnnamed, depth+1, found)
}

// references reports every cross-element pointer in a unit: a 16-byte binary
// field that is not the element's own $ID. These are what a consuming module
// uses to name an element inside a marketplace module (a generalization, a
// microflow's retrieve, an access rule), so they are the direct evidence for
// whether an upgrade rewrites consumers or relies on IDs holding still.
func references(v any) []string {
	var out []string
	collectRefs(v, "", 0, &out)
	sort.Strings(out)
	return out
}

func collectRefs(v any, path string, depth int, out *[]string) {
	if depth > 64 {
		return
	}
	if m, ok := asMap(v); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "$ID" {
				continue
			}
			if b, ok := m[k].(primitive.Binary); ok && len(b.Data) == 16 {
				*out = append(*out, fmt.Sprintf("%s/%s=%s", path, k, types.BlobToUUID(b.Data)))
				continue
			}
			collectRefs(m[k], path+"/"+k, depth+1, out)
		}
		return
	}
	if arr, ok := asSlice(v); ok {
		for i, item := range arr {
			collectRefs(item, fmt.Sprintf("%s[%d]", path, i), depth+1, out)
		}
	}
}

func indexedPath(base string, m map[string]any, i int) string {
	if name, ok := asString(m, "Name"); ok && name != "" {
		// The name is appended by emit; keep the base clean so the index does not
		// leak into the key of a named element.
		_ = name
		return base
	}
	return fmt.Sprintf("%s[%d]", base, i)
}

// hashWithoutIDs fingerprints a unit's contents with every $ID stripped, so that
// "same element, edited" is distinguishable from "same element, untouched"
// independently of whether IDs moved.
//
// Reference fields (Pointer, ...Ref) are deliberately NOT stripped: if a
// referenced element was renumbered, the referring unit's hash should change —
// that is how a renumber propagating into consumers becomes visible.
func hashWithoutIDs(v any) string {
	h := sha256.New()
	writeCanonical(h, stripIDs(v))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func stripIDs(v any) any {
	if m, ok := asMap(v); ok {
		out := make(map[string]any, len(m))
		for k, val := range m {
			if k == "$ID" {
				continue
			}
			out[k] = stripIDs(val)
		}
		return out
	}
	if arr, ok := asSlice(v); ok {
		out := make([]any, len(arr))
		for i, item := range arr {
			out[i] = stripIDs(item)
		}
		return out
	}
	return v
}

// writeCanonical serialises deterministically — map iteration order must never
// reach the hash, or two snapshots of the same project would differ.
func writeCanonical(h interface{ Write([]byte) (int, error) }, v any) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		_, _ = h.Write([]byte("{"))
		for _, k := range keys {
			_, _ = h.Write([]byte(k))
			_, _ = h.Write([]byte(":"))
			writeCanonical(h, t[k])
			_, _ = h.Write([]byte(","))
		}
		_, _ = h.Write([]byte("}"))
	case []any:
		_, _ = h.Write([]byte("["))
		for _, item := range t {
			writeCanonical(h, item)
			_, _ = h.Write([]byte(","))
		}
		_, _ = h.Write([]byte("]"))
	default:
		_, _ = fmt.Fprintf(h, "%v", v)
	}
}
