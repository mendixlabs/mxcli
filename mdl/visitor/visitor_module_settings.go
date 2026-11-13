// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitAlterModuleJarDepStatement handles:
//
//	ALTER MODULE name ADD JAR DEPENDENCY [IF NOT EXISTS] (...)
//	ALTER MODULE name SET JAR DEPENDENCY 'coord' VERSION '...'
//	ALTER MODULE name SET JAR DEPENDENCY 'coord' INCLUDED true|false
//	ALTER MODULE name SET JAR DEPENDENCY 'coord' ADD EXCLUSION '...'
//	ALTER MODULE name SET JAR DEPENDENCY 'coord' DROP EXCLUSION '...'
//	ALTER MODULE name DROP JAR DEPENDENCY 'coord'
func (b *Builder) ExitAlterModuleJarDepStatement(ctx *parser.AlterModuleJarDepStatementContext) {
	var moduleName string
	if qn := ctx.QualifiedName(); qn != nil {
		moduleName = getQualifiedNameText(qn)
	} else if id := ctx.IDENTIFIER(); id != nil {
		moduleName = id.GetText()
	}
	if moduleName == "" {
		return
	}

	stmt := &ast.AlterModuleJarDepStmt{ModuleName: moduleName}

	for _, rawAction := range ctx.AllAlterModuleJarDepAction() {
		actionCtx := rawAction.(*parser.AlterModuleJarDepActionContext)
		action := parseJarDepAction(actionCtx)
		if action != nil {
			stmt.Actions = append(stmt.Actions, action)
		}
	}

	if len(stmt.Actions) > 0 {
		b.statements = append(b.statements, stmt)
	}
}

func parseJarDepAction(ctx *parser.AlterModuleJarDepActionContext) ast.JarDepAction {
	if ctx.ADD() != nil && ctx.JAR() != nil && ctx.DEPENDENCY() != nil && ctx.LPAREN() != nil {
		// ADD JAR DEPENDENCY (group = '...', artifact = '...', ...)
		action := &ast.AddJarDepAction{Included: true, IfNotExists: ctx.IfNotExists() != nil} // default included = true
		for _, rawProp := range ctx.AllJarDepProperty() {
			propCtx := rawProp.(*parser.JarDepPropertyContext)
			key := strings.ToLower(identifierOrKeywordText(propCtx.IdentifierOrKeyword()))
			var val string
			var boolVal bool
			isBool := false
			if sl := propCtx.STRING_LITERAL(); sl != nil {
				val = unquoteStringLit(sl)
			} else if bl := propCtx.BooleanLiteral(); bl != nil {
				boolVal = strings.EqualFold(bl.GetText(), "true")
				isBool = true
			}
			switch key {
			case "group":
				action.Group = val
			case "artifact":
				action.Artifact = val
			case "version":
				action.Version = val
			case "included":
				if isBool {
					action.Included = boolVal
				}
			}
		}
		return action
	}

	if ctx.SET() != nil && ctx.JAR() != nil && ctx.DEPENDENCY() != nil {
		// The coordinate is STRING_LITERAL(0)
		strs := ctx.AllSTRING_LITERAL()
		if len(strs) == 0 {
			return nil
		}
		coordinate := unquoteStringLit(strs[0])

		if ctx.VERSION() != nil && len(strs) >= 2 {
			// SET JAR DEPENDENCY 'coord' VERSION 'version'
			return &ast.SetJarDepVersionAction{
				Coordinate: coordinate,
				Version:    unquoteStringLit(strs[1]),
			}
		}

		if ctx.INCLUDED() != nil && ctx.BooleanLiteral() != nil {
			// SET JAR DEPENDENCY 'coord' INCLUDED true|false
			included := strings.EqualFold(ctx.BooleanLiteral().GetText(), "true")
			return &ast.SetJarDepIncludedAction{
				Coordinate: coordinate,
				Included:   included,
			}
		}

		if ctx.ADD() != nil && ctx.EXCLUSION() != nil && len(strs) >= 2 {
			// SET JAR DEPENDENCY 'coord' ADD EXCLUSION 'excl'
			return &ast.AddJarDepExclusionAction{
				Coordinate: coordinate,
				Exclusion:  unquoteStringLit(strs[1]),
			}
		}

		if ctx.DROP() != nil && ctx.EXCLUSION() != nil && len(strs) >= 2 {
			// SET JAR DEPENDENCY 'coord' DROP EXCLUSION 'excl'
			return &ast.DropJarDepExclusionAction{
				Coordinate: coordinate,
				Exclusion:  unquoteStringLit(strs[1]),
			}
		}

		return nil
	}

	if ctx.DROP() != nil && ctx.JAR() != nil && ctx.DEPENDENCY() != nil {
		// DROP JAR DEPENDENCY 'coord'
		strs := ctx.AllSTRING_LITERAL()
		if len(strs) == 0 {
			return nil
		}
		return &ast.DropJarDepAction{Coordinate: unquoteStringLit(strs[0])}
	}

	return nil
}

