// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// mappingHandlerTypeRule is the rule ID for "an import mapping's custom handler
// feeds a JSON member to a microflow parameter of an incompatible type".
const mappingHandlerTypeRule = "MDL107"

// A custom object handler's `P: member` feeds a JSON member to the microflow's
// parameter P, and the two types must agree. Measured on MxBuild 11.16.1, ONE
// import mapping per project (CE1613 hides every other error), against a sample
// holding a string, an integer, a decimal, a boolean and a date-time; every pairing
// not listed here is CE0282 "The microflow parameter … does not match the specified
// type":
//
//	String parameter         any member type
//	Integer parameter        an Integer member
//	Long parameter           an Integer member
//	Decimal parameter        an Integer or a Decimal member
//	Boolean, DateTime param  a member of the same type
//
// check and exec accepted every rejected pairing and wrote the mapping.
//
// The member is found by NAME in the whole structure rather than by following the
// handler's element path through the mapping, which would duplicate the builder's
// array and two-level-item handling (and drift from it, as a CE0281 mirror would).
// It is judged only when every place that name occurs is a value of one and the same
// known type; a name that is absent, or that is a string in one place and an integer
// in another, is left to the build. Only `path` sources are judged — `parent` and
// `parameter` feed objects. A microflow or structure that cannot be found, and a
// parameter of any other type (object, list, enumeration, date), is not judged.
func ValidateMappingHandlerTypes(prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	if prog == nil {
		return nil
	}
	scriptFlows := map[string]map[string]string{} // lower-cased microflow QN -> parameter -> type name
	scriptSamples := map[string]string{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			params := map[string]string{}
			for _, p := range s.Parameters {
				params[p.Name] = scriptParamTypeName(p.Type)
			}
			scriptFlows[strings.ToLower(s.Name.String())] = params
		case *ast.CreateJsonStructureStmt:
			scriptSamples[s.Name.String()] = s.JsonSnippet
		}
	}

	var projectFlows map[string]map[string]string
	flowParams := func(qn string) (map[string]string, bool) {
		if p, ok := scriptFlows[strings.ToLower(qn)]; ok {
			return p, true
		}
		if projectFlows == nil {
			projectFlows = loadProjectMicroflowParamTypes(open)
		}
		p, ok := projectFlows[strings.ToLower(qn)]
		return p, ok
	}
	structures := map[string][]*types.JsonElement{}
	structureElems := func(ref ast.QualifiedName) []*types.JsonElement {
		key := ref.String()
		if elems, done := structures[key]; done {
			return elems
		}
		var elems []*types.JsonElement
		if sample, ok := scriptSamples[key]; ok {
			elems, _ = types.BuildJsonElementsFromSnippet(sample, nil, nil)
		} else if open != nil {
			if b := open(); b != nil {
				if js, err := b.GetJsonStructureByQualifiedName(ref.Module, ref.Name); err == nil && js != nil {
					elems = js.Elements
				}
			}
		}
		structures[key] = elems
		return elems
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		m, ok := stmt.(*ast.CreateImportMappingStmt)
		if !ok || m.RootElement == nil || m.SchemaKind != "JSON_STRUCTURE" || m.SchemaRef.Module == "" {
			continue
		}
		var walk func(e *ast.ImportMappingElementDef)
		walk = func(e *ast.ImportMappingElementDef) {
			if h := e.CustomHandler; h != nil && h.Microflow != "" {
				qn := qualifyIn(m.Name.Module, h.Microflow)
				if params, known := flowParams(qn); known {
					out = append(out, handlerTypeViolations(m.Name, h, qn, params, structureElems(m.SchemaRef))...)
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

// scriptParamTypeName names the parameter types the matrix was measured for; any
// other type is "" and is not judged.
func scriptParamTypeName(t ast.DataType) string {
	switch t.Kind {
	case ast.TypeString:
		return "String"
	case ast.TypeInteger:
		return "Integer"
	case ast.TypeLong:
		return "Long"
	case ast.TypeDecimal:
		return "Decimal"
	case ast.TypeBoolean:
		return "Boolean"
	case ast.TypeDateTime:
		return "DateTime"
	}
	return ""
}

// loadProjectMicroflowParamTypes reads every project microflow's parameter types,
// keyed by lower-cased qualified name. A type outside the measured set reads "".
func loadProjectMicroflowParamTypes(open func() backend.FullBackend) map[string]map[string]string {
	out := map[string]map[string]string{}
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
		if mf == nil || names[string(mf.ContainerID)] == "" {
			continue
		}
		params := map[string]string{}
		for _, p := range mf.Parameters {
			if p == nil || p.Type == nil {
				continue
			}
			switch n := p.Type.GetTypeName(); n {
			case "String", "Integer", "Long", "Decimal", "Boolean", "DateTime":
				params[p.Name] = n
			default:
				params[p.Name] = ""
			}
		}
		out[strings.ToLower(names[string(mf.ContainerID)]+"."+mf.Name)] = params
	}
	return out
}

// handlerTypeCompatible is the measured matrix: may a member of type member be fed
// to a parameter of type param.
func handlerTypeCompatible(param, member string) bool {
	switch param {
	case "String":
		return true
	case "Integer", "Long":
		return member == "Integer"
	case "Decimal":
		return member == "Integer" || member == "Decimal"
	}
	return param == member
}

// memberPrimitiveType is the one type every occurrence of the member has, or "" when
// the name is absent, is not a value, is of a type outside the measured set, or
// differs between occurrences.
func memberPrimitiveType(elems []*types.JsonElement, member string) string {
	suffix := "|" + strings.ReplaceAll(member, "/", "|")
	found := ""
	ambiguous := false
	var walk func(es []*types.JsonElement)
	walk = func(es []*types.JsonElement) {
		for _, e := range es {
			if e == nil {
				continue
			}
			if strings.HasSuffix(e.Path, suffix) || (!strings.Contains(member, "/") && e.ExposedName == member) {
				switch {
				case e.ElementType != "Value":
					ambiguous = true
				case found == "":
					found = e.PrimitiveType
				case found != e.PrimitiveType:
					ambiguous = true
				}
			}
			walk(e.Children)
		}
	}
	walk(elems)
	switch found {
	case "String", "Integer", "Decimal", "Boolean", "DateTime":
		if !ambiguous {
			return found
		}
	}
	return ""
}

func handlerTypeViolations(mapping ast.QualifiedName, h *ast.MappingCustomHandlerDef, qn string, params map[string]string, elems []*types.JsonElement) []linter.Violation {
	loc := linter.Location{Module: mapping.Module, DocumentType: "import mapping", DocumentName: mapping.Name}
	var out []linter.Violation
	for _, p := range h.Parameters {
		if p == nil || p.Source != "path" || p.Path == "" {
			continue
		}
		paramType := params[p.Parameter]
		if paramType == "" {
			continue
		}
		memberType := memberPrimitiveType(elems, p.Path)
		if memberType == "" || handlerTypeCompatible(paramType, memberType) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   mappingHandlerTypeRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"import mapping %s: the handler `by %s` feeds %s, a %s member, to parameter %s of type %s — "+
					"mxbuild reports CE0282 \"The microflow parameter '%s' is of type '%s'. It does not match the specified type '%s'\"",
				mapping.String(), h.Microflow, p.Path, memberType, p.Parameter, paramType, p.Parameter, paramType, memberType),
			Location: loc,
			Suggestion: fmt.Sprintf("Declare %s.%s as %s, or key on a %s member. Only a String parameter takes every member type; "+
				"Decimal also takes an Integer.", qn, p.Parameter, memberType, paramType),
		})
	}
	return out
}
