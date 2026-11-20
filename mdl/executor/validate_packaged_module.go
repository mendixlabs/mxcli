// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A module held as modules/<Name>.mxmodule is read-only (ako/mxcli#1780): the
// writer refuses every write into it (mpr.PackagedUnitError), and exec rolls
// the script back. validatePackagedModuleTarget makes check --references say
// so first, as it does for the other refusals exec would make.

// writeTargetModule returns the module a model-writing statement writes into,
// or "" when it names none this can tell. Creates and drops come from the
// tables the duplicate check keeps; alters, renames and moves from the first
// qualified name the statement holds, which is its target.
func writeTargetModule(stmt ast.Statement) string {
	switch s := stmt.(type) {
	case *ast.CreateModuleStmt:
		return s.Name
	case *ast.DropModuleStmt:
		return s.Name
	case *ast.AlterModuleSetStmt:
		return s.ModuleName
	case *ast.AlterModuleJarDepStmt:
		return s.ModuleName
	}
	moduleOf := func(qn string) string {
		if i := strings.IndexByte(qn, '.'); i > 0 {
			return qn[:i]
		}
		return ""
	}
	if dt, name, _ := stmtCreateInfo(stmt); dt != "" {
		return moduleOf(name)
	}
	if dt, name := stmtDropInfo(stmt); dt != "" {
		return moduleOf(name)
	}
	v := reflect.ValueOf(stmt)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	name := v.Type().Name()
	if !strings.HasPrefix(name, "Alter") && !strings.HasPrefix(name, "Rename") && !strings.HasPrefix(name, "Move") {
		return ""
	}
	qnType := reflect.TypeOf(ast.QualifiedName{})
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Type() == qnType && v.Type().Field(i).IsExported() {
			return f.Interface().(ast.QualifiedName).Module
		}
	}
	return ""
}

// validatePackagedModuleTarget refuses a statement that writes into a
// packaged module, worded as the writer's refusal.
func validatePackagedModuleTarget(ctx *ExecContext, stmt ast.Statement) error {
	module := writeTargetModule(stmt)
	if module == "" {
		return nil
	}
	m, err := findModule(ctx, module)
	if err != nil || m == nil || m.PackageFile == "" {
		return nil
	}
	return fmt.Errorf("module %s is a packaged module (%s): it is read-only, as nothing in the project file "+
		"holds its documents; change it in the package's source project and re-import the package", m.Name, m.PackageFile)
}
