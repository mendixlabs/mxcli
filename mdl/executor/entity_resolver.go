// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// flowEntityResolver finds an entity's ID by qualified name for a flow's
// parameter and return types. It decodes only the domain model of the module
// named, each at most once: the builders used to decode every domain model in
// the project — a microflow once per lookup — which on a large project was a
// third of the cost of a one-activity `create or modify` (ako/mxcli#2094).
//
// It lives for one build, so it can never serve a domain model older than the
// statement that asked.
type flowEntityResolver struct {
	ctx *ExecContext
	// moduleIDs is listed on the first lookup: a flow with no entity-typed
	// parameter or return type needs neither the modules nor a domain model.
	moduleIDs map[string]model.ID
	byModule  map[model.ID]*domainmodel.DomainModel
	// all is every domain model, loaded only when a module's own could not be
	// read on its own (a backend without GetDomainModel answers nil).
	all    []*domainmodel.DomainModel
	loaded bool
	// err is the first backend failure, so a builder reports it rather than
	// call the entity missing.
	err error
}

func newFlowEntityResolver(ctx *ExecContext) *flowEntityResolver {
	return &flowEntityResolver{ctx: ctx, byModule: map[model.ID]*domainmodel.DomainModel{}}
}

// failed records the first backend failure.
func (r *flowEntityResolver) failed(err error) {
	if r.err == nil {
		r.err = err
	}
}

// domainModel is the domain model of the module with this ID, read once.
func (r *flowEntityResolver) domainModel(moduleID model.ID) *domainmodel.DomainModel {
	if dm, ok := r.byModule[moduleID]; ok {
		return dm
	}
	dm, err := r.ctx.Backend.GetDomainModel(moduleID)
	if err != nil || dm == nil {
		dm = r.fromAll(moduleID)
	}
	r.byModule[moduleID] = dm
	return dm
}

// fromAll is the module's domain model out of the full listing, the builders'
// former path, kept for a backend that cannot read one module's.
func (r *flowEntityResolver) fromAll(moduleID model.ID) *domainmodel.DomainModel {
	if !r.loaded {
		var err error
		r.all, err = r.ctx.Backend.ListDomainModels()
		r.loaded = true
		if err != nil {
			r.failed(err)
		}
	}
	for _, dm := range r.all {
		if dm.ContainerID == moduleID {
			return dm
		}
	}
	return nil
}

// resolve is the entity's ID, or "" when the module or the entity is not
// there (or the backend failed: see err).
func (r *flowEntityResolver) resolve(qn ast.QualifiedName) model.ID {
	if r.moduleIDs == nil {
		modules, err := r.ctx.Backend.ListModules()
		if err != nil {
			r.failed(err)
			return ""
		}
		r.moduleIDs = make(map[string]model.ID, len(modules))
		for _, m := range modules {
			r.moduleIDs[m.Name] = m.ID
		}
	}
	moduleID, ok := r.moduleIDs[qn.Module]
	if !ok {
		return ""
	}
	dm := r.domainModel(moduleID)
	if dm == nil {
		return ""
	}
	for _, ent := range dm.Entities {
		if ent.Name == qn.Name {
			return ent.ID
		}
	}
	return ""
}
