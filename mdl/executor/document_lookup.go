// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Lookups of one document by name or ID that decode that document only.
//
// A describe used to find its document, and every snippet, flow, page and
// layout it names, by listing all documents of the type, and on the modelsdk
// backend a listing decodes each one. A source catalog build describes every
// document, so it paid that once per document: 40% of a first build's CPU on
// FactoryManagement (#1850). The modelsdk backend lists headers without
// decoding and decodes one unit by ID; another backend falls back to its
// listing.

type snippetLookup interface {
	ListSnippetHeaders() ([]types.DocumentHeader, error)
	GetSnippet(id model.ID) (*pages.Snippet, error)
}

type nanoflowHeaderLister interface {
	ListNanoflowHeaders() ([]types.DocumentHeader, error)
}

type pageHeaderLister interface {
	ListPageHeaders() ([]types.DocumentHeader, error)
}

type layoutHeaderLister interface {
	ListLayoutHeaders() ([]types.DocumentHeader, error)
}

// decodeWhere decodes, with get, the documents whose header keep accepts.
func decodeWhere[E any](hs []types.DocumentHeader, keep func(types.DocumentHeader) bool, get func(model.ID) (*E, error)) ([]*E, error) {
	var out []*E
	for _, h := range hs {
		if !keep(h) {
			continue
		}
		d, err := get(h.ID)
		if err != nil {
			return nil, err
		}
		if d != nil {
			out = append(out, d)
		}
	}
	return out, nil
}

// snippetHeaders is every snippet's ID, container and name.
func snippetHeaders(be backend.FullBackend) ([]types.DocumentHeader, error) {
	if l, ok := be.(snippetLookup); ok {
		return l.ListSnippetHeaders()
	}
	all, err := be.ListSnippets()
	if err != nil {
		return nil, err
	}
	out := make([]types.DocumentHeader, len(all))
	for i, s := range all {
		out[i] = types.DocumentHeader{ID: s.ID, ContainerID: s.ContainerID, Name: s.Name, Excluded: s.Excluded}
	}
	return out, nil
}

// snippetsNamed is the project's snippets whose local name is name.
func snippetsNamed(be backend.FullBackend, name string) ([]*pages.Snippet, error) {
	if l, ok := be.(snippetLookup); ok {
		hs, err := l.ListSnippetHeaders()
		if err != nil {
			return nil, err
		}
		return decodeWhere(hs, func(h types.DocumentHeader) bool { return h.Name == name }, l.GetSnippet)
	}
	all, err := be.ListSnippets()
	if err != nil {
		return nil, err
	}
	var out []*pages.Snippet
	for _, s := range all {
		if s.Name == name {
			out = append(out, s)
		}
	}
	return out, nil
}

// nanoflowHeaders is every nanoflow's ID, container and name.
func nanoflowHeaders(be backend.FullBackend) ([]types.DocumentHeader, error) {
	if l, ok := be.(nanoflowHeaderLister); ok {
		return l.ListNanoflowHeaders()
	}
	all, err := be.ListNanoflows()
	if err != nil {
		return nil, err
	}
	out := make([]types.DocumentHeader, len(all))
	for i, nf := range all {
		out[i] = types.DocumentHeader{ID: nf.ID, ContainerID: nf.ContainerID, Name: nf.Name, Excluded: nf.Excluded}
	}
	return out, nil
}

// nanoflowsNamed is the project's nanoflows whose local name is name.
func nanoflowsNamed(be backend.FullBackend, name string) ([]*microflows.Nanoflow, error) {
	if l, ok := be.(nanoflowHeaderLister); ok {
		hs, err := l.ListNanoflowHeaders()
		if err != nil {
			return nil, err
		}
		return decodeWhere(hs, func(h types.DocumentHeader) bool { return h.Name == name }, be.GetNanoflow)
	}
	all, err := be.ListNanoflows()
	if err != nil {
		return nil, err
	}
	var out []*microflows.Nanoflow
	for _, nf := range all {
		if nf.Name == name {
			out = append(out, nf)
		}
	}
	return out, nil
}

// pagesWhere is the project's pages whose local name keep accepts.
func pagesWhere(be backend.FullBackend, keep func(name string) bool) ([]*pages.Page, error) {
	return pagesWhereHeader(be, func(h types.DocumentHeader) bool { return keep(h.Name) })
}

// pagesWhereHeader is the project's pages whose ID, container and name keep
// accepts. Only those are loaded: a caller that knows the module as well as
// the name says so here, and is not handed the page's namesakes from every
// other module to pick from (ako/mxcli#2160).
func pagesWhereHeader(be backend.FullBackend, keep func(types.DocumentHeader) bool) ([]*pages.Page, error) {
	if l, ok := be.(pageHeaderLister); ok {
		hs, err := l.ListPageHeaders()
		if err != nil {
			return nil, err
		}
		return decodeWhere(hs, keep, be.GetPage)
	}
	all, err := be.ListPages()
	if err != nil {
		return nil, err
	}
	var out []*pages.Page
	for _, p := range all {
		if keep(types.DocumentHeader{ID: p.ID, ContainerID: p.ContainerID, Name: p.Name}) {
			out = append(out, p)
		}
	}
	return out, nil
}

// layoutHeaders is every layout's ID, container and name.
func layoutHeaders(be backend.FullBackend) ([]types.DocumentHeader, error) {
	if l, ok := be.(layoutHeaderLister); ok {
		return l.ListLayoutHeaders()
	}
	all, err := be.ListLayouts()
	if err != nil {
		return nil, err
	}
	out := make([]types.DocumentHeader, len(all))
	for i, l := range all {
		out[i] = types.DocumentHeader{ID: l.ID, ContainerID: l.ContainerID, Name: l.Name}
	}
	return out, nil
}

// flowNames maps every microflow and nanoflow ID to its qualified name: what a
// flow's call actions resolve against. Read from headers, so no flow is
// decoded, and fresh: the cached microflow names miss a flow created earlier in
// the same script.
func flowNames(ctx *ExecContext, h *ContainerHierarchy, withNanoflows bool) (map[model.ID]string, error) {
	out := map[model.ID]string{}
	mfs, err := microflowHeaders(ctx)
	if err != nil {
		return nil, err
	}
	for _, mf := range mfs {
		out[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
	}
	if withNanoflows {
		nfs, err := nanoflowHeaders(ctx.Backend)
		if err != nil {
			return nil, err
		}
		for _, nf := range nfs {
			out[nf.ID] = h.GetQualifiedName(nf.ContainerID, nf.Name)
		}
	}
	return out, nil
}
