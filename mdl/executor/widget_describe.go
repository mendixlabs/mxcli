// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	mmpk "github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// DescribeWidget assembles everything mxcli knows about one widget: its
// properties (key, type, caption, category, required, default, enum options)
// and the dynamic rules its editor uses to hide properties under some
// configurations.
//
// It exists in the executor rather than in cmd/ so that the MDL statement
// `DESCRIBE WIDGET x` and the CLI `mxcli widget describe x` are the same code.
// That is the point of the statement: a widget was the one MDL extension point
// with no in-language DESCRIBE, which is why `mxcli widget init` had to generate
// documentation at all — and why that documentation could drift from what the
// parser accepts (mendixlabs/mxcli#1036).
//
// arg is an MDL keyword (COMBOBOX), a widget id
// (com.mendix.widget.web.combobox.Combobox), or one of a few built-in aliases.
// projectPath may be empty, in which case only mxcli's embedded knowledge is
// available and the answer is correspondingly thinner.
func DescribeWidget(arg, projectPath string) (*WidgetDescription, error) {
	registry, err := NewWidgetRegistry()
	if err != nil {
		return nil, mdlerrors.NewBackend("widget registry init", err)
	}
	_ = LoadProjectWidgetDefinitions(registry, projectPath)

	widgetID, def := resolveWidgetTarget(registry, arg)
	if widgetID == "" {
		return nil, widgetNotFoundError(registry, arg, projectPath)
	}

	desc := WidgetDescription{WidgetID: widgetID}
	if def != nil {
		desc.MDLName = def.MDLName
		desc.Kind = def.WidgetKind
	}
	if desc.Kind == "" {
		desc.Kind = "pluggable"
	}

	// Properties + version: prefer the project's installed .mpk (version-accurate,
	// and the only place a Marketplace widget appears); else mxcli's embedded
	// template.
	if projectPath != "" {
		if dir := projectDirOf(projectPath); dir != "" {
			if mpkPath, ferr := mmpk.FindMPK(dir, widgetID); ferr == nil && mpkPath != "" {
				if wd, perr := mmpk.ParseMPKForWidget(mpkPath, widgetID); perr == nil && wd != nil {
					desc.Name = wd.Name
					desc.Version = wd.Version
					desc.Source = "project .mpk"
					desc.Properties = propsFromMPK(wd)
					desc.Rules, desc.RuleCoverage = rulesFromProject(mpkPath, widgetID)
				}
			}
		}
	}
	desc.Containers = describeContainers(def)

	if desc.Source == "" {
		tmpl, terr := mwidgets.GetTemplate(widgetID)
		if terr != nil || tmpl == nil {
			return nil, mdlerrors.NewNotFoundMsg("widget", arg,
				"no installed .mpk and no embedded template for "+arg+
					" — open the project with -p to inspect a widget it has installed")
		}
		desc.Name = tmpl.Name
		desc.Version = tmpl.Version
		desc.Source = "embedded template"
		desc.Properties = propsFromTemplate(tmpl.Type)
		if def != nil {
			desc.Rules = rulesFromDef(def.PropertyVisibility)
		}
	}
	desc.defDefaults = definitionDefaults(def)
	desc.Example, desc.OmittedFromExample = buildUsageExample(desc)
	return &desc, nil
}

type DescribedProperty struct {
	Key      string              `json:"key"`
	Type     string              `json:"type"`
	Caption  string              `json:"caption,omitempty"`
	Category string              `json:"category,omitempty"`
	Required bool                `json:"required"`
	Default  string              `json:"default,omitempty"`
	System   bool                `json:"system,omitempty"`
	Enum     []string            `json:"enum,omitempty"`
	Children []DescribedProperty `json:"children,omitempty"`
}

// DescribedRule is one dynamic (visibility) rule of a widget's discovered format.
type DescribedRule struct {
	Property   string `json:"property"`
	HiddenWhen string `json:"hiddenWhen"`
	// Cond is the same condition in machine form. Kept alongside the English
	// so the usage example can EVALUATE it: a widget's required properties are
	// required only where visible, and Combo box lists eleven bindings of which
	// its mutually exclusive options-source modes leave about two.
	Cond *types.WidgetVisibilityCondition `json:"-"`
	// Rule is the whole rule, conjunction included. Cond above is only its FIRST
	// term, kept for readers that predate conjunctions; evaluate Rule.
	Rule types.WidgetVisibilityRule `json:"-"`
	// Nested marks a rule about an object-list ITEM's property rather than the
	// widget's own. Those are evaluated against the item, never the widget.
	Nested bool `json:"-"`
}

