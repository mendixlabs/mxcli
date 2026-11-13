// SPDX-License-Identifier: Apache-2.0

// Pluggable widget .def.json extraction. Shared between the `mxcli widget
// init` CLI command and in-executor entry points (refresh catalog, etc.).
//
// A widget definition file (.def.json) tells the pluggable widget engine
// which MDL keywords route into which widget property keys. The file is
// derived from the widget's .mpk package (the React widget bundle that
// Mendix Studio Pro and the runtime use). Whenever mxcli is upgraded and
// learns to emit new fields (e.g. `objectLists` for engine-routed widgets
// like Accordion / Maps / PopupMenu), existing on-disk definitions can
// become stale. RefreshWidgetDefinitions handles that transparently:
// generate fresh content, compare byte-by-byte, overwrite when drifted.
package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// WidgetDefRefreshStats reports the outcome of a RefreshWidgetDefinitions
// call.
type WidgetDefRefreshStats struct {
	Extracted int // newly written defs (no prior file)
	Refreshed int // content drifted — overwrote stale def
	UpToDate  int // content matched — no write
	Skipped   int // skipped (built-in or unparseable mpk)
	// Unchanged reports that nothing was looked at: widgets/ and the
	// definitions are as the same mxcli build left them (#1848).
	Unchanged bool
}

// RefreshWidgetDefinitions scans projectDir/widgets/ for .mpk files and
// (re)generates projectDir/.mxcli/widgets/<name>.def.json for each.
// Auto-refreshes definitions whose generated content has drifted (the case
// that triggers "unsupported widget type: group" after upgrading mxcli).
//
// projectPath is the path to the .mpr (or any file in its directory).
// force=true rewrites every .def.json unconditionally.
// If output is non-nil, per-widget changes are written with `+` (new) /
// `~` (refreshed) markers.
func RefreshWidgetDefinitions(projectPath string, force bool, output io.Writer) (WidgetDefRefreshStats, error) {
	projectDir := filepath.Dir(projectPath)
	widgetsDir := filepath.Join(projectDir, "widgets")
	outputDir := filepath.Join(projectDir, ".mxcli", "widgets")

	var stats WidgetDefRefreshStats

	matches, err := filepath.Glob(filepath.Join(widgetsDir, "*.mpk"))
	if err != nil {
		return stats, fmt.Errorf("failed to scan widgets directory: %w", err)
	}
	if len(matches) == 0 {
		return stats, nil
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return stats, fmt.Errorf("failed to create output directory: %w", err)
	}

	if !force && widgetStateUnchanged(projectDir) {
		stats.Unchanged = true
		return stats, nil
	}

	// Built-in registry — definitions hand-crafted in sdk/widgets/definitions/
	// (COMBOBOX, GALLERY, DATAGRID, filters, …). Skip those when extracting
	// from the project; the built-in def overrides any .mpk-derived one.
	builtinRegistry, _ := NewWidgetRegistry()

	widgets, unparseable := scanProjectWidgets(matches)
	stats.Skipped += unparseable

	for _, w := range widgets {
		mpkPath, mpkDef, mdlName := w.mpkPath, w.def, w.mdlName
		filename := strings.ToLower(mdlName) + ".def.json"
		outPath := filepath.Join(outputDir, filename)

		if builtinRegistry != nil {
			if _, ok := builtinRegistry.GetByWidgetID(mpkDef.ID); ok {
				stats.Skipped++
				continue
			}
		}

		defJSON := GenerateDefJSON(mpkDef, mdlName)
		// Lift property-visibility rules from the widget's editorConfig.js
		// (#574 Phase 2) so the generated .def.json carries the version-
		// specific applicability logic. Merge with the hand-transcribed table
		// rather than replace it: the static extractor skips some guard
		// shapes, so the hand-authored fallback fills the keys it misses.
		// Extracted rules win on conflict (they're version-specific).
		if rules := extractVisibilityRulesFromMPK(mpkPath, mpkDef.ID); len(rules) > 0 {
			defJSON.PropertyVisibility = mergeVisibilityRules(rules, widgetVisibilityRules[mpkDef.ID])
		}
		freshData, err := json.MarshalIndent(defJSON, "", "  ")
		if err != nil {
			log.Printf("warning: skipping %s: %v", mpkDef.ID, err)
			stats.Skipped++
			continue
		}
		freshData = append(freshData, '\n')

		existingData, existsErr := os.ReadFile(outPath)
		switch {
		case existsErr != nil:
			stats.Extracted++
		case bytes.Equal(existingData, freshData):
			if force {
				stats.Refreshed++
			} else {
				stats.UpToDate++
				continue
			}
		default:
			stats.Refreshed++
		}

		if err := writeDefinitionFile(outPath, freshData); err != nil {
			return stats, fmt.Errorf("failed to write %s: %w", outPath, err)
		}
		if output != nil {
			kind := "custom"
			if mpkDef.IsPluggable {
				kind = "pluggable"
			}
			marker := "+"
			if existsErr == nil {
				marker = "~"
			}
			fmt.Fprintf(output, "  %s %-12s %-20s %s\n", marker, kind, mdlName, mpkDef.ID)
		}
	}

	saveWidgetState(projectDir)
	return stats, nil
}

// projectWidget is one widget of a project's widgets/*.mpk, with the MDL name
// its definition is written under.
type projectWidget struct {
	mpkPath string
	def     *mpk.WidgetDefinition
	mdlName string
}

