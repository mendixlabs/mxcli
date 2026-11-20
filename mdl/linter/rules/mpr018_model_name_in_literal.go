// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ModelNameInLiteralRule flags a model element named inside a string literal,
// where renaming that element passes every check and breaks the app at runtime.
//
// From the benchmark that raised ako/mxcli#2405, in the mxcli-built app:
//
//	"The simulator tick is called by its name as text. A Java action receives
//	 the text "Simulation.SIM_Tick", so Studio Pro does not see the call:
//	 renaming or moving the microflow passes the checks, and the simulation
//	 stops, with an ERROR in the server log every second."
//
// Nothing in the toolchain guards this. Studio Pro's reference checking,
// mx check and `mxcli check --references` all work on real references; a
// qualified name inside a string is text to every one of them. The failure
// arrives at runtime, in production, in a flow nobody edited.
//
// mxcli has a particular stake in it: it makes renaming cheap and scriptable
// (`rename microflow`, `rename entity`), and `check --references` is sold as
// the thing that catches a stale reference. A string literal is the one case
// where that promise silently does not hold.
//
// # Two decisions that make this precise rather than noisy
//
// It reads string LITERALS, not expressions. An expression's own references are
// spelled with dots — an association path `$Var/Mod.Assoc/Attr`, an
// enumeration value `Mod.Enum.Value` — and those are real references Studio Pro
// resolves and renames. Only a single-quoted literal is opaque to it. Scanning
// whole expressions would report every association path in the project.
//
// It reads the fields that hold a VALUE, never those that hold a reference.
// `JavaActionCallAction.JavaAction`, `MicroflowCall.Microflow` and
// `JavaActionParameterMapping.Parameter` all carry qualified names and are all
// proper references; reporting them would flag every call in the project. The
// separation is by field name — Expression, Argument, Value — and it holds
// because a reference field is named after what it references. Walking by name
// rather than by a list of action types also means a new action type is covered
// the day it is added, as long as it follows that convention.
type ModelNameInLiteralRule struct{}

func NewModelNameInLiteralRule() *ModelNameInLiteralRule {
	return &ModelNameInLiteralRule{}
}

func (r *ModelNameInLiteralRule) ID() string                       { return "MPR018" }
func (r *ModelNameInLiteralRule) Name() string                     { return "ModelNameInLiteral" }
func (r *ModelNameInLiteralRule) Category() string                 { return "correctness" }
func (r *ModelNameInLiteralRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }
func (r *ModelNameInLiteralRule) Description() string {
	return "A model element named inside a string literal, where a rename passes every check and breaks the app at runtime"
}

// literalVerdict is what a literal turned out to be.
type literalVerdict int

const (
	// verdictNone: not a qualified name of this project.
	verdictNone literalVerdict = iota
	// verdictResolves: names a real element. A rename breaks it, unseen.
	verdictResolves
	// verdictDangling: a module of this project, but no such element. Probably
	// already stale — or a name that was never right.
	verdictDangling
)

func (r *ModelNameInLiteralRule) Check(ctx *linter.LintContext) []linter.Violation {
	if ctx == nil || ctx.Reader() == nil {
		return nil
	}
	modules, resolves := projectNameIndex(ctx)
	if len(modules) == 0 {
		return nil
	}

	var violations []linter.Violation
	for mf := range ctx.Microflows() {
		if ctx.IsExcluded(mf.ModuleName) || ctx.IsDocumentExcluded(mf.QualifiedName) {
			continue
		}
		full, err := ctx.FullMicroflow(model.ID(mf.ID))
		if err != nil || full == nil || full.ObjectCollection == nil {
			continue
		}
		// One finding per distinct literal per flow: the same name passed in
		// two places is one thing to fix, and a loop body repeats itself.
		seen := map[string]bool{}
		var found []string
		collectValueStrings(full.ObjectCollection.Objects, func(s string) {
			for _, lit := range expressionStringLiterals(s) {
				if seen[lit] {
					continue
				}
				if v := classifyLiteral(lit, modules, resolves); v != verdictNone {
					seen[lit] = true
					found = append(found, lit)
				}
			}
		})
		sort.Strings(found)
		for _, lit := range found {
			v := classifyLiteral(lit, modules, resolves)
			violations = append(violations, linter.Violation{
				RuleID:     r.ID(),
				Severity:   literalSeverity(v, r),
				Message:    literalMessage(v, lit, mf.DocumentNoun(), mf.QualifiedName),
				Location:   linter.Location{Module: mf.ModuleName, DocumentType: mf.DocumentNoun(), DocumentName: mf.Name, DocumentID: mf.ID},
				Suggestion: literalSuggestion(v),
			})
		}
	}
	return violations
}

