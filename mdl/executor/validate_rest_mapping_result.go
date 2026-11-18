// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// restMappingResultRule is the rule ID for "a REST call declared to return a
// list from a mapping that returns one object".
const restMappingResultRule = "MDL103"

// CE0243 "The mapping used to return a value of type 'List of X', but now
// returns a value of type 'X'." — a `rest call … returns mapping M as list of E`
// over an import mapping whose root is one object. Measured on MxBuild 11.16.1,
// one microflow per case:
//
//	object-rooted mapping   as E          clean
//	object-rooted mapping   as list of E  CE0243   <- check and exec accepted it
//	array-rooted mapping    as E          clean    (binds one out of a list)
//	array-rooted mapping    as list of E  clean
//	mapping rooted below an array, either form  clean
//
// Only the first row of the second kind is wrong. Binding ONE object out of a
// list-rooted mapping is a real pattern (Studio Pro's own
// PrivateCloudData.REST_GetEnvironmentByUUID), so `as E` is never judged.
//
// The mapping's shape comes from the same resolver MDL-MAP04 uses: the script
// when it creates the mapping and its JSON structure, the project otherwise. A
// shape it cannot establish — a root path, an XML or message-definition mapping,
// no project — leaves the statement alone.
func ValidateRestMappingResult(prog *ast.Program, projectPath string) []linter.Violation {
	if prog == nil {
		return nil
	}
	type use struct {
		kind string
		flow ast.QualifiedName
		call *ast.RestCallStmt
	}
	var uses []use
	collect := func(kind string, name ast.QualifiedName, body []ast.MicroflowStatement) {
		var walk func([]ast.MicroflowStatement)
		walk = func(stmts []ast.MicroflowStatement) {
			forEachMicroflowStatement(stmts, func(s ast.MicroflowStatement) {
				if c, ok := s.(*ast.RestCallStmt); ok && c.Result.Type == ast.RestResultMapping && c.Result.IsList {
					uses = append(uses, use{kind, name, c})
				}
				if h := getErrorHandlerBody(s); len(h) > 0 {
					walk(h)
				}
			})
		}
		walk(body)
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			collect("microflow", s.Name, s.Body)
		case *ast.CreateNanoflowStmt:
			collect("nanoflow", s.Name, s.Body)
		}
	}
	if len(uses) == 0 {
		return nil
	}

	shapes := newMappingShapeResolver(prog, projectPath)
	defer shapes.close()

	var out []linter.Violation
	for _, u := range uses {
		mapping := u.call.Result.MappingName
		if shapes.shape(mapping) != shapeObject {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   restMappingResultRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"`rest call … returns mapping %s as list of %s`: %s is object-rooted, so it returns ONE object, "+
					"not a list — exec and the check accept it and mxbuild reports CE0243 \"The mapping used to return "+
					"a value of type 'List of %s', but now returns a value of type '%s'\"",
				mapping, u.call.Result.ResultEntity, mapping, u.call.Result.ResultEntity, u.call.Result.ResultEntity),
			Location: linter.Location{Module: u.flow.Module, DocumentType: u.kind, DocumentName: u.flow.Name},
			Suggestion: fmt.Sprintf(
				"Declare the result as the entity — `returns mapping %s as %s` — or map a JSON structure whose root is an array.",
				mapping, u.call.Result.ResultEntity),
		})
	}
	return out
}
