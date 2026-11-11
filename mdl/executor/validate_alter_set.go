// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `ALTER PAGE … SET <prop> ON <widget>` was the one widget-property position
// reference checking never looked at.
//
// ValidateWidgetProperties resolves the properties of widgets a statement
// CARRIES — the trees under CREATE PAGE, and under ALTER's INSERT and REPLACE —
// against the widget definitions. A `SET` carries no widget: it names one that
// is already stored, so its property has to be resolved against the DOCUMENT,
// which that pass never opens. Both of these passed `check -p --references` and
// were then refused by `exec`, on a real 11.13.0 project:
//
//	set NoSuchProperty = 10 on dgProducts;
//	  exec -> pluggable property "NoSuchProperty" not found
//	set PageSize = 12 on noSuchWidget;
//	  exec -> widget "noSuchWidget" not found
//
// Same inversion validate_alter_target.go describes for the document itself:
// check is meant to be the strict gate and exec the thing that runs, and here
// exec was stricter — so a script passed every pre-flight and then stopped
// halfway, having applied the statements before the typo and none after.
//
// What makes this checkable at all is that the answer is not re-derived. The
// vocabulary a `SET` accepts is partly a switch in the setter and partly the
// stored widget's own template keys, which belong to whatever widget package the
// project has installed; anything that restated it here would drift, and drift
// silently in the direction that hurts. Instead the setter is RUN, against a
// throwaway copy of the document (pagemutator.Probe), and only its error is
// kept. Check and exec therefore disagree only if the copy differs from the
// original, which is the one thing that is cheap to guarantee.

// pageProbe is the optional capability the shared BSON page mutator offers: a
// discardable copy to dry-run an operation against, the property names the
// stored widget declares, and what the stored widget IS — its $Type and, for a
// pluggable widget, its widget id. The last is what lets ALTER STYLING's check
// judge a design property against the widget's own type (ako/mxcli#613).
//
// It is an assertion rather than an addition to backend.PageMutator on purpose.
// The MCP mutator has neither — and no pluggable path at all — so asserting is
// also what keeps this pass off a backend whose SET support is different, rather
// than reporting that difference as the author's mistake.
type pageProbe interface {
	Probe() (backend.PageMutator, error)
	ResolvesTarget(widgetRef, columnRef string) bool
	WidgetPropertyKeys(widgetRef, columnRef string) []string
	widgetStorageTyper
}

// validateAlterSetProperties dry-runs every ALTER … SET against the stored
// document and reports what exec would refuse.
func validateAlterSetProperties(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return nil
	}
	grows := documentsTheScriptAddsWidgetsTo(prog)
	opened := map[model.ID]pageProbe{}

	var errs []error
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterPageStmt)
		if !ok || !hasSetPropertyOp(s) || alterTargetComesFromScript(sc, s) {
			continue
		}
		unitID, containerID, containerType, err := resolveAlterPageUnit(ctx, s, h)
		if err != nil {
			// A target that does not resolve is validateAlterTarget's finding,
			// not this one's. Reporting it twice, in two wordings, reads as two
			// problems.
			continue
		}
		probe, ok := opened[unitID]
		if !ok {
			probe = openPageProbe(ctx, unitID)
			opened[unitID] = probe
		}
		if probe == nil {
			continue
		}
		label := fmt.Sprintf("alter %s %s", containerType, s.PageName.String())
		modName := h.GetModuleName(containerID)
		var earlier []*ast.SetPropertyOp
		for _, op := range s.Operations {
			set, ok := op.(*ast.SetPropertyOp)
			if !ok {
				continue
			}
			// A widget an INSERT or REPLACE in this script puts on the page is
			// not in the stored document, so a dry run would report it missing.
			// Only its EXISTENCE is affected, though — a target that does
			// resolve is checked as normal — and matching by name would not
			// work anyway: a DataGrid 2 column is addressed by a derived name,
			// so `column colTitle` is inserted under one name and set under
			// another. Its properties are checked where it is written, by
			// ValidateWidgetProperties.
			if grows[s.PageName.String()] && !probe.ResolvesTarget(set.Target.Widget, columnRefOf(set.Target)) {
				continue
			}
			errs = append(errs, checkSetOp(ctx, probe, label, set, modName, containerID, sc)...)
			errs = append(errs, checkDateFormatSet(ctx, probe, label, earlier, set, modName, containerID)...)
			earlier = append(earlier, set)
		}
	}
	return errs
}