// WidgetDescription is the full inspection result (also the JSON shape).
type WidgetDescription struct {
	WidgetID     string              `json:"widgetId"`
	MDLName      string              `json:"mdlName,omitempty"`
	Name         string              `json:"name,omitempty"`
	Version      string              `json:"version,omitempty"`
	Source       string              `json:"source"` // "project .mpk" | "embedded template"
	Kind         string              `json:"kind,omitempty"`
	Properties   []DescribedProperty `json:"properties"`
	Rules        []DescribedRule     `json:"dynamicRules"`
	RuleCoverage string              `json:"ruleCoverage,omitempty"`
	// Containers a widget's body can hold: child slots (a curly-brace block of
	// widgets) and object lists (repeating entries). Reported with whether MDL
	// can currently express each one, which is NOT a given — see
	// DescribedContainer.Authorable.
	Containers []DescribedContainer `json:"containers,omitempty"`
	// Example is re-executable MDL placing this widget, and OmittedFromExample
	// says what it leaves out. Both are derived by parsing, so the example is
	// guaranteed to parse and widens on its own as the grammar does.
	Example            string   `json:"example,omitempty"`
	OmittedFromExample []string `json:"omittedFromExample,omitempty"`

	// defDefaults is the value mxcli's own WidgetDefinition gives each property
	// when a script does not set one — the mapping's `default`, else a primitive
	// mapping's `value`. Unexported, so it never reaches the JSON output; it
	// exists only so the example's hide-rule narrowing resolves a property to
	// the SAME value MDL-WIDGET10 will.
	//
	// The .mpk alone is not enough: a selection property declares no
	// defaultValue there, so gallery's `itemSelection` looked indeterminable
	// and the example emitted the `keepSelection` that "Single" hides.
	defDefaults map[string]string
}

// DescribedContainer is one child slot or object list of a widget.
type DescribedContainer struct {
	Keyword     string `json:"keyword"`
	PropertyKey string `json:"propertyKey"`
	Kind        string `json:"kind"` // "child slot" | "object list"
	// Mode names the definition mode a child slot belongs to, for a slot that
	// is a container only in that mode — a combo box's custom content is
	// association mode's or database mode's (ako/mxcli#1744). Empty for a slot
	// of every configuration.
	Mode     string   `json:"mode,omitempty"`
	ItemKeys []string `json:"itemKeys,omitempty"`
	// Authorable reports whether `<keyword> name (…)` actually parses inside a
	// widget body today. It is derived by parsing a probe, never from a list:
	// the bug this description exists to help with (mendixlabs/mxcli#1036) was
	// two lists of keywords with nothing comparing them, and a third list here
	// would be the same mistake one layer up.
	Authorable bool `json:"authorable"`

	// ItemSlots lists the WIDGETS-typed slots inside one item of an object list
	// — a DataGrid column's `content` and `filter`, an Accordion group's
	// `headerContent`.
	//
	// These were missing, and their absence had teeth. `items` above lists only
	// the item's scalar sub-properties, so `describe widget datagrid` named
	// `controlbar` as the one filter-shaped thing a grid declares and said
	// nothing about a column taking a filter at all. An author asking this
	// command where the column filter goes was therefore steered to the
	// grid-wide filter bar — which renders "Unable to get filter store" at
	// runtime (ako/view-entity-examples FINDINGS §2). A description that omits
	// the right answer and offers a wrong-looking one is worse than silence.
	ItemSlots []DescribedItemSlot `json:"itemSlots,omitempty"`

	// items carries each sub-property's writable value as the WIDGET DEFINITION
	// records it — the mapping's `default`/`value` and its enumValues. Unexported,
	// so the JSON shape is unchanged.
	//
	// The .mpk is not always enough: ParseMPKForWidget returns 0 children for a
	// PopupMenu's `basicItems`, while the definition carries
	// {"propertyKey":"itemType","value":"item","enumValues":["item","divider"]}.
	// MDL-WIDGET08 reads the definition, so the example has to as well or the two
	// disagree about the same sub-property.
	items []DescribedProperty
}

// DescribedItemSlot is one widgets-typed slot inside an object-list item, with
// the two ways a widget reaches it: an explicit `<keyword> { … }` block, or —
// for the types in Accepts — being written directly in the item's body.
type DescribedItemSlot struct {
	Keyword     string   `json:"keyword"`
	PropertyKey string   `json:"propertyKey"`
	Accepts     []string `json:"accepts,omitempty"`
	// Default marks the slot that takes any child matching neither route
	// (defaultItemSlotKey). Exactly one slot per object list has it.
	Default bool `json:"default,omitempty"`
}

func resolveWidgetTarget(registry *WidgetRegistry, arg string) (string, *WidgetDefinition) {
	if strings.Contains(arg, ".") {
		if def, ok := registry.GetByWidgetID(arg); ok {
			return arg, def
		}
		return arg, nil // unknown to the registry, but a valid id to look up in the project
	}
	upper := strings.ToUpper(arg)
	if def, ok := registry.Get(upper); ok {
		return def.WidgetID, def
	}
	// Well-known widgets that are special-cased in the executor (no .def.json in the
	// registry) but that users still name by keyword.
	if id, ok := builtinWidgetAliases[upper]; ok {
		def, _ := registry.GetByWidgetID(id)
		return id, def
	}
	return "", nil
}

