// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

// ExportJSON was the last FullBackend method whose only caller was
// examples/read_project holding a concrete sdk/mpr reader — the bypass that kept
// the root package (modelsdk.go) on the legacy serializer. Implementing it here
// is what lets that example use a backend value, which is the legitimate way an
// entry leaves unimplemented_reachability_test.go's census: by closing the
// bypass, not by deleting the method.
//
// It is pure fan-out over this backend's own List* methods — no decoding of its
// own — so it cannot drift from what every other reader reports.

import (
	"encoding/json"
)

// ExportJSON renders the project's main document types as one indented JSON
// object.
//
// A section whose listing fails is emitted as null rather than aborting the
// export, matching the legacy reader's behaviour. That is deliberate for a
// debugging/inspection dump: a project with one unreadable document type should
// still tell you about the other seven. It does mean a null section is
// ambiguous between "failed to read" and "genuinely empty" — callers needing
// that distinction should use the individual List* methods, which return errors.
func (b *Backend) ExportJSON() ([]byte, error) {
	// Each listing is independent; a failure yields a nil section.
	modules, err := b.ListModules()
	if err != nil {
		modules = nil
	}
	domainModels, err := b.ListDomainModels()
	if err != nil {
		domainModels = nil
	}
	microflowsList, err := b.ListMicroflows()
	if err != nil {
		microflowsList = nil
	}
	nanoflows, err := b.ListNanoflows()
	if err != nil {
		nanoflows = nil
	}
	pagesList, err := b.ListPages()
	if err != nil {
		pagesList = nil
	}
	layouts, err := b.ListLayouts()
	if err != nil {
		layouts = nil
	}
	enumerations, err := b.ListEnumerations()
	if err != nil {
		enumerations = nil
	}
	constants, err := b.ListConstants()
	if err != nil {
		constants = nil
	}

	// Key names are the legacy reader's, so an existing consumer of this dump
	// keeps working across the engine swap.
	return json.MarshalIndent(map[string]any{
		"modules":      modules,
		"domainModels": domainModels,
		"microflows":   microflowsList,
		"nanoflows":    nanoflows,
		"pages":        pagesList,
		"layouts":      layouts,
		"enumerations": enumerations,
		"constants":    constants,
	}, "", "  ")
}
