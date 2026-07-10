// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"io"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck"
	exprhints "github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck/hints"
)

type ExecAdapter struct {
	out     io.Writer
	parser  exprcheck.Parser
	slots   exprcheck.SlotResolver
	catalog exprcheck.CatalogReader
}

func NewExecAdapter(out io.Writer, cat exprcheck.CatalogReader) *ExecAdapter {
	return &ExecAdapter{
		out:     out,
		parser:  exprcheck.NewParser(),
		slots:   exprcheck.DefaultSlotResolver(),
		catalog: cat,
	}
}

func (a *ExecAdapter) ExprToBSON(slotPath string, expr ast.Expression, microflow string) string {
	src := ""
	if se, ok := expr.(*ast.SourceExpr); ok {
		src = se.Source
	}
	if src == "" {
		return ""
	}
	_, hs := a.parser.Parse(src, exprcheck.Context{
		SlotPath:  slotPath,
		Microflow: microflow,
		Slots:     a.slots,
		Catalog:   a.catalog,
	})
	var hadError bool
	for _, h := range hs {
		if a.out != nil {
			_, _ = a.out.Write([]byte(exprhints.FormatText(h)))
			_, _ = a.out.Write([]byte("\n"))
		}
		if h.Severity == exprhints.SeverityError {
			hadError = true
		}
	}
	if hadError {
		return ""
	}
	return src
}