// scanProjectWidgets parses the given .mpk files and names every widget in
// them. A package bundles one widget or many (Charts.mpk). The second count
// is the packages that could not be parsed.
//
// Names are assigned over the whole project rather than per widget, because
// two widget IDs can derive the same name: KeyboardShortcut.mpk, a legacy
// custom widget, and itvisors.KeyboardShortcut.mpk, its pluggable successor,
// are both KEYBOARDSHORTCUT. Each overwrote the other's definition on every
// refresh, so one widget had none and every refresh rewrote both (#1848).
// Everything that maps a widget to its definition file must name it here.
func scanProjectWidgets(mpkPaths []string) ([]projectWidget, int) {
	var widgets []projectWidget
	unparseable := 0
	seenID := map[string]bool{}
	for _, mpkPath := range mpkPaths {
		defs, err := mpk.ParseMPKAll(mpkPath)
		if err != nil {
			log.Printf("warning: skipping %s: %v", filepath.Base(mpkPath), err)
			unparseable++
			continue
		}
		for _, def := range defs {
			// The same widget installed twice is one widget.
			if seenID[def.ID] {
				continue
			}
			seenID[def.ID] = true
			widgets = append(widgets, projectWidget{mpkPath: mpkPath, def: def, mdlName: DeriveMDLName(def.ID)})
		}
	}

	byName := map[string][]int{}
	taken := map[string]bool{}
	for i, w := range widgets {
		byName[w.mdlName] = append(byName[w.mdlName], i)
		taken[w.mdlName] = true
	}
	names := make([]string, 0, len(byName))
	for name, idx := range byName {
		if len(idx) > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		idx := byName[name]
		// The pluggable widget keeps the keyword (the one people build with
		// today), then the lowest ID, so the outcome does not depend on the
		// order the packages are read in.
		sort.Slice(idx, func(a, b int) bool {
			wa, wb := widgets[idx[a]].def, widgets[idx[b]].def
			if wa.IsPluggable != wb.IsPluggable {
				return wa.IsPluggable
			}
			return wa.ID < wb.ID
		})
		for _, i := range idx[1:] {
			widgets[i].mdlName = qualifiedMDLName(widgets[i].def, taken)
			taken[widgets[i].mdlName] = true
			log.Printf("info: widgets %s and %s both derive %s; %s is registered as %s",
				widgets[idx[0]].def.ID, widgets[i].def.ID, name, widgets[i].def.ID, widgets[i].mdlName)
		}
	}
	return widgets, unparseable
}

// genericIDSegments are widget-ID segments that say nothing about whose widget
// it is, so they do not make a name distinct.
var genericIDSegments = map[string]bool{"com": true, "mendix": true, "widget": true, "widgets": true, "web": true, "native": true}

// qualifiedMDLName names a widget whose derived name another widget keeps: its
// vendor segment in front (itvisors.keyboardshortcut.KeyboardShortcut →
// ITVISORSKEYBOARDSHORTCUT), CUSTOM in front for a legacy custom widget with no
// such segment (KeyboardShortcut.widget.KeyboardShortcut →
// CUSTOMKEYBOARDSHORTCUT), and a number behind as the last resort.
func qualifiedMDLName(def *mpk.WidgetDefinition, taken map[string]bool) string {
	base := DeriveMDLName(def.ID)
	parts := strings.Split(def.ID, ".")
	last := parts[len(parts)-1]
	var candidates []string
	for _, seg := range parts[:len(parts)-1] {
		seg = identifierPart(seg)
		if seg == "" || strings.EqualFold(seg, last) || genericIDSegments[strings.ToLower(seg)] {
			continue
		}
		candidates = append(candidates, strings.ToUpper(seg)+base)
		break
	}
	if !def.IsPluggable {
		candidates = append(candidates, "CUSTOM"+base)
	}
	for _, c := range candidates {
		if !taken[c] {
			return c
		}
	}
	for n := 2; ; n++ {
		if c := fmt.Sprintf("%s%d", base, n); !taken[c] {
			return c
		}
	}
}

// identifierPart keeps the letters, digits and underscores of s.
func identifierPart(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, s)
}

// widgetStateFile records what the last refresh saw, beside the definitions.
// It does not end in .def.json, so no definition scan reads it.
const widgetStateFile = "refresh-state.json"

// widgetStateFingerprint names the running mxcli build. A refresh is skipped
// only for the build that wrote the state: another may generate different
// definitions from the same packages. "" never skips. Tests replace it.
var widgetStateFingerprint = sourceFingerprint

type widgetRefreshState struct {
	Fingerprint string            `json:"fingerprint"`
	Files       map[string]string `json:"files"` // project-relative path → size|mtime
}

// widgetFiles stats every widgets/*.mpk and .mxcli/widgets/*.def.json. A
// stat, unlike a read, does not make Windows Defender scan the file.
func widgetFiles(projectDir string) map[string]string {
	files := map[string]string{}
	for _, pattern := range []string{
		filepath.Join(projectDir, "widgets", "*.mpk"),
		filepath.Join(projectDir, ".mxcli", "widgets", "*.def.json"),
	} {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			fi, err := os.Stat(m)
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(projectDir, m)
			if err != nil {
				rel = m
			}
			files[filepath.ToSlash(rel)] = fmt.Sprintf("%d|%d", fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return files
}

// widgetStateUnchanged reports whether the packages and definitions are
// exactly as the last refresh by this mxcli build left them, so refreshing
// again would only re-read every .mpk to rewrite nothing. That is the normal
// case for a refresh after a model edit (#1848).
func widgetStateUnchanged(projectDir string) bool {
	fp := widgetStateFingerprint()
	if fp == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(projectDir, ".mxcli", "widgets", widgetStateFile))
	if err != nil {
		return false
	}
	var prev widgetRefreshState
	if json.Unmarshal(data, &prev) != nil || prev.Fingerprint != fp {
		return false
	}
	return maps.Equal(prev.Files, widgetFiles(projectDir))
}

// saveWidgetState records what this refresh left behind. Best-effort: without
// it the next refresh just does the work again.
func saveWidgetState(projectDir string) {
	fp := widgetStateFingerprint()
	if fp == "" {
		return
	}
	data, err := json.Marshal(widgetRefreshState{Fingerprint: fp, Files: widgetFiles(projectDir)})
	if err != nil {
		return
	}
	_ = writeDefinitionFile(filepath.Join(projectDir, ".mxcli", "widgets", widgetStateFile), data)
}

// writeDefinitionFile replaces a .def.json so that a reader sees the old
// content or the new, never a part of either. Definitions are read while they
// are refreshed — by another mxcli process, the language server, or (until
// #1799) the describe workers of the same catalog build — and os.WriteFile
// truncates first: a reader that lands in between gets an empty or half file,
// fails to parse it, and loads a registry without that widget.
//
// The temporary file does not end in .def.json, so a directory scan never
// reads it. Where the rename cannot replace the file (Windows, while a reader
// holds it open), it falls back to the plain write.
func writeDefinitionFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return os.WriteFile(path, data, 0644)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil && cerr == nil {
		if err := os.Chmod(tmpPath, 0644); err == nil {
			if err := os.Rename(tmpPath, path); err == nil {
				return nil
			}
		}
	}
	_ = os.Remove(tmpPath)
	return os.WriteFile(path, data, 0644)
}

