// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2094 (mendixlabs/mxcli#1795): exec paid a fixed cost per
// statement that grew with the project, not with the change. Building a flow
// resolved each entity-typed parameter and return type by decoding EVERY
// domain model in the project (a microflow did it again for each lookup), and
// `create or modify microflow` found the stored flow by decoding EVERY
// microflow. On a 3,400-unit project the two were 54% of the profile of a
// one-activity `create or modify`.

// decodeCountingBackend counts the listings that decode a whole document type,
// and serves the by-name and by-module lookups the real backend has.
type decodeCountingBackend struct {
	*mock.MockBackend
	listDomainModels int
	getDomainModel   map[model.ID]int
	listMicroflows   int
	microflowsNamed  int
	dms              []*domainmodel.DomainModel
	mfs              []*microflows.Microflow
}

func (b *decodeCountingBackend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	b.listDomainModels++
	return b.dms, nil
}

func (b *decodeCountingBackend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	b.getDomainModel[moduleID]++
	for _, dm := range b.dms {
		if dm.ContainerID == moduleID {
			return dm, nil
		}
	}
	return nil, fmt.Errorf("domain model not found for module: %s", moduleID)
}

func (b *decodeCountingBackend) ListMicroflows() ([]*microflows.Microflow, error) {
	b.listMicroflows++
	return b.mfs, nil
}

func (b *decodeCountingBackend) ListMicroflowsNamed(name string) ([]*microflows.Microflow, error) {
	b.microflowsNamed++
	var out []*microflows.Microflow
	for _, mf := range b.mfs {
		if mf.Name == name {
			out = append(out, mf)
		}
	}
	return out, nil
}

func (b *decodeCountingBackend) ListMicroflowHeaders() ([]types.DocumentHeader, error) {
	var out []types.DocumentHeader
	for _, mf := range b.mfs {
		out = append(out, types.DocumentHeader{ID: mf.ID, ContainerID: mf.ContainerID, Name: mf.Name, Excluded: mf.Excluded})
	}
	return out, nil
}

// decodeCountingCtx is a project of six modules, each with a domain model,
// Shop holding Customer, and the stored microflows given.
func decodeCountingCtx(t *testing.T, stored func(shop, other *model.Module) []*microflows.Microflow) (*ExecContext, *decodeCountingBackend) {
	t.Helper()
	shop := mkModule("Shop")
	mods := []*model.Module{shop}
	dms := []*domainmodel.DomainModel{mkDomainModel(shop.ID, mkEntity(shop.ID, "Customer"))}
	for i := range 5 {
		m := mkModule(fmt.Sprintf("Other%d", i))
		mods = append(mods, m)
		dms = append(dms, mkDomainModel(m.ID, mkEntity(m.ID, "Customer")))
	}
	h := mkHierarchy(mods...)
	b := &decodeCountingBackend{getDomainModel: map[model.ID]int{}, dms: dms}
	if stored != nil {
		b.mfs = stored(shop, mods[1])
	}
	b.MockBackend = &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return mods, nil },
		ListNanoflowsFunc:   func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListRulesFunc:       func() ([]*microflows.Rule, error) { return nil, nil },
		CreateMicroflowFunc: func(*microflows.Microflow) error { return nil },
		CreateNanoflowFunc:  func(*microflows.Nanoflow) error { return nil },
		CreateRuleFunc:      func(*microflows.Rule) error { return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(b.MockBackend), withHierarchy(h))
	ctx.Backend = b
	return ctx, b
}

