// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// mappingHandlerParamRule is the rule ID for "an import mapping's custom handler
// names a parameter its microflow does not have".
const mappingHandlerParamRule = "MDL104"

// A custom object handler (`find … by M.MF ( P: member )`) must name exactly the
// microflow's parameters. Measured on MxBuild 11.16.1, ONE import mapping per
// project — a shared project is wrong, because CE1613 is an unresolved reference
// and hides every other error, which first made two of these look clean:
//
//	the microflow's own parameter names, all of them   clean
//	a name it does not have                            CE1613 "The selected parameter 'M.MF.P' no longer exists."
//	the right name in another case                     CE1613 (names are case-sensitive)
//	a valid name plus an unknown one                   CE1613
//	one of two parameters left out                     CE0277 "Parameter mapping of microflow for obtaining objects needs to be refreshed."
//	none named on a one-parameter microflow            CE0277
//	none named on a microflow with no parameters       clean
//	a name on a microflow with no parameters           CE1613
//
// check and exec accepted every rejected case and wrote the mapping. The build
// reports CE1613 before CE0277, so a handler that has an unknown name is judged on
// that alone; the missing parameters are reported once the names are right.
//
// A microflow that cannot be found — not the script's, not the project's, or no
// project — is not judged: exec already refuses one it cannot find, and an
// unanswered question is not "no". A parameter's TYPE against the member it is
// fed is a different error (CE0282) and is not judged here.
//
// This is NOT CE0281 ("Unknown value element … selected for parameter"), which the
// same construct raises for a MEMBER that is not in the JSON structure: exec
// refuses that one itself (mapping_customhandler.go), so mx check never sees it.
func ValidateMappingHandlerParams(prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	if prog == nil {
		return nil
	}
	script := map[string]map[string]bool{} // lower-cased microflow QN -> its parameter names, exactly as written
	for _, stmt := range prog.Statements {
		if mf, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			names := map[string]bool{}
			for _, p := range mf.Parameters {
				names[p.Name] = true
			}
			script[strings.ToLower(mf.Name.String())] = names
		}
	}

	var project map[string]map[string]bool
	loaded := false
	projectParams := func(qn string) (map[string]bool, bool) {
		if !loaded {
			loaded = true
			project = loadProjectMicroflowParams(open)
		}
		names, ok := project[strings.ToLower(qn)]
		return names, ok
	}
	declared := func(qn string) (map[string]bool, bool) {
		if names, ok := script[strings.ToLower(qn)]; ok {
			return names, true
		}
		return projectParams(qn)
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		m, ok := stmt.(*ast.CreateImportMappingStmt)
		if !ok || m.RootElement == nil {
			continue
		}
		var walk func(e *ast.ImportMappingElementDef)
		walk = func(e *ast.ImportMappingElementDef) {
			if h := e.CustomHandler; h != nil && h.Microflow != "" {
				qn := qualifyIn(m.Name.Module, h.Microflow)
				if names, known := declared(qn); known {
					out = append(out, handlerParamViolations(m.Name, h, qn, names)...)
				}
			}
			for _, c := range e.Children {
				if c != nil {
					walk(c)
				}
			}
		}
		walk(m.RootElement)
	}
	return out
}

// loadProjectMicroflowParams reads every project microflow's parameter names,
// keyed by lower-cased qualified name. Nothing is returned when no project can
// be read, which leaves every project microflow unjudged.
func loadProjectMicroflowParams(open func() backend.FullBackend) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	if open == nil {
		return out
	}
	b := open()
	if b == nil {
		return out
	}
	modules, err := b.ListModules()
	if err != nil {
		return out
	}
	names := map[string]string{}
	for _, mod := range modules {
		if mod != nil {
			names[string(mod.ID)] = mod.Name
		}
	}
	mfs, err := b.ListMicroflows()
	if err != nil {
		return out
	}
	for _, mf := range mfs {
		mod := names[string(mf.ContainerID)]
		if mf == nil || mod == "" {
			continue
		}
		params := map[string]bool{}
		for _, p := range mf.Parameters {
			if p != nil {
				params[p.Name] = true
			}
		}
		out[strings.ToLower(mod+"."+mf.Name)] = params
	}
	return out
}

// handlerParamList is a microflow's parameter names, sorted, for the suggestion.
func handlerParamList(m map[string]bool) string {
	if len(m) == 0 {
		return "(it declares none)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// handlerParamViolations judges one handler against its microflow's parameters.
func handlerParamViolations(mapping ast.QualifiedName, h *ast.MappingCustomHandlerDef, qn string, declared map[string]bool) []linter.Violation {
	loc := linter.Location{Module: mapping.Module, DocumentType: "import mapping", DocumentName: mapping.Name}
	var out []linter.Violation
	named := map[string]bool{}
	for _, p := range h.Parameters {
		if p == nil {
			continue
		}
		named[p.Parameter] = true
		if declared[p.Parameter] {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   mappingHandlerParamRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"import mapping %s: the handler `by %s` names a parameter %q, which %s does not declare — "+
					"mxbuild reports CE1613 \"The selected parameter '%s.%s' no longer exists\" "+
					"(parameter names are case-sensitive)",
				mapping.String(), h.Microflow, p.Parameter, qn, qn, p.Parameter),
			Location:   loc,
			Suggestion: fmt.Sprintf("Use %s's own parameter names: %s.", qn, handlerParamList(declared)),
		})
	}
	if len(out) > 0 {
		return out // CE1613 comes first; the missing ones are reported once the names are right
	}
	var missing []string
	for name := range declared {
		if !named[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return []linter.Violation{{
		RuleID:   mappingHandlerParamRule,
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"import mapping %s: the handler `by %s` does not name %s of %s's parameters — mxbuild reports "+
				"CE0277 \"Parameter mapping of microflow for obtaining objects needs to be refreshed\"",
			mapping.String(), h.Microflow, strings.Join(missing, ", "), qn),
		Location:   loc,
		Suggestion: fmt.Sprintf("Name every parameter of %s: %s (each as `Name: source`).", qn, handlerParamList(declared)),
	}}
}