// RefreshStaleWidgetDefinitions makes a project's `.mxcli/widgets/*.def.json`
// current before the engine reads them, in two cases:
//
//  1. No defs exist yet (project never `widget init`-ed) but `.mpk` widgets are
//     installed — generate them from the `.mpk` files. This makes `exec`
//     self-sufficient; without it, the first build of a project widget fails
//     with "unsupported widget type" telling the user to run `widget init`.
//  2. Defs exist but were generated by an older mxcli build (their
//     `generatorVersion` stamp is behind WidgetDefGeneratorVersion) — refresh
//     them, otherwise they'd silently emit stale BSON (spurious CE0463 after
//     the v0.12.0 widget work).
//
// Returns true if any def was generated or refreshed. The stamp check reads
// only `generatorVersion` from each def.json — no `.mpk` parsing — so the
// common "defs present and current" case is cheap; the expensive generate /
// regenerate only runs when something is missing or behind. (Mirrors what
// `refresh catalog` does via RefreshWidgetDefinitions.)
func RefreshStaleWidgetDefinitions(projectPath string) (bool, error) {
	if projectPath == "" {
		return false, nil
	}
	defsDir := filepath.Join(filepath.Dir(projectPath), ".mxcli", "widgets")
	entries, err := os.ReadDir(defsDir)

	hasDefs := false
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".def.json") {
				hasDefs = true
				break
			}
		}
	}

	// Case 1: no defs yet — generate from installed .mpk. RefreshWidgetDefinitions
	// no-ops (empty stats) when the project has no widgets/*.mpk, so this is
	// cheap on projects without pluggable widgets.
	if !hasDefs {
		stats, genErr := RefreshWidgetDefinitions(projectPath, false, nil)
		if genErr != nil {
			return false, genErr
		}
		return stats.Extracted > 0 || stats.Refreshed > 0, nil
	}

	// Case 2: defs exist — cheap stamp scan; refresh only if any is behind.
	stale := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".def.json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(defsDir, e.Name()))
		if readErr != nil {
			continue
		}
		var stamp struct {
			GeneratorVersion int `json:"generatorVersion"`
		}
		// Unparseable or unstamped (version 0) counts as stale.
		_ = json.Unmarshal(data, &stamp)
		if stamp.GeneratorVersion < WidgetDefGeneratorVersion {
			stale = true
			break
		}
	}
	if !stale {
		return false, nil
	}

	if _, err := RefreshWidgetDefinitions(projectPath, true, nil); err != nil {
		return false, err
	}
	return true, nil
}

// DeriveMDLName derives the uppercase MDL keyword name from a widget ID
// (e.g. "com.mendix.widget.web.accordion.Accordion" → "ACCORDION").
//
// A native mobile widget is prefixed with NATIVE
// ("com.mendix.widget.native.image.Image" → "NATIVEIMAGE"): Mendix ships most
// of them under the same last segment as a web widget or a built-in keyword
// (image, radiobuttons, badge, …), and the bare name let the native package
// take over the web keyword (mendixlabs/mxcli#1463).
func DeriveMDLName(widgetID string) string {
	parts := strings.Split(widgetID, ".")
	name := strings.ToUpper(parts[len(parts)-1])
	for _, seg := range parts[:len(parts)-1] {
		if strings.EqualFold(seg, "native") {
			if !strings.HasPrefix(name, "NATIVE") {
				name = "NATIVE" + name
			}
			break
		}
	}
	return name
}