// builtinWidgetAliases maps MDL keywords for executor-special-cased widgets (which
// have no .def.json registry entry) to their widget ids, so `widget describe` can
// resolve them by the same friendly names users write in MDL.
var builtinWidgetAliases = map[string]string{
	"DATAGRID":  "com.mendix.widget.web.datagrid.Datagrid",
	"DATAGRID2": "com.mendix.widget.web.datagrid.Datagrid",
}

// widgetNotFoundError builds a helpful error listing the known MDL names.
//
// Every remedy it names must work (ako/mxcli#663). It used to suggest "a full
// widget id (com.mendix.widget…)", which is a parse error in DESCRIBE WIDGET —
// the id has to be quoted there — and it listed only the embedded keywords of a
// project whose installed widgets had simply not been loaded.
func widgetNotFoundError(registry *WidgetRegistry, arg, projectPath string) error {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, d := range registry.All() {
		add(d.MDLName)
	}
	for alias := range builtinWidgetAliases {
		add(strings.ToLower(alias))
	}
	sort.Strings(names)
	scope := "installed in this project's widgets/"
	if projectPath == "" {
		scope = "built into mxcli (no project is open, so a project's installed widgets are not known; pass -p <app.mpr>)"
	}
	return fmt.Errorf("unknown widget %q: no widget by that name is %s. Known MDL keywords: %s. "+
		"A widget can also be named by its quoted id ('com.mendix.widget.web.combobox.Combobox'); `mxcli widget list` shows the ids",
		arg, scope, strings.Join(names, ", "))
}

// projectDirOf returns the directory containing widgets/ for a project path
// (accepts either the .mpr file or its directory).
func projectDirOf(projectPath string) string {
	if strings.EqualFold(filepath.Ext(projectPath), ".mpr") {
		return filepath.Dir(projectPath)
	}
	return projectPath
}

// propsFromMPK builds described properties from a parsed .mpk definition, in the
// widget's declared order (regular + system interleaved).
func propsFromMPK(wd *mmpk.WidgetDefinition) []DescribedProperty {
	order := wd.AllTopLevel
	if len(order) == 0 {
		order = wd.Properties
	}
	out := make([]DescribedProperty, 0, len(order))
	for _, p := range order {
		out = append(out, describedPropFromMPK(p))
	}
	return out
}

func describedPropFromMPK(p mmpk.PropertyDef) DescribedProperty {
	dp := DescribedProperty{
		Key:      p.Key,
		Type:     p.Type,
		Caption:  p.Caption,
		Category: p.Category,
		Required: p.Required,
		Default:  p.DefaultValue,
		System:   p.IsSystem,
	}
	if dp.System && dp.Type == "" {
		dp.Type = "system"
	}
	for _, ev := range p.EnumValues {
		dp.Enum = append(dp.Enum, ev.Key)
	}
	for _, c := range p.Children {
		dp.Children = append(dp.Children, describedPropFromMPK(c))
	}
	return dp
}

// propsFromTemplate walks an embedded template's Type map (ObjectType.PropertyTypes)
// to build described properties. Used when no project .mpk is available.
func propsFromTemplate(typ map[string]any) []DescribedProperty {
	objType, _ := typ["ObjectType"].(map[string]any)
	pts, _ := objType["PropertyTypes"].([]any)
	var out []DescribedProperty
	for _, pt := range pts {
		m, ok := pt.(map[string]any)
		if !ok {
			continue // leading array marker
		}
		out = append(out, describedPropFromTemplate(m))
	}
	return out
}

func describedPropFromTemplate(m map[string]any) DescribedProperty {
	dp := DescribedProperty{
		Key:      asString(m["PropertyKey"]),
		Caption:  asString(m["Caption"]),
		Category: asString(m["Category"]),
	}
	vt, _ := m["ValueType"].(map[string]any)
	if vt != nil {
		dp.Type = asString(vt["Type"])
		dp.Default = asString(vt["DefaultValue"])
		if r, ok := vt["Required"].(bool); ok {
			dp.Required = r
		}
		if evs, ok := vt["EnumerationValues"].([]any); ok {
			for _, ev := range evs {
				if em, ok := ev.(map[string]any); ok {
					if k := asString(em["_Key"]); k != "" {
						dp.Enum = append(dp.Enum, k)
					}
				}
			}
		}
		if nested, ok := vt["ObjectType"].(map[string]any); ok {
			if npts, ok := nested["PropertyTypes"].([]any); ok {
				for _, npt := range npts {
					if nm, ok := npt.(map[string]any); ok {
						dp.Children = append(dp.Children, describedPropFromTemplate(nm))
					}
				}
			}
		}
	}
	dp.System = isSystemPropKey(dp.Key)
	return dp
}

func isSystemPropKey(key string) bool {
	switch key {
	case "Label", "Visibility", "Editability", "Name", "TabIndex":
		return true
	}
	return false
}

// rulesFromProject extracts dynamic rules from the project's installed .mpk editor
// config, returning the rules and a coverage note (recognized / total hide-calls).
func rulesFromProject(mpkPath, widgetID string) ([]DescribedRule, string) {
	rules, recognized, total := ExtractWidgetVisibilityStats(mpkPath, widgetID)
	present := total > 0
	if !present {
		js, err := mmpk.ReadEditorConfig(mpkPath, widgetID)
		present = err == nil && strings.TrimSpace(js) != ""
	}
	return rulesToDescribed(rules), ruleCoverageText(present, recognized, total)
}