// checkDateFormatSet is exec's checkDateFormatAfterSet for check: a date
// picker left `DateFormat: Custom` with no pattern is CE0493, judged on the
// widget as the statement leaves it — its earlier SETs on that widget applied
// first, then this one whole — because the format and its pattern may be set
// in either order, in one SET or in two (ako/mxcli#1965).
func checkDateFormatSet(ctx *ExecContext, p pageProbe, label string, earlier []*ast.SetPropertyOp, op *ast.SetPropertyOp,
	modName string, modID model.ID) []error {
	touches := false
	for name := range op.Properties {
		if strings.EqualFold(name, "DateFormat") || strings.EqualFold(name, "CustomDateFormat") {
			touches = true
		}
	}
	if !touches || op.Target.IsColumn() || op.Target.Item != nil || op.Target.Widget == "" {
		return nil
	}
	probe, err := p.Probe()
	if err != nil {
		return nil
	}
	for _, e := range earlier {
		if e.Target.Widget == op.Target.Widget && !e.Target.IsColumn() && e.Target.Item == nil {
			_ = applySetProperties(ctx, probe, e, modName, modID) // its own errors are reported for it
		}
	}
	if applySetProperties(ctx, probe, op, modName, modID) != nil {
		return nil // reported by checkSetOp
	}
	if err := checkDateFormatAfterSet(probe, op); err != nil {
		return []error{mdlerrors.NewValidation(fmt.Sprintf("%s: %v", label, err))}
	}
	return nil
}

// openPageProbe opens the stored document and returns it as a prober, or nil
// when it cannot be established — an engine without the capability, a unit that
// will not load. Silence, never a finding: a false "no such property" blocks a
// script that would have worked, which is worse than the gap it replaces.
func openPageProbe(ctx *ExecContext, unitID model.ID) pageProbe {
	mutator, err := ctx.Backend.OpenPageForMutation(unitID)
	if err != nil {
		return nil
	}
	probe, ok := mutator.(pageProbe)
	if !ok {
		return nil
	}
	return probe
}

// checkSetOp dry-runs one SET, one property at a time.
//
// Per property rather than per op, on its own copy each time, so a statement
// setting four properties reports all four mistakes instead of stopping at the
// first — the difference between one round of correction and four.
func checkSetOp(ctx *ExecContext, p pageProbe, label string, op *ast.SetPropertyOp, modName string, modID model.ID, sc *scriptContext) []error {
	names := make([]string, 0, len(op.Properties))
	for name := range op.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		probe, err := p.Probe()
		if err != nil {
			return errs
		}
		one := &ast.SetPropertyOp{
			Target:     op.Target,
			Properties: map[string]any{name: op.Properties[name]},
		}
		if err := applySetProperties(ctx, probe, one, modName, modID); err != nil {
			if scriptDeclaresMissing(sc, err, modName) {
				continue
			}
			errs = append(errs, mdlerrors.NewValidation(fmt.Sprintf(
				"%s: %v%s", label, err, declaredPropertyHint(p, op.Target, name))))
			continue
		}
		for _, msg := range setImageRefErrors(ctx, p, op.Target, name, op.Properties[name], sc) {
			errs = append(errs, mdlerrors.NewValidation(fmt.Sprintf("%s: %s", label, msg)))
		}
	}
	// A column's properties are then judged TOGETHER, with the follow-up that
	// resets what the widget now hides: `ColumnWidth: autoFill, Size: 220` is
	// fine one property at a time and refused as a whole, since the width hides
	// the size, and `ColumnWidth: manual, Size: 220` is the reverse — judged
	// one at a time, Size alone would be hidden (mendixlabs/mxcli#1519).
	if len(errs) == 0 && (op.Target.IsColumn() || op.Target.Item != nil) {
		if probe, err := p.Probe(); err == nil {
			if err := applySetPropertyMutator(ctx, probe, op, modName, modID); err != nil && !scriptDeclaresMissing(sc, err, modName) {
				errs = append(errs, mdlerrors.NewValidation(fmt.Sprintf("%s: %v", label, err)))
			}
		}
	}
	// So is a Gallery's row click: legal or ambiguous depending on the click,
	// the selection and the trigger together, of which a SET may name one
	// (MDL-WIDGET36, ako/mxcli#854).
	if len(errs) == 0 && touchesGalleryClick(op) {
		errs = append(errs, checkGalleryClickSet(ctx, p, label, op, modName, modID, sc)...)
	}
	return errs
}