// GenerateDefJSON creates a skeleton WidgetDefinition from an mpk.WidgetDefinition.
// Properties are handled explicitly from MDL via the engine's explicit property pass,
// so no propertyMappings or childSlots are generated for unknown types.
func GenerateDefJSON(mpkDef *mpk.WidgetDefinition, mdlName string) *WidgetDefinition {
	widgetKind := "custom"
	if mpkDef.IsPluggable {
		widgetKind = "pluggable"
	}
	def := &WidgetDefinition{
		WidgetID:         mpkDef.ID,
		MDLName:          mdlName,
		WidgetKind:       widgetKind,
		TemplateFile:     strings.ToLower(mdlName) + ".json",
		DefaultEditable:  "Always",
		GeneratorVersion: WidgetDefGeneratorVersion,
	}

	// Generate property mappings and child slots from MPK property definitions.
	// Two passes: datasource first (association depends on entityContext set by datasource).
	var assocMappings []PropertyMapping
	for _, p := range mpkDef.Properties {
		// Object-list properties (e.g. Accordion `groups`, DataGrid `columns`)
		// are emitted as ObjectListMapping entries.
		if p.Type == "object" && p.IsList {
			def.ObjectLists = append(def.ObjectLists, makeObjectListMapping(mpkDef.ID, p))
			continue
		}
		switch p.Type {
		case "widgets":
			container := mdlContainerForWidgetSlot(mpkDef.ID, p.Key)
			def.ChildSlots = append(def.ChildSlots, ChildSlotMapping{
				PropertyKey:  p.Key,
				MDLContainer: container,
				Operation:    "widgets",
			})
		case "datasource":
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "DataSource",
				Operation:   "datasource",
				Description: p.Description,
			})
		case "attribute":
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "Attribute",
				Operation:   "attribute",
				Description: p.Description,
				MdlAliases:  propertyAliases[mpkDef.ID][p.Key],
			})
		case "textTemplate":
			// Emit a mapping for every top-level texttemplate property so its
			// content is authorable by the property's own MDL name (e.g. Badge
			// `value`, TreeNode `headerCaption`, Timeline `title`/`description`).
			// Previously these were skipped unless a hand-registered alias existed,
			// which silently dropped the caption (MDL-WIDGET01). The engine keeps
			// the template's default ClientTemplate when the property is left unset
			// (see applyOperation "texttemplate"), so emitting the mapping never
			// nulls a default — it only enables authoring. Registered aliases (e.g.
			// PieChart `seriesName` ← `SeriesName`) are still carried for widgets
			// that expose a friendlier MDL keyword.
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "TextTemplate",
				Operation:   "texttemplate",
				Description: p.Description,
				MdlAliases:  propertyAliases[mpkDef.ID][p.Key],
			})
		case "association":
			assocMappings = append(assocMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "Association",
				Operation:   "association",
				Description: p.Description,
			})
		case "selection":
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "Selection",
				Operation:   "selection",
				Default:     p.DefaultValue,
				Description: p.Description,
			})
		case "action":
			// Every action-typed property gets a mapping, so the engine's
			// applyOperation "action" can write it (ledger #67 — before that, none
			// did, and an action was dropped with no error or warning).
			//
			// The two slots MDL names — onClick → the widget's Action property,
			// onChange → OnChange — keep a Source and are read from those fixed AST
			// slots. Every OTHER action slot is emitted with NO Source and is
			// addressed by its own PropertyKey: `createFileAction: microflow …`.
			// That is the convention the object-list item mappings already use, so
			// it adds no concept, and it leaves actionSourceForKey meaning what it
			// meant — the MDL alias for a key, or none.
			//
			// Before this, an unaliased slot got no mapping at all: File Uploader
			// extracted to six action slots and none were authorable, so the widget
			// could be placed and not wired (#956).
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      actionSourceForKey(p.Key),
				Operation:   "action",
				Description: p.Description,
			})
		case "expression":
			// A top-level expression property is authored by its own key, like a
			// named action slot. It had no mapping at all, so it was a
			// known-but-unmapped key: the Calendar's `showEventDate` drew
			// MDL-WIDGET06 and its `editable` was unreachable (mendixlabs/mxcli#1578).
			// An unset one keeps the template's default (applyOperation).
			aliases := propertyAliases[mpkDef.ID][p.Key]
			if strings.EqualFold(p.Key, "editable") {
				if mpkDef.SystemPropertyKeys()["Editability"] {
					// `editable:` is the widget-level editability here, which the
					// package declares; the widget's own property of that name
					// stays unmapped rather than taking the setting's spelling.
					continue
				}
				// No Editability system property, so `editable:` can only mean
				// this property. EditableIf is the key the visitor gives its
				// bare-expression form; `Editable` already matches the key.
				aliases = append(append([]string(nil), aliases...), "EditableIf")
			}
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Operation:   "expression",
				Description: p.Description,
				MdlAliases:  aliases,
			})
		case "boolean", "integer", "decimal", "string", "enumeration":
			m := PropertyMapping{
				PropertyKey: p.Key,
				Operation:   "primitive",
				Description: p.Description,
			}
			if p.DefaultValue != "" {
				m.Value = p.DefaultValue
			}
			def.PropertyMappings = append(def.PropertyMappings, m)
		}
	}
	def.PropertyMappings = append(def.PropertyMappings, assocMappings...)

	// KnownProperties: every property the widget's definition (.mpk) declares that
	// has NO mapping in the generated def — i.e. a real property mxcli does not
	// persist. Computed purely from the two artifacts mxcli already has (the .mpk
	// property list and the generated mappings), with no per-widget knowledge. The
	// checker uses this to WARN "recognized but not persisted; the value will be
	// dropped" instead of silently discarding it or falsely rejecting it as an
	// unknown property. This is the general guard for the class behind ledger #67
	// (a type the generator doesn't map — e.g. `expression`, `icon`, or an action
	// slot with no MDL surface); the specific fix for that finding is the `action`
	// mapping above.
	mapped := make(map[string]bool)
	markMapped := func(key string) {
		if key != "" {
			mapped[strings.ToLower(key)] = true
		}
	}
	for _, m := range def.PropertyMappings {
		markMapped(m.PropertyKey)
		for _, a := range m.MdlAliases {
			markMapped(a)
		}
	}
	for _, cs := range def.ChildSlots {
		markMapped(cs.PropertyKey)
	}
	for _, ol := range def.ObjectLists {
		markMapped(ol.PropertyKey)
	}
	for _, m := range def.Modes {
		for _, pm := range m.PropertyMappings {
			markMapped(pm.PropertyKey)
			for _, a := range pm.MdlAliases {
				markMapped(a)
			}
		}
		for _, cs := range m.ChildSlots {
			markMapped(cs.PropertyKey)
		}
	}
	for _, p := range mpkDef.Properties {
		if p.Key == "" || mapped[strings.ToLower(p.Key)] {
			continue
		}
		def.KnownProperties = append(def.KnownProperties, p.Key)
	}

	def.PropertyVisibility = widgetVisibilityRules[mpkDef.ID]

	return def
}

// widgetVisibilityRules holds hand-authored property-visibility rules for
// widgets whose editorConfig.js hides TextTemplate properties under certain
// configurations. Until the JS extractor lands (#574 Phase 2), these are
// transcribed by hand from each widget's compiled editorConfig.js.
//
// The entries cover TextTemplate properties only, and that is NOT a statement
// that only TextTemplates matter. Any hidden property must hold its default: an
// enumeration hidden at a non-default value is CE0463 just the same (Timeline's
// groupByDayOptions in custom mode, ako/mxcli#1762). This table once made that
// rule look recognised — `check` warned about the Timeline's hidden templates,
// from here, while the enumerations in the very same hide went unchecked.
//
//	VideoPlayer (editorConfig.js):
//	  "expression"===e.type && hidePropertiesIn(["videoUrl","posterUrl"])
//	Timeline (editorConfig.js):
//	  e.customVisualization ? hidePropertiesIn(["title","description","icon","timeIndication",...]) : ...
//
// The extractor now lifts the whole Timeline hide (3.1.1, 3.2.2 and 3.2.3 all
// read 6 of 6), so its entry only matters for a package that ships no
// editorConfig.
//
// mergeVisibilityRules returns the extracted rules plus any hand-authored fallback
// rules whose PropertyKey the extractor did not cover. Extracted rules are
// version-specific (lifted from the installed .mpk's editorConfig.js) and win on
// conflict; the fallback fills guards the static extractor skips.
func mergeVisibilityRules(extracted, fallback []types.WidgetVisibilityRule) []types.WidgetVisibilityRule {
	if len(fallback) == 0 {
		return extracted
	}
	covered := make(map[string]bool, len(extracted))
	for _, r := range extracted {
		covered[r.PropertyKey] = true
	}
	merged := extracted
	for _, r := range fallback {
		if !covered[r.PropertyKey] {
			merged = append(merged, r)
		}
	}
	return merged
}