// ruleCoverageText says how far the listed rules can be trusted. A widget that
// ships an editorConfig.js whose hide calls are none of the toolkit helpers
// mxcli reads — a local `hideProperties(…)` that filters by key — listed
// "(none discovered)" and nothing else, which reads as "the widget hides
// nothing" (ako/mxcli#2507). It says the rules exist and were not read.
func ruleCoverageText(editorConfigPresent bool, recognized, total int) string {
	switch {
	case total > 0:
		return fmt.Sprintf("%d of %d editor hide-rules recognized", recognized, total)
	case editorConfigPresent:
		return "the widget ships an editorConfig.js, but none of its hide calls is one mxcli reads " +
			"(hidePropertyIn, hidePropertiesIn, hideNestedPropertiesIn): any property it hides is unknown here, " +
			"and a hidden property set to other than its default is CE0463"
	}
	return ""
}

func rulesFromDef(rules []types.WidgetVisibilityRule) []DescribedRule {
	return rulesToDescribed(rules)
}

func rulesToDescribed(rules []types.WidgetVisibilityRule) []DescribedRule {
	out := make([]DescribedRule, 0, len(rules))
	for _, r := range rules {
		if r.HiddenWhen == nil {
			continue
		}
		out = append(out, DescribedRule{
			Property:   r.PropertyKey,
			HiddenWhen: ruleText(r),
			Cond:       r.HiddenWhen,
			Rule:       r,
			Nested:     r.Nested(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Property < out[j].Property })
	return out
}

// ruleText renders a rule's whole condition as readable English, joining a
// conjunction with "and" so the reader sees every term rather than the
// innermost one, which alone reads as a far broader claim than the rule makes.
func ruleText(r types.WidgetVisibilityRule) string {
	conds := r.Conditions()
	parts := make([]string, 0, len(conds))
	for i := range conds {
		parts = append(parts, conditionText(&conds[i]))
	}
	return strings.Join(parts, " and ")
}

// rule returns the described rule in structured form, falling back to the
// single Cond for a DescribedRule built without one (the JSON shape and older
// callers carry only the condition).
func (r DescribedRule) rule() types.WidgetVisibilityRule {
	if r.Rule.HiddenWhen != nil {
		return r.Rule
	}
	return types.WidgetVisibilityRule{PropertyKey: r.Property, HiddenWhen: r.Cond}
}

// conditionText renders a visibility condition as readable English.
func conditionText(c *types.WidgetVisibilityCondition) string {
	switch c.Operator {
	case "eq":
		return fmt.Sprintf("%s = %q", c.PropertyKey, c.Value)
	case "ne":
		return fmt.Sprintf("%s ≠ %q", c.PropertyKey, c.Value)
	case "truthy":
		return fmt.Sprintf("%s is set", c.PropertyKey)
	case "falsy":
		return fmt.Sprintf("%s is not set", c.PropertyKey)
	case "empty":
		return fmt.Sprintf("%s is empty", c.PropertyKey)
	case "notempty":
		return fmt.Sprintf("%s is not empty", c.PropertyKey)
	case "in":
		return fmt.Sprintf("%s is one of %s", c.PropertyKey, strings.ReplaceAll(c.Value, ",", ", "))
	case "notin":
		return fmt.Sprintf("%s is not one of %s", c.PropertyKey, strings.ReplaceAll(c.Value, ",", ", "))
	default:
		return fmt.Sprintf("%s %s %q", c.PropertyKey, c.Operator, c.Value)
	}
}

func PrintWidgetDescription(out io.Writer, d WidgetDescription) {
	title := d.Name
	if title == "" {
		title = d.WidgetID
	}
	fmt.Fprintf(out, "Widget: %s", title)
	if d.MDLName != "" {
		fmt.Fprintf(out, " (%s)", d.MDLName)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  ID:      %s\n", d.WidgetID)
	if d.Version != "" {
		fmt.Fprintf(out, "  Version: %s\n", d.Version)
	}
	fmt.Fprintf(out, "  Kind:    %s\n", d.Kind)
	fmt.Fprintf(out, "  Source:  %s\n", d.Source)

	fmt.Fprintf(out, "\nProperties (%d):\n", countProps(d.Properties))
	printProps(out, d.Properties, 0)

	fmt.Fprintf(out, "\nDynamic property rules (%d):\n", len(d.Rules))
	if len(d.Rules) == 0 {
		fmt.Fprintln(out, "  (none discovered)")
	}
	for _, r := range d.Rules {
		fmt.Fprintf(out, "  %-40s hidden when %s\n", r.Property, r.HiddenWhen)
	}
	if d.RuleCoverage != "" {
		fmt.Fprintf(out, "  — %s\n", d.RuleCoverage)
	}

	if d.Example != "" {
		fmt.Fprintf(out, "\nMDL example (parses as written):\n")
		for _, line := range strings.Split(d.Example, "\n") {
			fmt.Fprintf(out, "  %s\n", line)
		}
		if len(d.OmittedFromExample) > 0 {
			fmt.Fprintf(out, "  -- omitted: %s\n", strings.Join(d.OmittedFromExample, "; "))
		}
	}

	if len(d.Containers) > 0 {
		fmt.Fprintf(out, "\nBody containers (%d):\n", len(d.Containers))
		for _, c := range d.Containers {
			mark := "  authorable"
			if !c.Authorable {
				mark = "  NOT authorable from MDL yet"
			}
			if c.Mode != "" {
				mark += " (" + c.Mode + " only)"
			}
			fmt.Fprintf(out, "  %-34s %-12s -> %s%s\n", c.Keyword, c.Kind, c.PropertyKey, mark)
			if len(c.ItemKeys) > 0 {
				fmt.Fprintf(out, "  %-34s   items: %s\n", "", strings.Join(c.ItemKeys, ", "))
			}
			for _, is := range c.ItemSlots {
				// Say how a widget REACHES the slot, not just that it exists —
				// the accepted types are the answer to "where does the column
				// filter go?", and they are the reason a filter belongs in the
				// column's own braces rather than in the grid's filter bar.
				how := "any other widget in the item body"
				if len(is.Accepts) > 0 {
					how = strings.Join(is.Accepts, " | ")
				} else if !is.Default {
					how = is.Keyword + " { … }"
				}
				fmt.Fprintf(out, "  %-34s   slot %s -> %s: %s\n", "", is.Keyword, is.PropertyKey, how)
			}
		}
	}
}

func countProps(props []DescribedProperty) int {
	n := 0
	for _, p := range props {
		n++
		n += countProps(p.Children)
	}
	return n
}

func printProps(out interface{ Write([]byte) (int, error) }, props []DescribedProperty, depth int) {
	indent := strings.Repeat("  ", depth+1)
	for _, p := range props {
		req := ""
		if p.Required {
			req = " required"
		}
		sys := ""
		if p.System {
			sys = " [system]"
		}
		line := fmt.Sprintf("%s%-34s %-13s", indent, p.Key, p.Type)
		extra := strings.TrimRight(req+sys, " ")
		if p.Default != "" {
			extra = strings.TrimSpace(extra + " default=" + p.Default)
		}
		if len(p.Enum) > 0 {
			extra = strings.TrimSpace(extra + " {" + strings.Join(p.Enum, "|") + "}")
		}
		if p.Category != "" {
			extra = strings.TrimSpace(extra + "  (" + p.Category + ")")
		}
		fmt.Fprintf(out, "%s %s\n", strings.TrimRight(line, " "), extra)
		if len(p.Children) > 0 {
			printProps(out, p.Children, depth+1)
		}
	}
}

// describeWidgetStmt is the DESCRIBE WIDGET handler. It prints exactly what
// `mxcli widget describe` prints, because it is the same function — see
// DescribeWidget for why that matters.
//
// The widget is named by MDL keyword or widget id; unlike every other DESCRIBE
// there is no qualified name, because a widget definition is not a document in
// the model. It comes from a package in the project (or from mxcli's embedded
// set), which is also why this reads no backend and works with no project open.
func describeWidgetStmt(ctx *ExecContext, name string) error {
	if name == "" {
		return mdlerrors.NewValidation("DESCRIBE WIDGET needs a widget: an MDL keyword (combobox) or a widget id ('com.mendix.widget.web.combobox.Combobox')")
	}
	projectPath := ""
	if ctx != nil && ctx.Backend != nil {
		projectPath = ctx.Backend.Path()
	}
	desc, err := DescribeWidget(name, projectPath)
	if err != nil {
		return err
	}
	PrintWidgetDescription(ctx.Output, *desc)
	return nil
}

// describeContainers lists a widget's child slots and object lists, each marked
// with whether MDL can currently express it.
func describeContainers(def *WidgetDefinition) []DescribedContainer {
	if def == nil {
		return nil
	}
	var out []DescribedContainer
	for _, cs := range def.ChildSlots {
		kw := strings.ToLower(cs.MDLContainer)
		out = append(out, DescribedContainer{
			Keyword: kw, PropertyKey: cs.PropertyKey, Kind: "child slot",
			Authorable: containerKeywordParses(kw, true),
		})
	}
	for i := range def.Modes {
		for _, cs := range def.Modes[i].ChildSlots {
			kw := strings.ToLower(cs.MDLContainer)
			out = append(out, DescribedContainer{
				Keyword: kw, PropertyKey: cs.PropertyKey, Kind: "child slot",
				Mode:       modeLabel(&def.Modes[i]),
				Authorable: containerKeywordParses(kw, true),
			})
		}
	}
	for _, ol := range def.ObjectLists {
		kw := strings.ToLower(ol.MDLContainer)
		c := DescribedContainer{
			Keyword: kw, PropertyKey: ol.PropertyKey, Kind: "object list",
			Authorable: containerKeywordParses(kw, false),
		}
		for _, ip := range ol.ItemProperties {
			c.ItemKeys = append(c.ItemKeys, ip.PropertyKey)
			def := ip.Default
			if def == "" && ip.Operation == "primitive" {
				def = ip.Value
			}
			c.items = append(c.items, DescribedProperty{
				Key: ip.PropertyKey, Type: ip.Operation, Default: def, Enum: ip.EnumValues,
			})
		}
		// Ask the engine which slot is the default rather than restating the
		// "content, else the first one" rule — a second copy of that rule is
		// how the container lists drifted apart in #1036.
		defaultKey := defaultItemSlotKey(&ol)
		for _, is := range ol.ItemSlots {
			c.ItemSlots = append(c.ItemSlots, DescribedItemSlot{
				Keyword:     strings.ToLower(is.MDLContainer),
				PropertyKey: is.PropertyKey,
				Accepts:     append([]string(nil), is.AcceptedChildTypes...),
				Default:     is.PropertyKey == defaultKey,
			})
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Keyword < out[j].Keyword })
	return out
}

// containerKeywordParses answers "can I write this inside a widget body?" by
// parsing a minimal page and checking for errors — deriving the answer from the
// grammar itself rather than restating it.
func containerKeywordParses(keyword string, slot bool) bool {
	if keyword == "" {
		return false
	}
	body := keyword + " probe1 (x: 'y')"
	if slot {
		body = keyword + " probe1 { dynamictext t (Content: 'x') }"
	}
	src := "create page Probe.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" +
		"  pluggablewidget 'probe.Widget' pw {\n    " + body + "\n  }\n}\n"
	_, errs := visitor.Build(src)
	return len(errs) == 0
}

// buildUsageExample renders MDL that places this widget, and returns it with a
// list of what it left out.
//
// Two rules make it worth printing at all, both learned from the generated .md
// this replaces (mendixlabs/mxcli#1036):
//
//  1. It emits only what PARSES. The head form, and every container, is chosen
//     by probing the real parser — so the example corrects itself as the grammar
//     gains ground, and cannot drift the way a hand-written template did.
//  2. It says what it omitted and why. The .md's example silently included
//     containers that could not be written, which is what made it misleading
//     rather than merely incomplete.
//
// The result is verified by parsing it before returning; if it somehow does not
// parse, the caller is told rather than handed a broken snippet.
func buildUsageExample(d WidgetDescription) (example string, omitted []string) {
	name := "widget1"

	// Head: the widget's own keyword when the grammar takes it, else the
	// explicit-id form. Probed, never assumed.
	head := "pluggablewidget '" + d.WidgetID + "' " + name
	if kw := strings.ToLower(d.MDLName); kw != "" && widgetKeywordParses(kw) {
		head = kw + " " + name
	}

	// Scalars: only those whose value can be written as a literal. A datasource,
	// attribute, action or expression needs a real name from the project, and
	// inventing one would produce an example that parses but cannot run.
	var props []string
	var needBinding []string
	for _, p := range d.Properties {
		if p.System {
			continue
		}
		// The two sources spell property types differently — a project .mpk
		// gives "datasource", the embedded template "DataSource" — so this
		// folds case. Matching only one spelling silently emptied the example
		// for every widget described without a project.
		// hiddenUnder applies to BOTH branches. It used to gate only the
		// binding branch, so the example emitted literals its own configuration
		// hides — `heightUnit: 'aspectRatio'` followed by the `height` that
		// choice hides — and mxcli's own MDL-WIDGET10 then warned about 32 of
		// them. The generator and the checker implement the same editorConfig
		// rules; disagreeing is worse than either alone, because the example is
		// what a reader copies.
		if hiddenUnder(d, p.Key) {
			continue
		}
		switch strings.ToLower(p.Type) {
		case "boolean", "integer", "enumeration", "string", "texttemplate":
			if p.Required {
				props = append(props, "  "+p.Key+": "+exampleLiteral(p))
			}
		case "attribute", "datasource", "action", "expression", "selection":
			if p.Required {
				needBinding = append(needBinding, p.Key+" ("+strings.ToLower(p.Type)+")")
			}
		}
	}

	// Names are numbered across the whole body: two widgets sharing a name on
	// one page is invalid, and the parser does not catch it — the same defect
	// the generated .md had.
	var body []string
	n := 0
	for _, c := range d.Containers {
		if !c.Authorable {
			omitted = append(omitted, c.Keyword)
			continue
		}
		// A mode's slot is a container in that mode only, and the example is
		// written in the unconditioned one, where check refuses the block
		// (MDL-WIDGET30) — a combo box's custom content (ako/mxcli#1744).
		if c.Mode != "" {
			omitted = append(omitted, c.Keyword+" — "+c.Mode+" only")
			continue
		}
		n++
		if c.Kind == "child slot" {
			body = append(body, fmt.Sprintf("  %s slot%d {\n    -- widgets for `%s`\n  }", c.Keyword, n, c.PropertyKey))
			continue
		}
		item := fmt.Sprintf("  %s item%d", c.Keyword, n)
		if k, lit := itemExampleLiteral(d, c); k != "" {
			item += " (" + k + ": " + lit + ")"
		}
		// Show the item's own body when a slot routes a widget type INTO it.
		// That routing is the part nobody guesses — a DataGrid column's filter
		// goes in the column's braces, and an example that stops at the closing
		// paren leaves the reader thinking a column has no body at all. Which is
		// how the gallery `filter { … }` form got written on a grid instead.
		var inner []string
		for _, is := range c.ItemSlots {
			if len(is.Accepts) == 0 {
				continue
			}
			n++
			inner = append(inner, fmt.Sprintf("    %s item%d   -- routed to this entry's `%s`",
				is.Accepts[0], n, is.PropertyKey))
		}
		if len(inner) > 0 {
			body = append(body, item+" {   -- one entry of `"+c.PropertyKey+"`\n"+
				strings.Join(inner, "\n")+"\n  }")
			continue
		}
		body = append(body, item+"   -- one entry of `"+c.PropertyKey+"`")
	}

	var sb strings.Builder
	sb.WriteString(head)
	if len(props) > 0 {
		sb.WriteString(" (\n" + strings.Join(props, ",\n") + "\n)")
	}
	if len(body) > 0 {
		sb.WriteString(" {\n" + strings.Join(body, "\n") + "\n}")
	}
	out := sb.String()

	if !pageBodyParses(out) {
		return "", append(omitted, "(example could not be generated for this widget)")
	}
	for _, n := range needBinding {
		omitted = append(omitted, n+" — needs a name from your project")
	}
	return out, omitted
}

// itemExampleLiteral picks the sub-property to show on an object-list item, and
// a value for it that the validator will accept.
//
// It used to take ItemKeys[0] and write `'…'`. For an ENUMERATION sub-property
// that is simply a wrong value, and mxcli's own MDL-WIDGET08 said so — "property
// `dataSet` has invalid value `…` — valid values are static, dynamic" — on 11 of
// the fixture's examples. The block claims to parse as written, and it did; it
// just did not CHECK as written, which is the more useful promise.
//
// The values were already in hand: propsFromMPK carries an object-list
// property's sub-properties as Children, with their enums and defaults, so the
// same exampleLiteral used for the widget's own scalars applies here.
//
// Preference order is deliberate: a sub-property with a derivable literal (a
// default, or an enumeration's first member) beats one without, because for a
// free-text sub-property there is no correct value to invent and a placeholder
// is the honest output — and the validator accepts any string, so it costs
// nothing.
func itemExampleLiteral(d WidgetDescription, c DescribedContainer) (key, literal string) {
	if len(c.ItemKeys) == 0 {
		return "", ""
	}
	children := map[string]DescribedProperty{}
	for _, p := range d.Properties {
		if !strings.EqualFold(p.Key, c.PropertyKey) {
			continue
		}
		for _, ch := range p.Children {
			children[strings.ToLower(ch.Key)] = ch
		}
	}
	// The definition WINS, for the same reason it does in exampleValues: it is
	// what MDL-WIDGET08 checks the value against.
	for _, it := range c.items {
		if it.Default == "" && len(it.Enum) == 0 {
			continue
		}
		children[strings.ToLower(it.Key)] = it
	}
	for _, k := range c.ItemKeys {
		ch, ok := children[strings.ToLower(k)]
		if !ok {
			continue
		}
		if ch.Default == "" && len(ch.Enum) == 0 {
			continue // nothing to derive; keep looking for one that has something
		}
		return k, exampleLiteral(ch)
	}
	// No sub-property offers a value. Show the first key with a placeholder —
	// it is a free-text slot, which the validator accepts.
	return c.ItemKeys[0], "'…'"
}

// exampleLiteral picks a writable value for a scalar property: its default when
// it has one, else the first enumeration value, else a placeholder.
func exampleLiteral(p DescribedProperty) string {
	switch strings.ToLower(p.Type) {
	case "boolean":
		if p.Default != "" {
			return p.Default
		}
		return "false"
	case "integer":
		if p.Default != "" {
			return p.Default
		}
		return "0"
	}
	if p.Default != "" {
		return "'" + p.Default + "'"
	}
	if len(p.Enum) > 0 {
		return "'" + p.Enum[0] + "'"
	}
	return "'…'"
}

// widgetKeywordParses reports whether `<keyword> name (…)` is accepted as a
// widget in a page body — the head-form half of the same probe the containers use.
func widgetKeywordParses(keyword string) bool {
	return pageBodyParses(keyword + " probe1 (someProp: 'x')")
}

// pageBodyParses puts a fragment in a minimal page and reports whether it parses.
func pageBodyParses(body string) bool {
	src := "create page Probe.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + body + "\n}\n"
	_, errs := visitor.Build(src)
	return len(errs) == 0
}

// isEmptinessOperator reports whether an operator's whole question is "did the
// author pick anything", making an unrecorded value determinable rather than
// unknown.
func isEmptinessOperator(op string) bool {
	return op == "empty" || op == "notempty"
}

// declaresProperty reports whether the widget declares a property with this key,
// at any nesting depth. A condition naming something the widget does not declare
// is not a property that is merely unset — it is a rule we misread, and guessing
// "" for it would invent a verdict.
func declaresProperty(d WidgetDescription, key string) bool {
	var walk func(props []DescribedProperty) bool
	walk = func(props []DescribedProperty) bool {
		for _, p := range props {
			if strings.EqualFold(p.Key, key) {
				return true
			}
			if walk(p.Children) {
				return true
			}
		}
		return false
	}
	return walk(d.Properties)
}

// exampleValues is the configuration the example describes: each scalar
// property's default, which is also what the example writes for the required
// ones. Visibility rules are evaluated against this.
func exampleValues(d WidgetDescription) map[string]string {
	values := map[string]string{}
	var walk func(props []DescribedProperty)
	walk = func(props []DescribedProperty) {
		for _, p := range props {
			if p.Default != "" {
				values[p.Key] = p.Default
			} else if len(p.Enum) > 0 {
				// An enumeration with no declared default takes its first value,
				// which is what Mendix shows in the editor.
				values[p.Key] = p.Enum[0]
			}
			walk(p.Children)
		}
	}
	walk(d.Properties)
	// The definition's default WINS over the package's. It is what mxcli writes
	// when a script is silent, and therefore what the validator concludes the
	// property holds — which is the whole point of resolving it here.
	for k, v := range d.defDefaults {
		if v != "" {
			values[k] = v
		}
	}
	return values
}

// definitionDefaults collects the value each mapping falls back to, mirroring
// widgetValueMap's own order: an explicit `default`, else a primitive mapping's
// `value` (the widget XML's defaultValue, which is where the generator and the
// checker have to agree).
func definitionDefaults(def *WidgetDefinition) map[string]string {
	if def == nil {
		return nil
	}
	out := map[string]string{}
	collect := func(mappings []PropertyMapping) {
		for _, m := range mappings {
			if m.PropertyKey == "" {
				continue
			}
			switch {
			case m.Default != "":
				out[m.PropertyKey] = m.Default
			case m.Operation == "primitive" && m.Value != "":
				out[m.PropertyKey] = m.Value
			case m.Operation == "selection":
				// An omitted `Selection:` is WRITTEN as None — the builder's own
				// behaviour, not a guess — and a selection property declares no
				// defaultValue in the .mpk, which is why the generator saw
				// DataGrid2's `itemSelection` as indeterminable and emitted the
				// `itemSelectionMethod` that "None" hides. Same reasoning, and
				// same three branches, as widgetValueMap.
				out[m.PropertyKey] = "None"
			}
			if m.Operation == "selection" {
				out[m.PropertyKey] = canonicalSelection(out[m.PropertyKey])
			}
		}
	}
	collect(def.PropertyMappings)
	// The example is a bare widget, so it builds in the unconditioned mode, and
	// that mode's constants are the ones the validator reads (widgetValueMap
	// ignores an inactive mode's, #1519). Collecting every mode let a
	// conditioned one speak for the example: Combo box's database mode set
	// `source` to "database", the example was narrowed for database mode, and
	// MDL-WIDGET10 then flagged the database-only settings it emitted
	// (mendixlabs/mxcli#1583). A definition with no unconditioned mode keeps
	// the old reading.
	modes := def.Modes
	for i := range def.Modes {
		if def.Modes[i].Condition == "" {
			modes = def.Modes[i : i+1]
			break
		}
	}
	for _, mode := range modes {
		collect(mode.PropertyMappings)
	}
	return out
}

// hiddenUnder reports whether a property is hidden in the configuration the
// example describes, so its binding need not be asked for.
//
// Conservative in the direction of asking too much rather than too little: a
// rule whose condition property has no determinable value does NOT prune, and
// nested rules (about an object-list item) never apply to the widget itself.
// Over-listing a binding costs the reader a moment; hiding one they actually
// need would send them to a build error, which is the failure this whole area
// keeps producing.
func hiddenUnder(d WidgetDescription, propertyKey string) bool {
	values := exampleValues(d)
	for _, r := range d.Rules {
		if r.Nested || r.Cond == nil || !strings.EqualFold(r.Property, propertyKey) {
			continue
		}
		fires, determinable := r.rule().Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
			if v, known := values[c.PropertyKey]; known {
				return v, true
			}
			// `empty`/`notempty` ask whether the author picked anything, and for
			// a property the widget DECLARES, "nothing recorded" is the answer,
			// not a gap: an unbound datasource or action holds "". Without this
			// the two operators could never fire, because exampleValues records
			// a property only when it has a non-empty default — which is exactly
			// what an unset datasource does not have.
			//
			// Deliberately scoped to these two operators. Feeding "" to eq/ne/
			// truthy/falsy would change verdicts for rules that already exist,
			// in the direction of hiding a binding the author needs — the one
			// failure this whole area keeps producing.
			if !isEmptinessOperator(c.Operator) || !declaresProperty(d, c.PropertyKey) {
				return "", false // indeterminable — do not guess, keep asking for it
			}
			return "", true
		})
		if determinable && fires {
			return true
		}
	}
	return false
}
