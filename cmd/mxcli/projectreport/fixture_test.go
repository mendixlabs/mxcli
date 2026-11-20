// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// fixture is an in-memory catalog filled row by row, so each collector is
// tested against exactly the catalog shape it reads.
type fixture struct {
	t   *testing.T
	cat *catalog.Catalog
	n   int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	return &fixture{t: t, cat: cat}
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.cat.CatalogDB().Exec(sql, args...); err != nil {
		f.t.Fatalf("%v\n%s", err, sql)
	}
}

func (f *fixture) id() string {
	f.n++
	return fmt.Sprintf("id-%d", f.n)
}

func (f *fixture) module(name, source string) {
	f.exec(`INSERT INTO modules_data (Id, Name, QualifiedName, Source) VALUES (?, ?, ?, ?)`, f.id(), name, name, source)
}

func (f *fixture) microflow(qn string, applyEA bool, ret string) {
	mod := moduleOf(qn)
	f.exec(`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ApplyEntityAccess, ReturnType)
		VALUES (?, ?, ?, ?, 'MICROFLOW', ?, ?)`, f.id(), shortName(qn), qn, mod, applyEA, ret)
}

func (f *fixture) page(qn string) {
	f.exec(`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName) VALUES (?, ?, ?, ?)`, f.id(), shortName(qn), qn, moduleOf(qn))
}

func (f *fixture) entity(qn, typ, gen string) {
	f.exec(`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, EntityType, Generalization) VALUES (?, ?, ?, ?, ?, ?)`,
		f.id(), shortName(qn), qn, moduleOf(qn), typ, gen)
}

func (f *fixture) ref(src, srcType, tgt, tgtType, kind string) {
	f.exec(`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetName, RefKind, ModuleName)
		VALUES (?, 'x', ?, ?, ?, ?, ?)`, srcType, src, tgtType, tgt, kind, moduleOf(src))
}

func (f *fixture) call(from, to string) { f.ref(from, "MICROFLOW", to, "MICROFLOW", "call") }

func (f *fixture) constant(qn, dataType, def string, exposed bool) {
	f.exec(`INSERT INTO constants_data (Id, Name, QualifiedName, ModuleName, DataType, DefaultValue, ExposedToClient)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, f.id(), shortName(qn), qn, moduleOf(qn), dataType, def, exposed)
}

func (f *fixture) roleMapping(userRole, moduleRole string) {
	f.exec(`INSERT INTO role_mappings (UserRoleName, ModuleRoleName, ModuleName) VALUES (?, ?, ?)`, userRole, moduleRole, moduleOf(moduleRole))
}

func (f *fixture) permission(moduleRole, elemType, elem, access, xpath string) {
	f.exec(`INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, AccessType, XPathConstraint, ModuleName)
		VALUES (?, ?, ?, ?, ?, ?)`, moduleRole, elemType, elem, access, xpath, moduleOf(elem))
}

func (f *fixture) param(mf, name, typ string) {
	f.exec(`INSERT INTO microflow_parameters_data (Id, MicroflowQualifiedName, ModuleName, Name, ParameterType)
		VALUES (?, ?, ?, ?, ?)`, f.id(), mf, moduleOf(mf), name, typ)
}

func (f *fixture) source(qn, text string) {
	f.exec(`INSERT INTO source (QualifiedName, ObjectType, SourceText, ModuleName, ElementId) VALUES (?, 'MICROFLOW', ?, ?, ?)`,
		qn, text, moduleOf(qn), f.id())
}

func mustContain(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}
