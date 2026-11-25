// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// The metrics activities (Studio Pro's "Metrics Activities": Counter, Increment
// Counter, Gauge). The metric name and value are words — the statement's main
// operand — and the dialog's remaining setting, Tags, is the ADR-0013 property
// list. It is new syntax, so an unknown or repeated key is an error under every
// language version (R11).
var meterSettingKeys = []string{"Tags"}

func buildIncrementCounterStatement(ctx parser.IIncrementCounterStatementContext) *ast.IncrementCounterStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.IncrementCounterStatementContext)
	stmt := &ast.IncrementCounterStmt{}
	exprs := c.AllExpression()
	if len(exprs) > 0 {
		stmt.Name = buildSourceExpression(exprs[0])
	}
	if c.BY() != nil && len(exprs) > 1 {
		stmt.By = buildSourceExpression(exprs[1])
	}
	stmt.Tags = meterTags(c.MeterSettings())
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func buildSetGaugeStatement(ctx parser.ISetGaugeStatementContext) *ast.SetGaugeStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.SetGaugeStatementContext)
	stmt := &ast.SetGaugeStmt{}
	exprs := c.AllExpression()
	if len(exprs) > 0 {
		stmt.Name = buildSourceExpression(exprs[0])
	}
	if len(exprs) > 1 {
		stmt.Value = buildSourceExpression(exprs[1])
	}
	stmt.Tags = meterTags(c.MeterSettings())
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

// meterTags reads the Tags map out of the settings list. Lenient — an unknown
// key or a misshapen value is skipped — because the statement builders have no
// error channel; checkMeterSettings reports it.
func meterTags(ctx parser.IMeterSettingsContext) []ast.MeterTag {
	settings, ok := ctx.(*parser.MeterSettingsContext)
	if !ok || settings == nil {
		return nil
	}
	var tags []ast.MeterTag
	for _, s := range settings.AllMeterSetting() {
		sc, ok := s.(*parser.MeterSettingContext)
		if !ok || sc == nil || sc.IdentifierOrKeyword() == nil {
			continue
		}
		if canonicalKey(identifierOrKeywordText(sc.IdentifierOrKeyword()), meterSettingKeys) != "Tags" {
			continue
		}
		vc, ok := sc.MeterSettingValue().(*parser.MeterSettingValueContext)
		if !ok || vc == nil || vc.LPAREN() == nil {
			continue
		}
		for _, t := range vc.AllMeterTag() {
			tc, ok := t.(*parser.MeterTagContext)
			if !ok || tc == nil {
				continue
			}
			kv := tc.AllExpression()
			if len(kv) != 2 {
				continue
			}
			tags = append(tags, ast.MeterTag{
				Key:   buildSourceExpression(kv[0]),
				Value: buildSourceExpression(kv[1]),
			})
		}
	}
	return tags
}

func (b *Builder) ExitIncrementCounterStatement(ctx *parser.IncrementCounterStatementContext) {
	b.checkMeterSettings("increment counter", ctx.MeterSettings())
}

func (b *Builder) ExitSetGaugeStatement(ctx *parser.SetGaugeStatementContext) {
	b.checkMeterSettings("set gauge", ctx.MeterSettings())
}

// checkMeterSettings reports an unknown or repeated key and a Tags value that
// is not a ( key: value, … ) map.
func (b *Builder) checkMeterSettings(what string, ctx parser.IMeterSettingsContext) {
	settings, ok := ctx.(*parser.MeterSettingsContext)
	if !ok || settings == nil {
		return
	}
	seen := map[string]bool{}
	for _, s := range settings.AllMeterSetting() {
		sc, ok := s.(*parser.MeterSettingContext)
		if !ok || sc == nil || sc.IdentifierOrKeyword() == nil {
			continue
		}
		line := sc.GetStart().GetLine()
		key := identifierOrKeywordText(sc.IdentifierOrKeyword())
		canonical := canonicalKey(key, meterSettingKeys)
		if canonical == "" {
			b.addError(unknownKeyError(line, what, key, meterSettingKeys))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: %s sets %s twice", line, what, canonical))
			continue
		}
		seen[canonical] = true
		vc, ok := sc.MeterSettingValue().(*parser.MeterSettingValueContext)
		if !ok || vc == nil {
			continue
		}
		if vc.LPAREN() == nil {
			b.addError(fmt.Errorf("line %d: %s: Tags takes a map ( 'key': value, … ), not %s", line, what, nodeText(vc)))
		}
	}
}