// checkGalleryClickSet is checkGalleryClickAfterSet for check: an error under
// `mdl 1;`, a warning under mdl 0, judged on a copy with the whole SET applied.
func checkGalleryClickSet(ctx *ExecContext, p pageProbe, label string, op *ast.SetPropertyOp, modName string, modID model.ID, sc *scriptContext) []error {
	probe, err := p.Probe()
	if err != nil || applySetProperties(ctx, probe, op, modName, modID) != nil {
		return nil // a SET that does not apply is reported above, or by exec
	}
	msg := storedGalleryClickAmbiguity(probe, label, op.Target.Widget)
	if msg == "" {
		return nil
	}
	if galleryClickRefused.Applies(ctx.LanguageVersion) {
		return []error{mdlerrors.NewValidation(fmt.Sprintf("%s — %s [MDL-WIDGET36]", msg, galleryClickSuggestion))}
	}
	if sc != nil {
		sc.warnings = append(sc.warnings, fmt.Sprintf("%s [%s] — %s. %s",
			msg, galleryClickRefused.Code, galleryClickSuggestion, galleryClickRefused.Warning(ctx.LanguageVersion)))
	}
	return nil
}

// setImageRefErrors resolves the image a SET writes onto a legacy image viewer
// — a static image's Image, a dynamic image's DefaultImage — as CREATE resolves
// the same property (mendixlabs/mxcli#1464). The dry run only proves the key is
// writable; the name it writes is a reference, and an unresolved one is CE1613
// a build later. Only for the stored widget types whose key that is
// (pages.ImageReferenceProperty), so a pluggable widget's `image` is untouched.
func setImageRefErrors(ctx *ExecContext, p pageProbe, target ast.WidgetRef, name string, value any, sc *scriptContext) []string {
	if p == nil || target.Widget == "" || columnRefOf(target) != "" {
		return nil
	}
	bsonType, _ := p.WidgetStorageType(target.Widget)
	key := pages.ImageReferenceProperty(bsonType)
	img, isString := value.(string)
	if key == "" || !strings.EqualFold(key, name) || !isString || strings.TrimSpace(img) == "" {
		return nil
	}
	return resolveImageRefs(ctx, []string{img}, sc)
}