var widgetVisibilityRules = map[string][]types.WidgetVisibilityRule{
	"com.mendix.widget.web.videoplayer.VideoPlayer": {
		{PropertyKey: "videoUrl", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "type", Operator: "eq", Value: "expression"}},
		{PropertyKey: "posterUrl", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "type", Operator: "eq", Value: "expression"}},
	},
	"com.mendix.widget.web.timeline.Timeline": {
		{PropertyKey: "title", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "customVisualization", Operator: "truthy"}},
		{PropertyKey: "description", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "customVisualization", Operator: "truthy"}},
		{PropertyKey: "timeIndication", HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "customVisualization", Operator: "truthy"}},
	},
}

// widgetSlotKeywordOverrides is the shared slot-keyword table in mdl/types,
// where the page mutator reads it too (`<widget>.<keyword>` addresses a slot).
var widgetSlotKeywordOverrides = types.WidgetSlotKeywordOverrides

// mdlContainerForWidgetSlot returns the MDL keyword for a widgets-typed
// property — see types.WidgetSlotKeyword.
func mdlContainerForWidgetSlot(widgetID, propertyKey string) string {
	return types.WidgetSlotKeyword(widgetID, propertyKey)
}

// itemSlotAcceptedChildTypes lists widget MDL keywords that route to a
// given item slot when they appear inside the item body without an explicit
// MDLContainer wrapper. Keyed by (widgetID, objectListPropertyKey,
// itemSlotPropertyKey) → list of widget Type keywords.
//
// Example: a DataGrid column accepts `textfilter`, `numberfilter`,
// `datefilter`, `dropdownfilter` inside its body, routing them to the
// column's `filter` slot rather than the default `content` slot.
var itemSlotAcceptedChildTypes = map[string]map[string]map[string][]string{
	"com.mendix.widget.web.datagrid.Datagrid": {
		"columns": {
			"filter": {"textfilter", "numberfilter", "datefilter", "dropdownfilter"},
		},
	},
}

// itemPropertyAliases is the shared alias table in mdl/types.
//
// It used to be declared here as a literal, with the page mutator keeping a
// second hand-written copy for the ALTER path. The two drifted — see
// types.ItemPropertyAliases — so the table moved to a package both can import
// and this is now a reference, not a duplicate.
var itemPropertyAliases = types.ItemPropertyAliases

// propertyAliases lists alternative MDL names for a widget's TOP-LEVEL properties
// (not object-list items — those use itemPropertyAliases). Needed where a widget
// has several attribute/texttemplate-typed properties whose friendly MDL keyword
// differs from the schema key. Keyed by (widgetID, propertyKey).
//
// Charts that bind their data at the widget level (PieChart, HeatMap — no series
// object-list) expose `seriesDataSource` + `seriesValueAttribute` + `seriesName`.
// The friendly MDL is `DataSource:` / `ValueAttribute:` / `SeriesName:`.
var propertyAliases = map[string]map[string][]string{
	"com.mendix.widget.web.piechart.PieChart": {
		"seriesValueAttribute": {"ValueAttribute"},
		"seriesName":           {"SeriesName"},
	},
	"com.mendix.widget.web.heatmap.HeatMap": {
		"seriesValueAttribute": {"ValueAttribute"},
	},
}

// makeObjectListMapping converts an MPK object-list PropertyDef (e.g. Accordion
// `groups`) into an ObjectListMapping. The MDL keyword is the singular form of
// the property key (groups → GROUP, basicItems → ITEM, series → SERIES,
// markers → MARKER).
// actionSourceForKey maps an action-typed property key to the BuildContext
// resolution source the engine understands. Only the action slots MDL can author
// today are wired: `onClick` → the widget's Action property, `onChange` →
// OnChange. Other action slots (e.g. DataGrid2 `onSelectionChange`) have no MDL
// surface yet, so they return "" and no mapping is emitted.
// Mendix's own pluggable widgets suffix their action slots — the Combobox names
// its on-change slot `onChangeEvent`, not `onChange` — so the bare key is not
// enough. One `Event`/`Action` suffix is stripped before matching (ledger #14).
// The stripping is deliberately narrow: a Combobox also carries
// `onChangeFilterInputEvent` and `onChangeDatabaseEvent`, which are separate
// properties — the first authored by its own key (#956, ako/mxcli#854), the
// second the database mode's change slot, which its definition sources from
// `OnChange:` explicitly — and must not answer to `OnChange:` here, or one
// `OnChange:` would write three different actions.
func actionSourceForKey(key string) string {
	k := strings.ToLower(key)
	// A slot keyed plain `action` is the widget's one action, and `action:` is
	// MDL's own keyword for it (Switch: "On change", key "action"). Trimmed as
	// a suffix below it became the empty name, so the slot took no source, the
	// keyword routed nowhere, and describe's `action: …` of a stored switch
	// was refused as MDL-WIDGET37 (ako/mxcli#2114).
	if k == "action" {
		return "OnClick"
	}
	k = strings.TrimSuffix(k, "event")
	k = strings.TrimSuffix(k, "action")
	switch k {
	case "onclick":
		return "OnClick"
	case "onchange":
		return "OnChange"
	}
	return ""
}

func makeObjectListMapping(widgetID string, p mpk.PropertyDef) ObjectListMapping {
	mapping := ObjectListMapping{
		PropertyKey:  p.Key,
		MDLContainer: deriveObjectListKeyword(p.Key),
	}
	aliases := itemPropertyAliases[widgetID][p.Key]
	slotAccepts := itemSlotAcceptedChildTypes[widgetID][p.Key]
	for _, child := range p.Children {
		if child.Type == "widgets" {
			mapping.ItemSlots = append(mapping.ItemSlots, ItemSlotMapping{
				PropertyKey:        child.Key,
				MDLContainer:       strings.ToUpper(child.Key),
				Operation:          "widgets",
				AcceptedChildTypes: slotAccepts[child.Key],
			})
			continue
		}
		op := operationForType(child.Type)
		if op == "" {
			continue
		}
		item := ItemPropertyMapping{
			PropertyKey: child.Key,
			Operation:   op,
			Description: child.Description,
			MdlAliases:  aliases[child.Key],
			DataSource:  child.DataSource,
			EnumValues:  child.EnumValues,
		}
		switch op {
		case "attribute":
			item.Source = "Attribute"
		case "datasource":
			item.Source = "DataSource"
		case "association":
			item.Source = "Association"
		case "primitive":
			if child.DefaultValue != "" {
				item.Value = child.DefaultValue
			}
		}
		mapping.ItemProperties = append(mapping.ItemProperties, item)
	}
	return mapping
}

