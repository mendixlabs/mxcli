// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sync"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// checkPassMemo holds what one check of a program has listed from the project.
//
// A check reads the project and never writes it, so the project is the same
// for its first statement and its last. The validators did not use that: each
// statement rebuilt its entity and association indexes and its flow signatures
// from a full listing of the domain models or the microflows, so a check cost
// statements × project — four listings of the domain models and three of the
// microflows for every page statement (ako/mxcli#2160).
//
// The memo lives on the ExecContext of one validateProgramWithWarnings call
// and nowhere else. During execution it is nil and every listing goes to the
// backend as before: a statement there must see what the one before it wrote.
//
// What it hands out is shared between the validators of the pass. They read
// it; none may change a listed document in place.
type checkPassMemo struct {
	mu           sync.Mutex
	domainModels *listed[*domainmodel.DomainModel]
	microflows   *listed[*microflows.Microflow]
	nanoflows    *listed[*microflows.Nanoflow]
	pages        *listed[*pages.Page]
	// templates is the widget template property metadata, loaded once per
	// widget for the pass rather than once per statement.
	templates func(string) map[string]pages.PropertyTypeIDEntry
}

// listed is one listing's answer, error included: a listing that failed once
// is not asked again within the pass.
type listed[T any] struct {
	items []T
	err   error
}

// memoListing returns the pass's listing, making it on first use. With no
// pass (memo nil) it calls list.
func memoListing[T any](memo *checkPassMemo, slot **listed[T], list func() ([]T, error)) ([]T, error) {
	if memo == nil {
		return list()
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	if *slot == nil {
		items, err := list()
		*slot = &listed[T]{items: items, err: err}
	}
	return (*slot).items, (*slot).err
}

// The listings below are what a validator calls in place of ctx.Backend.ListX:
// the pass's memo during a check, the backend otherwise.

func projectDomainModels(ctx *ExecContext) ([]*domainmodel.DomainModel, error) {
	if ctx.checkMemo == nil {
		return ctx.Backend.ListDomainModels()
	}
	return memoListing(ctx.checkMemo, &ctx.checkMemo.domainModels, ctx.Backend.ListDomainModels)
}

func projectMicroflows(ctx *ExecContext) ([]*microflows.Microflow, error) {
	if ctx.checkMemo == nil {
		return ctx.Backend.ListMicroflows()
	}
	return memoListing(ctx.checkMemo, &ctx.checkMemo.microflows, ctx.Backend.ListMicroflows)
}

func projectNanoflows(ctx *ExecContext) ([]*microflows.Nanoflow, error) {
	if ctx.checkMemo == nil {
		return ctx.Backend.ListNanoflows()
	}
	return memoListing(ctx.checkMemo, &ctx.checkMemo.nanoflows, ctx.Backend.ListNanoflows)
}

func projectPages(ctx *ExecContext) ([]*pages.Page, error) {
	if ctx.checkMemo == nil {
		return ctx.Backend.ListPages()
	}
	return memoListing(ctx.checkMemo, &ctx.checkMemo.pages, ctx.Backend.ListPages)
}
