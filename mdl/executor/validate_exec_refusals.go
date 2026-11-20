// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/translations"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// Refusals exec makes from project state that is not a document listing, and
// that `check -p` therefore did not predict (ako/mxcli#906). check predicted
// "already exists" for every document kind, because those go through one
// registry (CheckProjectConflicts); these each had their own lookup inside the
// exec handler, so check passed a statement exec then refused:
//
//   - `alter module … add jar dependency` for a coordinate the module has
//     (formula1, every run after the first);
//   - `create … translations … for <the source language>` (captrack, every run);
//   - `create demo user` for a user that exists — that one is a create, and now
//     goes through the registry like the other creates (stmtCreateKind).
//
// What keeps check and exec from disagreeing again is that the prediction CALLS
// exec's decision instead of restating it: the jar actions are replayed through
// applyJarDepAction on a copy of the stored settings, and the translations
// refusal is translationsRefusal, which execCreateTranslations calls too. A
// prediction is only made while the stored project is what exec will see; once
// an earlier statement of the script changes that state, check stays silent
// rather than guess.

// CheckExecRefusals returns the refusals exec would make for the statements of
// prog, given the connected project.
func CheckExecRefusals(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	var errs []error
	errs = append(errs, checkJarDependencyActions(ctx, prog)...)
	errs = append(errs, checkTranslationTargets(ctx, prog)...)
	return errs
}

// checkJarDependencyActions replays every `alter module … jar dependency`
// statement against a copy of the stored module settings, in script order, with
// the function exec applies them with. A refusal is reported once per module:
// after it exec has stopped, and what the module holds is no longer known.
func checkJarDependencyActions(ctx *ExecContext, prog *ast.Program) []error {
	settings := map[string]*types.ModuleSettings{}
	unknown := map[string]bool{} // modules whose state the script makes unpredictable
	var errs []error
	for i, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateModuleStmt:
			unknown[s.Name] = true
		case *ast.DropModuleStmt:
			unknown[s.Name] = true
		case *ast.AlterModuleJarDepStmt:
			if unknown[s.ModuleName] {
				continue
			}
			ms := settings[s.ModuleName]
			if ms == nil {
				module, err := ctx.Backend.GetModuleByName(s.ModuleName)
				if err != nil || module == nil {
					unknown[s.ModuleName] = true // a missing module is reported as itself
					continue
				}
				stored, err := ctx.Backend.GetModuleSettings(module.ID)
				if err != nil || stored == nil {
					unknown[s.ModuleName] = true
					continue
				}
				ms = cloneJarDependencies(stored)
				settings[s.ModuleName] = ms
			}
			for _, action := range s.Actions {
				if _, err := applyJarDepAction(ms, action, s.ModuleName); err != nil {
					errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
					unknown[s.ModuleName] = true
					break
				}
			}
		}
	}
	return errs
}

// cloneJarDependencies copies the part of the settings applyJarDepAction
// mutates, so the prediction never touches what the backend handed out.
func cloneJarDependencies(ms *types.ModuleSettings) *types.ModuleSettings {
	out := *ms
	out.JarDependencies = make([]*types.JarDependency, 0, len(ms.JarDependencies))
	for _, d := range ms.JarDependencies {
		if d == nil {
			continue
		}
		c := *d
		c.Exclusions = make([]*types.JarDependencyExclusion, 0, len(d.Exclusions))
		for _, e := range d.Exclusions {
			if e != nil {
				ec := *e
				c.Exclusions = append(c.Exclusions, &ec)
			}
		}
		out.JarDependencies = append(out.JarDependencies, &c)
	}
	return &out
}

// checkTranslationTargets reports a translations statement exec refuses. Once
// the script changes the language settings the stored source language is no
// longer what exec sees, and once it has written translations for a language a
// later plain CREATE of that language is decided by the script, not the
// project — the prediction stops at both.
func checkTranslationTargets(ctx *ExecContext, prog *ast.Program) []error {
	// The project settings are read only for a script that writes
	// translations; every other script would pay a settings read for nothing.
	hasTranslations := false
	for _, stmt := range prog.Statements {
		if _, ok := stmt.(*ast.CreateTranslationsStmt); ok {
			hasTranslations = true
			break
		}
	}
	if !hasTranslations {
		return nil
	}
	src := sourceLanguage(ctx)
	if src == "" {
		return nil
	}
	written := map[string]bool{}
	var errs []error
	for i, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.AlterSettingsStmt:
			if strings.EqualFold(s.Section, "LANGUAGE") {
				return errs
			}
		case *ast.CreateTranslationsStmt:
			lang := strings.ToLower(s.Language)
			if written[lang] {
				continue
			}
			written[lang] = true
			scope, err := translationScope(ctx, s.Module, s.WithoutMarketplace)
			if err != nil {
				continue // a missing module is reported by exec as itself
			}
			if err := translationsRefusal(ctx, s, src, scope); err != nil {
				errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
			}
		}
	}
	return errs
}

// translationsRefusal is the decision execCreateTranslations makes before it
// writes anything: no translations into the source language, and a plain CREATE
// does not overwrite a language that already has translations. check -p calls
// it too, so the two cannot disagree.
func translationsRefusal(ctx *ExecContext, s *ast.CreateTranslationsStmt, src string, scope translations.Scope) error {
	if strings.EqualFold(s.Language, src) {
		return mdlerrors.NewValidationf(
			"%s is the project's source language — writing translations into it would "+
				"overwrite the strings the rest of the model is keyed on", s.Language)
	}
	if s.Mode != ast.TranslationsCreate {
		return nil
	}
	// The language is the thing that exists, so bare CREATE refuses when it
	// already has translations — the same contract every other CREATE has.
	existing, err := translations.Languages(ctx.Backend, scope)
	if err != nil {
		return mdlerrors.NewBackend("read languages", err)
	}
	for _, l := range existing {
		if strings.EqualFold(l, s.Language) {
			return mdlerrors.NewValidationf(
				"%s already has translations — use `create or modify translations` to "+
					"merge these in, or `create or replace translations` to make this file "+
					"authoritative (which REMOVES translations it does not name)", s.Language)
		}
	}
	return nil
}