// deriveObjectListKeyword turns a property key like "groups" / "basicItems" /
// "series" / "markers" into an uppercase MDL keyword in the singular form.
func deriveObjectListKeyword(propertyKey string) string {
	overrides := map[string]string{
		"basicItems":     "ITEM",
		"customItems":    "CUSTOMITEM",
		"dynamicMarkers": "DYNAMICMARKER",
		"attributesList": "ATTR",
		"filterOptions":  "OPTION",
		"series":         "SERIES", // Latin singular == plural
	}
	if k, ok := overrides[propertyKey]; ok {
		return k
	}
	lower := strings.ToLower(propertyKey)
	singular := strings.TrimSuffix(lower, "s")
	return strings.ToUpper(singular)
}

// operationForType maps an MPK property type to the engine's operation name.
// Returns "" for unsupported types (which are skipped in object-list extraction).
func operationForType(t string) string {
	switch t {
	case "attribute":
		return "attribute"
	case "association":
		return "association"
	case "datasource":
		return "datasource"
	case "textTemplate":
		return "texttemplate"
	case "expression":
		return "expression"
	case "action":
		return "action"
	case "image":
		return "image"
	case "boolean", "integer", "decimal", "string", "enumeration":
		return "primitive"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Skill markdown generation
// ---------------------------------------------------------------------------

// RegenerateWidgetDocs scans projectDir/widgets/ for .mpk files and writes a
// per-widget .md skill file under .claude/skills/widgets/ (or
// .ai-context/skills/widgets/ when that directory exists). The docs combine
// human-readable info from the .mpk (descriptions, defaults) with the MDL
// keyword routing from the matching .def.json (object lists, child slots,
// MDL container keywords). Returns the number of files written.
func RegenerateWidgetDocs(projectPath string) (int, error) {
	projectDir := filepath.Dir(projectPath)
	widgetsDir := filepath.Join(projectDir, "widgets")
	defsDir := filepath.Join(projectDir, ".mxcli", "widgets")

	matches, err := filepath.Glob(filepath.Join(widgetsDir, "*.mpk"))
	if err != nil {
		return 0, fmt.Errorf("failed to scan widgets directory: %w", err)
	}
	if len(matches) == 0 {
		return 0, nil
	}

	// Make sure the .def.json files exist BEFORE rendering. They carry the MDL
	// keyword routing — child slots and object lists — and without them the
	// generated example collapses to a bare one-liner with no `{ … }` block at
	// all. `mxcli widget docs` on a fresh project used to emit exactly that, for
	// every widget, with no indication anything was missing: a data grid
	// documented as `PLUGGABLEWIDGET '…' widget1` and nothing about columns.
	// Only `refresh catalog` happened to generate defs first, so the output
	// depended on which command the user had run last.
	//
	// Best-effort: a project whose defs cannot be extracted still gets docs, just
	// the thinner ones — that is strictly better than no docs, and the caller is
	// told how many widgets ended up without routing.
	if _, defErr := RefreshWidgetDefinitions(projectPath, false, nil); defErr != nil {
		log.Printf("warning: widget definitions could not be refreshed, MDL examples will omit child slots: %v", defErr)
	}

	docsDirs := WidgetDocsDirs(projectDir)
	for _, d := range docsDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return 0, fmt.Errorf("failed to create docs directory: %w", err)
		}
	}

	// The embedded definitions cover the built-in widgets that are never
	// extracted to .def.json. Best-effort: a registry that cannot be built just
	// means those widgets fall back to the thinner rendering.
	registry, regErr := NewWidgetRegistry()
	if regErr != nil {
		log.Printf("warning: widget registry unavailable, built-in widgets will omit child slots: %v", regErr)
		registry = nil
	}

	var generated int
	var indexEntries []string
	var widgetNames []string
	var withoutRouting []string

	// A bundled .mpk (e.g. Charts.mpk) contains many widgetFiles; every one is
	// documented, not just the first (issue #679, bug 9a). The names are the
	// ones the definitions were written under.
	widgets, _ := scanProjectWidgets(matches)
	for _, w := range widgets {
		mpkDef, mdlName := w.def, w.mdlName
		filename := strings.ToLower(mdlName) + ".md"

		// Load the matching .def.json, falling back to the embedded definition.
		//
		// Built-in widgets like COMBOBOX and GALLERY have hand-crafted
		// definitions in sdk/widgets/definitions/ and are deliberately not
		// extracted per-project, so their .def.json never exists. Reading only
		// from disk therefore documented nine widgets — combobox, gallery, the
		// four data-grid filters, dropdownsort, image, barcodescanner — with an
		// MDL example missing every child block, as if extraction had failed.
		// The registry has had their routing all along.
		var def *WidgetDefinition
		defPath := filepath.Join(defsDir, strings.ToLower(mdlName)+".def.json")
		if data, err := os.ReadFile(defPath); err == nil {
			def = &WidgetDefinition{}
			if jsonErr := json.Unmarshal(data, def); jsonErr != nil {
				def = nil
			}
		}
		if def == nil && registry != nil {
			if embedded, ok := registry.Get(mdlName); ok {
				def = embedded
			}
		}

		doc := widgetDocMarkdown(mpkDef, def, mdlName)
		if writeErr := writeToAll(docsDirs, filename, doc); writeErr != nil {
			log.Printf("warning: failed to write %s: %v", filename, writeErr)
			continue
		}
		if def == nil {
			withoutRouting = append(withoutRouting, mdlName)
		}

		kind := "CUSTOMWIDGET"
		if mpkDef.IsPluggable {
			kind = "PLUGGABLEWIDGET"
		}
		indexEntries = append(indexEntries, fmt.Sprintf("| `%s` | [%s](%s) | `%s` | %s | %d |",
			kind, mdlName, filename, mpkDef.ID, mpkDef.Name, len(mpkDef.Properties)))
		widgetNames = append(widgetNames, mpkDef.Name)
		generated++
	}

	skill := widgetSkillMarkdown(indexEntries, widgetNames, withoutRouting)
	if err := writeToAll(docsDirs, "SKILL.md", skill); err != nil {
		return generated, fmt.Errorf("failed to write skill: %w", err)
	}
	// `_index.md` was the pre-#906 name, and it is not a skill: a leading
	// underscore hides it from a plain glob, and nothing discovered it. Retire it
	// so an upgraded project does not keep a second, staler index beside SKILL.md.
	for _, d := range docsDirs {
		_ = os.Remove(filepath.Join(d, "_index.md"))
	}

	return generated, nil
}