func literalSeverity(v literalVerdict, r *ModelNameInLiteralRule) linter.Severity {
	if v == verdictDangling {
		// Weaker on purpose: a string can legitimately look like a qualified
		// name, and nothing is provably broken by one that resolves to nothing.
		return linter.SeverityInfo
	}
	return r.DefaultSeverity()
}

func literalMessage(v literalVerdict, lit, noun, flow string) string {
	if v == verdictDangling {
		return fmt.Sprintf("The %s '%s' holds the text '%s', which looks like a qualified name in this project but resolves to nothing.",
			noun, flow, lit)
	}
	return fmt.Sprintf("The %s '%s' names '%s' as text, so no check sees the reference: renaming or moving it passes Studio Pro, mx check and 'mxcli check --references', and fails at runtime.",
		noun, flow, lit)
}

func literalSuggestion(v literalVerdict) string {
	if v == verdictDangling {
		return "If it is meant to name a model element, correct it; if it is ordinary text that happens to look like a qualified name, ignore this."
	}
	return "Pass the element as a real reference where the action allows one. Where it can only take text (a Java action taking a microflow name), " +
		"keep the name in one place — a constant — so a rename has one site to update, and note in the element's documentation that it is called by name."
}

// projectNameIndex returns the project's module names and every qualified name
// that resolves to a document or entity, both lower-cased because Mendix
// resolves names case-insensitively.
//
// Resolution spans EVERY module, platform ones included: an app's flow naming a
// System microflow as text is still a reference no check sees. Only the flows
// that are REPORTED are limited to what the user owns, by the caller.
func projectNameIndex(ctx *linter.LintContext) (modules, resolves map[string]bool) {
	modules, resolves = map[string]bool{}, map[string]bool{}
	mods, err := ctx.Reader().ListModules()
	if err != nil {
		return modules, resolves
	}
	for _, m := range mods {
		if m != nil && m.Name != "" {
			modules[strings.ToLower(m.Name)] = true
		}
	}
	add := func(qn string) {
		if qn != "" {
			resolves[strings.ToLower(qn)] = true
		}
	}
	for e := range ctx.Entities() {
		add(e.QualifiedName)
		for a := range ctx.AttributesFor(e.QualifiedName) {
			add(a.EntityQualifiedName + "." + a.Name)
		}
	}
	for m := range ctx.Microflows() {
		add(m.QualifiedName)
	}
	for p := range ctx.Pages() {
		add(p.QualifiedName)
	}
	for en := range ctx.Enumerations() {
		add(en.QualifiedName)
	}
	for s := range ctx.Snippets() {
		add(s.QualifiedName)
	}
	for c := range ctx.Constants() {
		add(c.QualifiedName)
	}
	return modules, resolves
}

// classifyLiteral decides what one literal is.
func classifyLiteral(lit string, modules, resolves map[string]bool) literalVerdict {
	mod, qn, ok := qualifiedNameLiteral(lit)
	if !ok {
		return verdictNone
	}
	// The prefix must be a module this project actually has. That single test
	// is what keeps prose, file paths, CSS selectors, URLs and Java packages
	// out: "System.Text.Json" is not reportable unless a module is named
	// System.Text, and a sentence does not survive the exact-match shape above.
	if !modules[strings.ToLower(mod)] {
		return verdictNone
	}
	if resolves[strings.ToLower(qn)] {
		return verdictResolves
	}
	return verdictDangling
}

