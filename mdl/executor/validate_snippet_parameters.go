// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateSnippetParameters (MDL087) refuses what no Mendix version stores on
// a snippet parameter: `optional` (Forms$SnippetParameter has no IsRequired).
//
// It used to refuse a primitive-typed parameter as well — mxbuild rejected
// every one with CE0046 "Invalid data type '<caption>'." (mendixlabs/mxcli#1028,
// measured in types.SnippetParameterTypeRule). Mendix 11.17 accepts them, so
// that refusal is now a version gate (pages/snippet_parameter_primitive in
// snippetPrimitiveParameterGates): this pass runs without a project and cannot
// know which side of 11.17 the script lands on, and refusing here would block
// the valid form at exec's pre-flight. `mxcli check -p` and exec still refuse
// a primitive below 11.17, through the gate, naming CE0046.
func validateSnippetParameters(stmt ast.Statement) []linter.Violation {
	var name ast.QualifiedName
	var params []ast.PageParameter
	switch s := stmt.(type) {
	case *ast.CreateSnippetStmtV3:
		name, params = s.Name, s.Parameters
	case *ast.AlterPageStmt:
		// `alter snippet … { add parameters … }` declares the same element
		// (mendixlabs/mxcli#1234).
		if !strings.EqualFold(s.ContainerType, "snippet") {
			return nil
		}
		name = s.PageName
		for _, op := range s.Operations {
			if add, ok := op.(*ast.AddParameterOp); ok {
				params = append(params, add.Parameter)
			}
		}
	default:
		return nil
	}

	var out []linter.Violation
	for _, p := range params {
		if p.Optional {
			out = append(out, linter.Violation{
				RuleID:   "MDL087",
				Severity: linter.SeverityError,
				Location: linter.Location{Module: name.Module, DocumentType: "snippet", DocumentName: name.Name},
				Message:  optionalSnippetParameterMessage(name, p.Name),
			})
		}
	}
	return out
}

// optionalSnippetParameterMessage refuses `optional` on a snippet parameter:
// Forms$SnippetParameter stores no IsRequired in any version, so the word
// would be dropped (mendixlabs/mxcli#1640).
func optionalSnippetParameterMessage(snippet ast.QualifiedName, param string) string {
	return fmt.Sprintf("snippet '%s' declares parameter $%s optional — only a page parameter can be "+
		"optional (Forms$PageParameter.IsRequired, Mendix 11.5+); a snippet parameter is always "+
		"supplied by the snippet call. Remove `optional`", snippet.String(), param)
}

// paramTypeSourceName names a primitive parameter type the way MDL spells it,
// for the message. It is deliberately not the CE0046 caption: the author is
// looking for the word in their own script.
func paramTypeSourceName(dt ast.DataType) string {
	switch dt.Kind {
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
	default:
		return "a primitive"
	}
}