func execOneFlow(t *testing.T, ctx *ExecContext, src string) {
	t.Helper()
	prog, errs := visitor.Build("mdl 1;\n" + src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var err error
	switch s := prog.Statements[0].(type) {
	case *ast.CreateMicroflowStmt:
		err = execCreateMicroflow(ctx, s)
	case *ast.CreateNanoflowStmt:
		err = execCreateNanoflow(ctx, s)
	case *ast.CreateRuleStmt:
		err = execCreateRule(ctx, s)
	default:
		t.Fatalf("unexpected statement %T", s)
	}
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func TestFlowBuildDecodesOnlyTheNamedModule_Issue2094(t *testing.T) {
	for kind, src := range map[string]string{
		"microflow": `create microflow Shop.ACT ($a: Shop.Customer, $l: list of Shop.Customer) returns Shop.Customer
begin
  return $a;
end;`,
		"nanoflow": `create nanoflow Shop.NF ($a: Shop.Customer, $l: list of Shop.Customer) returns Shop.Customer
begin
  return $a;
end;`,
		"rule": `create rule Shop.RL ($a: Shop.Customer, $l: list of Shop.Customer) returns boolean
begin
  return true;
end;`,
	} {
		t.Run(kind, func(t *testing.T) {
			ctx, b := decodeCountingCtx(t, nil)
			execOneFlow(t, ctx, src)
			t.Logf("%s: ListDomainModels=%d GetDomainModel=%v", kind, b.listDomainModels, b.getDomainModel)
			if b.listDomainModels != 0 {
				t.Errorf("building the %s decoded every domain model %d times; want 0 (only Shop's is needed)", kind, b.listDomainModels)
			}
			for id, n := range b.getDomainModel {
				if n > 1 {
					t.Errorf("domain model of %s decoded %d times in one build; want once", id, n)
				}
			}
		})
	}
}

// Finding a stored microflow by qualified name decodes only the microflows of
// that local name, and still prefers the live one over an excluded twin and
// the module asked for over another module's namesake.
func TestFindMicroflowByQualifiedNameDecodesOnlyNamesakes_Issue2094(t *testing.T) {
	var live *microflows.Microflow
	ctx, b := decodeCountingCtx(t, func(shop, other *model.Module) []*microflows.Microflow {
		mk := func(id string, mod *model.Module, name string, excluded bool) *microflows.Microflow {
			mf := &microflows.Microflow{ContainerID: mod.ID, Name: name, Excluded: excluded}
			mf.ID = model.ID(id)
			return mf
		}
		live = mk("live", shop, "ACT", false)
		return []*microflows.Microflow{
			mk("excluded", shop, "ACT", true),
			mk("namesake", other, "ACT", false),
			live,
			mk("unrelated", shop, "Other", false),
		}
	})
	got := findMicroflowByQualifiedName(ctx, "Shop.ACT")
	if got != live {
		t.Fatalf("found %v, want the live Shop.ACT", got)
	}
	if b.listMicroflows != 0 {
		t.Errorf("finding one microflow decoded every microflow %d times; want 0", b.listMicroflows)
	}
	if got := findMicroflowByQualifiedName(ctx, "Shop.Missing"); got != nil {
		t.Errorf("Shop.Missing: found %v, want nil", got.ID)
	}
}

// A backend that cannot read one module's domain model (the mock answers nil)
// still resolves through the full listing, and a failing listing is reported
// as the backend error it is, not as a missing entity.
func TestFlowEntityResolverFallbackAndFailure_Issue2094(t *testing.T) {
	shop := mkModule("Shop")
	cust := mkEntity(shop.ID, "Customer")
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{shop}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{mkDomainModel(shop.ID, cust)}, nil
		},
	}))
	r := newFlowEntityResolver(ctx)
	if got := r.resolve(ast.QualifiedName{Module: "Shop", Name: "Customer"}); got != cust.ID {
		t.Errorf("fallback resolve = %q, want %q", got, cust.ID)
	}
	if got := r.resolve(ast.QualifiedName{Module: "Nope", Name: "Customer"}); got != "" {
		t.Errorf("unknown module resolved to %q", got)
	}

	failing, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{shop}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, fmt.Errorf("disk on fire") },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
	}), withHierarchy(mkHierarchy(shop)))
	prog, errs := visitor.Build("mdl 1;\ncreate microflow Shop.ACT ($a: Shop.Customer) begin end;")
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	err := execCreateMicroflow(failing, prog.Statements[0].(*ast.CreateMicroflowStmt))
	if err == nil || !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("a failing domain model read gave %v; want the backend error", err)
	}
}