// qualifiedNameLiteral reports whether lit is EXACTLY a qualified name of two
// or three identifier segments (Module.Document, or Module.Entity.Member), and
// returns the module and the whole name.
//
// Exact, not contained: a literal that mentions a name in prose is a stale
// message at worst, and matching inside prose is where the false positives are.
func qualifiedNameLiteral(lit string) (module, qualified string, ok bool) {
	if lit == "" || lit != strings.TrimSpace(lit) {
		return "", "", false
	}
	parts := strings.Split(lit, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", false
	}
	for _, p := range parts {
		if !isIdentifier(p) {
			return "", "", false
		}
	}
	return parts[0], lit, true
}

// isIdentifier reports whether s looks like a Mendix name: a letter or
// underscore, then letters, digits or underscores.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// expressionStringLiterals returns the contents of every single-quoted literal
// in a Mendix expression, with doubled quotes unescaped.
//
// Mendix escapes a quote by doubling it ('it”s here'), not with a backslash,
// so ” inside a literal continues it rather than closing and reopening one.
// An unterminated quote yields nothing rather than reading to the end.
func expressionStringLiterals(expr string) []string {
	var out []string
	for i := 0; i < len(expr); i++ {
		if expr[i] != '\'' {
			continue
		}
		var b strings.Builder
		closed := false
		for j := i + 1; j < len(expr); j++ {
			if expr[j] != '\'' {
				b.WriteByte(expr[j])
				continue
			}
			if j+1 < len(expr) && expr[j+1] == '\'' {
				b.WriteByte('\'')
				j++
				continue
			}
			out = append(out, b.String())
			i = j
			closed = true
			break
		}
		if !closed {
			break // unterminated: nothing further is a literal
		}
	}
	return out
}

// valueFieldNames are the struct fields that hold a Mendix expression or value.
// Every other string field is a reference, a variable name or a caption — see
// the type comment for why that distinction is the whole design.
var valueFieldNames = map[string]bool{"Expression": true, "Argument": true, "Value": true}

// valueStrings returns the expression/value strings of one action, found by
// field name through reflection so a new action type needs no change here.
func valueStrings(action any) []string {
	var out []string
	var walk func(v reflect.Value, depth int)
	walk = func(v reflect.Value, depth int) {
		if depth > 8 || !v.IsValid() {
			return
		}
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem(), depth+1)
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), depth+1)
			}
		case reflect.Struct:
			t := v.Type()
			for i := 0; i < v.NumField(); i++ {
				f := t.Field(i)
				if !f.IsExported() {
					continue
				}
				fv := v.Field(i)
				if fv.Kind() == reflect.String {
					if valueFieldNames[f.Name] && fv.String() != "" {
						out = append(out, fv.String())
					}
					continue
				}
				walk(fv, depth+1)
			}
		}
	}
	walk(reflect.ValueOf(action), 0)
	return out
}

// collectValueStrings visits every action's value strings, descending into
// loops the way CONV020 does — a call inside a loop is still a call.
func collectValueStrings(objs []microflows.MicroflowObject, visit func(string)) {
	for _, o := range objs {
		switch a := o.(type) {
		case *microflows.LoopedActivity:
			if a.ObjectCollection != nil {
				collectValueStrings(a.ObjectCollection.Objects, visit)
			}
		case *microflows.ActionActivity:
			if a.Action != nil {
				for _, s := range valueStrings(a.Action) {
					visit(s)
				}
			}
		case *microflows.ExclusiveSplit:
			for _, s := range valueStrings(a) {
				visit(s)
			}
		}
	}
}
