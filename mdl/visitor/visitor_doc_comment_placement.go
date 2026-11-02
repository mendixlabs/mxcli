// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// # A doc comment before a statement that cannot store it (ako/mxcli#877)
//
// The grammar lets every statement start with a `/** … */` doc comment, and
// only a create of a document that has documentation reads it. Anywhere else —
// `drop microflow if exists X;` written between a flow's comment and its
// `create`, a `grant`, a `set` — the statement ignores it and the text is lost:
// mxcli-rest lost six flows' documentation that way, with `mx check`, `exec`
// and describe all clean. Each such comment is recorded with the next statement
// that would have stored it, and check warns (MDL089).

// enterStatementDocs remembers how many statements were built before the current
// top-level statement, so its exit knows which statements it produced.
func (b *Builder) enterStatementDocs(*parser.StatementContext) {
	b.docStmtStart = len(b.statements)
}

// exitStatementDocs records the statement's doc comment when none of the
// statements it built stores one, and fills in the next statement for the
// comments still waiting for one.
func (b *Builder) exitStatementDocs(ctx *parser.StatementContext) {
	start := b.docStmtStart
	if start > len(b.statements) {
		start = len(b.statements)
	}
	built := b.statements[start:]
	if len(built) == 0 {
		return // nothing built: a syntax error, reported already
	}
	head, line := statementHead(ctx)
	stores := storesDocumentation(ctx, built) || adoptsAddedAttributeDoc(ctx, built)
	if stores {
		for _, d := range b.detachedDocs[b.docsAwaitingNext:] {
			d.Next, d.NextLine = head, line
		}
		b.docsAwaitingNext = len(b.detachedDocs)
	}
	if doc := ctx.DocComment(); doc != nil && !stores {
		b.detachedDocs = append(b.detachedDocs, &ast.DetachedDocComment{
			Line: doc.GetStart().GetLine(), Statement: head,
		})
	}
}

// detachedDocComments returns the recorded comments, in source order.
func (b *Builder) detachedDocComments() []ast.DetachedDocComment {
	if len(b.detachedDocs) == 0 {
		return nil
	}
	out := make([]ast.DetachedDocComment, len(b.detachedDocs))
	for i, d := range b.detachedDocs {
		out[i] = *d
	}
	return out
}

// storesDocumentation reports whether a statement keeps its doc comment: it is
// a create, and what it builds has a documentation property the comment sets.
// No other statement reads the statement's doc comment, except an alter that
// adds one attribute (adoptsAddedAttributeDoc).
func storesDocumentation(ctx *parser.StatementContext, built []ast.Statement) bool {
	ddl := ctx.DdlStatement()
	if ddl == nil || ddl.CreateStatement() == nil {
		return false
	}
	for _, s := range built {
		t := reflect.TypeOf(s)
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			continue
		}
		for _, f := range []string{"Documentation", "OuterDocumentation", "DocumentationSet"} {
			if _, ok := t.FieldByName(f); ok {
				return true
			}
		}
	}
	return false
}

// adoptsAddedAttributeDoc gives the statement's doc comment to the attribute an
// `alter entity` / `alter entities` adds, as a comment above a create entity
// attribute documents it (ako/mxcli#1863). Only when the statement adds exactly
// one attribute that has no comment of its own: with two, which one the text
// was written for is a guess, and an attribute's own comment wins — both stay
// detached, so check still warns (MDL089).
func adoptsAddedAttributeDoc(ctx *parser.StatementContext, built []ast.Statement) bool {
	doc := ctx.DocComment()
	if doc == nil {
		return false
	}
	var added []*ast.Attribute
	collect := func(s *ast.AlterEntityStmt) {
		if s.Operation == ast.AlterEntityAddAttribute && s.Attribute != nil {
			added = append(added, s.Attribute)
		}
	}
	for _, s := range built {
		switch st := s.(type) {
		case *ast.AlterEntityStmt:
			collect(st)
		case *ast.AlterEntitiesStmt:
			for _, a := range st.Actions {
				collect(a)
			}
		}
	}
	if len(added) != 1 || added[0].Documentation != "" {
		return false
	}
	added[0].Documentation = extractDocComment(doc.GetText())
	return true
}

// statementHead is the start of the statement as written — its keywords and
// name, up to the first bracket, `;` or line end — and its line, for a message
// that points at it.
func statementHead(ctx *parser.StatementContext) (string, int) {
	var first antlr.Token
	if ddl := ctx.DdlStatement(); ddl != nil && ddl.CreateStatement() != nil && ddl.CreateStatement().CREATE() != nil {
		first = ddl.CreateStatement().CREATE().GetSymbol() // after any annotations
	} else {
		for i := 0; i < ctx.GetChildCount(); i++ {
			c, ok := ctx.GetChild(i).(antlr.ParserRuleContext)
			if !ok {
				continue
			}
			if _, isDoc := c.(*parser.DocCommentContext); isDoc {
				continue
			}
			first = c.GetStart()
			break
		}
	}
	if first == nil {
		return "", 0
	}
	stop := ctx.GetStop()
	if stop == nil || stop.GetStop() < first.GetStart() {
		return first.GetText(), first.GetLine()
	}
	text := first.GetInputStream().GetText(first.GetStart(), stop.GetStop())
	if i := strings.IndexAny(text, "\n({;"); i >= 0 {
		text = text[:i]
	}
	if i := strings.Index(strings.ToLower(text), " begin"); i >= 0 {
		text = text[:i]
	}
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > 100 {
		text = string(r[:100]) + "…"
	}
	return text, first.GetLine()
}