// widgetDocMarkdown produces the per-widget skill markdown. Combines mpkDef
// (for human descriptions, defaults, version) with def (for MDL keyword
// routing — object lists, child slots, property bindings). def may be nil for
// widgets without an extracted .def.json (e.g., hand-crafted built-ins).
func widgetDocMarkdown(mpkDef *mpk.WidgetDefinition, def *WidgetDefinition, mdlName string) string {
	var buf strings.Builder

	prefix := "CUSTOMWIDGET"
	if mpkDef.IsPluggable {
		prefix = "PLUGGABLEWIDGET"
	}

	buf.WriteString(fmt.Sprintf("# %s\n\n", mpkDef.Name))
	buf.WriteString(fmt.Sprintf("- **Widget ID:** `%s`\n", mpkDef.ID))
	buf.WriteString(fmt.Sprintf("- **Type:** %s\n", prefix))
	buf.WriteString(fmt.Sprintf("- **Version:** %s\n\n", mpkDef.Version))

	buf.WriteString("## MDL Example\n\n```sql\n")
	buf.WriteString(fmt.Sprintf("%s '%s' widget1", prefix, mpkDef.ID))
	if def != nil && (len(def.ChildSlots) > 0 || len(def.ObjectLists) > 0) {
		buf.WriteString(" {\n")
		// Names are required — `controlbar cb1 { … }`, never `controlbar { … }`
		// — and must be unique within the page, so they are numbered. Emitting
		// the nameless form made even the slots that DO parse unusable as
		// written (mendixlabs/mxcli#1036).
		for i, slot := range def.ChildSlots {
			buf.WriteString(fmt.Sprintf("  %s slot%d {\n    -- widgets for `%s`\n  }\n",
				strings.ToLower(slot.MDLContainer), i+1, slot.PropertyKey))
		}
		for i, ol := range def.ObjectLists {
			buf.WriteString(fmt.Sprintf("  %s item%d   -- one entry of `%s`\n",
				strings.ToLower(ol.MDLContainer), i+1, ol.PropertyKey))
		}
		buf.WriteString("}\n")
	} else {
		buf.WriteString("\n")
	}
	buf.WriteString("```\n\n")

	if len(mpkDef.Properties) > 0 {
		buf.WriteString("## Properties\n\n")
		buf.WriteString("| Property | Type | Required | Default | Values / notes | Group | Description |\n")
		buf.WriteString("|----------|------|----------|---------|----------------|-------|-------------|\n")
		for _, prop := range mpkDef.Properties {
			if prop.IsSystem {
				continue
			}
			writePropertyRow(&buf, prop, 0)
		}
		buf.WriteString("\n")
	}

	if def != nil && len(def.ChildSlots) > 0 {
		buf.WriteString("## Child Slots (curly-brace blocks)\n\n")
		buf.WriteString("| MDL keyword | Widget property |\n|-------------|----------------|\n")
		for _, s := range def.ChildSlots {
			buf.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", strings.ToLower(s.MDLContainer), s.PropertyKey))
		}
		buf.WriteString("\n")
	}

	if def != nil && len(def.ObjectLists) > 0 {
		buf.WriteString("## Object Lists (repeating child entries)\n\n")
		for _, ol := range def.ObjectLists {
			buf.WriteString(fmt.Sprintf("### `%s` → property `%s`\n\n", strings.ToLower(ol.MDLContainer), ol.PropertyKey))
			if len(ol.ItemProperties) > 0 {
				buf.WriteString("Item properties:\n\n")
				buf.WriteString("| Property | Operation |\n|----------|-----------|\n")
				for _, ip := range ol.ItemProperties {
					buf.WriteString(fmt.Sprintf("| `%s` | %s |\n", ip.PropertyKey, ip.Operation))
				}
				buf.WriteString("\n")
			}
			if len(ol.ItemSlots) > 0 {
				buf.WriteString("Item child slots:\n\n")
				buf.WriteString("| MDL keyword | Widget property |\n|-------------|----------------|\n")
				for _, s := range ol.ItemSlots {
					buf.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", strings.ToLower(s.MDLContainer), s.PropertyKey))
				}
				buf.WriteString("\n")
			}
		}
	}

	buf.WriteString(fmt.Sprintf("---\n\nRegenerated by `mxcli widget docs` and by `refresh catalog`. "+
		"For the same data live from the `.mpk` — including anything added by a widget upgrade since this "+
		"file was written — run `mxcli widget describe %s -p <app.mpr>`.\n", strings.ToLower(mdlName)))

	return buf.String()
}

// writePropertyRow renders one property, then its children indented beneath it.
//
// The three columns beyond name/type exist because their absence was the whole
// problem: an `enumeration` row showed its default but never its alternatives,
// and an `object` row showed nothing at all about the properties that go inside
// it — so `columns` in a data grid documented itself as "object, required" and
// left the reader unable to write a single column. Measured across a 42-widget
// project: 134 enumeration rows named 0 permitted values, 23 object rows exposed
// 0 children. `mxcli widget describe` had all of it; only this renderer dropped it.
func writePropertyRow(buf *strings.Builder, prop mpk.PropertyDef, depth int) {
	req := ""
	if prop.Required {
		req = "Yes"
	}

	// Enumerations name their options, and an object property announces the
	// children rendered beneath it. Their absence was the whole problem: an
	// `enumeration` row showed its default but never its alternatives, and an
	// `object` row said nothing about what goes inside — so `columns` in a data
	// grid documented itself as "object, required" and left the reader unable to
	// write a single column. Measured across a 42-widget project: 134 enumeration
	// rows named 0 permitted values, 23 object rows exposed 0 children.
	// `mxcli widget describe` reads the SAME PropertyDef and printed all of it;
	// only this renderer dropped it.
	var notes []string
	if len(prop.EnumValues) > 0 {
		keys := make([]string, 0, len(prop.EnumValues))
		for _, v := range prop.EnumValues {
			keys = append(keys, "`"+v+"`")
		}
		notes = append(notes, strings.Join(keys, " \\| "))
	}
	if prop.IsList {
		notes = append(notes, "list")
	}
	if prop.OnChange != "" {
		notes = append(notes, "on change → `"+prop.OnChange+"`")
	}
	if len(prop.Children) > 0 {
		notes = append(notes, fmt.Sprintf("%d sub-properties below", len(prop.Children)))
	}

	name := "`" + prop.Key + "`"
	if depth > 0 {
		name = strings.Repeat("&nbsp;", depth*4) + "↳ `" + prop.Key + "`"
	}

	desc := prop.Description
	if desc == "" {
		desc = prop.Caption
	}

	buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
		name, prop.Type, req, cellText(prop.DefaultValue),
		strings.Join(notes, "; "), cellText(prop.Category), cellText(desc)))

	for _, child := range prop.Children {
		if child.IsSystem {
			continue
		}
		writePropertyRow(buf, child, depth+1)
	}
}