// scriptDeclaresMissing reports whether a dry-run failed only because the SET
// names a microflow, nanoflow or page the script itself creates.
//
// The dry run resolves against the stored project, and the session cache that
// lets exec find a document created a few statements earlier is filled only by
// executing — which check never does. So `set Action = microflow M.X on btn`
// after `create microflow M.X` was "microflow not found" in check while exec
// ran it fine, and since exec runs check first, the script could not run at all
// (ako/mxcli#969). What the setter would do with the resolved document is
// beyond a dry run without it; reference validation already covers a name the
// script does NOT declare, which still fails here.
func scriptDeclaresMissing(sc *scriptContext, err error, modName string) bool {
	if sc == nil {
		return false
	}
	var nf *mdlerrors.NotFoundError
	if !errors.As(err, &nf) {
		return false
	}
	var declared map[string]bool
	switch nf.Kind {
	case "microflow":
		declared = sc.microflows
	case "nanoflow":
		declared = sc.nanoflows
	case "page":
		declared = sc.pages
	case "snippet":
		declared = sc.snippets
	default:
		return false
	}
	name := unquoteQualifiedName(nf.Name)
	if !strings.Contains(name, ".") {
		name = modName + "." + name
	}
	return declared[name]
}

// declaredPropertyHint names what the widget does have, when the widget resolves
// and declares its own property keys. A near-miss is called out first: the
// spelling of a pluggable key is the thing authors get wrong (it is lowerCamel
// in the template, capitalised in DESCRIBE output), so `pagesize` against
// `pageSize` should read as a typo and not as a list to search.
func declaredPropertyHint(p pageProbe, target ast.WidgetRef, prop string) string {
	if target.Widget == "" {
		return "" // page-level SET: the setter's own error lists what it takes
	}
	keys := p.WidgetPropertyKeys(target.Widget, columnRefOf(target))
	if len(keys) == 0 {
		return ""
	}
	for _, k := range keys {
		if strings.EqualFold(k, prop) {
			return "" // it does have it — the failure is about the value
		}
	}
	hint := ""
	if near := nearestKey(prop, keys); near != "" {
		hint = fmt.Sprintf(" — did you mean `%s`?", near)
	}
	const max = 8
	if len(keys) > max {
		return fmt.Sprintf("%s (%s declares %s and %d more)",
			hint, target.Name(), strings.Join(keys[:max], ", "), len(keys)-max)
	}
	return fmt.Sprintf("%s (%s declares %s)", hint, target.Name(), strings.Join(keys, ", "))
}

// hasSetPropertyOp reports whether a statement carries anything this pass would
// look at, so a script of pure INSERTs never opens a document.
func hasSetPropertyOp(s *ast.AlterPageStmt) bool {
	for _, op := range s.Operations {
		if _, ok := op.(*ast.SetPropertyOp); ok {
			return true
		}
	}
	return false
}

// alterTargetComesFromScript reports whether the document this ALTER edits is
// one the script itself creates — in which case there is nothing stored to dry
// run against, and the widgets it names are checked where they are written.
func alterTargetComesFromScript(sc *scriptContext, s *ast.AlterPageStmt) bool {
	if sc == nil {
		return false
	}
	qn := s.PageName.String()
	if s.PageName.Module == "" || sc.modules[s.PageName.Module] {
		return true
	}
	switch strings.ToUpper(s.ContainerType) {
	case "SNIPPET":
		return sc.snippets[qn]
	case "LAYOUT":
		return sc.layouts[qn]
	default:
		return sc.pages[qn]
	}
}

// documentsTheScriptAddsWidgetsTo names the documents an INSERT or REPLACE
// somewhere in the script puts new widgets on — the ones whose stored widget
// tree is not the tree a later SET will run against.
//
// Deliberately not ordered: a SET before the INSERT that adds its target fails
// at exec too, so suppressing it here costs one unreported error, while getting
// the order wrong the other way costs a false one on a script that works. Those
// are not comparable — the second blocks the run.
func documentsTheScriptAddsWidgetsTo(prog *ast.Program) map[string]bool {
	grows := map[string]bool{}
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterPageStmt)
		if !ok {
			continue
		}
		for _, op := range s.Operations {
			switch op.(type) {
			case *ast.InsertWidgetOp, *ast.ReplaceWidgetOp:
				grows[s.PageName.String()] = true
			}
		}
	}
	return grows
}