// moduleSettingKeys are the keys of a module's settings list, named after the
// fields of Studio Pro's Module settings dialog (ako/mxcli#1921).
var moduleSettingKeys = []string{"Version", "ModuleType"}

// moduleSettingsVersion is the dialog's rule for a module version: SemVer
// major.minor.patch, with an optional pre-release and build suffix.
var moduleSettingsVersion = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

// moduleSettings reads a `( Version: '…', ModuleType: … )` list. An unknown
// key, a repeated key or a value of the wrong shape is an error here, so check
// and exec refuse the same statements.
func (b *Builder) moduleSettings(what string, body parser.IModuleSettingsBodyContext) ast.ModuleSettingsClause {
	var out ast.ModuleSettingsClause
	bc, ok := body.(*parser.ModuleSettingsBodyContext)
	if !ok || bc == nil {
		return out
	}
	seen := map[string]bool{}
	for _, p := range bc.AllModuleSettingProperty() {
		pc, ok := p.(*parser.ModuleSettingPropertyContext)
		if !ok || pc == nil || pc.IdentifierOrKeyword(0) == nil {
			continue
		}
		line := pc.GetStart().GetLine()
		key := identifierOrKeywordText(pc.IdentifierOrKeyword(0))
		canonical := canonicalKey(key, moduleSettingKeys)
		if canonical == "" {
			b.addError(unknownKeyError(line, what, key, moduleSettingKeys))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: %s sets %s twice", line, what, canonical))
			continue
		}
		seen[canonical] = true
		switch canonical {
		case "Version":
			lit := pc.STRING_LITERAL()
			if lit == nil {
				b.addError(fmt.Errorf("line %d: %s: Version takes a string, as in Version: '1.2.0'", line, what))
				continue
			}
			v := unquoteStringLit(lit)
			if !moduleSettingsVersion.MatchString(v) {
				b.addError(fmt.Errorf("line %d: %s: Version '%s' is not a SemVer version: Studio Pro takes major.minor.patch, as in '1.2.0'", line, what, v))
				continue
			}
			out.Version = &v
		case "ModuleType":
			var t string
			if pc.STRING_LITERAL() == nil && pc.IdentifierOrKeyword(1) != nil {
				t = strings.ToLower(identifierOrKeywordText(pc.IdentifierOrKeyword(1)))
			}
			if t != "app" && t != "addon" {
				b.addError(fmt.Errorf("line %d: %s: ModuleType takes app or addon, not %s", line, what, nodeText(pc.GetChild(2))))
				continue
			}
			out.ModuleType = &t
		}
	}
	return out
}

// ExitAlterModuleSetStatement handles ALTER MODULE name SET ( Key: value, … ).
func (b *Builder) ExitAlterModuleSetStatement(ctx *parser.AlterModuleSetStatementContext) {
	var moduleName string
	if qn := ctx.QualifiedName(); qn != nil {
		moduleName = getQualifiedNameText(qn)
	} else if id := ctx.IDENTIFIER(); id != nil {
		moduleName = id.GetText()
	}
	if moduleName == "" {
		return
	}
	stmt := &ast.AlterModuleSetStmt{ModuleName: moduleName}
	body := ctx.ModuleSettingsBody()
	if bc, ok := body.(*parser.ModuleSettingsBodyContext); ok && bc != nil && len(bc.AllModuleSettingProperty()) == 0 {
		b.addError(fmt.Errorf("line %d: alter module %s set () names no setting: it takes %s",
			ctx.GetStart().GetLine(), moduleName, strings.Join(moduleSettingKeys, ", ")))
		return
	}
	stmt.Settings = b.moduleSettings("module "+moduleName, body)
	if stmt.Settings.IsEmpty() {
		return // every key was refused above
	}
	b.statements = append(b.statements, stmt)
}