// cellText makes a value safe for a markdown table cell.
//
// Descriptions used to be cut at 77 characters plus an ellipsis, which reliably
// removed the operative half of the sentence ("Must include '%d' to denote number
// posit…"). They are kept whole now; newlines are folded because a table cell
// cannot hold one, and pipes are escaped because an unescaped one silently splits
// the row into extra columns.
func cellText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

// WidgetDocsDirs returns every skills directory the widget docs belong in.
//
// It used to be either/or — `.ai-context/skills/widgets/` when `.ai-context`
// existed, `.claude/skills/widgets/` otherwise — which meant a Claude project
// with `.ai-context/` present got its bundled skills in `.claude/skills/` and its
// widget docs only in the other tree. `mxcli init` writes skills to both, so
// these follow the same rule.
func WidgetDocsDirs(projectDir string) []string {
	var dirs []string
	if _, err := os.Stat(filepath.Join(projectDir, ".ai-context")); err == nil {
		dirs = append(dirs, filepath.Join(projectDir, ".ai-context", "skills", "widgets"))
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".claude")); err == nil {
		dirs = append(dirs, filepath.Join(projectDir, ".claude", "skills", "widgets"))
	}
	if len(dirs) == 0 {
		// Neither tree exists yet: keep the historical default so a project that
		// has never seen `mxcli init` still gets something.
		dirs = append(dirs, filepath.Join(projectDir, ".claude", "skills", "widgets"))
	}
	return dirs
}

// writeToAll writes one generated file into every destination directory.
func writeToAll(dirs []string, name, content string) error {
	for _, d := range dirs {
		if err := os.WriteFile(filepath.Join(d, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

// widgetSkillMarkdown renders the SKILL.md that fronts the per-widget files.
//
// This is the navigation half of the Agent Skills progressive-disclosure shape:
// the description is always loaded, this body loads when the skill is invoked,
// and the per-widget files load only when the body sends a reader to one. The
// description names the project's actual widgets, which a hand-written skill
// cannot do — so "does this project have a chart widget?" is answerable from the
// skill listing alone, at no context cost.
func widgetSkillMarkdown(indexEntries, widgetNames, withoutRouting []string) string {
	var buf strings.Builder

	buf.WriteString("---\n")
	buf.WriteString("name: widgets\n")
	buf.WriteString("description: " + widgetSkillDescription(widgetNames) + "\n")
	buf.WriteString("---\n\n")

	buf.WriteString("# Widgets in this project\n\n")
	buf.WriteString("Generated from the `.mpk` files in `widgets/` by `mxcli widget docs` and by\n")
	buf.WriteString("`refresh catalog`. One file per widget holds its full property table, child\n")
	buf.WriteString("slots and object lists — **read the file for the widget you are placing**, not\n")
	buf.WriteString("this page.\n\n")

	buf.WriteString("| Prefix | Name | Widget ID | Display Name | Props |\n")
	buf.WriteString("|--------|------|-----------|--------------|-------|\n")
	for _, entry := range indexEntries {
		buf.WriteString(entry)
		buf.WriteString("\n")
	}

	buf.WriteString("\n## Usage in MDL\n\n```sql\n")
	buf.WriteString("-- React pluggable widgets\n")
	buf.WriteString("PLUGGABLEWIDGET 'com.mendix.widget.custom.badge.Badge' badge1\n\n")
	buf.WriteString("-- Legacy custom widgets\n")
	buf.WriteString("CUSTOMWIDGET 'com.company.OldWidget' legacy1\n")
	buf.WriteString("```\n\n")

	buf.WriteString("## When these files are not enough\n\n")
	buf.WriteString("They are a snapshot: a widget upgraded since the last `refresh catalog` is\n")
	buf.WriteString("described here as it was, not as it is. For the same data read live from the\n")
	buf.WriteString("`.mpk`, plus the dynamic visibility rules that are not rendered here at all:\n\n")
	buf.WriteString("```bash\n")
	buf.WriteString("mxcli widget describe <name> -p <app.mpr>    # e.g. datagrid, combobox\n")
	buf.WriteString("mxcli widget list -p <app.mpr>               # every widget, one line each\n")
	buf.WriteString("```\n\n")
	buf.WriteString("Prefer `describe` when a property does not behave as this file says it should.\n")

	if len(withoutRouting) > 0 {
		buf.WriteString("\n## Widgets without child-block routing\n\n")
		buf.WriteString("mxcli has no MDL child-slot mapping for these, so their example is the widget\n")
		buf.WriteString("line alone. For a leaf widget (a filter, an input) that is simply correct. If\n")
		buf.WriteString("one of them needs a `{ … }` block, the example will not tell you — check\n")
		buf.WriteString("`mxcli widget describe <name>` for properties of type `widgets` or `object`:\n\n")
		for _, n := range withoutRouting {
			buf.WriteString("- `" + strings.ToLower(n) + "`\n")
		}
	}

	return buf.String()
}

// widgetSkillDescription builds the frontmatter description, naming as many of
// the project's widgets as fit. The names are the point: they let a reader rule
// the skill in or out without opening it.
func widgetSkillDescription(names []string) string {
	const maxNames = 12
	shown := names
	suffix := ""
	if len(names) > maxNames {
		shown = names[:maxNames]
		suffix = fmt.Sprintf(" and %d more", len(names)-maxNames)
	}
	list := strings.Join(shown, ", ") + suffix
	return fmt.Sprintf("%q", "The pluggable and custom widgets installed in THIS project and how to write them in MDL — "+
		list+". Use before placing any PLUGGABLEWIDGET or CUSTOMWIDGET on a page, or when a widget's "+
		"property names, enumeration values or child blocks need checking.")
}
